// Cloudflare Pages Function: change your own password.
//   POST /api/auth/password   JSON {current, password}; header x-ssg-auth: 1
//                             or the same fields (+ next?) from a plain HTML form
//
// Needs a live session (access cookie, or the refresh cookie after a pause)
// AND the current password; wrong current passwords count towards the
// account's login lock. The new one must pass the rules (length, not the
// address). Every OTHER session of the account is revoked; this one stays.
// JSON: 200 {ok: true, revoked}; 401 not_authenticated | invalid_credentials;
// 400 {error: <rule>}; 429 while locked. A form gets a 303 to the safe `next`
// (default /auth/account) with ?ok=password_changed or ?error=<code>.

import { DEFAULT_ITERATIONS } from "./_config";
import { hashingUnavailable, preparePost, whoAmI } from "./_context";
import { json, redirect } from "./_lib";
import type { Env } from "./_lib";
import { clearFailures, lockKey, lockState, recordFailure } from "./_lockout";
import { safeNext, withParams } from "./_next";
import { hashPassword, passwordProblem, verifyPassword } from "./_password";
import { isForm, readBody, str } from "./_request";
import { revokeUserSessions } from "./_sessions";
import { setPasswordHash } from "./_users";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await preparePost(request, env, true);
  if (ctx instanceof Response) return ctx;
  const { s, now } = ctx;
  const body = await readBody(request);
  const form = isForm(request);
  const back = safeNext(body?.next, "/auth/account");
  const answer = (status: number, data: Record<string, unknown>, headers: Record<string, string> = {}): Response => {
    if (!form) return json(data, status, headers);
    return redirect(withParams(back, status === 200 ? { ok: "password_changed" } : { error: String(data.error) }));
  };

  const who = await whoAmI(ctx, true);
  if (!who) return answer(401, { error: "not_authenticated" });
  const { user, session } = who;
  const key = await lockKey(env, "account", user.email);
  const state = await lockState(env, [key], now);
  if (state.retryAfter > 0) return answer(429, { error: "too_many_attempts" }, { "retry-after": String(state.retryAfter) });

  // Counted before the check (SR-02), given back by clearFailures on success.
  const over = await recordFailure(env, [{ key, kind: "account", threshold: s.lockAccount }], s, now);
  if (over > 0) return answer(429, { error: "too_many_attempts" }, { "retry-after": String(over) });
  const current = str(body?.current);
  const okCurrent = await verifyPassword(current.slice(0, s.passwordMax), user.password_hash, s.iterations, DEFAULT_ITERATIONS);
  if (!okCurrent) return answer(401, { error: "invalid_credentials" });
  await clearFailures(env, key);
  const problem = passwordProblem(body?.password, user.email, s);
  if (problem) return answer(400, { error: problem });

  let hash: string;
  try {
    hash = await hashPassword(str(body?.password), s.iterations);
  } catch {
    return hashingUnavailable();
  }
  await setPasswordHash(env, user.id, hash, now);
  const revoked = await revokeUserSessions(env, user.id, "password_change", now, session.id);
  return answer(200, { ok: true, revoked });
};
