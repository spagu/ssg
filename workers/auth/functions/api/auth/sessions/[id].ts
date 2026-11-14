// Cloudflare Pages Function: sign out one of my sessions.
//   DELETE /api/auth/sessions/:id   header x-ssg-auth: 1 → 200 {ok: true, current}
//   POST   /api/auth/sessions/:id   the same from a plain HTML form (field next?)
//
// Only the caller's own sessions: anyone else's id, an ended session and an
// unknown id are all 404 session_not_found. Ending the current session also
// clears the cookies (`current: true`), like logout. Found through the access
// cookie or, after a pause, the refresh cookie (this path is under
// /api/auth/), so the account page's buttons work without JavaScript; 401
// not_authenticated without a session. A form gets a 303: to the safe `next`
// (default /auth/account) with ?ok=session_ended or ?error=session_not_found;
// to AUTH_LOGIN_URL?ok=signed_out when it ended the current session.

import { param } from "../_admin";
import { endOwnSession } from "../_account";
import { preparePost, whoAmI } from "../_context";
import { clearCookies } from "../_cookies";
import { json, redirect } from "../_lib";
import type { Env } from "../_lib";
import { safeNext, withParams } from "../_next";
import { isForm, readBody } from "../_request";

const end: PagesFunction<Env> = async ({ request, env, params }) => {
  const form = request.method === "POST";
  const ctx = await preparePost(request, env, form);
  if (ctx instanceof Response) return ctx;
  const back = form && isForm(request) ? safeNext((await readBody(request))?.next, "/auth/account") : "/auth/account";
  const who = await whoAmI(ctx, true);
  if (!who) return form ? redirect(withParams(back, { error: "not_authenticated" })) : json({ error: "not_authenticated" }, 401);
  const id = param(params, "id");
  const r = await endOwnSession(env, who.user.id, id, ctx.now);
  if (r === "not_found") return form ? redirect(withParams(back, { error: "session_not_found" })) : json({ error: "session_not_found" }, 404);
  const current = id === who.session.id;
  const cookies = current ? clearCookies() : [];
  if (form) return redirect(current ? withParams(ctx.s.loginUrl, { ok: "signed_out" }) : withParams(back, { ok: "session_ended" }), cookies);
  return json({ ok: true, current }, 200, {}, cookies);
};

export const onRequestDelete = end;
export const onRequestPost = end;
