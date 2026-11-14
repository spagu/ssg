// Who may trigger a rebuild. A leading underscore keeps this file out of the
// Pages route table.
//
// Two ways, both for machines (a CMS webhook, a cron, CI):
//   1. the shared REPUBLISH_KEY — Bearer, X-Republish-Key or ?key= — as always;
//   2. when REPUBLISH_AUTH_* is set (GO-141): a service access token from the
//      `auth` worker (POST /api/auth/token) as `Authorization: Bearer`, whose
//      permissions include `republish-trigger:run`, checked locally by the
//      portable verifier. Variables: REPUBLISH_AUTH_ISSUER,
//      REPUBLISH_AUTH_LOGIN_URL, and REPUBLISH_AUTH_PUBLIC_KEY or
//      REPUBLISH_AUTH_JWKS_URL; REPUBLISH_AUTH_AUDIENCE optional.
// Without REPUBLISH_AUTH_* this is requireKey, unchanged: the answers are
// byte for byte those of 1.9.8 (test/compat-key.test.ts). With both set, both
// are accepted — the key opens this one action and nothing else, so accepting
// it beside `auth` bypasses no permission — and REPUBLISH_KEY may be left
// unset. A half-set REPUBLISH_AUTH_* is a 503, never a silent fall-back.
//
// A person's session cookie is never read: only the bearer header reaches the
// verifier, which takes service tokens only on that channel. A "Republish"
// button for a signed-in person would be a separate, CSRF-checked route; it
// is deliberately not part of this endpoint.

import { json, presentedKey, requireKey, timingSafeEqual } from "./_lib";
import type { Env } from "./_lib";
import { can, configFrom, denied, forbidden, verifySession } from "./_session-verifier";

export const RUN_PERMISSION = "republish-trigger:run";

// authorizeTrigger returns null when the caller may trigger, else the answer to send.
export async function authorizeTrigger(request: Request, env: Env): Promise<Response | null> {
  const auth = configFrom(env, "REPUBLISH");
  if (!auth) return requireKey(request, env);
  if (!env.REPUBLISH_TOKEN || !env.REPUBLISH_PROVIDER) return json({ error: "republish not configured" }, 503);
  // Only the bearer header, on a POST: no cookie can be read, and a failure is
  // always JSON (a webhook is never redirected to a login page).
  const bare = new Request(request.url, { method: "POST", headers: { authorization: request.headers.get("authorization") ?? "" } });
  if (auth.configProblem) return denied(bare, auth, "not_configured");
  const key = presentedKey(request);
  if (env.REPUBLISH_KEY && key && timingSafeEqual(key, env.REPUBLISH_KEY)) return null;
  const v = await verifySession(bare, auth);
  if (!v.ok) return denied(bare, auth, v.reason);
  return can(v.perms, RUN_PERMISSION) ? null : forbidden(RUN_PERMISSION);
}
