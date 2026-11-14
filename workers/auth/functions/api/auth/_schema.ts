// Runtime schema self-init for the auth worker's own D1 database (AUTH_DB).
// A leading underscore keeps this file out of the Pages route table.
//
// Every statement is CREATE … IF NOT EXISTS, so it is a no-op once the tables
// exist, and it runs at most once per isolate per database (the promise is
// cached). Keep the statements in sync with schema.sql — a test compares them.
//
// Migrations: a later change appends { version, statements } to MIGRATIONS
// (guarded ALTERs: a duplicate-column error is ignored, two isolates may race
// to the same ALTER) and mirrors the change in TABLES and schema.sql. Never
// DROP, never change a column's type. Version 2 (GO-133) is _rbac-schema.ts:
// roles, groups, grants, the owner-invariant triggers and the built-in roles —
// in MIGRATIONS only, so a fresh database and an upgraded one take the same
// path (TABLES, then every migration in order).
//
// A NEW table needs no version: CREATE … IF NOT EXISTS in TABLES reaches an
// existing database as surely as a fresh one, on the next ensureSchema. That
// is how GO-134's invitations and password_resets (_invites-schema.ts) join,
// and GO-151's identities, oauth_flows, session_methods and their triggers
// (_provider-schema.ts) and auth_events (_audit.ts), and GO-139's Access group
// mapping (_access-schema.ts), and GO-138's signing_keys (_admin-keys.ts); a version is for
// changing what already exists (an ALTER, a seed).
//
// Tables (schema version 1):
//   users            one row per person; password_hash is NULL for an account
//                    without a password (a login provider only, GO-151)
//   sessions         one row per login: the CURRENT refresh token's SHA-256,
//                    never the token; revoked_at + revoke_reason end it early
//   refresh_history  every refresh token a session has rotated away from, with
//                    its successor: a replayed one is theft (or the two-tab race)
//   login_attempts   failure counters and locks, per account and per IP, keyed
//                    by a salted hash — never the address or the IP itself
//   schema_meta      the schema version, per component

import type { Env } from "./_lib";
import { INVITES_TABLES } from "./_invites-schema";
import { RBAC_MIGRATION } from "./_rbac-schema";
import { SERVICE_TABLES } from "./_service-schema";
import { PROVIDER_TABLES } from "./_provider-schema";
import { AUDIT_TABLES } from "./_audit";
import { ACCESS_TABLES } from "./_access-schema";
import { MFA_TABLES } from "./_mfa-schema";
import { KEY_TABLES } from "./_admin-keys";

export const COMPONENT = "auth";
export const BASE_VERSION = 1;

export const TABLES = [
  `CREATE TABLE IF NOT EXISTS schema_meta (
     component  TEXT PRIMARY KEY,
     version    INTEGER NOT NULL,
     updated_at TEXT NOT NULL
   )`,
  `CREATE TABLE IF NOT EXISTS users (
     id                   TEXT PRIMARY KEY,
     email                TEXT NOT NULL UNIQUE,
     display_name         TEXT NOT NULL DEFAULT '',
     password_hash        TEXT,
     status               TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'blocked')),
     must_change_password INTEGER NOT NULL DEFAULT 0,
     created_at           TEXT NOT NULL,
     updated_at           TEXT NOT NULL,
     last_login_at        TEXT
   )`,
  `CREATE TABLE IF NOT EXISTS sessions (
     id            TEXT PRIMARY KEY,
     user_id       TEXT NOT NULL,
     refresh_hash  TEXT NOT NULL UNIQUE,
     created_at    TEXT NOT NULL,
     last_used_at  TEXT NOT NULL,
     expires_at    TEXT NOT NULL,
     revoked_at    TEXT,
     revoke_reason TEXT,
     ip_hash       TEXT,
     ua_hash       TEXT
   )`,
  `CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions (user_id, revoked_at)`,
  `CREATE TABLE IF NOT EXISTS refresh_history (
     hash           TEXT PRIMARY KEY,
     session_id     TEXT NOT NULL,
     successor_hash TEXT NOT NULL,
     rotated_at     TEXT NOT NULL
   )`,
  `CREATE INDEX IF NOT EXISTS idx_refresh_history_session ON refresh_history (session_id)`,
  `CREATE TABLE IF NOT EXISTS login_attempts (
     key             TEXT PRIMARY KEY,
     kind            TEXT NOT NULL,
     failures        INTEGER NOT NULL DEFAULT 0,
     last_failure_at TEXT NOT NULL,
     locked_until    TEXT
   )`,
  ...INVITES_TABLES,
  ...SERVICE_TABLES,
  ...PROVIDER_TABLES,
  ...AUDIT_TABLES,
  ...ACCESS_TABLES,
  ...MFA_TABLES,
  ...KEY_TABLES,
];

export interface Migration {
  version: number;
  statements: string[];
}

// Append, never edit or reorder.
export const MIGRATIONS: Migration[] = [RBAC_MIGRATION];

const ready = new WeakMap<D1Database, Promise<void>>();

// ensureSchema creates the tables once per isolate and applies pending
// migrations. On failure it clears the cache so a later request retries.
export function ensureSchema(env: Env): Promise<void> {
  const db = env.AUTH_DB;
  let p = ready.get(db);
  if (!p) {
    p = migrate(db, MIGRATIONS).catch((e) => {
      ready.delete(db);
      throw e;
    });
    ready.set(db, p);
  }
  return p;
}

// migrate is ensureSchema without the cache, with the migration list passed in
// so tests can exercise a version bump.
export async function migrate(db: D1Database, migrations: Migration[]): Promise<void> {
  const now = new Date().toISOString();
  await db.batch([
    ...TABLES.map((s) => db.prepare(s)),
    db.prepare("INSERT OR IGNORE INTO schema_meta (component, version, updated_at) VALUES (?, ?, ?)").bind(COMPONENT, BASE_VERSION, now),
  ]);
  const row = await db.prepare("SELECT version FROM schema_meta WHERE component = ?").bind(COMPONENT).first<{ version: number }>();
  let current = (row as { version: number }).version;
  for (const m of migrations) {
    if (m.version <= current) continue;
    for (const s of m.statements) {
      try {
        await db.prepare(s).run();
      } catch (e) {
        if (!/duplicate column/i.test(String(e))) throw e;
      }
    }
    await db
      .prepare("UPDATE schema_meta SET version = ?, updated_at = ? WHERE component = ? AND version < ?")
      .bind(m.version, now, COMPONENT, m.version)
      .run();
    current = m.version;
  }
}
