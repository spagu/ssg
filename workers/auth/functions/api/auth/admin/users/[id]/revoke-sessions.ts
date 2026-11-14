// Cloudflare Pages Function: end every session of an account now.
//   POST /api/auth/admin/users/:id/revoke-sessions   → 200 {ok: true, revoked: n}
//
// Requires `auth:admin`. The account's next refresh fails, so a changed grant
// cannot wait for it: the person logs in again and gets a token with the
// permissions as they are now. This worker refuses the account's tokens at
// once; other workers accept an already-issued token until it expires (at
// most AUTH_TOKEN_MINUTES). Revoking one's own sessions logs oneself out.
// 404 user_not_found. Another account's sessions need a recent second factor
// when the caller has one (GO-140): 403 step_up_required; never those of an
// account holding more than the caller (403 escalation, SR-04).

import { authorize, notFound, param } from "../../../_admin";
import { adminEvent, mayActOn } from "../../../_admin-sensitive";
import { json } from "../../../_lib";
import type { Env } from "../../../_lib";
import { requireStepUp } from "../../../_mfa-step-up";
import { revokeUserSessions } from "../../../_sessions";
import { findUserById } from "../../../_users";

export const onRequestPost: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const user = await findUserById(env, param(params, "id"));
  if (!user) return notFound("user");
  const refused = await mayActOn(caller, user.id);
  if (refused) return refused;
  // GO-140: ending someone else's sessions needs a recent second factor.
  const stepUp = user.id === caller.user.id ? null : await requireStepUp(caller.ctx, caller);
  if (stepUp) return stepUp;
  const revoked = await revokeUserSessions(env, user.id, "admin_revoke", caller.ctx.now);
  // GO-138: audit sessions_revoked.
  await adminEvent(caller, "sessions_revoked", user.id, { scope: "account", revoked });
  return json({ ok: true, revoked });
};
