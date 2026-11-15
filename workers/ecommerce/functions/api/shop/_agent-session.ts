// An agent checkout session as a D1 row, and the only ways it may change —
// shared by ACP (acp_sessions) and UCP (ucp_sessions), whose rows have the
// same columns and differ only in their status words.
//
// Why D1 and not a Durable Object (#197 asked for "D1 or Durable Objects"):
//
//   - The session ends as an order, an invoice and download tokens, and all of
//     those are D1 rows already. Keeping the session in the same database
//     means one store to back up, export and erase, one place the panel reads,
//     and no second copy of an order that could disagree with the first.
//   - What a Durable Object would add is a single-threaded writer. The writes
//     here need less than that: each one is a compare-and-swap on `version`
//     (and the completion on `status`), so two concurrent updates cannot both
//     land and a session cannot be completed twice. A loser re-reads and
//     either retries or is told why not.
//   - Pages Functions cannot define a Durable Object class; a DO would need a
//     separate Worker, binding and deployment for every shop that turns an
//     agent checkout on.
//
// What D1 does not give is a transaction across the call to Stripe, so each
// completion is built from steps that are each safe to repeat.

import type { Env } from "./_env";
import { nowISO } from "./_lib";

/** The two tables; interpolated into SQL, so never taken from input. */
export type SessionTable = "acp_sessions" | "ucp_sessions";

export interface AgentSessionRow<S extends string = string> {
  id: string;
  status: S;
  version: number;
  currency: string;
  items_json: string;
  buyer_json: string | null;
  details_json: string | null;
  country: string | null;
  locale: string | null;
  priced_json: string;
  order_id: string | null;
  /** The plaintext key of the order this session made. It is the same key
   *  the storefront puts in the buyer's thank-you URL, kept so the session can
   *  hand the platform the order's permalink on every later read. */
  order_key: string | null;
  created_at: string;
  updated_at: string;
  expires_at: string;
}

/** The columns a write may set. Names come from this list, never from input. */
export type AgentSessionPatch<S extends string = string> = Partial<
  Pick<
    AgentSessionRow<S>,
    "status" | "items_json" | "buyer_json" | "details_json" | "country" | "priced_json" | "order_id" | "order_key"
  >
>;

export interface SessionStore<S extends string> {
  insert(env: Env, row: AgentSessionRow<S>): Promise<void>;
  /** Writes `patch` only if nobody else wrote since `row` was read. False
   *  means the caller lost the race and must re-read. Because every write
   *  bumps `version`, a caller that checked the status of `row` first has in
   *  effect made the write conditional on that status too. */
  casUpdate(env: Env, row: AgentSessionRow<S>, patch: AgentSessionPatch<S>): Promise<boolean>;
  /** The session, with an overdue open one moved to `expired` on the way. */
  load(env: Env, id: string): Promise<AgentSessionRow<S> | null>;
  /** Links the order a completion wrote. The session is held by that
   *  completion (its status says so), so a plain write; the version is left
   *  alone so the completion's last CAS still matches. */
  attachOrder(env: Env, row: AgentSessionRow<S>, orderId: string, orderKey: string): Promise<void>;
}

export function sessionStore<S extends string>(
  table: SessionTable,
  states: { expired: S; isOpen: (status: S) => boolean },
): SessionStore<S> {
  const select = (env: Env, id: string) =>
    env.SHOP_DB.prepare(`SELECT * FROM ${table} WHERE id = ?`).bind(id).first<AgentSessionRow<S>>();

  const store: SessionStore<S> = {
    async insert(env, row) {
      await env.SHOP_DB.prepare(
        `INSERT INTO ${table} (id, status, version, currency, items_json, buyer_json, details_json, country, locale,
                               priced_json, order_id, order_key, created_at, updated_at, expires_at)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      )
        .bind(
          row.id, row.status, row.version, row.currency, row.items_json, row.buyer_json, row.details_json,
          row.country, row.locale, row.priced_json, row.order_id, row.order_key, row.created_at, row.updated_at,
          row.expires_at,
        )
        .run();
    },

    async casUpdate(env, row, patch) {
      const keys = Object.keys(patch) as Array<keyof AgentSessionPatch<S>>;
      const sets = keys.map((k) => `${k} = ?`).join(", ");
      const res = await env.SHOP_DB.prepare(
        `UPDATE ${table} SET ${sets}, version = version + 1, updated_at = ?
      WHERE id = ? AND version = ?`,
      )
        .bind(...keys.map((k) => patch[k] ?? null), nowISO(), row.id, row.version)
        .run();
      return res.meta.changes === 1;
    },

    async load(env, id) {
      const row = await select(env, id);
      if (!row) return null;
      // A completion in progress is never expired under its own feet: it
      // either finishes or is recovered by the next completion.
      if (states.isOpen(row.status) && row.expires_at < nowISO()) {
        await store.casUpdate(env, row, { status: states.expired } as AgentSessionPatch<S>);
        await cancelPendingOrder(env, row.order_id);
        return select(env, id);
      }
      return row;
    },

    async attachOrder(env, row, orderId, orderKey) {
      await env.SHOP_DB.prepare(`UPDATE ${table} SET order_id = ?, order_key = ? WHERE id = ? AND version = ?`)
        .bind(orderId, orderKey, row.id, row.version)
        .run();
    },
  };
  return store;
}

/** An order a session started but never paid goes when the session does. */
export async function cancelPendingOrder(env: Env, orderId: string | null): Promise<void> {
  if (!orderId) return;
  await env.SHOP_DB.prepare(`UPDATE orders SET status = 'cancelled', updated_at = ? WHERE id = ? AND status = 'pending'`)
    .bind(nowISO(), orderId)
    .run();
}

export function parseJson<T>(value: string | null, fallback: T): T {
  if (!value) return fallback;
  return JSON.parse(value) as T;
}
