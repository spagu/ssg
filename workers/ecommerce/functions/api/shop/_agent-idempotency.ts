// Idempotency keys for the agent checkouts — the mechanism ACP (RFC §6) and
// UCP (Message Signatures, "Idempotency Key Requirements") describe alike:
//
//   - the same key with the same body replays the stored response, status
//     included, with `Idempotent-Replayed: true`, and runs nothing again;
//   - the same key with another body is refused (each protocol says how);
//   - the same key while the first request still runs is refused with
//     Retry-After;
//   - a 5xx is never stored: the retry runs as a fresh request;
//   - keys are kept at least 24 hours; an older key is a new request.
//
// What differs between the protocols is a policy: which table, how a body is
// fingerprinted (ACP compares canonical JSON, UCP the SHA-256 of the raw
// bytes), whether a missing key is an error (ACP) or simply an unkeyed
// request (UCP), and the error bodies. Everything else is this file.
//
// The INSERT OR IGNORE is the lock, the same pattern as the payment webhooks'
// inbox: two requests racing with one key produce exactly one runner. D1 has
// no transaction that spans a fetch to Stripe, so the handlers are written to
// be safe to re-run instead (a session's version counter, the order's status
// guard, Stripe's own idempotency key), and a key whose runner died is taken
// over after `staleMs`.

import type { Env } from "./_env";
import { toResponse, type AgentResult } from "./_agent-common";
import { nowISO } from "./_lib";

/** The two tables; the name is interpolated into SQL, so it comes from this
 *  list and never from input. */
export type IdempotencyTable = "acp_idempotency" | "ucp_idempotency";

export interface IdempotencyPolicy {
  table: IdempotencyTable;
  /** The body's fingerprint, compared on a repeated key. */
  fingerprint: (raw: string) => Promise<string>;
  /** A key older than this is a new request. */
  retentionMs: number;
  /** Rows older than this are deleted when a new key is claimed. */
  purgeAfterMs: number;
  /** A key still "in flight" after this long belonged to a request that died. */
  staleMs: number;
  /** The answer to a request without a key, or null when one may go unkeyed. */
  missingKey: AgentResult | null;
  tooLong: AgentResult;
  conflict: AgentResult;
  inFlight: AgentResult;
}

interface KeyRow {
  body_hash: string;
  state: string;
  status: number | null;
  response_json: string | null;
  created_at: string;
}

export type Claim = { proceed: true } | { proceed: false; response: Response };

export interface IdempotencyStore {
  claim(env: Env, scope: string, key: string | null, raw: string): Promise<Claim>;
  settle(env: Env, scope: string, key: string, result: AgentResult): Promise<void>;
  release(env: Env, scope: string, key: string): Promise<void>;
}

const ago = (ms: number): string => new Date(Date.now() - ms).toISOString();
const refuse = (result: AgentResult): Claim => ({ proceed: false, response: toResponse(result) });

export function idempotencyStore(policy: IdempotencyPolicy): IdempotencyStore {
  const { table } = policy;

  /** Claims a row someone else held — an expired key or a dead runner's — but
   *  only if it is still the row we read. */
  async function takeOver(env: Env, scope: string, key: string, seenAt: string, hash: string): Promise<Claim> {
    const res = await env.SHOP_DB.prepare(
      `UPDATE ${table} SET body_hash = ?, state = 'in_flight', status = NULL, response_json = NULL, created_at = ?
      WHERE scope = ? AND key = ? AND created_at = ?`,
    )
      .bind(hash, nowISO(), scope, key, seenAt)
      .run();
    return res.meta.changes === 1 ? { proceed: true } : refuse(policy.inFlight);
  }

  async function release(env: Env, scope: string, key: string): Promise<void> {
    await env.SHOP_DB.prepare(`DELETE FROM ${table} WHERE scope = ? AND key = ? AND state = 'in_flight'`)
      .bind(scope, key)
      .run();
  }

  return {
    async claim(env, scope, key, raw) {
      if (!key) return policy.missingKey ? refuse(policy.missingKey) : { proceed: true };
      if (key.length > 255) return refuse(policy.tooLong);
      const hash = await policy.fingerprint(raw);
      const db = env.SHOP_DB;

      const inserted = await db
        .prepare(`INSERT OR IGNORE INTO ${table} (scope, key, body_hash, state, created_at) VALUES (?, ?, ?, 'in_flight', ?)`)
        .bind(scope, key, hash, nowISO())
        .run();
      if (inserted.meta.changes === 1) {
        await db.prepare(`DELETE FROM ${table} WHERE created_at < ?`).bind(ago(policy.purgeAfterMs)).run();
        return { proceed: true };
      }

      const row = (await db
        .prepare(`SELECT body_hash, state, status, response_json, created_at FROM ${table} WHERE scope = ? AND key = ?`)
        .bind(scope, key)
        .first<KeyRow>())!;

      // Past retention the key is new again. The UPDATE names the row it read,
      // so two requests reusing an expired key still produce one runner.
      if (row.created_at < ago(policy.retentionMs)) return takeOver(env, scope, key, row.created_at, hash);
      if (row.body_hash !== hash) return refuse(policy.conflict);
      if (row.state === "in_flight") {
        if (row.created_at < ago(policy.staleMs)) return takeOver(env, scope, key, row.created_at, row.body_hash);
        return refuse(policy.inFlight);
      }
      return {
        proceed: false,
        response: new Response(row.response_json, {
          status: row.status ?? 200,
          headers: {
            "content-type": "application/json; charset=utf-8",
            "cache-control": "no-store",
            "x-content-type-options": "nosniff",
            "idempotency-key": key,
            "idempotent-replayed": "true",
          },
        }),
      };
    },

    /** Stores the outcome for replay — or, for a 5xx, forgets the key, so the
     *  retry runs again. */
    async settle(env, scope, key, result) {
      if (result.status >= 500) return release(env, scope, key);
      await env.SHOP_DB.prepare(`UPDATE ${table} SET state = 'done', status = ?, response_json = ? WHERE scope = ? AND key = ?`)
        .bind(result.status, JSON.stringify(result.body), scope, key)
        .run();
    },

    release,
  };
}
