// Cloudflare Pages Function: log in with an e-mail address and a password.
//   POST /api/auth/login   JSON {email, password, next?, "cf-turnstile-response"?}
//                          or the same fields from a plain HTML form
//
// JSON: 200 {user, expiresAt, sessionExpiresAt} + the two session cookies;
// 401 {error: "invalid_credentials"} for an unknown address, a wrong password
// and a blocked account alike — same status, same body, and the same hashing
// work (a dummy derivation when there is no hash to check), so neither the
// answer nor its time says whether an account exists. 429 while locked, 403
// when Turnstile is required and missing. A form gets a 303 instead: to the
// safe `next` path on success, back to AUTH_LOGIN_URL?error=<code> on failure.
// 404 method_disabled when AUTH_PASSWORD_LOGIN=off (GO-151); every success is
// an audit event (_audit.ts).
// 500 {error: "permissions_too_large", detail} when the account's permissions
// do not fit in the access cookie (GO-133): the session just made is ended,
// the reason logged, and a form goes back with ?error=permissions_too_large.
// GO-140: the session is made by startSession (_signin.ts), like every other
// way in. An account with a second factor gets no session here: 401
// {error: "second_factor_required", factors, challengeUrl, expiresAt} with the
// pending-step cookie (a form: 303 to /auth/mfa); POST /api/auth/mfa/verify
// finishes the sign-in. A JSON success may add `enrol: true` (a role needs a
// second factor the account has not set up; a form goes to /auth/mfa/enrol).

import { DEFAULT_ITERATIONS } from "./_config";
import { permissionsTooLarge, preparePost } from "./_context";
import { clientIP, json, normEmail, redirect } from "./_lib";
import type { Env } from "./_lib";
import { clearFailures, lockKey, lockState, recordFailure, releaseAttempt } from "./_lockout";
import { safeNext, withParams } from "./_next";
import { hashPassword, needsUpgrade, verifyPassword } from "./_password";
import { isForm, readBody, str } from "./_request";
import { CHALLENGE_PAGE } from "./_mfa-signin";
import { purgeStale } from "./_sessions";
import { passwordEnabled } from "./_provider-methods";
import { signInRedirect, startSession } from "./_signin";
import { verifyTurnstile } from "./_turnstile";
import { findUserByEmail, publicUser, touchLogin, upgradeHash } from "./_users";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await preparePost(request, env, true);
  if (ctx instanceof Response) return ctx;
  const { s, now } = ctx;
  const form = isForm(request);
  const body = await readBody(request);
  const next = safeNext(body?.next, "");

  const fail = (error: string, status: number, headers: Record<string, string> = {}): Response => {
    if (!form) return json({ error }, status, headers);
    return redirect(withParams(s.loginUrl, { error, next }));
  };

  // AUTH_PASSWORD_LOGIN=off (GO-151): the method does not exist here.
  if (!passwordEnabled(env)) return fail("method_disabled", 404);
  const email = normEmail(body?.email);
  const password = str(body?.password);
  if (!email || !password) return fail("invalid_request", 400);

  const ip = clientIP(request);
  const accountKey = await lockKey(env, "account", email);
  const ipKey = await lockKey(env, "ip", ip);
  const state = await lockState(env, [accountKey, ipKey], now);
  if (state.retryAfter > 0) return fail("too_many_attempts", 429, { "retry-after": String(state.retryAfter) });
  if (s.turnstileSecret && state.failures >= s.turnstileAfter) {
    if (!(await verifyTurnstile(s.turnstileSecret, str(body?.["cf-turnstile-response"]), ip))) return fail("turnstile_required", 403);
  }

  // The attempt is counted before the password is checked (SR-02): parallel
  // guesses cannot all slip past the lock read above.
  const ipEntry = { key: ipKey, kind: "ip" as const, threshold: s.lockIP };
  const over = await recordFailure(env, [{ key: accountKey, kind: "account", threshold: s.lockAccount }, ipEntry], s, now);
  if (over > 0) return fail("too_many_attempts", 429, { "retry-after": String(over) });

  const user = await findUserByEmail(env, email);
  // A password longer than any we accept cannot be right; it is checked
  // against the dummy (cut to the cap first, so its length costs nothing).
  const tooLong = [...password].length > s.passwordMax;
  const stored = tooLong ? null : (user?.password_hash ?? null);
  const ok = await verifyPassword(tooLong ? password.slice(0, s.passwordMax) : password, stored, s.iterations, DEFAULT_ITERATIONS);
  if (!ok || !user || user.status !== "active") return fail("invalid_credentials", 401);

  // The IP counter only gets this attempt back: clearing it on success would
  // let one source reset its count by logging into its own account between guesses.
  await clearFailures(env, accountKey);
  await releaseAttempt(env, [ipEntry]);
  if (needsUpgrade(stored, s.iterations)) {
    try {
      await upgradeHash(env, user.id, stored as string, await hashPassword(password, s.iterations), now);
    } catch {
      // The runtime refused the configured cost (above its cap): keep the
      // working hash and say so — without anything about the account.
      console.warn(`auth: re-hashing at ${s.iterations} PBKDF2 iterations failed; lower AUTH_PBKDF2_ITERATIONS`);
    }
  }
  await touchLogin(env, user.id, now);
  await purgeStale(env, now);
  const started = await startSession(ctx, user, "password", next);
  if (form) return started.ok === false ? fail("permissions_too_large", 500) : signInRedirect(ctx, started, next || s.afterLoginUrl);
  if (started.ok === "second_factor") {
    const { factors, expiresAt } = started;
    return json({ error: "second_factor_required", factors, challengeUrl: CHALLENGE_PAGE, expiresAt }, 401, {}, [started.cookie]);
  }
  if (!started.ok) return permissionsTooLarge(started.tooLarge);
  const { session, issued } = started;
  return json({ user: publicUser(user), expiresAt: issued.exp, sessionExpiresAt: session.expires_at, ...(started.enrol ? { enrol: true } : {}) }, 200, {}, issued.cookies);
};
