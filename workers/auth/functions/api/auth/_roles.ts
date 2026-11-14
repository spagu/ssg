// Roles and their permissions. A leading underscore keeps this file out of the
// Pages route table. Policy (who may change what) is the routes' business;
// what must never happen — losing the last owner — is refused by the database
// (_rbac-schema.ts) and reported here as "owner_required".

import { iso } from "./_lib";
import type { Env } from "./_lib";
import { isOwnerViolation, isUnique } from "./_rbac-schema";

export interface Role {
  id: string;
  name: string;
  description: string;
  builtIn: boolean;
  permissions: string[];
  createdAt: string;
  updatedAt: string;
}

interface RoleRow {
  id: string;
  name: string;
  description: string;
  built_in: number;
  created_at: string;
  updated_at: string;
}

export const ROLE_NAME = /^[a-z][a-z0-9-]{0,63}$/;

function shape(r: RoleRow, perms: Map<string, string[]>): Role {
  return {
    id: r.id,
    name: r.name,
    description: r.description,
    builtIn: r.built_in === 1,
    permissions: perms.get(r.id) ?? [],
    createdAt: r.created_at,
    updatedAt: r.updated_at,
  };
}

async function permissionsOf(env: Env, roleId: string | null): Promise<Map<string, string[]>> {
  const sql = "SELECT role_id, permission FROM role_permissions" + (roleId ? " WHERE role_id = ?" : "") + " ORDER BY permission";
  const stmt = env.AUTH_DB.prepare(sql);
  const { results } = await (roleId ? stmt.bind(roleId) : stmt).all<{ role_id: string; permission: string }>();
  const out = new Map<string, string[]>();
  for (const r of results) out.set(r.role_id, [...(out.get(r.role_id) ?? []), r.permission]);
  return out;
}

export async function listRoles(env: Env): Promise<Role[]> {
  const { results } = await env.AUTH_DB.prepare("SELECT * FROM roles ORDER BY name").all<RoleRow>();
  const perms = await permissionsOf(env, null);
  return results.map((r) => shape(r, perms));
}

export async function getRole(env: Env, id: string): Promise<Role | null> {
  const row = await env.AUTH_DB.prepare("SELECT * FROM roles WHERE id = ?").bind(id).first<RoleRow>();
  return row ? shape(row, await permissionsOf(env, id)) : null;
}

export interface RoleInput {
  name: string;
  description: string;
  permissions: string[];
}

// createRole inserts the role and its permissions in one batch; "name_taken"
// when another role has the name.
export async function createRole(env: Env, r: RoleInput, now: number): Promise<Role | "name_taken"> {
  const db = env.AUTH_DB;
  const id = crypto.randomUUID();
  try {
    await db.batch([
      db
        .prepare("INSERT INTO roles (id, name, description, built_in, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)")
        .bind(id, r.name, r.description, iso(now), iso(now)),
      db.prepare("INSERT INTO role_permissions (role_id, permission) SELECT ?, value FROM json_each(?)").bind(id, JSON.stringify(r.permissions)),
    ]);
  } catch (e) {
    if (isUnique(e)) return "name_taken";
    throw e;
  }
  return (await getRole(env, id)) as Role;
}

export type RoleChange = Partial<RoleInput>;
export type RoleFailure = "not_found" | "name_taken" | "built_in_name" | "owner_required";

// updateRole changes what the patch names. A new permission list is applied
// as a difference — rows no longer wanted are deleted, new ones inserted — so
// a role that keeps `*` never passes through a state without it.
export async function updateRole(env: Env, id: string, patch: RoleChange, now: number): Promise<Role | RoleFailure> {
  const db = env.AUTH_DB;
  const before = await getRole(env, id);
  if (!before) return "not_found";
  if (before.builtIn && patch.name !== undefined && patch.name !== before.name) return "built_in_name";
  const statements = [
    db
      .prepare("UPDATE roles SET name = ?, description = ?, updated_at = ? WHERE id = ?")
      .bind(patch.name ?? before.name, patch.description ?? before.description, iso(now), id),
  ];
  if (patch.permissions) {
    const list = JSON.stringify(patch.permissions);
    statements.push(
      db.prepare("DELETE FROM role_permissions WHERE role_id = ? AND permission NOT IN (SELECT value FROM json_each(?))").bind(id, list),
      db
        .prepare("INSERT OR IGNORE INTO role_permissions (role_id, permission) SELECT ?, value FROM json_each(?) WHERE EXISTS (SELECT 1 FROM roles WHERE id = ?)")
        .bind(id, list, id),
    );
  }
  try {
    await db.batch(statements);
  } catch (e) {
    if (isUnique(e)) return "name_taken";
    if (isOwnerViolation(e)) return "owner_required";
    throw e;
  }
  return (await getRole(env, id)) ?? "not_found";
}

// deleteRole removes the role, its permissions and every grant of it, in one
// batch: all of it or nothing.
export async function deleteRole(env: Env, id: string): Promise<"ok" | "not_found" | "owner_required"> {
  const db = env.AUTH_DB;
  try {
    const [, , role] = await db.batch([
      db.prepare("DELETE FROM grants WHERE role_id = ?").bind(id),
      db.prepare("DELETE FROM role_permissions WHERE role_id = ?").bind(id),
      db.prepare("DELETE FROM roles WHERE id = ?").bind(id),
    ]);
    return role.meta.changes === 1 ? "ok" : "not_found";
  } catch (e) {
    if (isOwnerViolation(e)) return "owner_required";
    throw e;
  }
}
