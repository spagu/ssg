// The wrapper every UCP route goes through, in this order:
//
//   UCP off → 404 (as if the routes did not exist)
//   body read (capped) → rate limit (credential + address, _ratelimit)
//   → configuration, X-API-Key and UCP-Agent (_ucp-auth)
//   schema → shop configured → JSON parsed
//   Idempotency-Key claimed (POST/PUT, when sent) → the operation → settled
//
// and every answer echoes Request-Id, and Idempotency-Key when one was sent.

import type { Env } from "./_env";
import { spendAgentBudget, toResponse, parseBody, type AgentOpInput } from "./_agent-common";
import { idempotencyStore } from "./_agent-idempotency";
import { audit } from "./_audit";
import { shopOrigin } from "./_fulfil";
import { sha256hex } from "./_lib";
import { ensureSchema } from "./_schema";
import { shopIsConfigured } from "./_settings";
import { ucpGate } from "./_ucp-auth";
import { ucpEnabled, ucpError, type UcpResult } from "./_ucp-http";

/** Bodies here are a few items and an address; anything near this is not. */
export const MAX_BODY_BYTES = 64 * 1024;

export type Operation = (input: AgentOpInput) => Promise<UcpResult>;

const inFlight: UcpResult = {
  ...ucpError(409, "idempotency_in_flight", "A request with this Idempotency-Key is still being processed."),
  headers: { "retry-after": "1" },
};

/** UCP's policy for the shared store (_agent-idempotency.ts), from
 *  signatures.md "Idempotency Key Requirements": the body is compared by the
 *  SHA-256 of its raw bytes; a mismatch is 409; keys are kept the
 *  recommended 48 hours. The key itself is optional (checkout-rest.md: "When
 *  provided, the server MUST …"); the OpenAPI marks it required — UCP.md,
 *  ambiguity 3. */
const keys = idempotencyStore({
  table: "ucp_idempotency",
  fingerprint: (raw) => sha256hex(raw),
  retentionMs: 48 * 3600 * 1000,
  purgeAfterMs: 7 * 24 * 3600 * 1000,
  staleMs: 60_000,
  missingKey: null,
  tooLong: ucpError(400, "invalid_idempotency_key", "Idempotency-Key may be at most 255 characters."),
  conflict: ucpError(409, "idempotency_conflict", "Idempotency-Key has already been used with a different request body."),
  inFlight,
});

export function ucpRoute(operation: Operation, opts: { mutates: boolean }): PagesFunction<Env> {
  return async (context) => {
    const { request, env } = context;
    if (!ucpEnabled(env)) return new Response("Not Found", { status: 404 });

    const echo: Record<string, string> = {};
    const requestId = request.headers.get("request-id");
    if (requestId) echo["request-id"] = requestId.slice(0, 255);
    const key = opts.mutates ? request.headers.get("idempotency-key") : null;
    if (key) echo["idempotency-key"] = key.slice(0, 255);
    const answer = (result: UcpResult): Response => toResponse(result, echo);

    if (!env.SHOP_DB) return answer(ucpError(503, "not_configured", "The shop database is not bound."));
    const raw = await request.text();
    if (raw.length > MAX_BODY_BYTES) return answer(ucpError(413, "invalid_request", "The request body is too large."));

    const budget = await spendAgentBudget(env, request, "ucp", request.headers.get("x-api-key") ?? "");
    if (!budget.ok) {
      return answer({ ...ucpError(429, "rate_limited", "Too many requests; retry after the time in Retry-After."), headers: { "retry-after": String(budget.retryAfter) } });
    }
    const denied = await ucpGate(request, env);
    if (denied) return answer(denied);

    await ensureSchema(env);
    const configured = await shopIsConfigured(env);
    if (!configured.ok) return answer(ucpError(503, "shop_not_configured", "The shop is not configured to sell yet."));

    const parsed = parseBody(raw);
    if (!parsed.ok) return answer(ucpError(400, "invalid_request", "The body is not a JSON object."));

    const scope = `${request.method} ${new URL(request.url).pathname}`;
    if (key) {
      let claim;
      try {
        claim = await keys.claim(env, scope, key, raw);
      } catch {
        // signatures.md: "On storage failure — fail closed (reject request
        // with 503)". Running unkeyed could charge twice.
        return answer({ ...ucpError(503, "unavailable", "The request could not be recorded; retry."), headers: { "retry-after": "1" } });
      }
      if (!claim.proceed) {
        for (const [name, value] of Object.entries(echo)) claim.response.headers.set(name, value);
        return claim.response;
      }
    }

    let result: UcpResult;
    try {
      result = await operation({
        request,
        env,
        body: parsed.value,
        id: String(context.params.id ?? ""),
        origin: await shopOrigin(env, request),
        waitUntil: (p) => context.waitUntil(p),
      });
    } catch (e) {
      // Never the message to the caller: it can carry SQL or a provider's
      // wording. The audit log has it for the owner.
      if (key) await keys.release(env, scope, key);
      await audit(env, "ucp", "ucp.error", scope, { error: String(e).slice(0, 300) });
      return answer(ucpError(500, "internal_error", "The request could not be processed."));
    }
    if (key) await keys.settle(env, scope, key, result);
    return answer(result);
  };
}
