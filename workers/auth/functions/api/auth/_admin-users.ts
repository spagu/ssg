// The admin panel's view of accounts (GO-138): a filtered, paged list, and
// everyone's effective permissions for the access matrix. A leading
// underscore keeps this file out of the Pages route table. Read-only; the
// changes are _users.ts (setUserStatus, deleteUser).
//
// Paging is by address (unique, the list's order): `cursor` is the last
// address of the previous page, so a page never repeats or skips an account
// that existed when the first page was read.

import type { Env } from "./_lib";

export const PAGE_DEFAULT = 50;
export const PAGE_MAX = 200;

export interface UserFilter {
  q: string; // part of the address or the name, any case
  status: "" | "active" | "blocked";
  roleId: string; // holds the role, directly or through a group
  groupId: string; // a member of the group
  cursor: string;
  limit: number;
}

export interface ListedUser {
  id: string;
  email: string;
  name: string;
  status: "active" | "blocked";
  createdAt: string;
  lastLoginAt: string | null;
  mustChangePassword: boolean;
  hasPassword: boolean;
  owner: boolean; // the owner role reaches the account (directly or by a group)
}

interface Row {
  id: string;
  email: string;
  display_name: string;
  status: "active" | "blocked";
  created_at: string;
  last_login_at: string | null;
  must_change_password: number;
  has_password: number;
  owner: number;
}

const shape = (r: Row): ListedUser => ({
  id: r.id,
  email: r.email,
  name: r.display_name,
  status: r.status,
  createdAt: r.created_at,
  lastLoginAt: r.last_login_at,
  mustChangePassword: r.must_change_password === 1,
  hasPassword: r.has_password === 1,
  owner: r.owner === 1,
});

// readFilter turns query parameters into a filter; null names the bad one.
export function readFilter(q: URLSearchParams): UserFilter | string {
  const status = q.get("status") ?? "";
  if (status !== "" && status !== "active" && status !== "blocked") return "status: active or blocked";
  const raw = q.get("limit");
  const limit = raw === null ? PAGE_DEFAULT : Number(raw);
  if (!Number.isInteger(limit) || limit < 1 || limit > PAGE_MAX) return `limit: 1–${PAGE_MAX}`;
  const term = (q.get("q") ?? "").trim().toLowerCase();
  if ([...term].length > 200) return "q: at most 200 characters";
  return { q: term, status, roleId: q.get("roleId") ?? "", groupId: q.get("groupId") ?? "", cursor: q.get("cursor") ?? "", limit };
}

const MEMBER = "EXISTS (SELECT 1 FROM group_members m WHERE m.group_id = g.group_id AND m.user_id = u.id)";

// listUsers: one page of accounts matching the filter, by address.
export async function listUsers(env: Env, f: UserFilter): Promise<{ items: ListedUser[]; nextCursor: string | null }> {
  const { results } = await env.AUTH_DB.prepare(
    `SELECT u.id, u.email, u.display_name, u.status, u.created_at, u.last_login_at, u.must_change_password,
            (u.password_hash IS NOT NULL) AS has_password,
            EXISTS (SELECT 1 FROM owner_holders o WHERE o.user_id = u.id) AS owner
       FROM users u
      WHERE u.email > ?
        AND (? = '' OR u.status = ?)
        AND (? = '' OR instr(lower(u.email), ?) > 0 OR instr(lower(u.display_name), ?) > 0)
        AND (? = '' OR EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = ? AND gm.user_id = u.id))
        AND (? = '' OR EXISTS (SELECT 1 FROM grants g WHERE g.role_id = ? AND (g.user_id = u.id OR ${MEMBER})))
      ORDER BY u.email LIMIT ?`,
  )
    .bind(f.cursor, f.status, f.status, f.q, f.q, f.q, f.groupId, f.groupId, f.roleId, f.roleId, f.limit + 1)
    .all<Row>();
  const items = results.slice(0, f.limit).map(shape);
  return { items, nextCursor: results.length > f.limit ? items[items.length - 1].email : null };
}

// permissionsOf: the effective permissions (sorted, de-duplicated) of each
// account given — the access matrix's rows. One statement for the page.
export async function permissionsOf(env: Env, userIds: readonly string[]): Promise<Map<string, string[]>> {
  const out = new Map<string, string[]>(userIds.map((id) => [id, []]));
  if (!userIds.length) return out;
  const { results } = await env.AUTH_DB.prepare(
    `SELECT DISTINCT u.id AS user_id, rp.permission
       FROM users u
       JOIN grants g ON g.user_id = u.id OR ${MEMBER}
       JOIN role_permissions rp ON rp.role_id = g.role_id
      WHERE u.id IN (SELECT value FROM json_each(?))
      ORDER BY rp.permission`,
  )
    .bind(JSON.stringify(userIds))
    .all<{ user_id: string; permission: string }>();
  for (const r of results) (out.get(r.user_id) as string[]).push(r.permission);
  return out;
}
