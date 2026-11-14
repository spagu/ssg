// Cloudflare Pages Function: end every session of the signed-in account.
//   POST /api/auth/logout-all   header x-ssg-auth: 1, or a plain HTML form (field next?)
//
// 200 {ok: true, revoked: <sessions ended>} and both cookies cleared (a form:
// 303 to the safe `next`, default AUTH_LOGIN_URL); 401 {error:
// "not_authenticated"} without a live session. Every device must log in
// again; access tokens already issued expire within AUTH_TOKEN_MINUTES.

import { preparePost, whoAmI } from "./_context";
import { clearCookies } from "./_cookies";
import { json, redirect } from "./_lib";
import type { Env } from "./_lib";
import { safeNext } from "./_next";
import { isForm, readBody } from "./_request";
import { revokeUserSessions } from "./_sessions";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await preparePost(request, env, true);
  if (ctx instanceof Response) return ctx;
  const who = await whoAmI(ctx, true);
  if (!who) return json({ error: "not_authenticated" }, 401);
  const revoked = await revokeUserSessions(env, who.user.id, "logout_all", ctx.now);
  if (isForm(request)) return redirect(safeNext((await readBody(request))?.next, ctx.s.loginUrl), clearCookies());
  return json({ ok: true, revoked }, 200, {}, clearCookies());
};
