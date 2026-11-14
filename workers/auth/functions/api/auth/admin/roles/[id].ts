// Cloudflare Pages Function: one role.
//   GET    /api/auth/admin/roles/:id   → 200 {role}
//   PUT    /api/auth/admin/roles/:id   {name?, description?, permissions?} → 200 {role}
//   DELETE /api/auth/admin/roles/:id   → 200 {ok: true} (its grants go with it)
//
// Requires `auth:admin`. 404 role_not_found. PUT changes only the fields sent;
// a new `permissions` replaces the list, and any permission it ADDS must be
// one the caller holds (403 escalation), and so must any it REMOVES (SR-04). Built-in roles keep their names (409
// built_in_role). 409 owner_required when the change would leave no active
// owner — deleting `owner`, or taking `*` out of it. GO-138: adding `*` or
// `auth:*` needs a recent second factor (403 step_up_required); audit
// role_changed / role_deleted.

import { authorize, body, escalation, invalid, nameTaken, notFound, ownerRequired, param, roleAnswer, text } from "../../_admin";
import { adminEvent, handsOutAdmin, mayTakeAway, stepUpWhen } from "../../_admin-sensitive";
import { json } from "../../_lib";
import type { Env } from "../../_lib";
import { beyond, parsePermissions } from "../../_perms";
import { ROLE_NAME, deleteRole, getRole, updateRole } from "../../_roles";
import type { RoleChange } from "../../_roles";

export const onRequestGet: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const role = await getRole(env, param(params, "id"));
  return role ? json({ role: roleAnswer(env, role) }) : notFound("role");
};

export const onRequestPut: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const b = await body(request);
  if (b instanceof Response) return b;
  const before = await getRole(env, param(params, "id"));
  if (!before) return notFound("role");
  const patch: RoleChange = {};
  const name = text(b, "name", 64);
  if (name !== undefined) {
    if (!name || !ROLE_NAME.test(name)) return invalid("name: a lower-case slug, [a-z][a-z0-9-]*, at most 64 characters");
    patch.name = name;
  }
  const description = text(b, "description", 500);
  if (description === null) return invalid("description: a string of at most 500 characters");
  if (description !== undefined) patch.description = description;
  if ("permissions" in b) {
    const perms = parsePermissions(b.permissions);
    if (!perms.ok) return invalid(perms.detail);
    const added = perms.perms.filter((p) => !before.permissions.includes(p));
    const over = beyond(caller.perms, added);
    if (over.length) return escalation(over);
    // GO-138: adding `*` or `auth:*` hands it to every holder: a recent second factor.
    const stepUp = await stepUpWhen(caller, handsOutAdmin(added));
    if (stepUp) return stepUp;
    // SR-04: taking permissions out of a role takes them from its holders.
    const refused = await mayTakeAway(caller, before.permissions.filter((p) => !perms.perms.includes(p)));
    if (refused) return refused;
    patch.permissions = perms.perms;
  }
  const role = await updateRole(env, before.id, patch, caller.ctx.now);
  if (role === "not_found") return notFound("role");
  if (role === "name_taken") return nameTaken();
  if (role === "owner_required") return ownerRequired();
  if (role === "built_in_name") return json({ error: "built_in_role", detail: "a built-in role keeps its name" }, 409);
  await adminEvent(caller, "role_changed", null, { role: role.name });
  return json({ role: roleAnswer(env, role) });
};

export const onRequestDelete: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const before = await getRole(env, param(params, "id"));
  if (!before) return notFound("role");
  // SR-04: deleting a role takes its permissions from every holder.
  const refused = await mayTakeAway(caller, before.permissions);
  if (refused) return refused;
  const r = await deleteRole(env, before.id);
  if (r === "not_found") return notFound("role");
  if (r === "owner_required") return ownerRequired();
  await adminEvent(caller, "role_deleted", null, { role: before.name });
  return json({ ok: true });
};
