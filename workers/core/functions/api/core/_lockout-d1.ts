// The shared-password lockout kept in CORE_DB (GO-149): the same rule as the
// in-memory store (_lockout.ts) — max failures within a window lock the key
// for one window — but shared by every isolate, so an attacker spread over
// isolates gets no extra guesses. A leading underscore keeps this file out of
// the Pages route table.
//
// A failure is one statement (an upsert whose arithmetic reads the row's old
// values), so two isolates failing at once both count. Rows whose window and
// lock have both ended are deleted with every failure: the table holds only
// addresses that failed recently.

import type { D1Database } from "./_lib";
import type { LockoutStore } from "./_lockout";
import { ensureSchema } from "./_schema";

export function d1Lockout(db: D1Database): LockoutStore {
  return {
    async lockedFor(key, now) {
      await ensureSchema(db);
      const row = await db.prepare("SELECT locked_until FROM core_lockout WHERE key = ?1").bind(key).first<{ locked_until: number }>();
      return row && row.locked_until > now ? Math.ceil((row.locked_until - now) / 1000) : 0;
    },
    async fail(key, now, max, windowMs) {
      await ensureSchema(db);
      const [counted] = (await db.batch([
        db
          .prepare(
            `INSERT INTO core_lockout (key, failures, window_start, locked_until)
             VALUES (?1, 1, ?2, CASE WHEN 1 >= ?3 THEN ?2 + ?4 ELSE 0 END)
             ON CONFLICT(key) DO UPDATE SET
               failures = CASE WHEN window_start + ?4 <= ?2 THEN 1 ELSE failures + 1 END,
               window_start = CASE WHEN window_start + ?4 <= ?2 THEN ?2 ELSE window_start END,
               locked_until = CASE WHEN (CASE WHEN window_start + ?4 <= ?2 THEN 1 ELSE failures + 1 END) >= ?3 THEN ?2 + ?4 ELSE locked_until END
             RETURNING failures`,
          )
          .bind(key, now, max, windowMs),
        db.prepare("DELETE FROM core_lockout WHERE window_start + ?2 <= ?1 AND locked_until <= ?1").bind(now, windowMs),
      ])) as { results: { failures: number }[] }[];
      return counted.results[0].failures;
    },
    async clear(key) {
      await ensureSchema(db);
      await db.prepare("DELETE FROM core_lockout WHERE key = ?1").bind(key).run();
    },
  };
}

const durable = new WeakMap<D1Database, LockoutStore>();

// durableLockout: the one D1 store per database (and isolate).
export function durableLockout(db: D1Database): LockoutStore {
  let store = durable.get(db);
  if (!store) durable.set(db, (store = d1Lockout(db)));
  return store;
}
