// Cloudflare Pages Function: withdraw an invitation.
//   DELETE /api/auth/admin/invitations/:id   → 200 {ok: true}
//
// Requires `auth:invite`. Its link stops working at once. Withdrawing one
// already withdrawn is not an error; 409 invitation_accepted once it was
// accepted (the account exists — block or delete it instead); 404
// invitation_not_found.

import { authorize, notFound, param } from "../../../_admin";
import { adminEvent } from "../../../_admin-sensitive";
import { revokeInvitation } from "../../../_invites";
import { json } from "../../../_lib";
import type { Env } from "../../../_lib";

export const onRequestDelete: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:invite");
  if (caller instanceof Response) return caller;
  const r = await revokeInvitation(env, param(params, "id"), caller.user.id, caller.ctx.now);
  if (r === "not_found") return notFound("invitation");
  if (r === "accepted") return json({ error: "invitation_accepted" }, 409);
  // GO-138: audit invitation_revoked.
  await adminEvent(caller, "invitation_revoked", null, { invitation: param(params, "id") });
  return json({ ok: true });
};
