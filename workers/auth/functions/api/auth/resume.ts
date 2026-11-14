// Cloudflare Pages Function: pick a session up again by following a link.
//   GET /api/auth/resume?next=<path>
//
// The pages under /auth/ see only the access cookie (minutes); the refresh
// cookie is scoped to /api/auth/. When a page finds no valid access token it
// sends the browser here: with a live refresh token this rotates it exactly
// like POST /api/auth/refresh, sets the new cookies and answers 303 to the
// safe `next` (default /auth/); otherwise 303 to AUTH_LOGIN_URL?next=… — with
// error=session_ended and the cookies cleared when a token was presented but
// is dead. That keeps the account page working without JavaScript.
//
// A GET that rotates a token: the refresh cookie is SameSite=Strict, so a
// cross-site page cannot make the browser send it here, and a request that
// says it comes from another site (Sec-Fetch-Site other than same-origin or
// none) is sent to the login page without touching anything.

import { issueCookies, prepare } from "./_context";
import { REFRESH_COOKIE, clearCookies, readCookie } from "./_cookies";
import { isToken, redirect } from "./_lib";
import type { Env } from "./_lib";
import { safeNext, withParams } from "./_next";
import { revokeSession, rotateRefresh } from "./_sessions";
import { findUserById } from "./_users";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await prepare(request, env);
  if (ctx instanceof Response) return ctx;
  const next = safeNext(new URL(request.url).searchParams.get("next"), "/auth/");
  const login = (error = "", cookies: string[] = []) => redirect(withParams(ctx.s.loginUrl, { next, error }), cookies);
  const site = request.headers.get("sec-fetch-site");
  if (site !== null && site !== "same-origin" && site !== "none") return login();

  const token = readCookie(request, REFRESH_COOKIE);
  if (!token) return login();
  if (!isToken(token)) return login("session_ended", clearCookies());
  const r = await rotateRefresh(env, token, ctx.s.graceSeconds, ctx.now);
  if (r.kind === "invalid" || r.kind === "reused") return login("session_ended", clearCookies());
  const user = await findUserById(env, r.session.user_id);
  if (!user || user.status !== "active") {
    await revokeSession(env, r.session.id, "user_inactive", ctx.now);
    return login("session_ended", clearCookies());
  }
  const { cookies, tooLarge } = await issueCookies(ctx, user, r.session, r.kind === "rotated" ? r.refreshToken : null);
  if (tooLarge) return login("permissions_too_large", cookies);
  return redirect(next, cookies);
};
