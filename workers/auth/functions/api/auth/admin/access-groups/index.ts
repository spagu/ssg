// Cloudflare Pages Function: the Cloudflare Access group mapping (GO-139).
//   GET  /api/auth/admin/access-groups   → 200 {items: [mapping], nextCursor: null}
//   POST /api/auth/admin/access-groups   {accessGroup, groupId} → 201 {mapping, resynced}
//
// Requires `auth:admin`. A mapping puts everyone whose Access token names
// `accessGroup` into the group here at their Access sign-in — and so hands
// them the group's roles: like adding a member, the caller must hold every
// permission of the group (403 escalation {permissions}) unless the caller
// holds *. The new mapping is applied at once to everyone whose groups an
// Access sign-in recorded (`resynced`: how many had a membership change);
// their permissions follow at their next token refresh. A mapping is
// {id, accessGroup, groupId, groupName, createdBy, createdAt}. A group
// holding `*` or `auth:*` needs a recent second factor, as adding a member
// does (403 step_up_required, SR-05). Audit: access_mapping_added. 404
// group_not_found; 409 mapping_exists; 400 invalid_request.

import { authorize, body, escalation, invalid, notFound } from "../../_admin";
import { ACCESS_GROUP_RULE, accessGroupName, createMapping, listMappings } from "../../_access-groups";
import { resyncEveryone } from "../../_access-sync";
import { adminEvent, handsOutAdmin, stepUpWhen } from "../../_admin-sensitive";
import { groupPermissions } from "../../_grants";
import { getGroup } from "../../_groups";
import { json } from "../../_lib";
import type { Env } from "../../_lib";
import { beyond } from "../../_perms";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  return json({ items: await listMappings(env), nextCursor: null });
};

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const b = await body(request);
  if (b instanceof Response) return b;
  const accessGroup = accessGroupName(b.accessGroup);
  if (!accessGroup) return invalid(ACCESS_GROUP_RULE);
  if (typeof b.groupId !== "string" || !b.groupId) return invalid("groupId: required");
  const group = await getGroup(env, b.groupId);
  if (!group) return notFound("group");
  const perms = await groupPermissions(env, group.id);
  const over = beyond(caller.perms, perms);
  if (over.length) return escalation(over);
  const stepUp = await stepUpWhen(caller, handsOutAdmin(perms));
  if (stepUp) return stepUp;
  const mapping = await createMapping(env, accessGroup, group.id, caller.user.id, caller.ctx.now);
  if (mapping === "exists") return json({ error: "mapping_exists" }, 409);
  await adminEvent(caller, "access_mapping_added", null, { accessGroup, group: group.name });
  return json({ mapping, resynced: await resyncEveryone(env, caller.ctx.now) }, 201);
};
