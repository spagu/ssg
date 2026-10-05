// Runtime schema self-init. A leading underscore keeps this file out of the
// Pages route table — it is imported by the handlers, never served.
//
// The worker creates its D1 table (and indexes) on first use, so binding the D1
// database is the only setup step. Every statement is IF NOT EXISTS, so it is a
// no-op once the table exists, and it runs at most once per isolate (the
// promise is cached).
//
// Keep these statements in sync with schema.sql, which stays as the canonical
// schema for anyone who prefers to apply it by hand. A future column goes in as
// a guarded ALTER TABLE after the batch, the way the comments worker added
// parent_id.

import { Env } from "./_lib";

const STATEMENTS = [
  `CREATE TABLE IF NOT EXISTS subscribers (
     id              TEXT PRIMARY KEY,
     email           TEXT NOT NULL UNIQUE,
     status          TEXT NOT NULL,
     language        TEXT,
     source_page     TEXT,
     tags            TEXT NOT NULL DEFAULT '',
     consent_text    TEXT NOT NULL,
     consent_at      TEXT NOT NULL,
     token           TEXT NOT NULL UNIQUE,
     created_at      TEXT NOT NULL,
     updated_at      TEXT NOT NULL,
     confirmed_at    TEXT,
     unsubscribed_at TEXT,
     confirm_sent_at TEXT,
     ip_hash         TEXT,
     ua_hash         TEXT
   )`,
  `CREATE INDEX IF NOT EXISTS idx_subscribers_status ON subscribers (status, language)`,
  `CREATE INDEX IF NOT EXISTS idx_subscribers_updated ON subscribers (status, updated_at)`,
];

let ready: Promise<void> | null = null;

// ensureSchema creates the table + indexes once per isolate. On failure it
// clears the cache so a later request retries.
export function ensureSchema(env: Env): Promise<void> {
  if (!ready) {
    ready = (async () => {
      await env.NEWSLETTER_DB.batch(STATEMENTS.map((s) => env.NEWSLETTER_DB.prepare(s)));
    })().catch((e) => {
      ready = null;
      throw e;
    });
  }
  return ready;
}
