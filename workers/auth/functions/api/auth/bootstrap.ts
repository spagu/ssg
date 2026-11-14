// Cloudflare Pages Function: create the first account.
//   POST /api/auth/bootstrap   JSON {token, email, password, name?}; header x-ssg-auth: 1
//
// Works only while `users` is empty and AUTH_BOOTSTRAP_TOKEN is set; otherwise
// 404 {error: "not_found"}, as if the route did not exist. The token is
// compared in constant time; wrong tokens count towards a per-IP lock (5, then
// the growing delay of the login lock). The INSERT itself carries the "table
// is empty" condition, so of two concurrent calls exactly one creates an
// account and the other gets 404. 201 {user, expiresAt, sessionExpiresAt} and
// the session cookies: the new account is logged in. Remove the secret after
// use (the route is inert anyway once an account exists).
//
// The account is granted the built-in `owner` role (`*`), in the statement
// right after the INSERT; it is the site's first administrator.

import { emitEvent } from "./_audit";
import { hashingUnavailable, issueCookies, permissionsTooLarge, preparePost } from "./_context";
import { grantFirstOwner } from "./_grants";
import { clientIP, json, looksLikeEmail, normEmail, secretEqual } from "./_lib";
import type { Env } from "./_lib";
import { clearFailures, lockKey, lockState, recordFailure } from "./_lockout";
import { hashPassword, passwordProblem } from "./_password";
import { readBody, str } from "./_request";
import { createSession, revokeSession } from "./_sessions";
import { anyUser, createUser, findUserById, publicUser, touchLogin } from "./_users";
import type { User } from "./_users";

const BOOTSTRAP_FAILURES = 5;

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await preparePost(request, env, false);
  if (ctx instanceof Response) return ctx;
  const { s, now } = ctx;
  const notFound = () => json({ error: "not_found" }, 404);
  const expected = env.AUTH_BOOTSTRAP_TOKEN;
  if (!expected || (await anyUser(env))) return notFound();

  const key = await lockKey(env, "bootstrap", clientIP(request));
  const state = await lockState(env, [key], now);
  if (state.retryAfter > 0) return json({ error: "too_many_attempts" }, 429, { "retry-after": String(state.retryAfter) });

  // Counted before the comparison (SR-02); a success clears it below.
  const over = await recordFailure(env, [{ key, kind: "bootstrap", threshold: BOOTSTRAP_FAILURES }], s, now);
  if (over > 0) return json({ error: "too_many_attempts" }, 429, { "retry-after": String(over) });
  const body = await readBody(request);
  if (!(await secretEqual(str(body?.token), expected))) return json({ error: "invalid_token" }, 401);
  const email = normEmail(body?.email);
  if (!looksLikeEmail(email)) return json({ error: "invalid_email" }, 400);
  const problem = passwordProblem(body?.password, email, s);
  if (problem) return json({ error: problem }, 400);

  let hash: string;
  try {
    hash = await hashPassword(str(body?.password), s.iterations);
  } catch {
    return hashingUnavailable();
  }
  const id = await createUser(env, { email, name: str(body?.name).trim().slice(0, 200), passwordHash: hash }, now, true);
  if (!id) return notFound();
  await grantFirstOwner(env, id, now);
  await clearFailures(env, key);
  const user = (await findUserById(env, id)) as User;
  const { session, refreshToken } = await createSession(env, id, request, s.sessionSeconds, now);
  const { cookies, exp, tooLarge } = await issueCookies(ctx, user, session, refreshToken);
  if (tooLarge) {
    await revokeSession(env, session.id, "permissions_too_large", now);
    return permissionsTooLarge(tooLarge);
  }
  // The first account's creation and first sign-in belong in the audit log
  // like any other, and the account page's "last sign-in" should not say
  // "never" for the account that just signed in.
  await touchLogin(env, id, now);
  await emitEvent(env, { kind: "account_created", userId: id, method: "password", detail: { via: "bootstrap" } }, now);
  await emitEvent(env, { kind: "signin", userId: id, method: "password", detail: { sessionId: session.id, via: "bootstrap" } }, now);
  return json({ user: publicUser(user), expiresAt: exp, sessionExpiresAt: session.expires_at }, 201, {}, cookies);
};
