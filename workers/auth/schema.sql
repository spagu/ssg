-- auth worker — D1 schema, versions 1 and 2 (binding AUTH_DB).
--
-- The worker creates these tables itself on first use
-- (functions/api/auth/_schema.ts, _rbac-schema.ts, _invites-schema.ts); this file is the same
-- schema for applying by hand:
--   npx wrangler d1 execute ssg-auth --remote --file=workers/auth/schema.sql
-- A test keeps the two identical. It records version 1 on purpose: the
-- built-in roles of version 2 are rows, not schema, and the worker writes
-- them (and makes the oldest active account the owner) on its first request.
--
-- Nothing here holds a secret in clear: passwords are PBKDF2 hashes, refresh
-- tokens SHA-256 hashes, IP / User-Agent salted hashes (only with
-- AUTH_HASH_SALT), and login_attempts keys salted hashes.

CREATE TABLE IF NOT EXISTS schema_meta (
  component  TEXT PRIMARY KEY,
  version    INTEGER NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
  id                   TEXT PRIMARY KEY,
  email                TEXT NOT NULL UNIQUE,        -- trimmed, lower-cased
  display_name         TEXT NOT NULL DEFAULT '',
  password_hash        TEXT,                        -- pbkdf2-sha256$<iterations>$<salt>$<hash>; NULL = no password
  status               TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'blocked')),
  must_change_password INTEGER NOT NULL DEFAULT 0,
  created_at           TEXT NOT NULL,
  updated_at           TEXT NOT NULL,
  last_login_at        TEXT
);

CREATE TABLE IF NOT EXISTS sessions (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL,
  refresh_hash  TEXT NOT NULL UNIQUE,               -- SHA-256 of the current refresh token
  created_at    TEXT NOT NULL,
  last_used_at  TEXT NOT NULL,
  expires_at    TEXT NOT NULL,
  revoked_at    TEXT,
  revoke_reason TEXT,                               -- logout | logout_all | password_change | refresh_reuse | user_inactive | admin_revoke | permissions_too_large
  ip_hash       TEXT,
  ua_hash       TEXT
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions (user_id, revoked_at);

CREATE TABLE IF NOT EXISTS refresh_history (
  hash           TEXT PRIMARY KEY,                  -- SHA-256 of a rotated-away refresh token
  session_id     TEXT NOT NULL,
  successor_hash TEXT NOT NULL,
  rotated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_refresh_history_session ON refresh_history (session_id);

CREATE TABLE IF NOT EXISTS login_attempts (
  key             TEXT PRIMARY KEY,                 -- salted hash of kind + address / IP
  kind            TEXT NOT NULL,                    -- account | ip | bootstrap
  failures        INTEGER NOT NULL DEFAULT 0,
  last_failure_at TEXT NOT NULL,
  locked_until    TEXT
);

-- Version 2 (GO-133): roles, groups, grants. Flat: no role inherits from a
-- role, no group contains a group.

CREATE TABLE IF NOT EXISTS roles (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE,                 -- lower-case slug; built-in names never change
  description TEXT NOT NULL DEFAULT '',
  built_in    INTEGER NOT NULL DEFAULT 0,           -- 1 = seeded: owner, newsletter-editor, newsletter-viewer, comments-moderator
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS role_permissions (
  role_id    TEXT NOT NULL,
  permission TEXT NOT NULL,                         -- worker:action, worker:* or *
  PRIMARY KEY (role_id, permission)
);

CREATE TABLE IF NOT EXISTS groups (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS group_members (
  group_id TEXT NOT NULL,
  user_id  TEXT NOT NULL,
  added_by TEXT,                                    -- users.id of the administrator
  added_at TEXT NOT NULL,
  PRIMARY KEY (group_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_group_members_user ON group_members (user_id);

CREATE TABLE IF NOT EXISTS grants (
  id         TEXT PRIMARY KEY,
  role_id    TEXT NOT NULL,
  user_id    TEXT,                                  -- exactly one of user_id / group_id
  group_id   TEXT,
  granted_by TEXT,                                  -- users.id of the administrator; NULL = the system
  granted_at TEXT NOT NULL,
  CHECK ((user_id IS NULL) <> (group_id IS NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_grants_user ON grants (user_id, role_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_grants_group ON grants (group_id, role_id) WHERE group_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_grants_role ON grants (role_id);

-- The owner invariant: at least one active user holds the owner role
-- (directly or through a group) and the role holds '*'. Every change that
-- would end it is undone by the database, with the error 'owner_required'.

CREATE VIEW IF NOT EXISTS owner_holders AS
  SELECT g.user_id AS user_id FROM grants g JOIN roles r ON r.id = g.role_id
  WHERE r.name = 'owner' AND g.user_id IS NOT NULL
  UNION
  SELECT m.user_id FROM grants g JOIN roles r ON r.id = g.role_id
  JOIN groups gr ON gr.id = g.group_id JOIN group_members m ON m.group_id = g.group_id
  WHERE r.name = 'owner';

CREATE VIEW IF NOT EXISTS active_owners AS
  SELECT u.id FROM users u JOIN owner_holders h ON h.user_id = u.id
  WHERE u.status = 'active'
  AND EXISTS (SELECT 1 FROM roles r JOIN role_permissions rp ON rp.role_id = r.id WHERE r.name = 'owner' AND rp.permission = '*');

CREATE TRIGGER IF NOT EXISTS auth_owner_grant_delete AFTER DELETE ON grants
  WHEN OLD.role_id = (SELECT id FROM roles WHERE name = 'owner') AND NOT EXISTS (SELECT 1 FROM active_owners)
  BEGIN SELECT RAISE(ABORT, 'owner_required'); END;

CREATE TRIGGER IF NOT EXISTS auth_owner_grant_update AFTER UPDATE ON grants
  WHEN OLD.role_id = (SELECT id FROM roles WHERE name = 'owner') AND NOT EXISTS (SELECT 1 FROM active_owners)
  BEGIN SELECT RAISE(ABORT, 'owner_required'); END;

CREATE TRIGGER IF NOT EXISTS auth_owner_member_delete AFTER DELETE ON group_members
  WHEN EXISTS (SELECT 1 FROM grants g WHERE g.group_id = OLD.group_id AND g.role_id = (SELECT id FROM roles WHERE name = 'owner')) AND NOT EXISTS (SELECT 1 FROM active_owners)
  BEGIN SELECT RAISE(ABORT, 'owner_required'); END;

CREATE TRIGGER IF NOT EXISTS auth_owner_member_update AFTER UPDATE ON group_members
  WHEN EXISTS (SELECT 1 FROM grants g WHERE g.group_id = OLD.group_id AND g.role_id = (SELECT id FROM roles WHERE name = 'owner')) AND NOT EXISTS (SELECT 1 FROM active_owners)
  BEGIN SELECT RAISE(ABORT, 'owner_required'); END;

CREATE TRIGGER IF NOT EXISTS auth_owner_group_delete AFTER DELETE ON groups
  WHEN EXISTS (SELECT 1 FROM grants g WHERE g.group_id = OLD.id AND g.role_id = (SELECT id FROM roles WHERE name = 'owner')) AND NOT EXISTS (SELECT 1 FROM active_owners)
  BEGIN SELECT RAISE(ABORT, 'owner_required'); END;

CREATE TRIGGER IF NOT EXISTS auth_owner_group_update AFTER UPDATE OF id ON groups
  WHEN EXISTS (SELECT 1 FROM grants g WHERE g.group_id = OLD.id AND g.role_id = (SELECT id FROM roles WHERE name = 'owner')) AND NOT EXISTS (SELECT 1 FROM active_owners)
  BEGIN SELECT RAISE(ABORT, 'owner_required'); END;

CREATE TRIGGER IF NOT EXISTS auth_owner_permission_delete AFTER DELETE ON role_permissions
  WHEN OLD.role_id = (SELECT id FROM roles WHERE name = 'owner') AND OLD.permission = '*' AND NOT EXISTS (SELECT 1 FROM active_owners)
  BEGIN SELECT RAISE(ABORT, 'owner_required'); END;

CREATE TRIGGER IF NOT EXISTS auth_owner_permission_update AFTER UPDATE ON role_permissions
  WHEN OLD.role_id = (SELECT id FROM roles WHERE name = 'owner') AND NOT EXISTS (SELECT 1 FROM active_owners)
  BEGIN SELECT RAISE(ABORT, 'owner_required'); END;

CREATE TRIGGER IF NOT EXISTS auth_owner_role_delete AFTER DELETE ON roles
  WHEN OLD.name = 'owner' AND NOT EXISTS (SELECT 1 FROM active_owners)
  BEGIN SELECT RAISE(ABORT, 'owner_required'); END;

CREATE TRIGGER IF NOT EXISTS auth_owner_role_update AFTER UPDATE OF id, name ON roles
  WHEN OLD.name = 'owner' AND NOT EXISTS (SELECT 1 FROM active_owners)
  BEGIN SELECT RAISE(ABORT, 'owner_required'); END;

CREATE TRIGGER IF NOT EXISTS auth_owner_user_update AFTER UPDATE OF id, status ON users
  WHEN OLD.status = 'active' AND OLD.id IN (SELECT user_id FROM owner_holders) AND NOT EXISTS (SELECT 1 FROM active_owners)
  BEGIN SELECT RAISE(ABORT, 'owner_required'); END;

CREATE TRIGGER IF NOT EXISTS auth_owner_user_delete AFTER DELETE ON users
  WHEN OLD.status = 'active' AND OLD.id IN (SELECT user_id FROM owner_holders) AND NOT EXISTS (SELECT 1 FROM active_owners)
  BEGIN SELECT RAISE(ABORT, 'owner_required'); END;

-- GO-134: invitations and password reset links (new tables, no new
-- version — see _schema.ts). Only the
-- SHA-256 of each one-time token is stored, never the token.
CREATE TABLE IF NOT EXISTS invitations (
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
);
CREATE INDEX IF NOT EXISTS idx_invitations_email ON invitations (email, accepted_at, revoked_at);

CREATE TABLE IF NOT EXISTS password_resets (
  id         TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  created_by TEXT,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  used_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_password_resets_user ON password_resets (user_id, used_at);


-- GO-141: service accounts and their tokens (new tables, no new version).
-- A service account has no users row, so it can never sign in, be invited or
-- reset, or count as an owner. Only the SHA-256 of each token's secret is stored.
CREATE TABLE IF NOT EXISTS service_accounts (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_by  TEXT,
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS service_tokens (
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
);
CREATE INDEX IF NOT EXISTS idx_service_tokens_account ON service_tokens (service_account_id);

-- GO-151: login providers (new tables and triggers, no new version). An
-- identity is the pair
-- (provider, subject); its email is what the provider showed when it was
-- linked, for display only. A flow row lives at most 10 minutes and is deleted
-- by its callback; provider tokens are never stored.
CREATE TABLE IF NOT EXISTS identities (
  id           TEXT PRIMARY KEY,
  provider     TEXT NOT NULL,
  subject      TEXT NOT NULL,
  user_id      TEXT NOT NULL,
  email        TEXT NOT NULL DEFAULT '',
  linked_at    TEXT NOT NULL,
  last_used_at TEXT,
  UNIQUE (provider, subject),
  UNIQUE (user_id, provider)
);

CREATE TABLE IF NOT EXISTS oauth_flows (
  handle_hash  TEXT PRIMARY KEY,                   -- SHA-256 of the random handle in the flow cookie
  provider     TEXT NOT NULL,
  state_hash   TEXT NOT NULL,
  nonce        TEXT NOT NULL,
  verifier     TEXT,                               -- PKCE code verifier (NULL for a provider without PKCE)
  redirect_uri TEXT NOT NULL,
  purpose      TEXT NOT NULL CHECK (purpose IN ('signin', 'invite', 'link')),
  next         TEXT NOT NULL DEFAULT '',
  invite_hash  TEXT,                               -- SHA-256 of the invitation token (purpose invite)
  user_id      TEXT,                               -- purpose link: the signed-in account
  session_id   TEXT,
  created_at   TEXT NOT NULL,
  expires_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_oauth_flows_expiry ON oauth_flows (expires_at);

CREATE TABLE IF NOT EXISTS session_methods (
  session_id TEXT PRIMARY KEY,                     -- sessions.id
  method     TEXT NOT NULL                         -- how it was started: password | google | microsoft | apple | github | facebook | access
);

-- Every account keeps a password or at least one identity (removing the
-- password is a conditional UPDATE in the worker; these guard the identities).
CREATE TRIGGER IF NOT EXISTS auth_method_identity_delete AFTER DELETE ON identities
  WHEN EXISTS (SELECT 1 FROM users u WHERE u.id = OLD.user_id AND u.password_hash IS NULL)
     AND NOT EXISTS (SELECT 1 FROM identities i WHERE i.user_id = OLD.user_id)
  BEGIN SELECT RAISE(ABORT, 'method_required'); END;

CREATE TRIGGER IF NOT EXISTS auth_method_identity_move AFTER UPDATE OF user_id ON identities
  WHEN OLD.user_id <> NEW.user_id AND EXISTS (SELECT 1 FROM users u WHERE u.id = OLD.user_id AND u.password_hash IS NULL)
     AND NOT EXISTS (SELECT 1 FROM identities i WHERE i.user_id = OLD.user_id)
  BEGIN SELECT RAISE(ABORT, 'method_required'); END;

-- GO-151: the audit trail of sign-ins and sign-in methods (read by GO-138).
CREATE TABLE IF NOT EXISTS auth_events (
  id      TEXT PRIMARY KEY,
  at      TEXT NOT NULL,
  kind    TEXT NOT NULL,                           -- signin | signin_refused | method_linked | … (EventKind in _audit.ts); kept AUTH_AUDIT_RETENTION_DAYS (365)
  user_id TEXT,
  method  TEXT,
  detail  TEXT NOT NULL DEFAULT '{}'               -- JSON: reasons and addresses, never a token or a secret
);
CREATE INDEX IF NOT EXISTS idx_auth_events_at ON auth_events (at);
CREATE INDEX IF NOT EXISTS idx_auth_events_user ON auth_events (user_id, at);

-- GO-139: Cloudflare Access group mapping (new tables, no new version). An
-- Access identity is an `identities` row with provider 'access'.
CREATE TABLE IF NOT EXISTS access_group_map (
  id           TEXT PRIMARY KEY,
  access_group TEXT NOT NULL,                      -- the group as the identity provider names it in the Access token
  group_id     TEXT NOT NULL,                      -- groups.id
  created_by   TEXT,
  created_at   TEXT NOT NULL,
  UNIQUE (access_group, group_id)
);

CREATE TABLE IF NOT EXISTS access_synced_members (
  group_id  TEXT NOT NULL,                         -- a membership the sign-in sync ADDED; the only kind it removes
  user_id   TEXT NOT NULL,
  synced_at TEXT NOT NULL,
  PRIMARY KEY (group_id, user_id)
);

CREATE TABLE IF NOT EXISTS access_seen_groups (
  user_id TEXT PRIMARY KEY,
  groups  TEXT NOT NULL,                           -- JSON array: the groups of the last Access token
  seen_at TEXT NOT NULL
);

-- GO-140: the second factor (new tables, no new version; _mfa-schema.ts).
CREATE TABLE IF NOT EXISTS mfa_factors (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL,
  kind          TEXT NOT NULL CHECK (kind IN ('totp', 'passkey')),
  name          TEXT NOT NULL DEFAULT '',
  secret        TEXT,                              -- totp: AES-GCM ciphertext under AUTH_MFA_KEY, never the secret
  last_step     INTEGER NOT NULL DEFAULT 0,        -- totp: last accepted time step; that one and older are refused
  credential_id TEXT UNIQUE,                       -- passkey: base64url credential id
  public_key    TEXT,                              -- passkey: the public key as a JWK
  algorithm     INTEGER,                           -- passkey: COSE algorithm (-7 ES256, -257 RS256, -8 EdDSA)
  sign_count    INTEGER NOT NULL DEFAULT 0,        -- passkey: signature counter
  transports    TEXT NOT NULL DEFAULT '[]',        -- passkey: JSON array the browser reported
  confirmed_at  TEXT,                              -- NULL: a TOTP enrolment waiting for its first code
  created_at    TEXT NOT NULL,
  last_used_at  TEXT
);
CREATE INDEX IF NOT EXISTS idx_mfa_factors_user ON mfa_factors (user_id);

CREATE TABLE IF NOT EXISTS mfa_backup_codes (
  code_hash  TEXT PRIMARY KEY,                     -- SHA-256 of user id and code; never the code
  user_id    TEXT NOT NULL,
  created_at TEXT NOT NULL,
  used_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_mfa_backup_codes_user ON mfa_backup_codes (user_id);

CREATE TABLE IF NOT EXISTS mfa_pending (
  token_hash TEXT PRIMARY KEY,                     -- SHA-256 of the __Host-ssg-auth-mfa cookie
  user_id    TEXT NOT NULL,
  method     TEXT NOT NULL,                        -- the first factor that was passed
  next       TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  used_at    TEXT
);

CREATE TABLE IF NOT EXISTS session_factors (
  session_id  TEXT PRIMARY KEY,
  factor      TEXT NOT NULL,                       -- totp | backup | passkey
  verified_at TEXT NOT NULL                        -- sign-in or the last step-up
);

CREATE TABLE IF NOT EXISTS mfa_challenges (
  challenge  TEXT PRIMARY KEY,                     -- WebAuthn challenge, base64url, single use
  user_id    TEXT NOT NULL,
  purpose    TEXT NOT NULL,                        -- register | signin | step-up
  bound_to   TEXT NOT NULL,                        -- the pending handle's hash or the session id
  expires_at TEXT NOT NULL,
  used_at    TEXT
);

CREATE TABLE IF NOT EXISTS mfa_reset_tokens (
  token_hash TEXT PRIMARY KEY,                     -- SHA-256 of a used AUTH_MFA_RESET_TOKEN value
  user_id    TEXT NOT NULL,
  used_at    TEXT NOT NULL
);

-- GO-138: when this worker first signed with each key id (a JWK has no date),
-- for the key's age on the admin panel's Status view (new table, no version).
CREATE TABLE IF NOT EXISTS signing_keys (
  kid           TEXT PRIMARY KEY,                  -- RFC 7638 thumbprint (public)
  first_used_at TEXT NOT NULL
);

INSERT OR IGNORE INTO schema_meta (component, version, updated_at) VALUES ('auth', 1, '1970-01-01T00:00:00.000Z');
