// Tables of GO-151: linked provider identities, sign-in flows in progress,
// and the invariant that no account is left without a way in. A leading
// underscore keeps this file out of the Pages route table. _schema.ts lists
// PROVIDER_TABLES in TABLES (new tables and triggers need no schema version);
// schema.sql carries the same (a test compares).
//
//   identities   one row per account at a provider linked to an account here:
//                the pair (provider, subject) is the identity — unique, so one
//                provider account opens at most one account here — and an
//                account links at most one account per provider. `email` is
//                the address the provider showed when the link was made, for
//                display only: it decides nothing.
//   oauth_flows  one row per sign-in started at a provider, keyed by the
//                SHA-256 of the random handle in the browser's flow cookie:
//                the state (as a hash), the nonce, the PKCE verifier, where to
//                go next and why (sign in | accept an invitation | link to the
//                signed-in account). Consumed — deleted — by the one callback
//                that presents the handle, whatever its outcome; 10 minutes at
//                most. An invitation rides here as the SHA-256 of its token.
//   session_methods  how each session was started ("password", a provider id;
//                GO-139 and GO-140 add theirs) — what the per-role method
//                policy reads at every token issue. A table of its own rather
//                than a sessions column: no schema version, and a session
//                without a row (every session from before) was a password one.
//
// The last-method invariant: every account keeps a password or at least one
// identity. The triggers refuse (RAISE ABORT 'method_required') removing the
// last identity of an account without a password, and moving it to another
// account. Removing a password is one conditional statement
// (_provider-identities.ts removePassword: only WHERE an identity exists).
// Like the owner invariant (_rbac-schema.ts), each runs inside one statement
// and D1 runs one statement at a time, so of two concurrent removals of an
// account's last two methods the second sees the first and changes nothing.
// Deleting the account itself (users row first, _users.ts) is not blocked:
// the condition looks for an existing account. (Clearing a password by hand
// in SQL is not policed: the worker never does it but through removePassword.)

export const METHOD_REQUIRED = "method_required";
export const isMethodViolation = (e: unknown): boolean => String(e).includes(METHOD_REQUIRED);

const noOtherWay = (user: string) =>
  `EXISTS (SELECT 1 FROM users u WHERE u.id = ${user} AND u.password_hash IS NULL)
     AND NOT EXISTS (SELECT 1 FROM identities i WHERE i.user_id = ${user})`;

export const PROVIDER_TABLES = [
  `CREATE TABLE IF NOT EXISTS identities (
     id           TEXT PRIMARY KEY,
     provider     TEXT NOT NULL,
     subject      TEXT NOT NULL,
     user_id      TEXT NOT NULL,
     email        TEXT NOT NULL DEFAULT '',
     linked_at    TEXT NOT NULL,
     last_used_at TEXT,
     UNIQUE (provider, subject),
     UNIQUE (user_id, provider)
   )`,
  `CREATE TABLE IF NOT EXISTS oauth_flows (
     handle_hash  TEXT PRIMARY KEY,
     provider     TEXT NOT NULL,
     state_hash   TEXT NOT NULL,
     nonce        TEXT NOT NULL,
     verifier     TEXT,
     redirect_uri TEXT NOT NULL,
     purpose      TEXT NOT NULL CHECK (purpose IN ('signin', 'invite', 'link')),
     next         TEXT NOT NULL DEFAULT '',
     invite_hash  TEXT,
     user_id      TEXT,
     session_id   TEXT,
     created_at   TEXT NOT NULL,
     expires_at   TEXT NOT NULL
   )`,
  `CREATE INDEX IF NOT EXISTS idx_oauth_flows_expiry ON oauth_flows (expires_at)`,
  `CREATE TABLE IF NOT EXISTS session_methods (
     session_id TEXT PRIMARY KEY,
     method     TEXT NOT NULL
   )`,
  `CREATE TRIGGER IF NOT EXISTS auth_method_identity_delete AFTER DELETE ON identities
     WHEN ${noOtherWay("OLD.user_id")}
   BEGIN SELECT RAISE(ABORT, 'method_required'); END`,
  `CREATE TRIGGER IF NOT EXISTS auth_method_identity_move AFTER UPDATE OF user_id ON identities
     WHEN OLD.user_id <> NEW.user_id AND ${noOtherWay("OLD.user_id")}
   BEGIN SELECT RAISE(ABORT, 'method_required'); END`,
];
