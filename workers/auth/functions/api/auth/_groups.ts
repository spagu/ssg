// Groups and their members. A leading underscore keeps this file out of the
// Pages route table. Groups do not nest: a member is always a user. Losing the
// last owner through a group (removing the member, deleting the group) is
// refused by the database and reported as "owner_required".

import { iso } from "./_lib";
import type { Env } from "./_lib";
import { isOwnerViolation, isUnique } from "./_rbac-schema";

export interface Member {
  userId: string;
  email: string;
  name: string;
  status: string;
  addedBy: string | null;
  addedAt: string;
}

export interface Group {
  id: string;
  name: string;
  description: string;
  memberCount: number;
  createdAt: string;
  updatedAt: string;
}

interface GroupRow {
  id: string;
  name: string;
  description: string;
  member_count: number;
  created_at: string;
  updated_at: string;
}

const shape = (r: GroupRow): Group => ({
  id: r.id,
  name: r.name,
  description: r.description,
  memberCount: r.member_count,
  createdAt: r.created_at,
  updatedAt: r.updated_at,
});

const SELECT = "SELECT g.*, (SELECT COUNT(*) FROM group_members m WHERE m.group_id = g.id) AS member_count FROM groups g";

export async function listGroups(env: Env): Promise<Group[]> {
  const { results } = await env.AUTH_DB.prepare(`${SELECT} ORDER BY g.name`).all<GroupRow>();
  return results.map(shape);
}

export async function getGroup(env: Env, id: string): Promise<Group | null> {
  const row = await env.AUTH_DB.prepare(`${SELECT} WHERE g.id = ?`).bind(id).first<GroupRow>();
  return row ? shape(row) : null;
}

export async function listMembers(env: Env, groupId: string): Promise<Member[]> {
  const { results } = await env.AUTH_DB.prepare(
    `SELECT m.user_id, u.email, u.display_name, u.status, m.added_by, m.added_at
     FROM group_members m JOIN users u ON u.id = m.user_id WHERE m.group_id = ? ORDER BY u.email`,
  )
    .bind(groupId)
    .all<{ user_id: string; email: string; display_name: string; status: string; added_by: string | null; added_at: string }>();
  return results.map((r) => ({ userId: r.user_id, email: r.email, name: r.display_name, status: r.status, addedBy: r.added_by, addedAt: r.added_at }));
}

export const GROUP_NAME_RULE = "name: 1–100 characters, no control characters";

// groupName: trimmed, 1–100 characters, no control characters; "" when unusable.
export function groupName(raw: unknown): string {
  const s = typeof raw === "string" ? raw.trim() : "";
  return s.length <= 100 && !/[\u0000-\u001f\u007f]/.test(s) ? s : "";
}

export async function createGroup(env: Env, name: string, description: string, now: number): Promise<Group | "name_taken"> {
  const id = crypto.randomUUID();
  try {
    await env.AUTH_DB.prepare("INSERT INTO groups (id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)")
      .bind(id, name, description, iso(now), iso(now))
      .run();
  } catch (e) {
    if (isUnique(e)) return "name_taken";
    throw e;
  }
  return (await getGroup(env, id)) as Group;
}

export async function updateGroup(env: Env, id: string, patch: { name?: string; description?: string }, now: number): Promise<Group | "not_found" | "name_taken"> {
  const before = await getGroup(env, id);
  if (!before) return "not_found";
  try {
    await env.AUTH_DB.prepare("UPDATE groups SET name = ?, description = ?, updated_at = ? WHERE id = ?")
      .bind(patch.name ?? before.name, patch.description ?? before.description, iso(now), id)
      .run();
  } catch (e) {
    if (isUnique(e)) return "name_taken";
    throw e;
  }
  return (await getGroup(env, id)) ?? "not_found";
}

// deleteGroup removes the group's grants, its members and the group in one
// batch, grants first: when the group is the last path to an owner, the very
// first statement is refused and nothing changes.
export async function deleteGroup(env: Env, id: string): Promise<"ok" | "not_found" | "owner_required"> {
  const db = env.AUTH_DB;
  try {
    const [, , group] = await db.batch([
      db.prepare("DELETE FROM grants WHERE group_id = ?").bind(id),
      db.prepare("DELETE FROM group_members WHERE group_id = ?").bind(id),
      db.prepare("DELETE FROM groups WHERE id = ?").bind(id),
    ]);
    return group.meta.changes === 1 ? "ok" : "not_found";
  } catch (e) {
    if (isOwnerViolation(e)) return "owner_required";
    throw e;
  }
}

// addMember: true when the user was added, false when already a member. The
// caller has checked that both the group and the user exist.
export async function addMember(env: Env, groupId: string, userId: string, by: string, now: number): Promise<boolean> {
  const res = await env.AUTH_DB.prepare(
    "INSERT OR IGNORE INTO group_members (group_id, user_id, added_by, added_at) SELECT ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM groups WHERE id = ?)",
  )
    .bind(groupId, userId, by, iso(now), groupId)
    .run();
  return res.meta.changes === 1;
}

export async function removeMember(env: Env, groupId: string, userId: string): Promise<"ok" | "not_found" | "owner_required"> {
  try {
    const res = await env.AUTH_DB.prepare("DELETE FROM group_members WHERE group_id = ? AND user_id = ?").bind(groupId, userId).run();
    return res.meta.changes === 1 ? "ok" : "not_found";
  } catch (e) {
    if (isOwnerViolation(e)) return "owner_required";
    throw e;
  }
}
