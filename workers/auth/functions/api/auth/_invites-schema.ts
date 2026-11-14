// Tables of GO-134: invitations and password reset links. A leading
// underscore keeps this file out of the Pages route table. _schema.ts lists
// INVITES_TABLES in TABLES (new tables need no schema version: CREATE … IF
// NOT EXISTS reaches existing databases too); schema.sql carries the same
// tables (a test compares).
//
//   invitations      one row per invitation: the address, the roles and groups
//                    it will hand out (JSON arrays of ids, re-checked at
//                    acceptance), the language of its mail, who sent it, and
//                    the SHA-256 of its one-time token — never the token.
//                    Status is derived: revoked_at, else accepted_at, else
//                    expires_at decides. At most one open invitation per
//                    address: inviting again revokes the earlier one.
//   password_resets  one row per reset link: the account, the token's SHA-256,
//                    who asked (an administrator's id, or NULL for "forgot
//                    password"), the expiry and when it was used. A new link
//                    for an account retires the earlier unused ones.

export const INVITES_TABLES = [
  `CREATE TABLE IF NOT EXISTS invitations (
     id               TEXT PRIMARY KEY,
     email            TEXT NOT NULL,
     display_name     TEXT NOT NULL DEFAULT '',
     token_hash       TEXT NOT NULL UNIQUE,
     role_ids         TEXT NOT NULL DEFAULT '[]',
     group_ids        TEXT NOT NULL DEFAULT '[]',
     lang             TEXT NOT NULL DEFAULT 'en',
     invited_by       TEXT,
     created_at       TEXT NOT NULL,
     expires_at       TEXT NOT NULL,
     accepted_at      TEXT,
     accepted_user_id TEXT,
     revoked_at       TEXT,
     revoked_by       TEXT,
     revoke_reason    TEXT
   )`,
  `CREATE INDEX IF NOT EXISTS idx_invitations_email ON invitations (email, accepted_at, revoked_at)`,
  `CREATE TABLE IF NOT EXISTS password_resets (
     id         TEXT PRIMARY KEY,
     user_id    TEXT NOT NULL,
     token_hash TEXT NOT NULL UNIQUE,
     created_by TEXT,
     created_at TEXT NOT NULL,
     expires_at TEXT NOT NULL,
     used_at    TEXT
   )`,
  `CREATE INDEX IF NOT EXISTS idx_password_resets_user ON password_resets (user_id, used_at)`,
];
