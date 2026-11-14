// Cloudflare Pages Function: one group.
//   GET    /api/auth/admin/groups/:id   → 200 {group, members: [member], grants: [grant]}
//   PUT    /api/auth/admin/groups/:id   {name?, description?} → 200 {group}
//   DELETE /api/auth/admin/groups/:id   → 200 {ok: true} (its members and grants go with it)
//
// Requires `auth:admin`. A member is {userId, email, name, status, addedBy,
// addedAt}; a grant {id, roleId, roleName, userId, groupId, grantedBy,
// grantedAt}. 404 group_not_found; 409 name_taken; 409 owner_required when
// deleting the group would leave no active owner. DELETE takes the group's
// permissions from its members: the caller must cover them (403 escalation;
// `*` / `auth:*` also a recent second factor — SR-04). Audit: group_deleted.

import { authorize, body, invalid, nameTaken, notFound, ownerRequired, param, text } from "../../../_admin";
import { adminEvent, mayTakeAway } from "../../../_admin-sensitive";
import { groupPermissions, listGrants } from "../../../_grants";
import { GROUP_NAME_RULE, deleteGroup, getGroup, groupName, listMembers, updateGroup } from "../../../_groups";
import { json } from "../../../_lib";
import type { Env } from "../../../_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const group = await getGroup(env, param(params, "id"));
  if (!group) return notFound("group");
  return json({ group, members: await listMembers(env, group.id), grants: await listGrants(env, { groupId: group.id }) });
};

export const onRequestPut: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const b = await body(request);
  if (b instanceof Response) return b;
  const patch: { name?: string; description?: string } = {};
  if ("name" in b) {
    patch.name = groupName(b.name);
    if (!patch.name) return invalid(GROUP_NAME_RULE);
  }
  const description = text(b, "description", 500);
  if (description === null) return invalid("description: a string of at most 500 characters");
  if (description !== undefined) patch.description = description;
  const group = await updateGroup(env, param(params, "id"), patch, caller.ctx.now);
  if (group === "not_found") return notFound("group");
  if (group === "name_taken") return nameTaken();
  return json({ group });
};

export const onRequestDelete: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const group = await getGroup(env, param(params, "id"));
  if (!group) return notFound("group");
  const refused = await mayTakeAway(caller, await groupPermissions(env, group.id));
  if (refused) return refused;
  const r = await deleteGroup(env, group.id);
  if (r === "not_found") return notFound("group");
  if (r === "owner_required") return ownerRequired();
  await adminEvent(caller, "group_deleted", null, { group: group.name });
  return json({ ok: true });
};
