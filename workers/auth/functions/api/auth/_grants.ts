// Grants (role → user or group) and what they add up to: a user's effective
// permissions, with where each one comes from. A leading underscore keeps this
// file out of the Pages route table.
//
// Effective permissions = the union of the permissions of every role granted
// to the user directly or to a group the user belongs to — de-duplicated and
// sorted, wildcards kept as written (`*` is never expanded into a list: the
// token stays small and a permission added to a worker later is covered).

import { iso } from "./_lib";
import type { Env } from "./_lib";
import { OWNER_ROLE_NAME, isOwnerViolation } from "./_rbac-schema";

export interface Grant {
  id: string;
  roleId: string;
  roleName: string;
  userId: string | null;
  groupId: string | null;
  grantedBy: string | null; // users.id of the administrator; null = the system (bootstrap, upgrade)
  grantedAt: string;
}

interface GrantRow {
  id: string;
  role_id: string;
  role_name: string;
  user_id: string | null;
  group_id: string | null;
  granted_by: string | null;
  granted_at: string;
}

const shape = (r: GrantRow): Grant => ({
  id: r.id,
  roleId: r.role_id,
  roleName: r.role_name,
  userId: r.user_id,
  groupId: r.group_id,
  grantedBy: r.granted_by,
  grantedAt: r.granted_at,
});

const SELECT = "SELECT g.*, r.name AS role_name FROM grants g JOIN roles r ON r.id = g.role_id";

export interface GrantFilter {
  roleId?: string;
  userId?: string;
  groupId?: string;
}

// listGrants lists grants, narrowed by any of role, user and group.
export async function listGrants(env: Env, f: GrantFilter = {}): Promise<Grant[]> {
  const where: string[] = [];
  const binds: string[] = [];
  for (const [column, value] of [["g.role_id", f.roleId], ["g.user_id", f.userId], ["g.group_id", f.groupId]] as const) {
    if (value) {
      where.push(`${column} = ?`);
      binds.push(value);
    }
  }
  const sql = `${SELECT}${where.length ? " WHERE " + where.join(" AND ") : ""} ORDER BY r.name, g.granted_at, g.id`;
  const { results } = await env.AUTH_DB.prepare(sql).bind(...binds).all<GrantRow>();
  return results.map(shape);
}

export type Target = { userId: string; groupId?: undefined } | { groupId: string; userId?: undefined };

// addGrant grants a role to a user or a group; "exists" when that grant is
// already there. The caller has checked that the role and the target exist.
export async function addGrant(env: Env, roleId: string, to: Target, by: string, now: number): Promise<Grant | "exists"> {
  const id = crypto.randomUUID();
  const res = await env.AUTH_DB.prepare(
    `INSERT OR IGNORE INTO grants (id, role_id, user_id, group_id, granted_by, granted_at)
     SELECT ?, ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM roles WHERE id = ?)`,
  )
    .bind(id, roleId, to.userId ?? null, to.groupId ?? null, by, iso(now), roleId)
    .run();
  if (res.meta.changes !== 1) return "exists";
  return shape((await env.AUTH_DB.prepare(`${SELECT} WHERE g.id = ?`).bind(id).first<GrantRow>()) as GrantRow);
}

// removeGrant revokes one grant; the database refuses it when it is the last
// path to an active owner.
export async function removeGrant(env: Env, id: string): Promise<"ok" | "not_found" | "owner_required"> {
  try {
    const res = await env.AUTH_DB.prepare("DELETE FROM grants WHERE id = ?").bind(id).run();
    return res.meta.changes === 1 ? "ok" : "not_found";
  } catch (e) {
    if (isOwnerViolation(e)) return "owner_required";
    throw e;
  }
}

// grantFirstOwner makes userId the owner unless someone already holds the
// owner role — the bootstrap's first account. Returns whether it granted.
export async function grantFirstOwner(env: Env, userId: string, now: number): Promise<boolean> {
  const res = await env.AUTH_DB.prepare(
    `INSERT INTO grants (id, role_id, user_id, group_id, granted_by, granted_at)
     SELECT ?, r.id, ?, NULL, NULL, ? FROM roles r
     WHERE r.name = ? AND NOT EXISTS (SELECT 1 FROM grants WHERE role_id = r.id)`,
  )
    .bind(crypto.randomUUID(), userId, iso(now), OWNER_ROLE_NAME)
    .run();
  return res.meta.changes === 1;
}

export type Source =
  | { roleId: string; roleName: string; via: "user" }
  | { roleId: string; roleName: string; via: "group"; groupId: string; groupName: string };

export interface Provenance {
  name: string;
  sources: Source[];
}

interface SourceRow {
  permission: string;
  role_id: string;
  role_name: string;
  group_id: string | null;
  group_name: string | null;
}

// A grant counts for the user when it names the user, or names an existing
// group the user is a member of.
const SOURCES = `SELECT rp.permission, r.id AS role_id, r.name AS role_name, g.group_id, gr.name AS group_name
  FROM grants g
  JOIN roles r ON r.id = g.role_id
  JOIN role_permissions rp ON rp.role_id = g.role_id
  LEFT JOIN groups gr ON gr.id = g.group_id
  WHERE g.user_id = ?
     OR (gr.id IS NOT NULL AND EXISTS (SELECT 1 FROM group_members m WHERE m.group_id = g.group_id AND m.user_id = ?))`;

// permissionSources is the effective list with provenance: every permission,
// sorted, with each role that gives it — directly, or via which group.
export async function permissionSources(env: Env, userId: string): Promise<Provenance[]> {
  const { results } = await env.AUTH_DB.prepare(SOURCES).bind(userId, userId).all<SourceRow>();
  const out = new Map<string, Source[]>();
  for (const r of results) {
    const source: Source =
      r.group_id === null
        ? { roleId: r.role_id, roleName: r.role_name, via: "user" }
        : { roleId: r.role_id, roleName: r.role_name, via: "group", groupId: r.group_id, groupName: r.group_name as string };
    out.set(r.permission, [...(out.get(r.permission) ?? []), source]);
  }
  // Default sort = UTF-16 code unit order, the same in every runtime. Labels
  // are unique (one grant per role and target), so the comparator never ties.
  const label = (s: Source) => `${s.roleName}\0${s.via === "group" ? s.groupName : ""}`;
  return [...out.keys()].sort().map((name) => ({ name, sources: (out.get(name) as Source[]).sort((a, b) => (label(a) < label(b) ? -1 : 1)) }));
}

// effectivePermissions is what goes into the access token's `perms`.
export async function effectivePermissions(env: Env, userId: string): Promise<string[]> {
  return (await permissionSources(env, userId)).map((p) => p.name);
}

// groupPermissions: the union of the roles granted to a group — what adding a
// member hands that member.
export async function groupPermissions(env: Env, groupId: string): Promise<string[]> {
  const { results } = await env.AUTH_DB.prepare(
    "SELECT DISTINCT rp.permission FROM grants g JOIN role_permissions rp ON rp.role_id = g.role_id WHERE g.group_id = ?",
  )
    .bind(groupId)
    .all<{ permission: string }>();
  return results.map((r) => r.permission).sort();
}
