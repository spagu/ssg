// Cloudflare Pages Function: roles.
//   GET  /api/auth/admin/roles   → 200 {items: [role], nextCursor: null}
//   POST /api/auth/admin/roles   {name, description?, permissions} → 201 {role}
//
// Requires `auth:admin`. A role is {id, name, description, builtIn,
// permissions, unknownPermissions, createdAt, updatedAt}. `name` is a
// lower-case slug ([a-z][a-z0-9-]*, at most 64), `description` at most 500
// characters, `permissions` worker:action / worker:* / * (at most 64). 400
// invalid_request {detail}; 409 name_taken; 403 escalation {permissions} when
// the role would hold something the caller does not (unless the caller holds *).
// Audit (GO-138): role_created.

import { authorize, body, escalation, invalid, nameTaken, roleAnswer, text } from "../../_admin";
import { adminEvent } from "../../_admin-sensitive";
import { json } from "../../_lib";
import type { Env } from "../../_lib";
import { beyond, parsePermissions } from "../../_perms";
import { ROLE_NAME, createRole, listRoles } from "../../_roles";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  return json({ items: (await listRoles(env)).map((r) => roleAnswer(env, r)), nextCursor: null });
};

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const b = await body(request);
  if (b instanceof Response) return b;
  const name = text(b, "name", 64);
  if (!name || !ROLE_NAME.test(name)) return invalid("name: a lower-case slug, [a-z][a-z0-9-]*, at most 64 characters");
  const description = text(b, "description", 500);
  if (description === null) return invalid("description: a string of at most 500 characters");
  const perms = parsePermissions(b.permissions);
  if (!perms.ok) return invalid(perms.detail);
  const over = beyond(caller.perms, perms.perms);
  if (over.length) return escalation(over);
  const role = await createRole(env, { name, description: description ?? "", permissions: perms.perms }, caller.ctx.now);
  if (role === "name_taken") return nameTaken();
  await adminEvent(caller, "role_created", null, { role: role.name });
  return json({ role: roleAnswer(env, role) }, 201);
};
