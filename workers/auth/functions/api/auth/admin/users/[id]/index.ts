// Cloudflare Pages Function: one account (GO-138).
//   GET    /api/auth/admin/users/:id   → 200 {user, methods, secondFactor, groups, grants, permissions, sessions, events}
//   DELETE /api/auth/admin/users/:id   → 200 {ok: true}
//
// Requires `auth:admin`. GET: the account (`owner` true when the owner role
// reaches it), how it signs in ({password, identities: [{id, provider,
// email, linkedAt, lastUsedAt}]}), its second factor ({enabled, factors:
// [{id, kind, name, createdAt, lastUsedAt}], backupCodesLeft} — never a
// secret), its groups, direct grants, effective permissions with where each
// comes from, live sessions and its last 20 audit events.
//
// DELETE removes the account with its grants, memberships, sessions and
// sign-in methods; its audit events stay. Sensitive: a recent second factor
// (403 step_up_required). Never one's own account (400 own_account), never
// an account holding more than the caller (403 escalation), never the last
// active owner (409 owner_required). 404 user_not_found. Audit:
// account_deleted (with the address, so the log still says who).

import { authorize, escalation, notFound, ownerRequired, param } from "../../../_admin";
import { adminEvent, stepUpWhen } from "../../../_admin-sensitive";
import { userDetail } from "../../../_admin-user-detail";
import { effectivePermissions } from "../../../_grants";
import { json } from "../../../_lib";
import type { Env } from "../../../_lib";
import { beyond } from "../../../_perms";
import { deleteUser, findUserById } from "../../../_users";

export const onRequestGet: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const user = await findUserById(env, param(params, "id"));
  return user ? json(await userDetail(env, user, caller.ctx.now)) : notFound("user");
};

export const onRequestDelete: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const user = await findUserById(env, param(params, "id"));
  if (!user) return notFound("user");
  if (user.id === caller.user.id) return json({ error: "own_account", detail: "you cannot delete the account you are signed in with" }, 400);
  const over = beyond(caller.perms, await effectivePermissions(env, user.id));
  if (over.length) return escalation(over);
  const stepUp = await stepUpWhen(caller, true);
  if (stepUp) return stepUp;
  const done = await deleteUser(env, user.id);
  if (done === "owner_required") return ownerRequired();
  if (done === "not_found") return notFound("user");
  await adminEvent(caller, "account_deleted", user.id, { email: user.email });
  return json({ ok: true });
};
