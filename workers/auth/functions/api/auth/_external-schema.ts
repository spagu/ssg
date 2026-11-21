// Tables of #435, the external JWT issuer. A leading underscore keeps this
// file out of the Pages route table. _schema.ts lists EXTERNAL_TABLES in
// TABLES (new tables need no schema version: CREATE … IF NOT EXISTS reaches
// an existing database on its next ensureSchema); schema.sql carries the
// same (a test compares). An external identity itself is a row of
// `identities` with provider "jwt:<issuer>" (_provider-schema.ts).
//
//   jwt_used_tokens     each accepted token's id — a SHA-256 of issuer and
//                       `jti`, never the value — until the token could no
//                       longer pass anyway: single use (RFC 7519 §4.1.7).
//                       Expired rows go with every new one.
//   jwt_synced_members  the group memberships the roles mapping ADDED — the
//   jwt_synced_grants   … and role grants: the only ones it may ever remove
//                       (_synced.ts). A grant or membership an administrator
//                       made by hand has no row here and is never touched.

import { iso, sha256hex } from "./_lib";
import type { Env } from "./_lib";

export const EXTERNAL_TABLES = [
  `CREATE TABLE IF NOT EXISTS jwt_used_tokens (
     jti_hash   TEXT PRIMARY KEY,
     expires_at TEXT NOT NULL
   )`,
  `CREATE INDEX IF NOT EXISTS idx_jwt_used_tokens_expiry ON jwt_used_tokens (expires_at)`,
  `CREATE TABLE IF NOT EXISTS jwt_synced_members (
     group_id  TEXT NOT NULL,
     user_id   TEXT NOT NULL,
     synced_at TEXT NOT NULL,
     PRIMARY KEY (group_id, user_id)
   )`,
  `CREATE TABLE IF NOT EXISTS jwt_synced_grants (
     role_id   TEXT NOT NULL,
     user_id   TEXT NOT NULL,
     synced_at TEXT NOT NULL,
     PRIMARY KEY (role_id, user_id)
   )`,
];

// spendToken records a token id until `untilMs`; false when it was already
// spent (a replay). One transaction: the purge of expired ids and the insert.
export async function spendToken(env: Env, issuer: string, jti: string, untilMs: number, now: number): Promise<boolean> {
  const db = env.AUTH_DB;
  const hash = await sha256hex(`${issuer}\0${jti}`);
  const [, ins] = await db.batch([
    db.prepare("DELETE FROM jwt_used_tokens WHERE expires_at <= ?").bind(iso(now)),
    db.prepare("INSERT OR IGNORE INTO jwt_used_tokens (jti_hash, expires_at) VALUES (?, ?)").bind(hash, iso(untilMs)),
  ]);
  return ins.meta.changes === 1;
}
