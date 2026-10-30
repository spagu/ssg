// Runtime schema self-init. A leading underscore keeps this file out of the
// Pages route table — it is imported by the handlers, never served.
//
// The worker creates its D1 table on first use, so binding the database is the
// only setup step. Every statement is IF NOT EXISTS, so it is a no-op once the
// table exists, and it runs at most once per isolate (the promise is cached).
//
// Keep these statements in sync with schema.sql, which stays as the canonical
// schema for anyone who prefers to apply it by hand.

import type { Env } from "./_lib";

const STATEMENTS = [
  `CREATE TABLE IF NOT EXISTS push_subscriptions (
     endpoint   TEXT PRIMARY KEY,
     p256dh     TEXT NOT NULL,
     auth       TEXT NOT NULL,
     ua_hash    TEXT,
     created_at TEXT NOT NULL,
     updated_at TEXT NOT NULL
   )`,
];

let ready: Promise<void> | null = null;

// ensureSchema creates the table once per isolate. On failure it clears the
// cache so a later request retries.
export function ensureSchema(env: Env): Promise<void> {
  if (!ready) {
    ready = (async () => {
      await env.PUSH_DB.batch(STATEMENTS.map((s) => env.PUSH_DB.prepare(s)));
    })().catch((e) => {
      ready = null;
      throw e;
    });
  }
  return ready;
}
