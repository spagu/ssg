// Group memberships from Access groups (GO-139). A leading underscore keeps
// this file out of the Pages route table.
//
// At each Access sign-in whose token carries groups, the person's memberships
// in the MAPPED groups are made to match the token:
//   - a mapped group the token names: the person is added, and the membership
//     marked as synchronised (access_synced_members). Already a member by an
//     administrator's hand: left as it is, not marked — it stays manual;
//   - a group the person is in through the synchronisation, which the token no
//     longer earns (the Access group left the token, or its mapping was
//     removed): the membership is removed, with its mark.
// A membership without a mark is never removed. Groups nobody mapped are
// never looked at. A token WITHOUT a groups claim changes nothing — an
// identity provider that stopped sending groups must not empty everyone's
// memberships (docs/ACCESS.md).
//
// Each change is its own transaction, written so that two sign-ins of the
// same person at once cannot leave a membership without its mark or the
// reverse: the mark is inserted only while no membership exists, the
// membership only while the mark does; a removal deletes the membership only
// while it is marked. The owner invariant (_rbac-schema.ts) holds: removing
// the last active owner's way to the owner role aborts that one removal — it
// is skipped, logged and reported, the others go on.
//
// The groups seen are kept (access_seen_groups), so that a change of the
// mapping is applied to everyone at once (resyncEveryone); permissions follow
// at each person's next token — a refresh — like any change of membership.

import { emitEvent } from "./_audit";
import { ACCESS_METHOD } from "./_access-config";
import { iso } from "./_lib";
import type { Env } from "./_lib";
import { isOwnerViolation } from "./_rbac-schema";
import { byCodeUnit } from "./_perms";

export interface SyncResult {
  added: string[]; // group names
  removed: string[];
  kept: string[]; // a removal the owner invariant refused
}

interface Named {
  id: string;
  name: string;
}

async function addOne(env: Env, groupId: string, userId: string, at: string): Promise<boolean> {
  const db = env.AUTH_DB;
  const [, member] = await db.batch([
    db
      .prepare(
        `INSERT OR IGNORE INTO access_synced_members (group_id, user_id, synced_at)
         SELECT ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM group_members WHERE group_id = ? AND user_id = ?)`,
      )
      .bind(groupId, userId, at, groupId, userId),
    db
      .prepare(
        `INSERT OR IGNORE INTO group_members (group_id, user_id, added_by, added_at)
         SELECT ?, ?, NULL, ? WHERE EXISTS (SELECT 1 FROM access_synced_members WHERE group_id = ? AND user_id = ?)`,
      )
      .bind(groupId, userId, at, groupId, userId),
  ]);
  return member.meta.changes === 1;
}

async function removeOne(env: Env, groupId: string, userId: string): Promise<"removed" | "gone" | "owner_required"> {
  const db = env.AUTH_DB;
  try {
    const [member] = await db.batch([
      db
        .prepare("DELETE FROM group_members WHERE group_id = ? AND user_id = ? AND EXISTS (SELECT 1 FROM access_synced_members WHERE group_id = ? AND user_id = ?)")
        .bind(groupId, userId, groupId, userId),
      db.prepare("DELETE FROM access_synced_members WHERE group_id = ? AND user_id = ?").bind(groupId, userId),
    ]);
    return member.meta.changes === 1 ? "removed" : "gone";
  } catch (e) {
    if (isOwnerViolation(e)) return "owner_required";
    throw e;
  }
}

// syncGroups applies `accessGroups` (the token's) to one person.
export async function syncGroups(env: Env, userId: string, accessGroups: string[], now: number): Promise<SyncResult> {
  const { results: maps } = await env.AUTH_DB.prepare(
    "SELECT m.access_group, g.id, g.name FROM access_group_map m JOIN groups g ON g.id = m.group_id",
  ).all<Named & { access_group: string }>();
  const { results: marked } = await env.AUTH_DB.prepare(
    "SELECT s.group_id AS id, COALESCE(g.name, s.group_id) AS name FROM access_synced_members s LEFT JOIN groups g ON g.id = s.group_id WHERE s.user_id = ?",
  )
    .bind(userId)
    .all<Named>();
  const token = new Set(accessGroups);
  const wanted = new Map(maps.filter((m) => token.has(m.access_group)).map((m) => [m.id, m.name]));
  const out: SyncResult = { added: [], removed: [], kept: [] };
  const at = iso(now);
  for (const [id, name] of wanted) if (await addOne(env, id, userId, at)) out.added.push(name);
  for (const g of marked) {
    if (wanted.has(g.id)) continue;
    const r = await removeOne(env, g.id, userId);
    if (r === "removed") out.removed.push(g.name);
    if (r === "owner_required") {
      console.warn(`auth: Access group sync kept account ${userId} in group ${g.id}: removing it would leave no active owner`);
      out.kept.push(g.name);
    }
  }
  for (const list of [out.added, out.removed, out.kept]) list.sort(byCodeUnit);
  return out;
}

// applyAccessGroups is syncGroups plus the record of the groups seen and an
// audit event when anything changed (or was kept by the owner invariant).
export async function applyAccessGroups(env: Env, userId: string, accessGroups: string[], now: number, via: "signin" | "mapping"): Promise<SyncResult> {
  if (via === "signin") {
    await env.AUTH_DB.prepare(
      `INSERT INTO access_seen_groups (user_id, groups, seen_at) VALUES (?, ?, ?)
       ON CONFLICT (user_id) DO UPDATE SET groups = excluded.groups, seen_at = excluded.seen_at`,
    )
      .bind(userId, JSON.stringify(accessGroups), iso(now))
      .run();
  }
  const r = await syncGroups(env, userId, accessGroups, now);
  if (r.added.length || r.removed.length || r.kept.length) {
    const detail = { via, added: r.added.join(", "), removed: r.removed.join(", "), keptForOwner: r.kept.join(", ") };
    await emitEvent(env, { kind: "groups_synced", userId, method: ACCESS_METHOD, detail }, now);
  }
  return r;
}

// resyncEveryone applies the (changed) mapping to every person whose groups an
// Access sign-in recorded; returns how many of them had a membership change.
export async function resyncEveryone(env: Env, now: number): Promise<number> {
  const { results } = await env.AUTH_DB.prepare("SELECT s.user_id, s.groups FROM access_seen_groups s JOIN users u ON u.id = s.user_id ORDER BY s.user_id").all<{
    user_id: string;
    groups: string;
  }>();
  let changed = 0;
  for (const row of results) {
    const r = await applyAccessGroups(env, row.user_id, JSON.parse(row.groups) as string[], now, "mapping");
    if (r.added.length || r.removed.length) changed++;
  }
  return changed;
}
