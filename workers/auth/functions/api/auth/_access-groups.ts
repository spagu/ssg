// The Access group mapping (GO-139): which Access group puts a person in which
// group here. A leading underscore keeps this file out of the Pages route
// table. The admin API (admin/access-groups/) edits it; _access-sync.ts
// applies it at each Access sign-in, and again for everyone already seen when
// it changes.

import { iso } from "./_lib";
import type { Env } from "./_lib";
import { isUnique } from "./_rbac-schema";

export interface Mapping {
  id: string;
  accessGroup: string;
  groupId: string;
  groupName: string | null; // null when the group was deleted since (the mapping then does nothing)
  createdBy: string | null;
  createdAt: string;
}

interface Row {
  id: string;
  access_group: string;
  group_id: string;
  group_name: string | null;
  created_by: string | null;
  created_at: string;
}

const shape = (r: Row): Mapping => ({
  id: r.id,
  accessGroup: r.access_group,
  groupId: r.group_id,
  groupName: r.group_name,
  createdBy: r.created_by,
  createdAt: r.created_at,
});

const SELECT = "SELECT m.*, g.name AS group_name FROM access_group_map m LEFT JOIN groups g ON g.id = m.group_id";

export async function listMappings(env: Env): Promise<Mapping[]> {
  const { results } = await env.AUTH_DB.prepare(`${SELECT} ORDER BY m.access_group, g.name, m.id`).all<Row>();
  return results.map(shape);
}

export const ACCESS_GROUP_RULE = "accessGroup: 1–256 characters, no control characters";

// accessGroupName: trimmed, 1–256 characters, no control characters; "" when unusable.
export function accessGroupName(raw: unknown): string {
  const s = typeof raw === "string" ? raw.trim() : "";
  return s.length <= 256 && !/[\u0000-\u001f\u007f]/.test(s) ? s : "";
}

// createMapping: the new mapping, or "exists" for a pair already mapped. The
// caller has checked that the group exists.
export async function createMapping(env: Env, accessGroup: string, groupId: string, by: string, now: number): Promise<Mapping | "exists"> {
  const id = crypto.randomUUID();
  try {
    await env.AUTH_DB.prepare("INSERT INTO access_group_map (id, access_group, group_id, created_by, created_at) VALUES (?, ?, ?, ?, ?)")
      .bind(id, accessGroup, groupId, by, iso(now))
      .run();
  } catch (e) {
    if (isUnique(e)) return "exists";
    throw e;
  }
  return shape((await env.AUTH_DB.prepare(`${SELECT} WHERE m.id = ?`).bind(id).first<Row>()) as Row);
}

export async function deleteMapping(env: Env, id: string): Promise<boolean> {
  const res = await env.AUTH_DB.prepare("DELETE FROM access_group_map WHERE id = ?").bind(id).run();
  return res.meta.changes === 1;
}
