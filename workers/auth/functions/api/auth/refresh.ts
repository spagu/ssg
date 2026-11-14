// Cloudflare Pages Function: exchange the refresh cookie for a new access token.
//   POST /api/auth/refresh   no body; header x-ssg-auth: 1
//
// 200 {user, expiresAt, sessionExpiresAt} + a new access cookie and, normally,
// a new refresh cookie (rotation on every use; see _sessions.ts for the
// two-tab grace and the theft detection). 401 {error: "session_ended"} with
// both cookies cleared when the token is unknown, expired, revoked, reused, or
// its account is blocked. Every refresh reads the account afresh, so a
// blocked account stops at the next refresh, at most AUTH_TOKEN_MINUTES later.
// Every refresh also reads the grants afresh (GO-133): the new token carries
// the permissions as they are now. 500 {error: "permissions_too_large",
// detail} when they no longer fit in a cookie — with the rotated refresh
// cookie, so the session lives on until an administrator narrows the roles.

import { issueCookies, permissionsTooLarge, preparePost } from "./_context";
import { REFRESH_COOKIE, clearCookies, readCookie } from "./_cookies";
import { isToken, json } from "./_lib";
import type { Env } from "./_lib";
import { revokeSession, rotateRefresh } from "./_sessions";
import { findUserById, publicUser } from "./_users";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await preparePost(request, env, false);
  if (ctx instanceof Response) return ctx;
  const ended = () => json({ error: "session_ended" }, 401, {}, clearCookies());

  const token = readCookie(request, REFRESH_COOKIE);
  if (!isToken(token)) return ended();
  const r = await rotateRefresh(env, token, ctx.s.graceSeconds, ctx.now);
  if (r.kind === "invalid" || r.kind === "reused") return ended();

  const user = await findUserById(env, r.session.user_id);
  if (!user || user.status !== "active") {
    await revokeSession(env, r.session.id, "user_inactive", ctx.now);
    return ended();
  }
  const { cookies, exp, tooLarge } = await issueCookies(ctx, user, r.session, r.kind === "rotated" ? r.refreshToken : null);
  if (tooLarge) return permissionsTooLarge(tooLarge, cookies);
  return json({ user: publicUser(user), expiresAt: exp, sessionExpiresAt: r.session.expires_at }, 200, {}, cookies);
};
