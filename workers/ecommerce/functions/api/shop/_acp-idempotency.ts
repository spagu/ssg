// Idempotency keys, as section 6 of the agentic checkout RFC defines them.
//
//   - every POST carries `Idempotency-Key` (else 400 idempotency_key_required);
//   - a key is scoped to the endpoint path (there is one authenticated
//     identity: the platform holding SHOP_ACP_API_KEY);
//   - the same key with the same body replays the stored response, status
//     included, with `Idempotent-Replayed: true`, and runs nothing again;
//   - the same key with another body is 422 idempotency_conflict;
//   - the same key while the first request still runs is 409
//     idempotency_in_flight with Retry-After;
//   - a 5xx is never stored: the retry runs as a fresh request;
//   - keys are kept at least 24 hours; an older key is a new request.
//
// The INSERT OR IGNORE is the lock, the same pattern as the payment webhooks'
// inbox: two requests racing with one key produce exactly one runner.
//
// The RFC recommends committing the key and the business write in one ACID
// transaction. D1 has no transaction that spans a fetch to Stripe, so the
// handlers are written to be safe to re-run instead (a session's version
// counter, the order's status guard, Stripe's own idempotency key), and a key
// whose runner died is taken over after IN_FLIGHT_STALE_MS.

import type { Env } from "./_env";
import { idempotencyStore } from "./_agent-idempotency";
import { acpError, canonicalJson, parseBody, type AcpResult } from "./_acp-http";
import { sha256hex } from "./_lib";

/** The RFC's minimum retention. */
export const IDEMPOTENCY_RETENTION_MS = 24 * 3600 * 1000;
/** A key still "in flight" after this long belonged to a request that died. */
export const IN_FLIGHT_STALE_MS = 60_000;

export type { Claim } from "./_agent-idempotency";

const inFlight: AcpResult = {
  ...acpError(409, "invalid_request", "idempotency_in_flight", "A request with this Idempotency-Key is currently being processed"),
  headers: { "retry-after": "1" },
};

/** ACP's policy for the shared store (_agent-idempotency.ts): canonical JSON
 *  as the fingerprint, a key required on every POST, 422 on a reused key. */
const store = idempotencyStore({
  table: "acp_idempotency",
  // Canonical JSON when the body parses, the bytes otherwise (RFC §6.2).
  fingerprint: (raw) => {
    const parsed = parseBody(raw);
    return sha256hex(parsed.ok ? canonicalJson(parsed.value) : raw);
  },
  retentionMs: IDEMPOTENCY_RETENTION_MS,
  purgeAfterMs: 7 * 24 * 3600 * 1000,
  staleMs: IN_FLIGHT_STALE_MS,
  missingKey: acpError(400, "invalid_request", "idempotency_key_required", "Idempotency-Key header is required on all POST requests"),
  tooLong: acpError(400, "invalid_request", "invalid_idempotency_key", "Idempotency-Key may be at most 255 characters."),
  conflict: acpError(422, "invalid_request", "idempotency_conflict", "Idempotency-Key has already been used with a different request body"),
  inFlight,
});

/** Claims `key` for this request, or answers in its place. */
export const claimKey = (env: Env, scope: string, key: string | null, raw: string) => store.claim(env, scope, key, raw);

/** Records the outcome against the key: stored for replay, or — for a 5xx —
 *  forgotten, so the retry runs again. */
export const settleKey = (env: Env, scope: string, key: string, result: AcpResult) => store.settle(env, scope, key, result);

export const releaseKey = (env: Env, scope: string, key: string) => store.release(env, scope, key);
