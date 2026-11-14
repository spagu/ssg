// Cloudflare Pages Function: the accounts (GO-138).
//   GET /api/auth/admin/users?q=&status=&roleId=&groupId=&cursor=&limit=
//       → 200 {items: [user], nextCursor}
//
// Requires `auth:admin`. Ordered by address; `nextCursor` (the last address)
// fetches the next page, null at the end. `q` matches part of the address or
// the name (any case); `status` active | blocked; `roleId` holds the role,
// directly or through a group; `groupId` is a member. `limit` 1–200 (50).
// A user is {id, email, name, status, createdAt, lastLoginAt,
// mustChangePassword, hasPassword, owner}. 400 invalid_request {detail}.

import { authorize, invalid } from "../../_admin";
import { listUsers, readFilter } from "../../_admin-users";
import { json } from "../../_lib";
import type { Env } from "../../_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const f = readFilter(new URL(request.url).searchParams);
  if (typeof f === "string") return invalid(f);
  return json(await listUsers(env, f));
};
