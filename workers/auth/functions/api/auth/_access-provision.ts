// Accounts made at a first Access sign-in (GO-139, AUTH_ACCESS_AUTO_CREATE=on,
// off by default). A leading underscore keeps this file out of the Pages route
// table.
//
// Auto-provisioning hands an account to whoever the Access policy lets
// through, so it must never mint an administrator: the default role is
// checked against the database before every Access sign-in, and the method
// refuses to start (503) while the role is the owner role, holds `*`, or holds
// any permission isSensitive (_service-perms.ts) flags — `auth:…`, an erase,
// a worker wildcard. The account gets that ONE role, no group, no password;
// its Access identity is linked in the same transaction, so there is never an
// account without a way in.

import type { AccessSetup } from "./_access-config";
import { ACCESS_METHOD } from "./_access-config";
import { iso } from "./_lib";
import type { Env } from "./_lib";
import type { Identity } from "./_provider";
import { identityInsert } from "./_provider-identities";
import { OWNER_ROLE_NAME, isUnique } from "./_rbac-schema";
import { isSensitive } from "./_service-perms";

// defaultRole: the role new accounts get, or the configuration problem.
export async function defaultRole(env: Env, s: AccessSetup): Promise<{ roleId: string } | { problem: string }> {
  const name = s.defaultRole;
  const bad = (why: string) => ({ problem: `AUTH_ACCESS_DEFAULT_ROLE "${name}" ${why}; auto-created accounts must never be administrators` });
  if (name === OWNER_ROLE_NAME) return bad("is the owner role");
  const role = await env.AUTH_DB.prepare("SELECT id FROM roles WHERE name = ?").bind(name).first<{ id: string }>();
  if (!role) return { problem: `AUTH_ACCESS_DEFAULT_ROLE "${name}" is not a role here: create it first, or correct the name` };
  const { results } = await env.AUTH_DB.prepare("SELECT permission FROM role_permissions WHERE role_id = ? ORDER BY permission").bind(role.id).all<{ permission: string }>();
  const sensitive = results.map((r) => r.permission).filter((p) => p.includes("*") || isSensitive(p));
  if (sensitive.length) return bad(`holds ${sensitive.join(", ")}`);
  return { roleId: role.id };
}

// createAccessAccount makes the account, its Access identity and its one
// grant in one batch; null when the address or the identity was taken
// meanwhile (a concurrent sign-in or invitation).
export async function createAccessAccount(env: Env, id: Identity, roleId: string, now: number): Promise<string | null> {
  const db = env.AUTH_DB;
  const userId = crypto.randomUUID();
  const at = iso(now);
  try {
    await db.batch([
      db
        .prepare(
          `INSERT INTO users (id, email, display_name, password_hash, status, must_change_password, created_at, updated_at)
           VALUES (?, ?, '', NULL, 'active', 0, ?, ?)`,
        )
        .bind(userId, id.email, at, at),
      identityInsert(env, userId, ACCESS_METHOD, id, now),
      db
        .prepare("INSERT INTO grants (id, role_id, user_id, group_id, granted_by, granted_at) SELECT ?, id, ?, NULL, NULL, ? FROM roles WHERE id = ?")
        .bind(crypto.randomUUID(), userId, at, roleId),
    ]);
  } catch (e) {
    if (isUnique(e)) return null;
    throw e;
  }
  return userId;
}
