-- D1 schema for the SSG newsletter worker (schema version 1).
-- Apply it:
--   npx wrangler d1 create ssg-newsletter
--   npx wrangler d1 execute ssg-newsletter --file=workers/newsletter/schema.sql --remote
--   (drop --remote to seed a local dev database)
--
-- The worker also creates this table on first use (functions/api/newsletter/
-- _schema.ts), so applying the file by hand is optional. Keep the two in sync.
--
-- Migrations: every statement is IF NOT EXISTS, so re-applying this file is
-- safe. A later version that adds a column will ship it as an
-- `ALTER TABLE subscribers ADD COLUMN …` run by _schema.ts (as the comments
-- worker does for parent_id), and list it in this header. If you manage D1 with
-- `wrangler d1 migrations`, copy this file to migrations/0001_newsletter.sql.

CREATE TABLE IF NOT EXISTS subscribers (
  id              TEXT PRIMARY KEY,          -- uuid
  email           TEXT NOT NULL UNIQUE,      -- lower-cased; one row per address
  status          TEXT NOT NULL,             -- pending | confirmed | unsubscribed
  language        TEXT,                      -- e.g. "pl", "pt-BR"; NULL when not given
  source_page     TEXT,                      -- page path the sign-up came from
  tags            TEXT NOT NULL DEFAULT '',  -- comma-separated, lower-case

  -- Proof of consent (GDPR Art. 7(1)): the exact wording the visitor agreed to
  -- and when. Refreshed on a re-subscribe, which is a new consent.
  consent_text    TEXT NOT NULL,
  consent_at      TEXT NOT NULL,             -- ISO 8601

  -- Random per-subscriber secret behind the confirm and unsubscribe links. Not
  -- derived from the email, so a link reveals nothing about its owner.
  token           TEXT NOT NULL UNIQUE,

  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL,
  confirmed_at    TEXT,
  unsubscribed_at TEXT,
  confirm_sent_at TEXT,                      -- throttles repeat confirmation mails

  -- The IP and User-Agent are kept only as salted hashes (NEWSLETTER_HASH_SALT):
  -- enough to spot one source signing up hundreds of addresses, not to
  -- re-identify anyone. Without a salt both stay NULL.
  ip_hash         TEXT,
  ua_hash         TEXT
);

-- Admin counts by status and language, and the CSV export.
CREATE INDEX IF NOT EXISTS idx_subscribers_status
  ON subscribers (status, language);

-- The retention purge: old pending / unsubscribed rows.
CREATE INDEX IF NOT EXISTS idx_subscribers_updated
  ON subscribers (status, updated_at);
