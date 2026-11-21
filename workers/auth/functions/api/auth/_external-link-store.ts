// The link of an external identity waiting for its confirmation (#435). A
// leading underscore keeps this file out of the Pages route table.
//
// The issuer's page posts a token to /api/auth/jwt/link, often from another
// site: that POST carries no session cookie (SameSite=Lax), so it cannot know
// the account. It verifies the token, spends it, and keeps the identity here;
// the browser gets a random handle in `__Host-ssg-auth-jwt-link` (Secure,
// HttpOnly, Path=/, SameSite=Lax so it survives the 303 to /auth/jwt/link).
// D1 keeps only its SHA-256. The handle lives LINK_SECONDS and works once:
// takeLinkPending deletes the row in the statement that reads it. Which
// account it joins is decided at the confirmation, by the session there.

import { readCookie } from "./_cookies";
import { iso, randomToken, sha256hex } from "./_lib";
import type { Env } from "./_lib";
import type { Identity } from "./_provider";

export const LINK_COOKIE = "__Host-ssg-auth-jwt-link";
export const LINK_SECONDS = 300;

const cookie = (value: string, maxAge: number): string => `${LINK_COOKIE}=${value}; Max-Age=${maxAge}; Path=/; Secure; HttpOnly; SameSite=Lax`;
export const clearLinkCookie = (): string => cookie("", 0);

type Row = { subject: string; email: string; email_verified: number };
const identityOf = (r: Row): Identity => ({ subject: r.subject, email: r.email, emailVerified: r.email_verified === 1, name: "" });

// createLinkPending keeps the identity and returns the handle's cookie;
// expired rows go on the way.
export async function createLinkPending(env: Env, id: Identity, now: number): Promise<string> {
  const handle = randomToken();
  await env.AUTH_DB.batch([
    env.AUTH_DB.prepare("DELETE FROM jwt_link_pending WHERE expires_at <= ?").bind(iso(now)),
    env.AUTH_DB.prepare("INSERT INTO jwt_link_pending (handle_hash, subject, email, email_verified, expires_at) VALUES (?, ?, ?, ?, ?)").bind(
      await sha256hex(handle),
      id.subject,
      id.email,
      id.emailVerified ? 1 : 0,
      iso(now + LINK_SECONDS * 1000),
    ),
  ]);
  return cookie(handle, LINK_SECONDS);
}

async function handleHash(request: Request): Promise<string | null> {
  const handle = readCookie(request, LINK_COOKIE);
  return handle ? sha256hex(handle) : null;
}

// readLinkPending: the live identity the request's handle stands for, or null.
export async function readLinkPending(env: Env, request: Request, now: number): Promise<Identity | null> {
  const hash = await handleHash(request);
  if (!hash) return null;
  const row = await env.AUTH_DB.prepare("SELECT subject, email, email_verified FROM jwt_link_pending WHERE handle_hash = ? AND expires_at > ?").bind(hash, iso(now)).first<Row>();
  return row ? identityOf(row) : null;
}

// takeLinkPending: the same, used up — a second confirmation finds nothing.
export async function takeLinkPending(env: Env, request: Request, now: number): Promise<Identity | null> {
  const hash = await handleHash(request);
  if (!hash) return null;
  const row = await env.AUTH_DB.prepare("DELETE FROM jwt_link_pending WHERE handle_hash = ? AND expires_at > ? RETURNING subject, email, email_verified")
    .bind(hash, iso(now))
    .first<Row>();
  return row ? identityOf(row) : null;
}
