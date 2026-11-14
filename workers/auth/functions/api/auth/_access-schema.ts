// Tables of GO-139: the Access group mapping and what it synchronised. A
// leading underscore keeps this file out of the Pages route table. _schema.ts
// lists ACCESS_TABLES in TABLES (new tables need no schema version);
// schema.sql carries the same (a test compares). An Access identity itself is
// a row of `identities` with provider "access" (_provider-schema.ts), so the
// last-method invariant covers an account whose only way in is Access.
//
//   access_group_map       an Access group (as the identity provider names it
//                          in the token) → a group here. Edited by an
//                          administrator (/api/auth/admin/access-groups).
//   access_synced_members  the memberships the sign-in synchronisation ADDED —
//                          the only ones it may ever remove. A membership an
//                          administrator added by hand has no row here and is
//                          never touched by the synchronisation.
//   access_seen_groups     the groups the person's last Access token carried,
//                          so a change of the mapping can be applied to every
//                          person at once (it then shows at their next token
//                          refresh) instead of waiting for their next sign-in.

export const ACCESS_TABLES = [
  `CREATE TABLE IF NOT EXISTS access_group_map (
     id           TEXT PRIMARY KEY,
     access_group TEXT NOT NULL,
     group_id     TEXT NOT NULL,
     created_by   TEXT,
     created_at   TEXT NOT NULL,
     UNIQUE (access_group, group_id)
   )`,
  `CREATE TABLE IF NOT EXISTS access_synced_members (
     group_id  TEXT NOT NULL,
     user_id   TEXT NOT NULL,
     synced_at TEXT NOT NULL,
     PRIMARY KEY (group_id, user_id)
   )`,
  `CREATE TABLE IF NOT EXISTS access_seen_groups (
     user_id TEXT PRIMARY KEY,
     groups  TEXT NOT NULL,
     seen_at TEXT NOT NULL
   )`,
];
