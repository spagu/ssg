// What the two agent checkouts — ACP (_acp-*.ts) and UCP (_ucp-*.ts) — have
// in common at the HTTP edge: a handler's result before it becomes a Response,
// and the two ways a JSON body is read and fingerprinted.
//
// Each protocol keeps its own error shape and headers; only what is genuinely
// the same lives here, so a change for one protocol cannot silently change the
// other.

import type { Env } from "./_env";
import { json, sha256hex } from "./_lib";
import { withinBudget } from "./_ratelimit";

/** What a protocol handler returns; the route wrapper turns it into a
 *  Response and stores it against the idempotency key. */
export interface AgentResult {
  status: number;
  body: unknown;
  headers?: Record<string, string>;
}

/** What a protocol operation is given by its route wrapper. */
export interface AgentOpInput {
  request: Request;
  env: Env;
  body: Record<string, unknown>;
  id: string;
  origin: string;
  waitUntil: (p: Promise<unknown>) => void;
}

/** A result as a Response, with the shop's JSON headers (no-store, nosniff):
 *  nothing an agent receives can be cached on the way. */
export function toResponse(result: AgentResult, headers: Record<string, string> = {}): Response {
  return json(result.body, result.status, { ...headers, ...result.headers });
}

/** JSON with object keys sorted at every level: two bodies that differ only in
 *  key order, or in `1.0` against `1`, hash the same (ACP RFC §6.2). Arrays
 *  keep their order and `null` stays distinct from an absent key. */
export function canonicalJson(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonicalJson).join(",")}]`;
  if (value !== null && typeof value === "object") {
    // Keys of one object are unique, so the comparator never sees a tie.
    const entries = Object.entries(value as Record<string, unknown>).sort(([a], [b]) => (a < b ? -1 : 1));
    return `{${entries.map(([k, v]) => `${JSON.stringify(k)}:${canonicalJson(v)}`).join(",")}}`;
  }
  return JSON.stringify(value);
}

/** Parses a body, or says it could not. An empty body is `{}`: a cancel's body
 *  is optional, and every route checks its own required fields. */
export function parseBody(raw: string): { ok: true; value: Record<string, unknown> } | { ok: false } {
  if (raw.trim() === "") return { ok: true, value: {} };
  try {
    const value = JSON.parse(raw) as unknown;
    if (value === null || typeof value !== "object" || Array.isArray(value)) return { ok: false };
    return { ok: true, value: value as Record<string, unknown> };
  } catch {
    return { ok: false };
  }
}

/** Spends one request of an agent checkout's budget (_ratelimit.ts), keyed by
 *  the caller's address and a hash of the credential it presented — the
 *  header as sent, valid or not, so a flood without a key shares one budget
 *  per address. Applied before authentication: refusing a flood is cheaper
 *  than checking it. */
export async function spendAgentBudget(
  env: Env,
  request: Request,
  bucket: "acp" | "ucp",
  credential: string,
): Promise<{ ok: boolean; retryAfter: number }> {
  const subject = (await sha256hex(`${bucket}:${credential}`)).slice(0, 32);
  const verdict = await withinBudget(env, request, bucket, subject);
  return { ok: verdict.ok, retryAfter: Math.max(1, verdict.retryAfter) };
}
