// Cloudflare Pages Function: revoke one grant.
//   DELETE /api/auth/admin/grants/:id   → 200 {ok: true}
//
// Requires `auth:admin`, and that the caller covers the role's permissions
// (403 escalation — taking away follows the rule for handing out, SR-04;
// `*` or `auth:*` also needs a recent second factor: 403 step_up_required).
// 404 grant_not_found; 409 owner_required when it is the last path to an
// active owner. The change reaches the account's token at
// its next refresh; POST …/users/:id/revoke-sessions forces that.

import { authorize, notFound, ownerRequired, param } from "../../_admin";
import { adminEvent, mayTakeAway } from "../../_admin-sensitive";
import { listGrants, removeGrant } from "../../_grants";
import { getRole } from "../../_roles";
import type { Role } from "../../_roles";
import { json } from "../../_lib";
import type { Env } from "../../_lib";

export const onRequestDelete: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const [before] = (await listGrants(env)).filter((g) => g.id === param(params, "id"));
  if (!before) return notFound("grant");
  // A grant's role exists: deleting a role deletes its grants in the same batch.
  const refused = await mayTakeAway(caller, ((await getRole(env, before.roleId)) as Role).permissions);
  if (refused) return refused;
  const r = await removeGrant(env, before.id);
  if (r === "not_found") return notFound("grant");
  if (r === "owner_required") return ownerRequired();
  // GO-138: audit grant_removed.
  await adminEvent(caller, "grant_removed", before.userId, { role: before.roleName, group: before.groupId });
  return json({ ok: true });
};
