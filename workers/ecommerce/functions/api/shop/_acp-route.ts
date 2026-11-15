// The wrapper every ACP route goes through, in this order:
//
//   ACP off → 404 (as if the routes did not exist)
//   body read (capped) → rate limit (credential + address, _ratelimit)
//   → authentication and API-Version (_acp-auth)
//   schema → shop configured → JSON parsed
//   Idempotency-Key claimed (POST) → the operation → the key settled
//
// and every answer echoes Request-Id, and Idempotency-Key on a POST.

import type { Env } from "./_env";
import { acpGate } from "./_acp-auth";
import { spendAgentBudget } from "./_agent-common";
import type { OpInput } from "./_acp-checkout";
import { acpEnabled, acpError, parseBody, toResponse, type AcpResult } from "./_acp-http";
import { claimKey, releaseKey, settleKey } from "./_acp-idempotency";
import { audit } from "./_audit";
import { shopOrigin } from "./_fulfil";
import { ensureSchema } from "./_schema";
import { shopIsConfigured } from "./_settings";

/** Bodies here are a few items and an address; anything near this is not. */
export const MAX_BODY_BYTES = 64 * 1024;

export type Operation = (input: OpInput) => Promise<AcpResult>;

export function acpRoute(operation: Operation, opts: { idempotent: boolean }): PagesFunction<Env> {
  return async (context) => {
    const { request, env } = context;
    if (!acpEnabled(env)) return new Response("Not Found", { status: 404 });

    const echo: Record<string, string> = {};
    const requestId = request.headers.get("request-id");
    if (requestId) echo["request-id"] = requestId.slice(0, 255);
    const answer = (result: AcpResult, extra: Record<string, string> = {}): Response => toResponse(result, { ...echo, ...extra });

    if (!env.SHOP_DB) return answer(acpError(503, "service_unavailable", "not_configured", "The shop database is not bound."));
    const raw = await request.text();
    if (raw.length > MAX_BODY_BYTES) return answer(acpError(413, "invalid_request", "body_too_large", "The request body is too large."));

    // Per platform credential + address; ACP's bearer token is the gate, so
    // this is what stops a leaked or looping client from hammering the shop.
    const budget = await spendAgentBudget(env, request, "acp", request.headers.get("authorization") ?? "");
    if (!budget.ok) {
      return answer({
        ...acpError(429, "invalid_request", "rate_limited", "Too many requests; retry after the time in Retry-After."),
        headers: { "retry-after": String(budget.retryAfter) },
      });
    }

    const denied = await acpGate(request, env, raw);
    if (denied) return answer(denied);

    await ensureSchema(env);
    const configured = await shopIsConfigured(env);
    if (!configured.ok) return answer(acpError(503, "service_unavailable", "shop_not_configured", "The shop is not configured to sell yet."));

    const parsed = parseBody(raw);
    if (!parsed.ok) return answer(acpError(400, "invalid_request", "invalid_json", "The body is not a JSON object."));

    const key = request.headers.get("idempotency-key");
    const scope = `${request.method} ${new URL(request.url).pathname}`;
    if (opts.idempotent) {
      const claim = await claimKey(env, scope, key, raw);
      if (!claim.proceed) {
        for (const [name, value] of Object.entries(echo)) claim.response.headers.set(name, value);
        return claim.response;
      }
    }
    const keyHeader: Record<string, string> = opts.idempotent ? { "idempotency-key": key! } : {};

    let result: AcpResult;
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
      if (opts.idempotent) await releaseKey(env, scope, key!);
      await audit(env, "acp", "acp.error", scope, { error: String(e).slice(0, 300) });
      return answer(acpError(500, "processing_error", "internal_error", "The request could not be processed."), keyHeader);
    }
    if (opts.idempotent) await settleKey(env, scope, key!, result);
    return answer(result, keyHeader);
  };
}
