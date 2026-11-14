// Cloudflare Pages Function: add a member to a group.
//   POST /api/auth/admin/groups/:id/members   {userId} → 201 {ok: true, added: true}
//                                                     (200 {added: false} when already a member)
//
// Requires `auth:admin`. Joining a group hands the member every role granted
// to the group, so the caller must hold all of those permissions (403
// escalation {permissions}) unless the caller holds *. 404 group_not_found /
// user_not_found; 400 invalid_request. A group holding `*` or `auth:*` needs a
// recent second factor when the caller has one (GO-140, GO-138): 403
// step_up_required. Audit: member_added.

import { authorize, body, escalation, invalid, notFound, param } from "../../../../_admin";
import { adminEvent, handsOutAdmin, stepUpWhen } from "../../../../_admin-sensitive";
import { groupPermissions } from "../../../../_grants";
import { addMember, getGroup } from "../../../../_groups";
import { json } from "../../../../_lib";
import type { Env } from "../../../../_lib";
import { beyond } from "../../../../_perms";
import { findUserById } from "../../../../_users";

export const onRequestPost: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const b = await body(request);
  if (b instanceof Response) return b;
  if (typeof b.userId !== "string" || !b.userId) return invalid("userId: required");
  const group = await getGroup(env, param(params, "id"));
  if (!group) return notFound("group");
  if (!(await findUserById(env, b.userId))) return notFound("user");
  const handed = await groupPermissions(env, group.id);
  const over = beyond(caller.perms, handed);
  if (over.length) return escalation(over);
  // GO-140, GO-138: joining a group that holds `*` or `auth:*`: a recent second factor.
  const stepUp = await stepUpWhen(caller, handsOutAdmin(handed));
  if (stepUp) return stepUp;
  const added = await addMember(env, group.id, b.userId, caller.user.id, caller.ctx.now);
  if (added) await adminEvent(caller, "member_added", b.userId, { group: group.name });
  return json({ ok: true, added }, added ? 201 : 200);
};
