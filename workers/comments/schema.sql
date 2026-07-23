-- D1 schema for the SSG comments worker.
-- Apply it:
--   npx wrangler d1 create ssg-comments
--   npx wrangler d1 execute ssg-comments --file=workers/comments/schema.sql --remote
--   (drop --remote to seed a local dev database)

CREATE TABLE IF NOT EXISTS comments (
  id          TEXT PRIMARY KEY,          -- uuid
  url         TEXT NOT NULL,             -- the page path the comment belongs to
  author      TEXT NOT NULL,             -- display name
  email       TEXT,                      -- optional; never shown, used for the avatar hash
  body        TEXT NOT NULL,
  status      TEXT NOT NULL DEFAULT 'pending', -- pending | approved | spam
  created_at  TEXT NOT NULL,             -- ISO 8601

  -- Compliance ("who and what"): the IP is PII, so only a salted hash is kept,
  -- alongside the user-agent. Enough to correlate and to answer an abuse report,
  -- not to re-identify a visitor.
  ip_hash     TEXT,
  user_agent  TEXT,

  -- Optional gravatar-style hash of the email for an avatar (no email stored in
  -- the clear is required for this).
  avatar_hash TEXT
);

-- The hot path: approved comments for one page, newest or oldest first.
CREATE INDEX IF NOT EXISTS idx_comments_url_status
  ON comments (url, status, created_at);

-- The moderation queue: everything pending, newest first.
CREATE INDEX IF NOT EXISTS idx_comments_status
  ON comments (status, created_at);
