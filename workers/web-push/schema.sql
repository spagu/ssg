-- web-push worker, schema version 1. The worker applies the same statements on
-- first use (functions/api/push/_schema.ts); this file is for applying it by
-- hand: npx wrangler d1 execute ssg-push --file=workers/web-push/schema.sql --remote
CREATE TABLE IF NOT EXISTS push_subscriptions (
  endpoint   TEXT PRIMARY KEY,
  p256dh     TEXT NOT NULL,
  auth       TEXT NOT NULL,
  ua_hash    TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
