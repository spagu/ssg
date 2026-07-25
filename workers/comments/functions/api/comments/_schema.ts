// Runtime schema self-init. A leading underscore keeps this file out of the
// Pages route table — it is imported by the handlers, never served.
//
// The worker creates its D1 table (and indexes) on first use, so binding the D1
// database is the ONLY setup step — no manual `wrangler d1 execute`. Every
// statement is IF NOT EXISTS, so it is a no-op once the table exists, and it
// runs at most once per isolate (the promise is cached).
//
// Keep these statements in sync with schema.sql, which stays as the canonical
// schema for anyone who prefers to apply it by hand.

import { Env } from "./_lib";

const STATEMENTS = [
  `CREATE TABLE IF NOT EXISTS comments (
     id          TEXT PRIMARY KEY,
     url         TEXT NOT NULL,
     author      TEXT NOT NULL,
     email       TEXT,
     body        TEXT NOT NULL,
     status      TEXT NOT NULL DEFAULT 'pending',
     created_at  TEXT NOT NULL,
     ip_hash     TEXT,
     user_agent  TEXT,
     avatar_hash TEXT,
     parent_id   TEXT
   )`,
  `CREATE INDEX IF NOT EXISTS idx_comments_url_status ON comments (url, status, created_at)`,
  `CREATE INDEX IF NOT EXISTS idx_comments_status ON comments (status, created_at)`,
];

let ready: Promise<void> | null = null;

// ensureSchema creates the table + indexes once per isolate, then runs any
// column migrations. Idempotent, cached so the common path pays nothing after
// the first request. On failure it clears the cache so a later request retries.
export function ensureSchema(env: Env): Promise<void> {
  if (!ready) {
    ready = (async () => {
      await env.COMMENTS_DB.batch(STATEMENTS.map((s) => env.COMMENTS_DB.prepare(s)));
      // Migration for databases created before threaded replies: add parent_id
      // if it's missing. SQLite has no "ADD COLUMN IF NOT EXISTS", so ignore the
      // duplicate-column error on an already-migrated table.
      try {
        await env.COMMENTS_DB.prepare("ALTER TABLE comments ADD COLUMN parent_id TEXT").run();
      } catch (e) {
        /* column already exists */
      }
    })().catch((e) => {
      ready = null;
      throw e;
    });
  }
  return ready;
}
