// Linked provider identities and an account's sign-in methods (GO-151): the
// data layer of the account page's "How you sign in" and of the callbacks. A
// leading underscore keeps this file out of the Pages route table.
//
// An identity is found ONLY by (provider, subject) — never by address. The
// last-method invariant is enforced in the database (_provider-schema.ts):
// removing the last identity of a password-less account aborts with
// method_required, and removePassword is one conditional statement.

import { iso } from "./_lib";
import type { Env } from "./_lib";
import type { Identity } from "./_provider";
import { isMethodViolation } from "./_provider-schema";
import { isUnique } from "./_rbac-schema";

export interface LinkedIdentity {
  id: string;
  provider: string;
  subject: string;
  user_id: string;
  email: string;
  linked_at: string;
  last_used_at: string | null;
}

export function findIdentity(env: Env, provider: string, subject: string): Promise<LinkedIdentity | null> {
  return env.AUTH_DB.prepare("SELECT * FROM identities WHERE provider = ? AND subject = ?").bind(provider, subject).first<LinkedIdentity>();
}

export async function listIdentities(env: Env, userId: string): Promise<LinkedIdentity[]> {
  const { results } = await env.AUTH_DB.prepare("SELECT * FROM identities WHERE user_id = ? ORDER BY linked_at, provider").bind(userId).all<LinkedIdentity>();
  return results;
}

// identityInsert links `id` to the account `userId` — on its own, or (with
// `onlyIfAcceptedBy`) inside the invitation batch, where it inserts only once
// the invitation carries the new account's id (_invites-accept.ts).
export function identityInsert(env: Env, userId: string, provider: string, id: Identity, now: number, onlyIfAcceptedBy = false): D1PreparedStatement {
  const guard = onlyIfAcceptedBy ? " WHERE EXISTS (SELECT 1 FROM invitations WHERE accepted_user_id = ?)" : "";
  const stmt = env.AUTH_DB.prepare(
    `INSERT INTO identities (id, provider, subject, user_id, email, linked_at, last_used_at) SELECT ?, ?, ?, ?, ?, ?, ?${guard}`,
  );
  const args = [crypto.randomUUID(), provider, id.subject, userId, id.email, iso(now), iso(now)];
  return onlyIfAcceptedBy ? stmt.bind(...args, userId) : stmt.bind(...args);
}

export type LinkResult = "ok" | "in_use" | "provider_linked";

// linkIdentity adds an identity to a signed-in account. "in_use": that
// provider account opens another account here; "provider_linked": this
// account already has an identity at that provider.
export async function linkIdentity(env: Env, userId: string, provider: string, id: Identity, now: number): Promise<LinkResult> {
  const existing = await findIdentity(env, provider, id.subject);
  if (existing) return existing.user_id === userId ? "ok" : "in_use";
  try {
    await identityInsert(env, userId, provider, id, now).run();
    return "ok";
  } catch (e) {
    if (!isUnique(e)) throw e;
    // A concurrent link of the same pair, or this account's own other one.
    const again = await findIdentity(env, provider, id.subject);
    return again && again.user_id !== userId ? "in_use" : "provider_linked";
  }
}

export async function touchIdentity(env: Env, id: string, now: number): Promise<void> {
  await env.AUTH_DB.prepare("UPDATE identities SET last_used_at = ? WHERE id = ?").bind(iso(now), id).run();
}

export type Removed = "ok" | "not_found" | "last_method";

// removeIdentity unlinks one of the account's identities.
export async function removeIdentity(env: Env, userId: string, identityId: string): Promise<Removed> {
  try {
    const res = await env.AUTH_DB.prepare("DELETE FROM identities WHERE id = ? AND user_id = ?").bind(identityId, userId).run();
    return res.meta.changes === 1 ? "ok" : "not_found";
  } catch (e) {
    if (isMethodViolation(e)) return "last_method";
    throw e;
  }
}

// removePassword clears the account's password — in ONE statement, and only
// while an identity remains, so it can never leave the account without a way
// in, however many removals run at once.
export async function removePassword(env: Env, userId: string, now: number): Promise<Removed> {
  const res = await env.AUTH_DB.prepare(
    `UPDATE users SET password_hash = NULL, must_change_password = 0, updated_at = ?
     WHERE id = ? AND password_hash IS NOT NULL AND EXISTS (SELECT 1 FROM identities WHERE user_id = ?)`,
  )
    .bind(iso(now), userId, userId)
    .run();
  if (res.meta.changes === 1) return "ok";
  const row = await env.AUTH_DB.prepare("SELECT password_hash FROM users WHERE id = ?").bind(userId).first<{ password_hash: string | null }>();
  return row?.password_hash ? "last_method" : "not_found";
}

// setFirstPassword gives a password to an account that has none; an account
// that has one changes it through POST /api/auth/password (current password).
export async function setFirstPassword(env: Env, userId: string, hash: string, now: number): Promise<boolean> {
  const res = await env.AUTH_DB.prepare("UPDATE users SET password_hash = ?, must_change_password = 0, updated_at = ? WHERE id = ? AND password_hash IS NULL")
    .bind(hash, iso(now), userId)
    .run();
  return res.meta.changes === 1;
}
