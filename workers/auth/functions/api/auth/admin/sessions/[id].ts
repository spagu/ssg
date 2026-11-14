// Cloudflare Pages Function: end one session, anyone's (GO-138).
//   DELETE /api/auth/admin/sessions/:id   → 200 {ok: true, current}
//
// Requires `auth:admin`. The session's next refresh fails and this worker
// refuses its tokens at once; other workers accept an already-issued token
// until it expires (at most AUTH_TOKEN_MINUTES + 30 s). Another account's
// session is sensitive: a recent second factor (403 step_up_required); one's
// own is not (`current: true` when it is the one making the request — the
// panel then signs out). Never a session of an account holding more than the
// caller (403 escalation, SR-04). 404 session_not_found (unknown or already ended).
// Audit: session_revoked.

import { authorize, notFound, param } from "../../_admin";
import { adminEvent, mayActOn, stepUpWhen } from "../../_admin-sensitive";
import { json } from "../../_lib";
import type { Env } from "../../_lib";
import { getSession, live, revokeSession } from "../../_sessions";

export const onRequestDelete: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const s = await getSession(env, param(params, "id"));
  if (!s || !live(s, caller.ctx.now)) return notFound("session");
  const refused = await mayActOn(caller, s.user_id);
  if (refused) return refused;
  const stepUp = await stepUpWhen(caller, s.user_id !== caller.user.id);
  if (stepUp) return stepUp;
  await revokeSession(env, s.id, "admin_revoke", caller.ctx.now);
  await adminEvent(caller, "session_revoked", s.user_id, { session: s.id });
  return json({ ok: true, current: s.id === caller.session.id });
};
