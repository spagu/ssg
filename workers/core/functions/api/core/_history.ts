// Status history (GO-149): when a module is seen to change between reachable
// and unreachable, a row records it — so the panel can say "unreachable since
// yesterday 14:10". A leading underscore keeps this file out of the Pages
// route table.
//
// ONLY ON OBSERVATION, NEVER ON A TIMER: the rows are written when someone
// opens or refreshes the home page (it asks /api/core/status, which calls
// observe). A module nobody looks at has no history, and costs nothing.
// Other answers (denied, unknown, invalid_manifest) say nothing about whether
// the module is up, so they are not recorded.
//
// Retention: rows older than CORE_HISTORY_DAYS (default 90) are deleted with
// every write — except each module's latest row, which says what it is now —
// and the table never holds more than MAX_HISTORY_ROWS.

import { intVar } from "./_lib";
import type { D1Database, D1Statement, Env } from "./_lib";
import { ensureSchema } from "./_schema";

export const OBSERVED = ["reachable", "unreachable"] as const;
export const MAX_HISTORY_ROWS = 5000;
const DAY_MS = 86_400_000;

export interface Latest {
  status: string;
  reason: string | null;
  at: string;
}
export interface HistoryRow {
  module: string;
  status: string;
  reason: string | null;
  at: string;
}

// latestStatuses: each module's latest recorded status.
export async function latestStatuses(db: D1Database): Promise<Map<string, Latest>> {
  await ensureSchema(db);
  const { results } = await db
    .prepare(
      `SELECT module_id, status, reason, observed_at FROM core_status_history h
       WHERE rowid = (SELECT rowid FROM core_status_history WHERE module_id = h.module_id ORDER BY observed_at DESC, rowid DESC LIMIT 1)`,
    )
    .all<{ module_id: string; status: string; reason: string | null; observed_at: string }>();
  return new Map(results.map((r) => [r.module_id, { status: r.status, reason: r.reason, at: r.observed_at }]));
}

// recentHistory: the latest changes across modules, newest first.
export async function recentHistory(db: D1Database, limit = 50): Promise<HistoryRow[]> {
  await ensureSchema(db);
  const { results } = await db
    .prepare("SELECT module_id, status, reason, observed_at FROM core_status_history ORDER BY observed_at DESC, rowid DESC LIMIT ?1")
    .bind(limit)
    .all<{ module_id: string; status: string; reason: string | null; observed_at: string }>();
  return results.map((r) => ({ module: r.module_id, status: r.status, reason: r.reason, at: r.observed_at }));
}

// record: one conditional insert — nothing is written when the module's latest
// row already says the same, so two isolates seeing the same flip at once
// write it (at most) once more, never a run of duplicates.
function record(db: D1Database, id: string, status: string, reason: string | null, at: string): D1Statement {
  return db
    .prepare(
      `INSERT INTO core_status_history (id, module_id, status, reason, observed_at)
       SELECT ?1, ?2, ?3, ?4, ?5
       WHERE COALESCE((SELECT status FROM core_status_history WHERE module_id = ?2 ORDER BY observed_at DESC, rowid DESC LIMIT 1), '') <> ?3`,
    )
    .bind(crypto.randomUUID(), id, status, reason, at);
}

export function purgeHistory(db: D1Database, cutoff: string): D1Statement[] {
  return [
    db
      .prepare(
        `DELETE FROM core_status_history WHERE observed_at < ?1
         AND observed_at < (SELECT MAX(observed_at) FROM core_status_history h WHERE h.module_id = core_status_history.module_id)`,
      )
      .bind(cutoff),
    db.prepare(`DELETE FROM core_status_history WHERE rowid IN (SELECT rowid FROM core_status_history ORDER BY observed_at DESC, rowid DESC LIMIT -1 OFFSET ?1)`).bind(MAX_HISTORY_ROWS),
  ];
}

export interface Observation {
  id: string;
  status: string;
  reason?: string;
}

// observe records every module whose observed status differs from its latest
// row. It never fails its caller: a status answer is worth more than a row of
// history, so a database error is swallowed.
export async function observe(env: Env, items: Observation[]): Promise<void> {
  const db = env.CORE_DB;
  const seen = items.filter((i) => (OBSERVED as readonly string[]).includes(i.status));
  if (!db || !seen.length) return;
  try {
    const latest = await latestStatuses(db);
    const now = Date.now();
    const at = new Date(now).toISOString();
    const flips = seen.filter((i) => latest.get(i.id)?.status !== i.status).map((i) => record(db, i.id, i.status, i.reason ?? null, at));
    if (!flips.length) return;
    const cutoff = new Date(now - intVar(env.CORE_HISTORY_DAYS, 90) * DAY_MS).toISOString();
    await db.batch([...flips, ...purgeHistory(db, cutoff)]);
  } catch {
    // see above
  }
}
