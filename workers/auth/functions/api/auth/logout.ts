// Cloudflare Pages Function: end this session.
//   POST /api/auth/logout   header x-ssg-auth: 1, or a plain HTML form (field next?)
//
// Revokes the session found through the access cookie or, when that has
// expired, the refresh cookie, and clears both cookies. Always 200 {ok: true}
// (a form: 303 to the safe `next`, default AUTH_LOGIN_URL) — logging out with
// no session is not an error. Tokens already handed out stay valid in other
// workers until they expire (AUTH_TOKEN_MINUTES); this worker refuses them now.
//
// GO-139: a session started through Cloudflare Access also ends at Access —
// otherwise the next visit to /auth/access would sign straight back in. After
// clearing this worker's cookies, a form gets its 303 to the team's Access
// logout (https://<team>.cloudflareaccess.com/cdn-cgi/access/logout) instead
// of `next`, and JSON gets {ok: true, accessLogoutUrl} for the page to open.
// The URL is built from AUTH_ACCESS_TEAM only — never from the request.

import { ACCESS_METHOD, configuredLogoutUrl } from "./_access-config";
import { preparePost, whoAmI } from "./_context";
import { clearCookies } from "./_cookies";
import { json, redirect } from "./_lib";
import type { Env } from "./_lib";
import { safeNext } from "./_next";
import { sessionMethod } from "./_provider-policy";
import { isForm, readBody } from "./_request";
import { revokeSession } from "./_sessions";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await preparePost(request, env, true);
  if (ctx instanceof Response) return ctx;
  const who = await whoAmI(ctx, true);
  if (who) await revokeSession(env, who.session.id, "logout", ctx.now);
  const accessOut = who && (await sessionMethod(env, who.session.id)) === ACCESS_METHOD ? configuredLogoutUrl(env) : null;
  if (isForm(request)) return redirect(accessOut ?? safeNext((await readBody(request))?.next, ctx.s.loginUrl), clearCookies());
  return json(accessOut ? { ok: true, accessLogoutUrl: accessOut } : { ok: true }, 200, {}, clearCookies());
};
