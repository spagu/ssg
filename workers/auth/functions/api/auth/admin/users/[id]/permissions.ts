// Cloudflare Pages Function: one account's effective permissions, with where
// each comes from.
//   GET /api/auth/admin/users/:id/permissions
//
// Requires `auth:admin`. 200 {userId, permissions: [{name, known, sources:
// [{roleId, roleName, via: "user"} | {roleId, roleName, via: "group",
// groupId, groupName}]}]} — sorted by name; a permission given by two roles
// lists both. 404 user_not_found.

import { authorize, notFound, param, permissionsAnswer } from "../../../_admin";
import { json } from "../../../_lib";
import type { Env } from "../../../_lib";
import { findUserById } from "../../../_users";

export const onRequestGet: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const user = await findUserById(env, param(params, "id"));
  if (!user) return notFound("user");
  return json(await permissionsAnswer(env, user.id));
};
