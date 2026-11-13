// Runtime schema self-init for newsletter-ui's OWN tables.
//
// The `subscribers` table belongs to the `newsletter` template and stays at its
// schema v1: this worker never creates, alters or indexes it, so it works on a
// database an existing `newsletter` deployment already filled, with no
// migration. Everything newsletter-ui needs lives in the tables below.
//
// Every statement is CREATE … IF NOT EXISTS, so it is a no-op once the tables
// exist, and it runs at most once per isolate (the promise is cached). Keep the
// statements in sync with schema.sql, the canonical copy for applying by hand.
//
// Migrations. A later change that adds a column appends to MIGRATIONS:
//   { version: 2, statements: ["ALTER TABLE campaigns ADD COLUMN preheader TEXT"] }
// and mirrors the column in TABLES and schema.sql (a fresh database gets it from
// CREATE, an old one from the ALTER). ensureSchema runs every migration newer
// than schema_meta's version, then records the new version. SQLite has no
// "ADD COLUMN IF NOT EXISTS", so a duplicate-column error is ignored — two
// isolates may race to the same ALTER — the guarded ALTER the comments worker
// uses. Never DROP, never change a column's type.

import type { Env } from "./lib";

export const COMPONENT = "newsletter-ui";

// The send engine's own tables (GO-126), schema version 2. They are in TABLES
// for a fresh database and in MIGRATIONS for one at version 1 (CREATE … IF NOT
// EXISTS either way, so running both is harmless).
//   send_plans         one row per started campaign: the keyset cursor over
//                      subscribers.id, whether every range has been planned,
//                      and the reason the engine paused it, if it did
//   send_batches       one row per planned range (after_id, until_id]:
//                      queued → done, or parked while the campaign is paused;
//                      retry_only = 1 (schema version 5) when the range was
//                      reopened for recipients in `retry` only (re-sending
//                      uncertain messages): nobody without a ledger row is mailed
//   send_rate          one counter row per UTC minute, shared by every consumer
//   send_dead_letters  queue jobs that ran out of retries, for the panel's count
export const ENGINE_TABLES = [
  `CREATE TABLE IF NOT EXISTS send_plans (
     campaign_id TEXT PRIMARY KEY,
     cursor      TEXT NOT NULL DEFAULT '',
     planned_all INTEGER NOT NULL DEFAULT 0,
     error       TEXT,
     created_at  TEXT NOT NULL,
     updated_at  TEXT NOT NULL
   )`,
  `CREATE TABLE IF NOT EXISTS send_batches (
     campaign_id TEXT NOT NULL,
     after_id    TEXT NOT NULL,
     until_id    TEXT NOT NULL,
     status      TEXT NOT NULL,
     created_at  TEXT NOT NULL,
     updated_at  TEXT NOT NULL,
     retry_only  INTEGER NOT NULL DEFAULT 0,
     PRIMARY KEY (campaign_id, after_id)
   )`,
  `CREATE INDEX IF NOT EXISTS idx_send_batches_status ON send_batches (campaign_id, status)`,
  `CREATE TABLE IF NOT EXISTS send_rate (
     minute  TEXT PRIMARY KEY,
     used    INTEGER NOT NULL DEFAULT 0,
     granted INTEGER NOT NULL DEFAULT 0
   )`,
  `CREATE TABLE IF NOT EXISTS send_dead_letters (
     id          TEXT PRIMARY KEY,
     campaign_id TEXT,
     job         TEXT NOT NULL,
     attempts    INTEGER NOT NULL DEFAULT 0,
     received_at TEXT NOT NULL
   )`,
];

// Campaign statistics (GO-129). click_links holds one row per link of a
// campaign: the redirect target and a plain counter. It has no column that can
// point at a person — no subscriber id, address, token, IP or User-Agent (a
// test asserts it) — so a click can never be tied to who made it. The index on
// campaign_sends serves the unsubscribe attribution (latest send per subscriber).
export const STATS_TABLES = [
  `CREATE TABLE IF NOT EXISTS click_links (
     campaign_id TEXT NOT NULL,
     link_id     TEXT NOT NULL,
     url         TEXT NOT NULL,
     clicks      INTEGER NOT NULL DEFAULT 0,
     created_at  TEXT NOT NULL,
     PRIMARY KEY (campaign_id, link_id)
   )`,
  `CREATE INDEX IF NOT EXISTS idx_campaign_sends_subscriber ON campaign_sends (subscriber_id, status, sent_at)`,
];

// Bounces (GO-127), schema version 4.
//   soft_bounces  soft bounces below NEWSLETTER_SOFT_BOUNCE_LIMIT, per address:
//                 a separate table, NOT a row in suppressions — every row
//                 there blocks mail, and a count below the limit must not.
//                 `source` says who counted it ('smtp' resets on the next
//                 accepted SMTP send, a provider's on its `delivered` event).
//   hook_events   webhook event ids already applied, so a redelivery counts
//                 once; pruned after a week (longer than any provider retries).
export const BOUNCE_TABLES = [
  `CREATE TABLE IF NOT EXISTS soft_bounces (
     email_hash TEXT PRIMARY KEY,
     email      TEXT NOT NULL,
     count      INTEGER NOT NULL DEFAULT 0,
     source     TEXT NOT NULL,
     detail     TEXT,
     updated_at TEXT NOT NULL
   )`,
  `CREATE INDEX IF NOT EXISTS idx_soft_bounces_email ON soft_bounces (email)`,
  `CREATE TABLE IF NOT EXISTS hook_events (
     id          TEXT PRIMARY KEY,
     received_at TEXT NOT NULL
   )`,
  `CREATE INDEX IF NOT EXISTS idx_hook_events_received ON hook_events (received_at)`,
];

export const TABLES = [
  `CREATE TABLE IF NOT EXISTS schema_meta (
     component  TEXT PRIMARY KEY,
     version    INTEGER NOT NULL,
     updated_at TEXT NOT NULL
   )`,
  `CREATE TABLE IF NOT EXISTS settings (
     key        TEXT PRIMARY KEY,
     value      TEXT NOT NULL,
     updated_at TEXT NOT NULL
   )`,
  `CREATE TABLE IF NOT EXISTS campaigns (
     id            TEXT PRIMARY KEY,
     subject       TEXT NOT NULL,
     body          TEXT NOT NULL DEFAULT '',
     body_format   TEXT NOT NULL DEFAULT 'markdown',
     text_override TEXT,
     from_email    TEXT,
     from_name     TEXT,
     segment       TEXT NOT NULL DEFAULT '{}',
     status        TEXT NOT NULL DEFAULT 'draft',
     scheduled_at  TEXT,
     started_at    TEXT,
     finished_at   TEXT,
     total_count   INTEGER NOT NULL DEFAULT 0,
     sent_count    INTEGER NOT NULL DEFAULT 0,
     failed_count  INTEGER NOT NULL DEFAULT 0,
     source_post   TEXT,
     created_at    TEXT NOT NULL,
     updated_at    TEXT NOT NULL
   )`,
  `CREATE INDEX IF NOT EXISTS idx_campaigns_status ON campaigns (status, scheduled_at)`,
  `CREATE TABLE IF NOT EXISTS campaign_sends (
     campaign_id   TEXT NOT NULL,
     subscriber_id TEXT NOT NULL,
     status        TEXT NOT NULL,
     attempts      INTEGER NOT NULL DEFAULT 0,
     error         TEXT,
     claimed_at    TEXT NOT NULL,
     sent_at       TEXT,
     UNIQUE (campaign_id, subscriber_id)
   )`,
  `CREATE INDEX IF NOT EXISTS idx_campaign_sends_status ON campaign_sends (campaign_id, status)`,
  `CREATE TABLE IF NOT EXISTS suppressions (
     email_hash TEXT PRIMARY KEY,
     email      TEXT,
     reason     TEXT NOT NULL,
     count      INTEGER NOT NULL DEFAULT 1,
     source     TEXT,
     created_at TEXT NOT NULL,
     updated_at TEXT NOT NULL
   )`,
  // Set-based exclusion of suppressed addresses joins on email (src/suppress.ts).
  `CREATE INDEX IF NOT EXISTS idx_suppressions_email ON suppressions (email)`,
  `CREATE TABLE IF NOT EXISTS auth_attempts (
     ip_hash      TEXT PRIMARY KEY,
     failures     INTEGER NOT NULL DEFAULT 0,
     window_start TEXT NOT NULL,
     locked_until TEXT
   )`,
  ...ENGINE_TABLES,
  ...STATS_TABLES,
  ...BOUNCE_TABLES,
];

export interface Migration {
  version: number;
  statements: string[];
}

// Append, never edit or reorder.
export const MIGRATIONS: Migration[] = [
  { version: 2, statements: ENGINE_TABLES }, // GO-126: the send engine
  { version: 3, statements: STATS_TABLES }, // GO-129: campaign statistics
  { version: 4, statements: BOUNCE_TABLES }, // GO-127: bounces and webhooks
  // Uncertain sends: a range reopened for re-sends only (src/engine-uncertain.ts).
  { version: 5, statements: ["ALTER TABLE send_batches ADD COLUMN retry_only INTEGER NOT NULL DEFAULT 0"] },
];

export const BASE_VERSION = 1;

// Caches are keyed by the database binding, so one isolate (or one test file)
// talking to two databases never mistakes one for the other.
const ready = new WeakMap<D1Database, Promise<void>>();
const hasSubscribers = new WeakSet<D1Database>();

// ensureSchema creates the tables once per isolate and applies pending
// migrations. On failure it clears the cache so a later request retries.
export function ensureSchema(env: Env): Promise<void> {
  const db = env.NEWSLETTER_DB;
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
    db
      .prepare("INSERT OR IGNORE INTO schema_meta (component, version, updated_at) VALUES (?, ?, ?)")
      .bind(COMPONENT, BASE_VERSION, now),
  ]);
  // The INSERT OR IGNORE above guarantees the row.
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

// subscribersReady reports whether the `newsletter` template's table exists. A
// positive answer is cached (the table does not disappear); a negative one is
// asked again, so deploying `newsletter` later needs no restart here.
export async function subscribersReady(env: Env): Promise<boolean> {
  const db = env.NEWSLETTER_DB;
  if (hasSubscribers.has(db)) return true;
  const row = await db.prepare("SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'subscribers'").first();
  if (row === null) return false;
  hasSubscribers.add(db);
  return true;
}

// schemaVersion is what /api/health reports.
export async function schemaVersion(env: Env): Promise<number> {
  const row = await env.NEWSLETTER_DB.prepare("SELECT version FROM schema_meta WHERE component = ?").bind(COMPONENT).first<{ version: number }>();
  return row ? row.version : 0;
}
