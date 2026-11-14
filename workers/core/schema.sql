-- core's own tables in the optional CORE_DB database (GO-149). The worker
-- creates them on first use; this file is for applying by hand:
--   wrangler d1 execute <database> --remote --file workers/core/schema.sql
-- It must describe exactly what functions/api/core/_schema.ts creates (a test
-- compares the two). Every name carries the core_ prefix, and schema_meta has
-- the shape the other templates use, so the database may be shared.

CREATE TABLE IF NOT EXISTS schema_meta (
  component  TEXT PRIMARY KEY,
  version    INTEGER NOT NULL,
  updated_at TEXT NOT NULL
);

-- Module settings: hidden from the navigation and the home page, position,
-- labels per language (JSON), an admin note. No row = the defaults.
CREATE TABLE IF NOT EXISTS core_module_settings (
  module_id  TEXT PRIMARY KEY,
  hidden     INTEGER NOT NULL DEFAULT 0,
  position   INTEGER,
  labels     TEXT NOT NULL DEFAULT '{}',
  note       TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  updated_by TEXT NOT NULL
);

-- Each observed change between reachable and unreachable.
CREATE TABLE IF NOT EXISTS core_status_history (
  id          TEXT PRIMARY KEY,
  module_id   TEXT NOT NULL,
  status      TEXT NOT NULL,
  reason      TEXT,
  observed_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_core_status_history_module ON core_status_history (module_id, observed_at);

-- Who changed which setting, and when.
CREATE TABLE IF NOT EXISTS core_changes (
  id        TEXT PRIMARY KEY,
  at        TEXT NOT NULL,
  actor     TEXT NOT NULL,
  actor_id  TEXT,
  module_id TEXT,
  action    TEXT NOT NULL,
  detail    TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_core_changes_at ON core_changes (at);

-- The shared-password lockout, durable across isolates.
CREATE TABLE IF NOT EXISTS core_lockout (
  key          TEXT PRIMARY KEY,
  failures     INTEGER NOT NULL,
  window_start INTEGER NOT NULL,
  locked_until INTEGER NOT NULL
);

INSERT OR IGNORE INTO schema_meta (component, version, updated_at) VALUES ('core', 1, '1970-01-01T00:00:00.000Z');
