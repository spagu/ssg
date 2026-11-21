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
// The marks and the transactions are _synced.ts's, shared with the external
// issuer's roles claim (#435).
//
// The groups seen are kept (access_seen_groups), so that a change of the
// mapping is applied to everyone at once (resyncEveryone); permissions follow
// at each person's next token — a refresh — like any change of membership.

import { emitEvent } from "./_audit";
import { ACCESS_METHOD } from "./_access-config";
import { iso } from "./_lib";
import type { Env } from "./_lib";
import { syncMarked } from "./_synced";
import type { Marks, SyncResult } from "./_synced";

export type { SyncResult } from "./_synced";

const ACCESS_MARKS: Marks = { table: "access_synced_members", col: "group_id", label: "Access group sync" };

// syncGroups applies `accessGroups` (the token's) to one person.
export async function syncGroups(env: Env, userId: string, accessGroups: string[], now: number): Promise<SyncResult> {
  const { results: maps } = await env.AUTH_DB.prepare(
    "SELECT m.access_group, g.id, g.name FROM access_group_map m JOIN groups g ON g.id = m.group_id",
  ).all<{ access_group: string; id: string; name: string }>();
  const token = new Set(accessGroups);
  return syncMarked(env, ACCESS_MARKS, userId, new Map(maps.filter((m) => token.has(m.access_group)).map((m) => [m.id, m.name])), now);
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
