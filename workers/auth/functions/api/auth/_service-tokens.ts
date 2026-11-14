// Service tokens (GO-141): the long-lived secret a script holds, and the data
// layer around it. A leading underscore keeps this file out of the Pages route
// table.
//
// The secret is `ssgsvc_<id>_<43 random base64url characters>` (256 random
// bits). The prefix lets secret scanners recognise a leaked one; `<id>` is the
// token's short public id — shown in lists, used as the lockout key, never
// enough to use the token. Only the SHA-256 of the whole secret is stored; the
// secret itself exists once, in the answer that creates it.

import { intVar, iso, randomToken, sha256hex } from "./_lib";
import type { Env } from "./_lib";

export const SECRET_PREFIX = "ssgsvc_";
const SECRET = /^ssgsvc_([0-9a-f]{12})_([A-Za-z0-9_-]{43})$/;
// TOUCH_MS: last_used_at is written at most this often per token, so a busy
// cron does not turn every exchange into a write.
export const TOUCH_MS = 5 * 60_000;

export interface ServiceSettings {
  maxDays: number; // AUTH_SERVICE_TOKEN_MAX_DAYS, default 365: the longest expiry a token may have
  defaultDays: number; // 90, or maxDays when that is lower
  accessSeconds: number; // AUTH_SERVICE_ACCESS_MINUTES, default 10: the exchanged access token's lifetime
}

export function serviceSettings(env: Env): ServiceSettings {
  const maxDays = intVar(env.AUTH_SERVICE_TOKEN_MAX_DAYS, 365);
  return { maxDays, defaultDays: Math.min(90, maxDays), accessSeconds: intVar(env.AUTH_SERVICE_ACCESS_MINUTES, 10) * 60 };
}

// parseSecret splits a presented secret into its public id, or null when it
// does not have the shape of one.
export function parseSecret(raw: string): { id: string } | null {
  const m = SECRET.exec(raw);
  return m ? { id: m[1] } : null;
}

export interface ServiceToken {
  id: string;
  serviceAccountId: string;
  name: string;
  display: string; // `ssgsvc_<id>_…`: recognisable, useless to a thief
  permissions: string[] | null;
  allowSensitive: string[];
  createdBy: string | null;
  createdAt: string;
  expiresAt: string;
  lastUsedAt: string | null;
  revokedAt: string | null;
  status: "active" | "expired" | "revoked";
}

export interface TokenRow {
  id: string;
  service_account_id: string;
  name: string;
  secret_hash: string;
  permissions: string | null;
  allow_sensitive: string;
  created_by: string | null;
  created_at: string;
  expires_at: string;
  last_used_at: string | null;
  revoked_at: string | null;
}

export function shapeToken(r: TokenRow, now: number): ServiceToken {
  const status = r.revoked_at ? "revoked" : Date.parse(r.expires_at) <= now ? "expired" : "active";
  return {
    id: r.id,
    serviceAccountId: r.service_account_id,
    name: r.name,
    display: `${SECRET_PREFIX}${r.id}_…`,
    permissions: r.permissions === null ? null : (JSON.parse(r.permissions) as string[]),
    allowSensitive: JSON.parse(r.allow_sensitive) as string[],
    createdBy: r.created_by,
    createdAt: r.created_at,
    expiresAt: r.expires_at,
    lastUsedAt: r.last_used_at,
    revokedAt: r.revoked_at,
    status,
  };
}

export interface NewToken {
  name: string;
  days: number;
  permissions: string[] | null;
  allowSensitive: string[];
}

// createToken stores a new token and returns it with its secret — the only
// time the secret leaves this worker.
export async function createToken(env: Env, accountId: string, t: NewToken, by: string, now: number): Promise<{ token: ServiceToken; secret: string }> {
  const id = [...crypto.getRandomValues(new Uint8Array(6))].map((b) => b.toString(16).padStart(2, "0")).join("");
  const secret = `${SECRET_PREFIX}${id}_${randomToken()}`;
  const row: TokenRow = {
    id,
    service_account_id: accountId,
    name: t.name,
    secret_hash: await sha256hex(secret),
    permissions: t.permissions === null ? null : JSON.stringify(t.permissions),
    allow_sensitive: JSON.stringify(t.allowSensitive),
    created_by: by,
    created_at: iso(now),
    expires_at: iso(now + t.days * 86_400_000),
    last_used_at: null,
    revoked_at: null,
  };
  await env.AUTH_DB.prepare(
    `INSERT INTO service_tokens (id, service_account_id, name, secret_hash, permissions, allow_sensitive, created_by, created_at, expires_at)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
  )
    .bind(row.id, row.service_account_id, row.name, row.secret_hash, row.permissions, row.allow_sensitive, row.created_by, row.created_at, row.expires_at)
    .run();
  return { token: shapeToken(row, now), secret };
}

export async function listTokens(env: Env, accountId: string, now: number): Promise<ServiceToken[]> {
  const { results } = await env.AUTH_DB.prepare("SELECT * FROM service_tokens WHERE service_account_id = ? ORDER BY created_at DESC, id")
    .bind(accountId)
    .all<TokenRow>();
  return results.map((r) => shapeToken(r, now));
}

// revokeToken ends one token of one account; revoking it again keeps the
// first revocation's time and author.
export async function revokeToken(env: Env, accountId: string, id: string, by: string, now: number): Promise<"ok" | "not_found"> {
  const res = await env.AUTH_DB.prepare(
    "UPDATE service_tokens SET revoked_at = COALESCE(revoked_at, ?), revoked_by = COALESCE(revoked_by, ?) WHERE id = ? AND service_account_id = ?",
  )
    .bind(iso(now), by, id, accountId)
    .run();
  return res.meta.changes === 1 ? "ok" : "not_found";
}

// usableToken finds the token behind a presented secret and says whether it
// may be exchanged now. The lookup is by the SHA-256 of the whole secret, so
// no secret is ever compared: an index search over digests of 256 random bits
// leaks nothing an attacker can steer (a timing difference tells them about a
// digest they cannot invert). Then: not revoked, not expired, its account
// active. Every way of failing returns the same null after the same single
// query, so unknown, revoked and expired look alike to the caller.
export async function usableToken(env: Env, secret: string, now: number): Promise<TokenRow | null> {
  const row = await env.AUTH_DB.prepare(
    "SELECT t.*, a.status AS account_status FROM service_tokens t JOIN service_accounts a ON a.id = t.service_account_id WHERE t.secret_hash = ?",
  )
    .bind(await sha256hex(secret))
    .first<TokenRow & { account_status: string }>();
  if (!row || row.revoked_at || Date.parse(row.expires_at) <= now || row.account_status !== "active") return null;
  return row;
}

// touchToken records the use, at most once per TOUCH_MS.
export async function touchToken(env: Env, t: TokenRow, now: number): Promise<void> {
  if (t.last_used_at && Date.parse(t.last_used_at) + TOUCH_MS > now) return;
  await env.AUTH_DB.prepare("UPDATE service_tokens SET last_used_at = ? WHERE id = ?").bind(iso(now), t.id).run();
}
