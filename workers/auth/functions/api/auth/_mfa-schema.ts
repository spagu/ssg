// The second factor's tables (GO-140). A leading underscore keeps this file
// out of the Pages route table. New tables only, so they need no schema
// version: _schema.ts lists MFA_TABLES in TABLES and schema.sql mirrors them.
//
//   mfa_factors       one row per authenticator app (kind totp) or passkey.
//                     totp: `secret` is AES-GCM ciphertext (_mfa-crypto.ts),
//                     never the secret; `last_step` the last accepted time
//                     step (replay protection); `confirmed_at` NULL while an
//                     enrolment waits for its first code. passkey: the
//                     credential id, the public key as a JWK, its COSE
//                     algorithm, the signature counter and the transports.
//   mfa_backup_codes  the SHA-256 of each one-time backup code (80 random bits,
//                     so no salt or slow hash is needed — like refresh tokens);
//                     used_at marks a spent one.
//   mfa_pending       the step between a first factor and the session: the
//                     SHA-256 of the handle in the browser's cookie, whose
//                     account, which first factor, where to go after; minutes
//                     long, single use.
//   session_factors   which second factor a session has passed, and when last
//                     (sign-in or step-up). The token policy and step-up read it.
//   mfa_challenges    WebAuthn challenges: random, server-side, single use,
//                     bound to an account, a purpose and the pending handle or
//                     session they were issued to.
//   mfa_reset_tokens  the SHA-256 of every break-glass token already used, so
//                     one value of AUTH_MFA_RESET_TOKEN works once.

export const MFA_TABLES = [
  `CREATE TABLE IF NOT EXISTS mfa_factors (
     id            TEXT PRIMARY KEY,
     user_id       TEXT NOT NULL,
     kind          TEXT NOT NULL CHECK (kind IN ('totp', 'passkey')),
     name          TEXT NOT NULL DEFAULT '',
     secret        TEXT,
     last_step     INTEGER NOT NULL DEFAULT 0,
     credential_id TEXT UNIQUE,
     public_key    TEXT,
     algorithm     INTEGER,
     sign_count    INTEGER NOT NULL DEFAULT 0,
     transports    TEXT NOT NULL DEFAULT '[]',
     confirmed_at  TEXT,
     created_at    TEXT NOT NULL,
     last_used_at  TEXT
   )`,
  `CREATE INDEX IF NOT EXISTS idx_mfa_factors_user ON mfa_factors (user_id)`,
  `CREATE TABLE IF NOT EXISTS mfa_backup_codes (
     code_hash  TEXT PRIMARY KEY,
     user_id    TEXT NOT NULL,
     created_at TEXT NOT NULL,
     used_at    TEXT
   )`,
  `CREATE INDEX IF NOT EXISTS idx_mfa_backup_codes_user ON mfa_backup_codes (user_id)`,
  `CREATE TABLE IF NOT EXISTS mfa_pending (
     token_hash TEXT PRIMARY KEY,
     user_id    TEXT NOT NULL,
     method     TEXT NOT NULL,
     next       TEXT NOT NULL DEFAULT '',
     created_at TEXT NOT NULL,
     expires_at TEXT NOT NULL,
     used_at    TEXT
   )`,
  `CREATE TABLE IF NOT EXISTS session_factors (
     session_id  TEXT PRIMARY KEY,
     factor      TEXT NOT NULL,
     verified_at TEXT NOT NULL
   )`,
  `CREATE TABLE IF NOT EXISTS mfa_challenges (
     challenge  TEXT PRIMARY KEY,
     user_id    TEXT NOT NULL,
     purpose    TEXT NOT NULL,
     bound_to   TEXT NOT NULL,
     expires_at TEXT NOT NULL,
     used_at    TEXT
   )`,
  `CREATE TABLE IF NOT EXISTS mfa_reset_tokens (
     token_hash TEXT PRIMARY KEY,
     user_id    TEXT NOT NULL,
     used_at    TEXT NOT NULL
   )`,
];
