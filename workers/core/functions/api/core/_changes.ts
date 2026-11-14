// The change log of module settings (GO-149): who changed what, and when. A
// leading underscore keeps this file out of the Pages route table.
//
// Who: in the `auth` mode the account (its address, and its id kept beside
// it); with Cloudflare Access the Access address; with a shared password
// nobody in particular, so the word "password". Never a credential.
//
// The rows are written in the same batch as the change itself (one D1
// transaction), so the log never misses a change that happened nor records
// one that did not. Retention: CORE_CHANGES_DAYS (default 365), and never
// more than MAX_CHANGE_ROWS, purged with every write.

import { intVar } from "./_lib";
import type { D1Database, D1Statement, Env } from "./_lib";
import { ensureSchema } from "./_schema";
import type { Caller } from "./_signin";

export const MAX_CHANGE_ROWS = 5000;
const DAY_MS = 86_400_000;

// Actions: hidden, shown, labels, note (one module, detail {from, to});
// order (detail {from: ids, to: ids}); import (detail {modules: count}).
export interface Change {
  module: string | null;
  action: "hidden" | "shown" | "labels" | "note" | "order" | "import";
  detail: Record<string, unknown>;
}
export interface Actor {
  actor: string;
  actorId: string | null;
}
export interface LoggedChange extends Change {
  at: string;
  actor: string;
}

export function actorOf(caller: Caller): Actor {
  if (caller.user) return { actor: caller.user.email || caller.user.id || caller.mode, actorId: caller.user.id ?? null };
  return { actor: caller.mode, actorId: null };
}

// changeStatements: the inserts for `changes` and the purge, to run in the
// batch that makes the change.
export function changeStatements(db: D1Database, env: Env, who: Actor, changes: Change[], now: number): D1Statement[] {
  const at = new Date(now).toISOString();
  const cutoff = new Date(now - intVar(env.CORE_CHANGES_DAYS, 365) * DAY_MS).toISOString();
  return [
    ...changes.map((c) =>
      db
        .prepare("INSERT INTO core_changes (id, at, actor, actor_id, module_id, action, detail) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)")
        .bind(crypto.randomUUID(), at, who.actor, who.actorId, c.module, c.action, JSON.stringify(c.detail)),
    ),
    db.prepare("DELETE FROM core_changes WHERE at < ?1").bind(cutoff),
    db.prepare("DELETE FROM core_changes WHERE rowid IN (SELECT rowid FROM core_changes ORDER BY at DESC, rowid DESC LIMIT -1 OFFSET ?1)").bind(MAX_CHANGE_ROWS),
  ];
}

const parse = (raw: string): Record<string, unknown> => {
  try {
    const v = JSON.parse(raw) as unknown;
    return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
  } catch {
    return {};
  }
};

// recentChanges: the latest changes, newest first.
export async function recentChanges(db: D1Database, limit = 50): Promise<LoggedChange[]> {
  await ensureSchema(db);
  const { results } = await db
    .prepare("SELECT at, actor, module_id, action, detail FROM core_changes ORDER BY at DESC, rowid DESC LIMIT ?1")
    .bind(limit)
    .all<{ at: string; actor: string; module_id: string | null; action: Change["action"]; detail: string }>();
  return results.map((r) => ({ at: r.at, actor: r.actor, module: r.module_id, action: r.action, detail: parse(r.detail) }));
}
