// Cloudflare Pages Function: set a new password with a one-time reset link.
//   POST /api/auth/reset   JSON {token, password}
//                          or the same fields from the plain HTML form of /auth/reset
//
// The token comes from an administrator (POST /api/auth/admin/users/:id/
// reset-link) or from "forgot password" (POST /api/auth/forgot, only with
// mail). The new password must pass the usual rules — a refused one does not
// use up the link. Then the link is used up (atomically: of two requests with
// the same token one wins), the password set, EVERY session of the account
// ended and its login lock cleared. Nobody is signed in: the person signs in
// with the new password.
// JSON: 200 {ok: true, revoked}; 410 reset_invalid (unknown, used, replaced or
// expired — one answer for all); 400 <password rule>; 503 hashing_unavailable.
// A form gets a 303 to AUTH_LOGIN_URL?ok=password_reset, or back to
// /auth/reset?token=…&error=<code> (without the token when it is the token
// that failed).

import { hashingUnavailable, preparePost } from "./_context";
import { isToken, json, redirect } from "./_lib";
import type { Env } from "./_lib";
import { clearFailures, lockKey } from "./_lockout";
import { withParams } from "./_next";
import { hashPassword, passwordProblem } from "./_password";
import { isForm, readBody, str } from "./_request";
import { consumeReset, openReset } from "./_resets";
import { revokeUserSessions } from "./_sessions";
import { findUserById, setPasswordHash } from "./_users";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await preparePost(request, env, true);
  if (ctx instanceof Response) return ctx;
  const { s, now } = ctx;
  const form = isForm(request);
  const b = await readBody(request);
  const token = str(b?.token);
  const fail = (error: string, status: number, keepToken = true): Response =>
    form ? redirect(withParams("/auth/reset", { token: keepToken ? token : "", error })) : json({ error }, status);

  const open = isToken(token) ? await openReset(env, token, now) : null;
  const user = open ? await findUserById(env, open.userId) : null;
  if (!user) return fail("reset_invalid", 410, false);
  const problem = passwordProblem(b?.password, user.email, s);
  if (problem) return fail(problem, 400);
  let hash: string;
  try {
    hash = await hashPassword(str(b?.password), s.iterations);
  } catch {
    return hashingUnavailable();
  }
  if ((await consumeReset(env, token, now)) !== user.id) return fail("reset_invalid", 410, false);
  await setPasswordHash(env, user.id, hash, now);
  await clearFailures(env, await lockKey(env, "account", user.email));
  const revoked = await revokeUserSessions(env, user.id, "password_reset", now);
  if (form) return redirect(withParams(s.loginUrl, { ok: "password_reset" }));
  return json({ ok: true, revoked });
};
