// core's own tables in the optional CORE_DB database (GO-149). A leading
// underscore keeps this file out of the Pages route table.
//
// Every statement is CREATE … IF NOT EXISTS (or INSERT OR IGNORE), so it is a
// no-op once the tables exist; it runs at most once per isolate and database
// (the promise is cached; a failure is forgotten so the next request retries).
// Keep the statements in sync with schema.sql, the copy for applying by hand
// (test/schema.test.ts compares the two).
//
// All tables carry the core_ prefix, and schema_meta has the shape the other
// templates use, so CORE_DB may be a database another worker already uses.
//
// Migrations: a later change appends { version: 2, statements: [...] } to
// MIGRATIONS and mirrors it in TABLES and schema.sql. Never DROP, never change
// a column's type.

import type { D1Database } from "./_lib";

export const COMPONENT = "core";
export const BASE_VERSION = 1;

export const TABLES = [
  `CREATE TABLE IF NOT EXISTS schema_meta (
     component  TEXT PRIMARY KEY,
     version    INTEGER NOT NULL,
     updated_at TEXT NOT NULL
   )`,
  // One row per module that has a setting; a module without a row has the
  // defaults (shown, the registry's order, the manifest's title, no note).
  // labels is a JSON object {lang: label}.
  `CREATE TABLE IF NOT EXISTS core_module_settings (
     module_id  TEXT PRIMARY KEY,
     hidden     INTEGER NOT NULL DEFAULT 0,
     position   INTEGER,
     labels     TEXT NOT NULL DEFAULT '{}',
     note       TEXT NOT NULL DEFAULT '',
     updated_at TEXT NOT NULL,
     updated_by TEXT NOT NULL
   )`,
  // A row each time a module is seen to change between reachable and
  // unreachable (only when someone looks: no cron).
  `CREATE TABLE IF NOT EXISTS core_status_history (
     id          TEXT PRIMARY KEY,
     module_id   TEXT NOT NULL,
     status      TEXT NOT NULL,
     reason      TEXT,
     observed_at TEXT NOT NULL
   )`,
  `CREATE INDEX IF NOT EXISTS idx_core_status_history_module ON core_status_history (module_id, observed_at)`,
  // Who changed which setting, when. actor is the signed-in address, or the
  // sign-in mode when it names nobody ("password", "access").
  `CREATE TABLE IF NOT EXISTS core_changes (
     id        TEXT PRIMARY KEY,
     at        TEXT NOT NULL,
     actor     TEXT NOT NULL,
     actor_id  TEXT,
     module_id TEXT,
     action    TEXT NOT NULL,
     detail    TEXT NOT NULL DEFAULT '{}'
   )`,
  `CREATE INDEX IF NOT EXISTS idx_core_changes_at ON core_changes (at)`,
  // The shared-password lockout, durable across isolates (_lockout-d1.ts).
  `CREATE TABLE IF NOT EXISTS core_lockout (
     key          TEXT PRIMARY KEY,
     failures     INTEGER NOT NULL,
     window_start INTEGER NOT NULL,
     locked_until INTEGER NOT NULL
   )`,
  `INSERT OR IGNORE INTO schema_meta (component, version, updated_at) VALUES ('core', 1, '1970-01-01T00:00:00.000Z')`,
];

const ready = new WeakMap<D1Database, Promise<void>>();

// ensureSchema creates the tables once per isolate and database.
export function ensureSchema(db: D1Database): Promise<void> {
  let p = ready.get(db);
  if (!p) {
    p = db.batch(TABLES.map((sql) => db.prepare(sql))).then(() => undefined);
    p.catch(() => ready.delete(db));
    ready.set(db, p);
  }
  return p;
}
