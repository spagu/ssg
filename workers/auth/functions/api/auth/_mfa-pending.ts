// The step between a first factor and a session (GO-140). A leading
// underscore keeps this file out of the Pages route table.
//
// After a correct password (or a provider's answer) for an account with a
// second factor, nothing is signed in yet: the browser gets a random handle in
// `__Host-ssg-auth-mfa` (Secure, HttpOnly, Path=/, no Domain; SameSite=Lax so
// it survives the redirect chain back from a login provider) and D1 keeps its
// SHA-256 with the account, the first factor passed and where to go after.
// The handle lives PENDING_SECONDS, works once (consumePending marks it used
// in the same statement that checks it is unused), and is bound to its
// account on the server — what the browser sends is never asked "which
// account": a code is always checked against the handle's own account.

import { readCookie } from "./_cookies";
import { iso, randomToken, sha256hex } from "./_lib";
import type { Env } from "./_lib";
import { findUserById } from "./_users";
import type { User } from "./_users";

export const PENDING_COOKIE = "__Host-ssg-auth-mfa";
export const PENDING_SECONDS = 300;

const cookie = (value: string, maxAge: number): string => `${PENDING_COOKIE}=${value}; Max-Age=${maxAge}; Path=/; Secure; HttpOnly; SameSite=Lax`;
export const clearPendingCookie = (): string => cookie("", 0);

export interface Pending {
  hash: string;
  user: User;
  method: string; // the first factor passed
  next: string;
  expiresAt: string;
}

// createPending stores a new handle for this account and returns its cookie.
// Old rows (used, or expired more than a day ago) are swept on the way.
export async function createPending(env: Env, userId: string, method: string, next: string, now: number): Promise<{ cookie: string; expiresAt: string }> {
  const token = randomToken();
  const expiresAt = iso(now + PENDING_SECONDS * 1000);
  await env.AUTH_DB.batch([
    env.AUTH_DB.prepare("DELETE FROM mfa_pending WHERE expires_at <= ? OR used_at <= ?").bind(iso(now - 86_400_000), iso(now - 86_400_000)),
    env.AUTH_DB.prepare("INSERT INTO mfa_pending (token_hash, user_id, method, next, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)").bind(
      await sha256hex(token),
      userId,
      method,
      next,
      iso(now),
      expiresAt,
    ),
  ]);
  return { cookie: cookie(token, PENDING_SECONDS), expiresAt };
}

// readPending: the live, unused handle the request carries, with its active
// account — or null.
export async function readPending(env: Env, request: Request, now: number): Promise<Pending | null> {
  const token = readCookie(request, PENDING_COOKIE);
  if (!token) return null;
  const hash = await sha256hex(token);
  const row = await env.AUTH_DB.prepare("SELECT user_id, method, next, expires_at FROM mfa_pending WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?")
    .bind(hash, iso(now))
    .first<{ user_id: string; method: string; next: string; expires_at: string }>();
  if (!row) return null;
  const user = await findUserById(env, row.user_id);
  if (!user || user.status !== "active") return null;
  return { hash, user, method: row.method, next: row.next, expiresAt: row.expires_at };
}

// consumePending marks the handle used; true for exactly one caller.
export async function consumePending(env: Env, hash: string, now: number): Promise<boolean> {
  const r = await env.AUTH_DB.prepare("UPDATE mfa_pending SET used_at = ? WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?").bind(iso(now), hash, iso(now)).run();
  return r.meta.changes === 1;
}
