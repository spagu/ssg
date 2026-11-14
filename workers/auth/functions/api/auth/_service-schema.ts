// Tables of GO-141: service accounts and their tokens. A leading underscore
// keeps this file out of the Pages route table. _schema.ts lists
// SERVICE_TABLES in TABLES (new tables need no schema version); schema.sql
// carries the same tables (a test compares).
//
// Why a table of its own and not a flag on `users`: everything that lets a
// PERSON in reads `users` — the login form (by address), invitations (they
// create a `users` row), reset links (they point at one), group membership
// (it checks one exists) — and the owner invariant counts `active_owners`,
// which joins `users`. A service account has no `users` row, so none of those
// can reach it, by construction rather than by a check someone could forget.
// It still receives roles through `grants` — its id goes in `grants.user_id`
// (ids are UUIDs, one space) — so effectivePermissions works unchanged; and a
// grant of `owner` to it never satisfies the invariant (no `users` row).
//
//   service_accounts  a name, a description, active or disabled, who made it
//   service_tokens    one row per token: `id` is the short public id that is
//                     also part of the secret (`ssgsvc_<id>_<random>`);
//                     `secret_hash` the SHA-256 of the whole secret — never the
//                     secret; `permissions` a JSON array narrowing the
//                     account's (NULL = none); `allow_sensitive` the sensitive
//                     permissions an owner let through (_service-perms.ts);
//                     expires_at is required; revoked_at ends it early.

export const SERVICE_TABLES = [
  `CREATE TABLE IF NOT EXISTS service_accounts (
     id          TEXT PRIMARY KEY,
     name        TEXT NOT NULL UNIQUE,
     description TEXT NOT NULL DEFAULT '',
     status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
     created_by  TEXT,
     created_at  TEXT NOT NULL,
     updated_at  TEXT NOT NULL
   )`,
  `CREATE TABLE IF NOT EXISTS service_tokens (
     id                 TEXT PRIMARY KEY,
     service_account_id TEXT NOT NULL,
     name               TEXT NOT NULL DEFAULT '',
     secret_hash        TEXT NOT NULL UNIQUE,
     permissions        TEXT,
     allow_sensitive    TEXT NOT NULL DEFAULT '[]',
     created_by         TEXT,
     created_at         TEXT NOT NULL,
     expires_at         TEXT NOT NULL,
     last_used_at       TEXT,
     revoked_at         TEXT,
     revoked_by         TEXT
   )`,
  `CREATE INDEX IF NOT EXISTS idx_service_tokens_account ON service_tokens (service_account_id)`,
];
