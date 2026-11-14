// Cloudflare Pages Function: the second step of signing in (GO-140).
//   POST /api/auth/mfa/verify   {code} | {backupCode} — the challenge page's plain forms or JSON with x-ssg-auth: 1
//                               {passkey: <assertion>} — JSON, from the page's script
//
// Needs the pending-step cookie a first factor left (_mfa-pending.ts): the
// account is the handle's own, never one the request names. The code, backup
// code or passkey answer is checked for that account (_mfa-verify.ts, with
// its lockout); then the handle is used up and the session made by
// openSession — recording which second factor was passed and when.
// JSON: 200 {user, expiresAt, sessionExpiresAt, next} + the session cookies;
// a form gets a 303 to the `next` the sign-in started with. Refusals: 401
// mfa_pending_invalid (no handle, expired, used — a form goes to the login
// page with ?error=mfa_expired); 401 invalid_code; 429 too_many_attempts
// (retry-after); 503 mfa_unavailable (AUTH_MFA_KEY unusable, for app codes);
// 500 permissions_too_large; 404 not_found while the second factor is off.

import { preparePost } from "../_context";
import { json, redirect } from "../_lib";
import type { Env } from "../_lib";
import { mfaEnabled } from "../_mfa-crypto";
import { notFound } from "../_mfa-gate";
import { clearPendingCookie, consumePending, readPending } from "../_mfa-pending";
import { CHALLENGE_PAGE } from "../_mfa-signin";
import { checkSecondFactor } from "../_mfa-verify";
import { withParams } from "../_next";
import { isForm, readBody } from "../_request";
import { openSession } from "../_signin";
import { publicUser } from "../_users";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await preparePost(request, env, true);
  if (ctx instanceof Response) return ctx;
  if (!mfaEnabled(env)) return notFound();
  const form = isForm(request);
  const body = (await readBody(request)) ?? {};
  const expired = (): Response =>
    form ? redirect(withParams(ctx.s.loginUrl, { error: "mfa_expired" }), [clearPendingCookie()]) : json({ error: "mfa_pending_invalid" }, 401, {}, [clearPendingCookie()]);
  const pending = await readPending(env, request, ctx.now);
  if (!pending) return expired();

  const checked = await checkSecondFactor(ctx, pending.user, body, "signin", pending.hash);
  if (!checked.ok) {
    const headers: Record<string, string> = checked.error === "too_many_attempts" ? { "retry-after": String(checked.retryAfter) } : {};
    return form ? redirect(withParams(CHALLENGE_PAGE, { error: checked.error, use: body.backupCode ? "backup" : "" })) : json({ error: checked.error }, checked.status, headers);
  }
  if (!(await consumePending(env, pending.hash, ctx.now))) return expired();
  const started = await openSession(ctx, pending.user, pending.method, checked.factor);
  const next = pending.next || ctx.s.afterLoginUrl;
  if (!started.ok) {
    const out = [clearPendingCookie()];
    return form ? redirect(withParams(ctx.s.loginUrl, { error: "permissions_too_large" }), out) : json({ error: "permissions_too_large", detail: started.tooLarge }, 500, {}, out);
  }
  const cookies = [clearPendingCookie(), ...started.issued.cookies];
  if (form) return redirect(next, cookies);
  return json({ user: publicUser(pending.user), expiresAt: started.issued.exp, sessionExpiresAt: started.session.expires_at, next }, 200, {}, cookies);
};
