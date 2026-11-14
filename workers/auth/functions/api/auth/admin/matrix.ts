// Cloudflare Pages Function: who holds what, for the access matrix (GO-138).
//   GET /api/auth/admin/matrix?q=&status=&roleId=&groupId=&cursor=&limit=
//       → 200 {items: [{id, email, name, status, permissions}], nextCursor}
//
// Requires `auth:admin`. The accounts as GET /api/auth/admin/users pages and
// filters them, each with its effective permissions (every role, direct or
// through a group; wildcards as written). The panel crosses them with the
// registry (GET /api/auth/permissions) into users × modules. What grants say
// now — a session's token catches up at its next refresh.

import { authorize, invalid } from "../_admin";
import { listUsers, permissionsOf, readFilter } from "../_admin-users";
import { json } from "../_lib";
import type { Env } from "../_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const f = readFilter(new URL(request.url).searchParams);
  if (typeof f === "string") return invalid(f);
  const page = await listUsers(env, f);
  const perms = await permissionsOf(env, page.items.map((u) => u.id));
  const items = page.items.map((u) => ({ id: u.id, email: u.email, name: u.name, status: u.status, permissions: perms.get(u.id) as string[] }));
  return json({ items, nextCursor: page.nextCursor });
};
