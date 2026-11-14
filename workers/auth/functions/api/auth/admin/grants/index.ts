// Cloudflare Pages Function: grants — a role given to a user or to a group.
//   GET  /api/auth/admin/grants?roleId=&userId=&groupId=   → 200 {items: [grant], nextCursor: null}
//   POST /api/auth/admin/grants   {roleId, userId} or {roleId, groupId} → 201 {grant}
//
// Requires `auth:admin`. `userId` may also name a service account (GO-141).
// A grant is {id, roleId, roleName, userId, groupId,
// grantedBy, grantedAt}; the filters narrow the list (any combination). The
// caller must hold every permission of the role (403 escalation
// {permissions}) unless the caller holds *. 404 role_not_found /
// user_not_found / group_not_found; 409 grant_exists; 400 invalid_request.
// A role holding `*` (owner) or `auth:*` needs a recent second factor when the
// caller has one (GO-140, GO-138): 403 step_up_required. Audit: grant_added.

import { authorize, body, escalation, invalid, notFound } from "../../_admin";
import { adminEvent, handsOutAdmin, stepUpWhen } from "../../_admin-sensitive";
import { addGrant, listGrants } from "../../_grants";
import type { Target } from "../../_grants";
import { getGroup } from "../../_groups";
import { json } from "../../_lib";
import type { Env } from "../../_lib";
import { beyond } from "../../_perms";
import { getRole } from "../../_roles";
import { getServiceAccount } from "../../_service-accounts";
import { findUserById } from "../../_users";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const q = new URL(request.url).searchParams;
  const items = await listGrants(env, { roleId: q.get("roleId") ?? "", userId: q.get("userId") ?? "", groupId: q.get("groupId") ?? "" });
  return json({ items, nextCursor: null });
};

const id = (v: unknown): string => (typeof v === "string" ? v : "");

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const b = await body(request);
  if (b instanceof Response) return b;
  const roleId = id(b.roleId);
  const userId = id(b.userId);
  const groupId = id(b.groupId);
  if (!roleId || Boolean(userId) === Boolean(groupId)) return invalid("roleId and exactly one of userId, groupId");
  const role = await getRole(env, roleId);
  if (!role) return notFound("role");
  let to: Target;
  if (userId) {
    // A service account (GO-141) takes roles the same way: its id as userId.
    if (!(await findUserById(env, userId)) && !(await getServiceAccount(env, userId))) return notFound("user");
    to = { userId };
  } else {
    if (!(await getGroup(env, groupId))) return notFound("group");
    to = { groupId };
  }
  const over = beyond(caller.perms, role.permissions);
  if (over.length) return escalation(over);
  // GO-140, GO-138: handing out `*` (the owner role) or `auth:*` needs a recent second factor.
  const stepUp = await stepUpWhen(caller, handsOutAdmin(role.permissions));
  if (stepUp) return stepUp;
  const grant = await addGrant(env, role.id, to, caller.user.id, caller.ctx.now);
  if (grant === "exists") return json({ error: "grant_exists" }, 409);
  await adminEvent(caller, "grant_added", userId || null, { role: role.name, group: groupId || null });
  return json({ grant }, 201);
};
