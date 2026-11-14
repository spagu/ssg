// Schema version 2 (GO-133): roles, groups, grants — and the owner invariant,
// enforced by the database itself. A leading underscore keeps this file out of
// the Pages route table. _schema.ts appends RBAC_MIGRATION to MIGRATIONS;
// schema.sql carries the same tables, indexes and triggers (a test compares).
//
// The model is flat on purpose, so the result can be worked out by hand:
//   roles             a name and a set of permissions (role_permissions)
//   groups            a set of users (group_members); groups never nest
//   grants            one role → one user OR one group, with who and when
// A user's permissions are the union over the roles granted to the user and
// to the user's groups. Roles never inherit from roles.
//
// The owner invariant: there is always at least one ACTIVE user holding the
// `owner` role — directly or through a group — and that role holds `*`. The
// role is found by its name, which is why built-in names cannot change. The
// triggers below refuse (RAISE ABORT 'owner_required') any delete or update
// that ends it: revoking the grant, removing the member, deleting the group,
// deleting the role or its `*`, renaming it, blocking or deleting the user.
// A trigger runs inside the statement that fired it, and D1 runs one statement
// (or one batch) at a time, so of two concurrent removals of the last two
// owners the second sees the first and fails; a failing batch rolls back
// whole. Each trigger fires only when the changed row mattered to the
// invariant, so unrelated changes are never blocked by an already-broken
// state (a database without any account yet).

export const OWNER_ROLE_NAME = "owner";
export const OWNER_REQUIRED = "owner_required";

// isOwnerViolation: the error D1 (or SQLite) throws when a trigger refused.
export const isOwnerViolation = (e: unknown): boolean => String(e).includes(OWNER_REQUIRED);
// isUnique: a UNIQUE constraint refused the row (a name already taken).
export const isUnique = (e: unknown): boolean => /UNIQUE constraint failed/i.test(String(e));

const NOW = "strftime('%Y-%m-%dT%H:%M:%fZ', 'now')";
const NEW_ID = "lower(hex(randomblob(16)))";
const OWNER = "(SELECT id FROM roles WHERE name = 'owner')";

// Two views state the invariant once; every trigger reads them.
//   owner_holders  every user the owner role reaches, directly or through a
//                  group (whatever the account's status)
//   active_owners  those of them who are active — while the role holds `*`
export const OWNER_VIEWS = [
  `CREATE VIEW IF NOT EXISTS owner_holders AS
     SELECT g.user_id AS user_id FROM grants g JOIN roles r ON r.id = g.role_id
     WHERE r.name = 'owner' AND g.user_id IS NOT NULL
     UNION
     SELECT m.user_id FROM grants g JOIN roles r ON r.id = g.role_id
       JOIN groups gr ON gr.id = g.group_id JOIN group_members m ON m.group_id = g.group_id
     WHERE r.name = 'owner'`,
  `CREATE VIEW IF NOT EXISTS active_owners AS
     SELECT u.id FROM users u JOIN owner_holders h ON h.user_id = u.id
     WHERE u.status = 'active'
       AND EXISTS (SELECT 1 FROM roles r JOIN role_permissions rp ON rp.role_id = r.id WHERE r.name = 'owner' AND rp.permission = '*')`,
];

// guard: after `event`, when the changed row was part of an owner path and
// no active owner is left, the whole statement (and its batch) is undone.
const guard = (name: string, event: string, when: string) =>
  `CREATE TRIGGER IF NOT EXISTS ${name} ${event}
     WHEN ${when} AND NOT EXISTS (SELECT 1 FROM active_owners)
   BEGIN SELECT RAISE(ABORT, 'owner_required'); END`;
const groupHoldsOwner = (group: string) => `EXISTS (SELECT 1 FROM grants g WHERE g.group_id = ${group} AND g.role_id = ${OWNER})`;

export const RBAC_TABLES = [
  `CREATE TABLE IF NOT EXISTS roles (
     id          TEXT PRIMARY KEY,
     name        TEXT NOT NULL UNIQUE,
     description TEXT NOT NULL DEFAULT '',
     built_in    INTEGER NOT NULL DEFAULT 0,
     created_at  TEXT NOT NULL,
     updated_at  TEXT NOT NULL
   )`,
  `CREATE TABLE IF NOT EXISTS role_permissions (
     role_id    TEXT NOT NULL,
     permission TEXT NOT NULL,
     PRIMARY KEY (role_id, permission)
   )`,
  `CREATE TABLE IF NOT EXISTS groups (
     id          TEXT PRIMARY KEY,
     name        TEXT NOT NULL UNIQUE,
     description TEXT NOT NULL DEFAULT '',
     created_at  TEXT NOT NULL,
     updated_at  TEXT NOT NULL
   )`,
  `CREATE TABLE IF NOT EXISTS group_members (
     group_id TEXT NOT NULL,
     user_id  TEXT NOT NULL,
     added_by TEXT,
     added_at TEXT NOT NULL,
     PRIMARY KEY (group_id, user_id)
   )`,
  `CREATE INDEX IF NOT EXISTS idx_group_members_user ON group_members (user_id)`,
  `CREATE TABLE IF NOT EXISTS grants (
     id         TEXT PRIMARY KEY,
     role_id    TEXT NOT NULL,
     user_id    TEXT,
     group_id   TEXT,
     granted_by TEXT,
     granted_at TEXT NOT NULL,
     CHECK ((user_id IS NULL) <> (group_id IS NULL))
   )`,
  `CREATE UNIQUE INDEX IF NOT EXISTS idx_grants_user ON grants (user_id, role_id) WHERE user_id IS NOT NULL`,
  `CREATE UNIQUE INDEX IF NOT EXISTS idx_grants_group ON grants (group_id, role_id) WHERE group_id IS NOT NULL`,
  `CREATE INDEX IF NOT EXISTS idx_grants_role ON grants (role_id)`,
];

export const OWNER_TRIGGERS = [
  guard("auth_owner_grant_delete", "AFTER DELETE ON grants", `OLD.role_id = ${OWNER}`),
  guard("auth_owner_grant_update", "AFTER UPDATE ON grants", `OLD.role_id = ${OWNER}`),
  guard("auth_owner_member_delete", "AFTER DELETE ON group_members", groupHoldsOwner("OLD.group_id")),
  guard("auth_owner_member_update", "AFTER UPDATE ON group_members", groupHoldsOwner("OLD.group_id")),
  guard("auth_owner_group_delete", "AFTER DELETE ON groups", groupHoldsOwner("OLD.id")),
  guard("auth_owner_group_update", "AFTER UPDATE OF id ON groups", groupHoldsOwner("OLD.id")),
  guard("auth_owner_permission_delete", "AFTER DELETE ON role_permissions", `OLD.role_id = ${OWNER} AND OLD.permission = '*'`),
  guard("auth_owner_permission_update", "AFTER UPDATE ON role_permissions", `OLD.role_id = ${OWNER}`),
  guard("auth_owner_role_delete", "AFTER DELETE ON roles", "OLD.name = 'owner'"),
  guard("auth_owner_role_update", "AFTER UPDATE OF id, name ON roles", "OLD.name = 'owner'"),
  guard("auth_owner_user_update", "AFTER UPDATE OF id, status ON users", "OLD.status = 'active' AND OLD.id IN (SELECT user_id FROM owner_holders)"),
  guard("auth_owner_user_delete", "AFTER DELETE ON users", "OLD.status = 'active' AND OLD.id IN (SELECT user_id FROM owner_holders)"),
];

export interface BuiltInRole {
  name: string;
  description: string;
  permissions: string[];
}

// BUILT_IN_ROLES are created once, with the schema, and are ordinary roles
// afterwards: their permissions and descriptions can be edited; their names
// cannot (documentation and the owner invariant refer to them). The names
// come from the manifests of newsletter-ui and comments — a test checks.
export const BUILT_IN_ROLES: BuiltInRole[] = [
  { name: OWNER_ROLE_NAME, description: "Everything, in every worker. There is always at least one active owner.", permissions: ["*"] },
  {
    name: "newsletter-editor",
    description: "Writes and sends campaigns: view, edit drafts, send. Cannot import, export, erase or change suppressions.",
    permissions: ["newsletter:edit", "newsletter:read", "newsletter:send"],
  },
  { name: "newsletter-viewer", description: "Views subscribers, campaigns and statistics; changes nothing.", permissions: ["newsletter:read"] },
  {
    name: "comments-moderator",
    description: "Views the moderation queue, approves comments and marks spam. Cannot delete or import.",
    permissions: ["comments:moderate", "comments:read"],
  },
];

const quote = (s: string) => `'${s.replaceAll("'", "''")}'`;

const SEEDS = BUILT_IN_ROLES.flatMap((r) => [
  `INSERT OR IGNORE INTO roles (id, name, description, built_in, created_at, updated_at)
   VALUES (${NEW_ID}, ${quote(r.name)}, ${quote(r.description)}, 1, ${NOW}, ${NOW})`,
  ...r.permissions.map((p) => `INSERT OR IGNORE INTO role_permissions (role_id, permission) SELECT id, ${quote(p)} FROM roles WHERE name = ${quote(r.name)}`),
]);

// A database that had accounts before version 2 (GO-132 alone) makes its
// oldest active account the owner — the one the bootstrap created — so the
// upgrade never leaves a site without anyone allowed to administer it.
const FIRST_OWNER = `INSERT INTO grants (id, role_id, user_id, group_id, granted_by, granted_at)
   SELECT ${NEW_ID}, r.id, u.id, NULL, NULL, ${NOW}
   FROM roles r, (SELECT id FROM users WHERE status = 'active' ORDER BY created_at, id LIMIT 1) u
   WHERE r.name = '${OWNER_ROLE_NAME}' AND NOT EXISTS (SELECT 1 FROM grants WHERE role_id = r.id)`;

export const RBAC_MIGRATION = { version: 2, statements: [...RBAC_TABLES, ...OWNER_VIEWS, ...OWNER_TRIGGERS, ...SEEDS, FIRST_OWNER] };
