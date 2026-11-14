// The users table. A leading underscore keeps this file out of the route table.
//
// GO-132 creates users only through the bootstrap (the first administrator);
// invitations (GO-134) and login providers (GO-151) add the other ways in and
// call createUser with a null password hash where there is no password.
// setUserStatus and deleteUser are the data layer of the admin panel (GO-138);
// both respect the owner invariant (GO-133).

import { iso } from "./_lib";
import type { Env } from "./_lib";
import { isOwnerViolation } from "./_rbac-schema";

export interface User {
  id: string;
  email: string;
  display_name: string;
  password_hash: string | null;
  status: "active" | "blocked";
  must_change_password: number;
  created_at: string;
  updated_at: string;
  last_login_at: string | null;
}

// PublicUser is what an answer may carry: no hash, no internal flags.
export interface PublicUser {
  id: string;
  email: string;
  name: string;
  mustChangePassword: boolean;
}

export const publicUser = (u: User): PublicUser => ({
  id: u.id,
  email: u.email,
  name: u.display_name,
  mustChangePassword: u.must_change_password === 1,
});

export function findUserByEmail(env: Env, email: string): Promise<User | null> {
  return env.AUTH_DB.prepare("SELECT * FROM users WHERE email = ?").bind(email).first<User>();
}

export function findUserById(env: Env, id: string): Promise<User | null> {
  return env.AUTH_DB.prepare("SELECT * FROM users WHERE id = ?").bind(id).first<User>();
}

export interface NewUser {
  email: string; // already normalised (normEmail)
  name: string;
  passwordHash: string | null;
}

// createUser inserts one account. With `onlyIfEmpty` the INSERT happens only
// while the table is empty, in the same statement — the bootstrap's guarantee
// that two concurrent calls cannot both create a first administrator. Returns
// the new id, or null when nothing was inserted.
export async function createUser(env: Env, u: NewUser, now: number, onlyIfEmpty = false): Promise<string | null> {
  const id = crypto.randomUUID();
  const at = iso(now);
  const res = await env.AUTH_DB.prepare(
    `INSERT INTO users (id, email, display_name, password_hash, status, must_change_password, created_at, updated_at)
     SELECT ?, ?, ?, ?, 'active', 0, ?, ? WHERE ? = 0 OR NOT EXISTS (SELECT 1 FROM users)`,
  )
    .bind(id, u.email, u.name, u.passwordHash, at, at, onlyIfEmpty ? 1 : 0)
    .run();
  return res.meta.changes === 1 ? id : null;
}

export async function anyUser(env: Env): Promise<boolean> {
  return (await env.AUTH_DB.prepare("SELECT 1 AS one FROM users LIMIT 1").first()) !== null;
}

export async function setPasswordHash(env: Env, id: string, hash: string, now: number): Promise<void> {
  await env.AUTH_DB.prepare("UPDATE users SET password_hash = ?, must_change_password = 0, updated_at = ? WHERE id = ?")
    .bind(hash, iso(now), id)
    .run();
}

// upgradeHash replaces a hash only if it is still the one that was verified, so
// a password changed meanwhile is never overwritten with the old password.
export async function upgradeHash(env: Env, id: string, from: string, to: string, now: number): Promise<void> {
  await env.AUTH_DB.prepare("UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ? AND password_hash = ?")
    .bind(to, iso(now), id, from)
    .run();
}

export async function touchLogin(env: Env, id: string, now: number): Promise<void> {
  await env.AUTH_DB.prepare("UPDATE users SET last_login_at = ? WHERE id = ?").bind(iso(now), id).run();
}

// setUserStatus blocks or unblocks an account. Blocking the last active owner
// is refused by the database (_rbac-schema.ts): "owner_required". Ending the
// account's sessions is the caller's choice (revokeUserSessions).
export async function setUserStatus(env: Env, id: string, status: User["status"], now: number): Promise<"ok" | "not_found" | "owner_required"> {
  try {
    const res = await env.AUTH_DB.prepare("UPDATE users SET status = ?, updated_at = ? WHERE id = ?").bind(status, iso(now), id).run();
    return res.meta.changes === 1 ? "ok" : "not_found";
  } catch (e) {
    if (isOwnerViolation(e)) return "owner_required";
    throw e;
  }
}

// deleteUser removes an account with its grants, memberships and sessions, in
// one batch. When that would leave no active owner the first statement that
// removes an owner path is refused and nothing at all is deleted.
export async function deleteUser(env: Env, id: string): Promise<"ok" | "not_found" | "owner_required"> {
  const db = env.AUTH_DB;
  try {
    const results = await db.batch([
      db.prepare("DELETE FROM grants WHERE user_id = ?").bind(id),
      db.prepare("DELETE FROM group_members WHERE user_id = ?").bind(id),
      db.prepare("DELETE FROM refresh_history WHERE session_id IN (SELECT id FROM sessions WHERE user_id = ?)").bind(id),
      db.prepare("DELETE FROM sessions WHERE user_id = ?").bind(id),
      db.prepare("DELETE FROM users WHERE id = ?").bind(id),
      // GO-151: after the account (the last-method trigger looks for it), its
      // provider identities — so the same provider account can be linked again.
      db.prepare("DELETE FROM identities WHERE user_id = ?").bind(id),
    ]);
    return results[4].meta.changes === 1 ? "ok" : "not_found";
  } catch (e) {
    if (isOwnerViolation(e)) return "owner_required";
    throw e;
  }
}
