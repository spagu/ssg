// Accepting an invitation: the account, its roles and groups, in one batch.
// A leading underscore keeps this file out of the Pages route table.
//
// The batch is one transaction, so of two acceptances of the same token
// exactly one creates an account: the first statement marks the invitation
// accepted with a fresh user id, and every later statement inserts only for
// the invitation carrying that id — the loser's statements match nothing. An
// address that has an account meanwhile makes the users INSERT fail, and the
// whole batch (the "accepted" mark included) rolls back.
//
// Roles and groups are the ones recorded at invite time, re-checked here: a
// role or group deleted meanwhile is joined away (never a dangling grant) and
// reported as skipped.

import { iso, sha256hex } from "./_lib";
import type { Env } from "./_lib";
import { isUnique } from "./_rbac-schema";
import type { Invitation } from "./_invites";

export type Accepted =
  | { kind: "accepted"; userId: string; skipped: { roles: string[]; groups: string[] } }
  | { kind: "invalid" }
  | { kind: "account_exists" };

async function existing(env: Env, table: "roles" | "groups", ids: string[]): Promise<Set<string>> {
  const { results } = await env.AUTH_DB.prepare(`SELECT id FROM ${table} WHERE id IN (SELECT value FROM json_each(?))`)
    .bind(JSON.stringify(ids))
    .all<{ id: string }>();
  return new Set(results.map((r) => r.id));
}

// acceptInvitation creates the account behind `token`. passwordHash is null
// for an account without a password (a login provider, GO-151).
export async function acceptInvitation(
  env: Env,
  inv: Invitation,
  token: string,
  account: { name: string; passwordHash: string | null },
  now: number,
): Promise<Accepted> {
  return acceptByHash(env, inv, await sha256hex(token), account, now);
}

// acceptByHash is acceptInvitation for a caller that holds only the token's
// SHA-256 (a login-provider flow keeps nothing else, GO-151). `extra` adds
// statements to the same transaction for the new account's id — the linked
// identity of a provider account — which must insert only for the invitation
// carrying that id, like the grants below.
export async function acceptByHash(
  env: Env,
  inv: Invitation,
  tokenHash: string,
  account: { name: string; passwordHash: string | null },
  now: number,
  extra: (userId: string) => D1PreparedStatement[] = () => [],
): Promise<Accepted> {
  const db = env.AUTH_DB;
  const userId = crypto.randomUUID();
  const at = iso(now);
  const roles = await existing(env, "roles", inv.roleIds);
  const groups = await existing(env, "groups", inv.groupIds);
  const mine = "SELECT * FROM invitations WHERE accepted_user_id = ?";
  let results: D1Result[];
  try {
    results = await db.batch([
      db
        .prepare(
          "UPDATE invitations SET accepted_at = ?, accepted_user_id = ? WHERE token_hash = ? AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?",
        )
        .bind(at, userId, tokenHash, at),
      db
        .prepare(
          `INSERT INTO users (id, email, display_name, password_hash, status, must_change_password, created_at, updated_at)
           SELECT ?, i.email, ?, ?, 'active', 0, ?, ? FROM (${mine}) i`,
        )
        .bind(userId, account.name, account.passwordHash, at, at, userId),
      db
        .prepare(
          `INSERT OR IGNORE INTO grants (id, role_id, user_id, group_id, granted_by, granted_at)
           SELECT lower(hex(randomblob(16))), r.id, ?, NULL, i.invited_by, ?
           FROM (${mine}) i, json_each(i.role_ids) j JOIN roles r ON r.id = j.value`,
        )
        .bind(userId, at, userId),
      db
        .prepare(
          `INSERT OR IGNORE INTO group_members (group_id, user_id, added_by, added_at)
           SELECT g.id, ?, i.invited_by, ? FROM (${mine}) i, json_each(i.group_ids) j JOIN groups g ON g.id = j.value`,
        )
        .bind(userId, at, userId),
      ...extra(userId),
    ]);
  } catch (e) {
    if (isUnique(e)) return { kind: "account_exists" };
    throw e;
  }
  if (results[1].meta.changes !== 1) return { kind: "invalid" };
  return {
    kind: "accepted",
    userId,
    skipped: { roles: inv.roleIds.filter((id) => !roles.has(id)), groups: inv.groupIds.filter((id) => !groups.has(id)) },
  };
}
