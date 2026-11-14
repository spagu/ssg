// Cloudflare Pages Function: remove a member from a group.
//   DELETE /api/auth/admin/groups/:id/members/:userId   → 200 {ok: true}
//
// Requires `auth:admin`, and that the caller covers the group's permissions
// (403 escalation; `*` / `auth:*` also a recent second factor — SR-04).
// 404 member_not_found; 409 owner_required when the
// member is the last active owner and holds the role through this group.

import { authorize, notFound, ownerRequired, param } from "../../../../_admin";
import { adminEvent, mayTakeAway } from "../../../../_admin-sensitive";
import { groupPermissions } from "../../../../_grants";
import { removeMember } from "../../../../_groups";
import { json } from "../../../../_lib";
import type { Env } from "../../../../_lib";

export const onRequestDelete: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const refused = await mayTakeAway(caller, await groupPermissions(env, param(params, "id")));
  if (refused) return refused;
  const r = await removeMember(env, param(params, "id"), param(params, "userId"));
  if (r === "not_found") return notFound("member");
  if (r === "owner_required") return ownerRequired();
  // GO-138: audit member_removed.
  await adminEvent(caller, "member_removed", param(params, "userId"), { groupId: param(params, "id") });
  return json({ ok: true });
};
