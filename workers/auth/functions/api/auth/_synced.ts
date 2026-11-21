// Memberships and grants a sign-in synchronises from someone else's claims —
// the Cloudflare Access groups (GO-139, _access-sync.ts) and an external
// issuer's roles claim (#435, _external-roles.ts). A leading underscore keeps
// this file out of the Pages route table.
//
// Each synchronised row has a MARK in its own table: the marks are the only
// rows a synchronisation may ever remove. A membership or grant an
// administrator added by hand has no mark and is never touched.
//
// Each change is its own transaction, written so that two sign-ins of the
// same person at once cannot leave a row without its mark or the reverse: the
// mark is inserted only while no row exists, the row only while the mark
// does; a removal deletes the row only while it is marked. The owner
// invariant (_rbac-schema.ts) holds: removing the last active owner's way to
// the owner role aborts that one removal — it is skipped, logged and
// reported, the others go on.

import { iso } from "./_lib";
import type { Env } from "./_lib";
import { byCodeUnit } from "./_perms";
import { isOwnerViolation } from "./_rbac-schema";

// Marks names the tables of one kind of synchronised row. Every name here is
// a constant of this code, never a value from a request or a token.
export interface Marks {
  table: string; // (<col>, user_id, synced_at), PRIMARY KEY (<col>, user_id)
  col: "group_id" | "role_id";
  label: string; // for the log line of a kept removal
}

const TARGET = {
  group_id: {
    rows: "group_members",
    names: "groups",
    insert: "INSERT OR IGNORE INTO group_members (group_id, user_id, added_by, added_at) SELECT ?, ?, NULL, ?",
  },
  role_id: {
    rows: "grants",
    names: "roles",
    insert: "INSERT OR IGNORE INTO grants (id, role_id, user_id, group_id, granted_by, granted_at) SELECT lower(hex(randomblob(16))), ?, ?, NULL, NULL, ?",
  },
} as const;

export interface SyncResult {
  added: string[]; // names
  removed: string[];
  kept: string[]; // a removal the owner invariant refused
}

async function addOne(env: Env, m: Marks, id: string, userId: string, at: string): Promise<boolean> {
  const db = env.AUTH_DB;
  const t = TARGET[m.col];
  const [, row] = await db.batch([
    db
      .prepare(`INSERT OR IGNORE INTO ${m.table} (${m.col}, user_id, synced_at) SELECT ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM ${t.rows} WHERE ${m.col} = ? AND user_id = ?)`)
      .bind(id, userId, at, id, userId),
    db.prepare(`${t.insert} WHERE EXISTS (SELECT 1 FROM ${m.table} WHERE ${m.col} = ? AND user_id = ?)`).bind(id, userId, at, id, userId),
  ]);
  return row.meta.changes === 1;
}

async function removeOne(env: Env, m: Marks, id: string, userId: string): Promise<"removed" | "gone" | "owner_required"> {
  const db = env.AUTH_DB;
  const t = TARGET[m.col];
  try {
    const [row] = await db.batch([
      db
        .prepare(`DELETE FROM ${t.rows} WHERE ${m.col} = ? AND user_id = ? AND EXISTS (SELECT 1 FROM ${m.table} WHERE ${m.col} = ? AND user_id = ?)`)
        .bind(id, userId, id, userId),
      db.prepare(`DELETE FROM ${m.table} WHERE ${m.col} = ? AND user_id = ?`).bind(id, userId),
    ]);
    return row.meta.changes === 1 ? "removed" : "gone";
  } catch (e) {
    if (isOwnerViolation(e)) return "owner_required";
    throw e;
  }
}

// syncMarked makes the person's marked rows match `wanted` (id → name): adds
// what is wanted, removes what is marked and no longer wanted.
export async function syncMarked(env: Env, m: Marks, userId: string, wanted: Map<string, string>, now: number): Promise<SyncResult> {
  const t = TARGET[m.col];
  const { results: marked } = await env.AUTH_DB.prepare(
    `SELECT s.${m.col} AS id, COALESCE(n.name, s.${m.col}) AS name FROM ${m.table} s LEFT JOIN ${t.names} n ON n.id = s.${m.col} WHERE s.user_id = ?`,
  )
    .bind(userId)
    .all<{ id: string; name: string }>();
  const out: SyncResult = { added: [], removed: [], kept: [] };
  const at = iso(now);
  for (const [id, name] of wanted) if (await addOne(env, m, id, userId, at)) out.added.push(name);
  for (const row of marked) {
    if (wanted.has(row.id)) continue;
    const r = await removeOne(env, m, row.id, userId);
    if (r === "removed") out.removed.push(row.name);
    if (r === "owner_required") {
      console.warn(`auth: ${m.label} kept account ${userId} in ${t.names === "groups" ? "group" : "role"} ${row.id}: removing it would leave no active owner`);
      out.kept.push(row.name);
    }
  }
  for (const list of [out.added, out.removed, out.kept]) list.sort(byCodeUnit);
  return out;
}
