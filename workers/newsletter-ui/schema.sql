-- D1 schema for the newsletter-ui worker (schema version 1).
--
-- These are newsletter-ui's OWN tables. The `subscribers` table belongs to the
-- `newsletter` template and is never created, altered or indexed from here.
--
-- The worker creates these tables on first use (src/schema.ts), so applying
-- the file by hand is optional:
--   npx wrangler d1 execute ssg-newsletter --file=workers/newsletter-ui/schema.sql --remote
-- Keep it in sync with src/schema.ts (test/schema-sql.test.ts checks that).
--
-- Migrations: every statement is IF NOT EXISTS. A later version adds a column
-- through src/schema.ts MIGRATIONS (a guarded ALTER TABLE … ADD COLUMN),
-- records the version in schema_meta, and adds the column here too.

-- Which schema version of which component this database carries.
CREATE TABLE IF NOT EXISTS schema_meta (
  component  TEXT PRIMARY KEY,             -- "newsletter-ui"
  version    INTEGER NOT NULL,
  updated_at TEXT NOT NULL
);

-- Panel settings (sender, sending rate…). Secrets never go here.
CREATE TABLE IF NOT EXISTS settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,                -- JSON
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS campaigns (
  id            TEXT PRIMARY KEY,          -- uuid
  subject       TEXT NOT NULL,
  body          TEXT NOT NULL DEFAULT '',
  body_format   TEXT NOT NULL DEFAULT 'markdown',  -- markdown | html
  text_override TEXT,                      -- plain-text part; NULL = generated from body
  from_email    TEXT,                      -- NULL = the configured default sender
  from_name     TEXT,
  segment       TEXT NOT NULL DEFAULT '{}',-- JSON: languages, tags (always status = confirmed)
  status        TEXT NOT NULL DEFAULT 'draft',  -- draft | scheduled | sending | paused | cancelled | sent
  scheduled_at  TEXT,                      -- ISO 8601
  started_at    TEXT,
  finished_at   TEXT,
  total_count   INTEGER NOT NULL DEFAULT 0,
  sent_count    INTEGER NOT NULL DEFAULT 0,
  failed_count  INTEGER NOT NULL DEFAULT 0,
  source_post   TEXT,                      -- the post a draft was made from, if any
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

-- The cron's question: which scheduled campaigns are due.
CREATE INDEX IF NOT EXISTS idx_campaigns_status
  ON campaigns (status, scheduled_at);

-- One row per campaign and subscriber: written as `claimed` before the mail
-- goes out, `sent` / `failed` after. The UNIQUE key is what makes a retried
-- batch skip everyone already handled.
CREATE TABLE IF NOT EXISTS campaign_sends (
  campaign_id   TEXT NOT NULL,
  subscriber_id TEXT NOT NULL,             -- subscribers.id (no foreign key: that table is not ours)
  status        TEXT NOT NULL,             -- claimed | retry | sent | failed | skipped | uncertain
  attempts      INTEGER NOT NULL DEFAULT 0,
  error         TEXT,
  claimed_at    TEXT NOT NULL,
  sent_at       TEXT,
  UNIQUE (campaign_id, subscriber_id)
);

CREATE INDEX IF NOT EXISTS idx_campaign_sends_status
  ON campaign_sends (campaign_id, status);

-- Addresses that must not be mailed. Keyed by a hash of the address, so the
-- row can outlive an erasure request (email set to NULL) and still stop the
-- address from coming back through an import.
CREATE TABLE IF NOT EXISTS suppressions (
  email_hash TEXT PRIMARY KEY,
  email      TEXT,                         -- NULL after erasure
  reason     TEXT NOT NULL,                -- hard | soft | complaint | manual
  count      INTEGER NOT NULL DEFAULT 1,   -- soft bounces in a row
  source     TEXT,                         -- smtp | webhook name | import | admin
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

-- Excluding suppressed addresses from a query over subscribers joins on the
-- clear-text address (SQLite cannot compute the hash; see src/suppress.ts).
CREATE INDEX IF NOT EXISTS idx_suppressions_email
  ON suppressions (email);

-- Failed HTTP Basic logins per client (salted hash of the IP), for the
-- lockout. Short-lived: a row is replaced when its window ends and deleted on
-- a successful login.
CREATE TABLE IF NOT EXISTS auth_attempts (
  ip_hash      TEXT PRIMARY KEY,
  failures     INTEGER NOT NULL DEFAULT 0,
  window_start TEXT NOT NULL,
  locked_until TEXT
);

-- ---------------------------------------------------------------------------
-- Schema version 2 (GO-126): the send engine's own tables.

-- One row per started campaign: the keyset cursor over subscribers.id up to
-- which ranges have been planned, whether all of them have, and why the
-- engine paused the campaign (a render or configuration error), if it did.
CREATE TABLE IF NOT EXISTS send_plans (
  campaign_id TEXT PRIMARY KEY,
  cursor      TEXT NOT NULL DEFAULT '',    -- last subscribers.id planned; '' = none yet
  planned_all INTEGER NOT NULL DEFAULT 0,  -- 1 = no subscriber after cursor matched
  error       TEXT,
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);

-- One row per queue job: the subscriber range (after_id, until_id].
CREATE TABLE IF NOT EXISTS send_batches (
  campaign_id TEXT NOT NULL,
  after_id    TEXT NOT NULL,               -- exclusive lower bound; '' = the start
  until_id    TEXT NOT NULL,               -- inclusive upper bound
  status      TEXT NOT NULL,               -- queued | parked | done
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL,
  retry_only  INTEGER NOT NULL DEFAULT 0,  -- v5: reopened to re-send `retry` rows only
  PRIMARY KEY (campaign_id, after_id)
);

CREATE INDEX IF NOT EXISTS idx_send_batches_status
  ON send_batches (campaign_id, status);

-- Messages handed to the transport per UTC minute ("2026-11-13T12:34"), one
-- atomic counter row shared by every consumer.
CREATE TABLE IF NOT EXISTS send_rate (
  minute  TEXT PRIMARY KEY,
  used    INTEGER NOT NULL DEFAULT 0,
  granted INTEGER NOT NULL DEFAULT 0       -- what the last reservation got
);

-- Queue jobs that ran out of retries. The work itself is not lost (the
-- planner re-queues an unfinished range); this is what the panel counts.
CREATE TABLE IF NOT EXISTS send_dead_letters (
  id          TEXT PRIMARY KEY,            -- uuid
  campaign_id TEXT,
  job         TEXT NOT NULL,               -- the job body, JSON
  attempts    INTEGER NOT NULL DEFAULT 0,
  received_at TEXT NOT NULL
);

-- Campaign statistics (GO-129, schema version 3). One row per link of a
-- campaign: the only place a click redirect may go, and a plain counter.
-- No column here can point at a person (no subscriber id, address, token,
-- IP or User-Agent), so a click is never tied to who made it.
CREATE TABLE IF NOT EXISTS click_links (
  campaign_id TEXT NOT NULL,
  link_id     TEXT NOT NULL,               -- first 16 hex of sha256(url): the same for every recipient
  url         TEXT NOT NULL,               -- the target, as the rendered mail had it
  clicks      INTEGER NOT NULL DEFAULT 0,  -- GET requests to the redirect (scanners included)
  created_at  TEXT NOT NULL,
  PRIMARY KEY (campaign_id, link_id)
);

-- The unsubscribe attribution looks up a subscriber's latest send.
CREATE INDEX IF NOT EXISTS idx_campaign_sends_subscriber
  ON campaign_sends (subscriber_id, status, sent_at);

-- ---------------------------------------------------------------------------
-- Schema version 4 (GO-127): bounces and webhooks.

-- Soft bounces below NEWSLETTER_SOFT_BOUNCE_LIMIT, per address. Not rows in
-- suppressions on purpose: every row there blocks mail, a count below the
-- limit must not. Reaching the limit moves the address to suppressions.
CREATE TABLE IF NOT EXISTS soft_bounces (
  email_hash TEXT PRIMARY KEY,             -- the suppression key of the address
  email      TEXT NOT NULL,
  count      INTEGER NOT NULL DEFAULT 0,   -- soft bounces in a row
  source     TEXT NOT NULL,                -- smtp | resend | mailchannels: who resets it
  detail     TEXT,                         -- the last bounce's words
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_soft_bounces_email
  ON soft_bounces (email);

-- Webhook events already applied ("resend:<svix-id>", "mailchannels:<hash>"),
-- so a redelivered event counts once. Pruned after seven days.
CREATE TABLE IF NOT EXISTS hook_events (
  id          TEXT PRIMARY KEY,
  received_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_hook_events_received
  ON hook_events (received_at);

-- ---------------------------------------------------------------------------
-- Schema version 6 (GO-137): the admin audit trail.

-- One row per state-changing admin request that reached its handler. The
-- target is an id from the path (campaign, subscriber, suppression hash),
-- never an address, so an erasure leaves nothing of the person here. Purged
-- on write after NEWSLETTER_AUDIT_RETENTION_DAYS (default 365).
CREATE TABLE IF NOT EXISTS admin_audit (
  id          TEXT PRIMARY KEY,
  at          TEXT NOT NULL,               -- ISO 8601, UTC
  mode        TEXT NOT NULL,               -- auth | password | access
  actor_id    TEXT,                        -- the auth account id (auth mode only)
  actor_email TEXT,                        -- the auth account's address (auth mode only)
  action      TEXT NOT NULL,               -- the route's audit action, e.g. campaign.schedule
  target      TEXT,                        -- e.g. campaign:<id>, subscriber:<id>
  status      INTEGER NOT NULL             -- the HTTP status the request was answered with
);

CREATE INDEX IF NOT EXISTS idx_admin_audit_at
  ON admin_audit (at, id);
