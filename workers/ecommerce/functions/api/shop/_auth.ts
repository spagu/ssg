// Who is allowed into the admin panel.
//
// Two modes, and they are mutually exclusive on purpose. With Cloudflare Access
// configured the seller's identity provider does the work and there is no
// password here to steal or rotate — so the login endpoint stops existing
// (S-15). Without it, a small built-in scheme: PBKDF2 passwords, short-lived
// access tokens, rotating refresh cookies.

import { authMode, type Env } from "./_env";
import { verifyAccessJwt } from "./_access";
import {
  base64url,
  base64urlToBytes,
  fail,
  newId,
  nowISO,
  randomToken,
  sha256hex,
  timingSafeEqual,
} from "./_lib";
import type { AdminIdentity, AdminUserRow } from "./_types";

// ── Passwords ───────────────────────────────────────────────────────────────
//
// PBKDF2-SHA256 because WebCrypto has no Argon2. The iteration count is stored
// in the hash itself, so it can be raised later without a migration: an old
// hash keeps verifying with its own parameters.

const PBKDF2_ITERATIONS = 600000;

export async function hashPassword(password: string, iterations = PBKDF2_ITERATIONS): Promise<string> {
  const salt = crypto.getRandomValues(new Uint8Array(16));
  const bits = await derive(password, salt, iterations);
  return `pbkdf2$sha256$${iterations}$${base64url(salt)}$${base64url(new Uint8Array(bits))}`;
}

export async function verifyPassword(password: string, stored: string): Promise<boolean> {
  const parts = stored.split("$");
  if (parts.length !== 5 || parts[0] !== "pbkdf2" || parts[1] !== "sha256") return false;
  const iterations = Number.parseInt(parts[2] ?? "0", 10);
  if (!Number.isFinite(iterations) || iterations < 1000) return false;
  const salt = base64urlToBytes(parts[3] ?? "");
  const bits = await derive(password, salt, iterations);
  return timingSafeEqual(base64url(new Uint8Array(bits)), parts[4] ?? "");
}

async function derive(password: string, salt: Uint8Array, iterations: number): Promise<ArrayBuffer> {
  const key = await crypto.subtle.importKey("raw", new TextEncoder().encode(password), "PBKDF2", false, [
    "deriveBits",
  ]);
  return crypto.subtle.deriveBits({ name: "PBKDF2", salt, iterations, hash: "SHA-256" }, key, 256);
}

// ── Access tokens ───────────────────────────────────────────────────────────

const ACCESS_TTL_S = 15 * 60;
const REFRESH_TTL_DAYS = 7;
const AUDIENCE = "ecommerce-admin";

interface JwtPayload {
  sub: string;
  email: string;
  role: "owner" | "staff";
  jti: string;
  aud: string;
  iss: string;
  exp: number;
  iat: number;
}

async function hmac(secret: string, data: string): Promise<Uint8Array> {
  const key = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  return new Uint8Array(await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(data)));
}

export async function signJwt(env: Env, user: AdminUserRow, issuer: string): Promise<string> {
  const secret = env.SHOP_JWT_SECRET;
  if (!secret) throw new Error("SHOP_JWT_SECRET is not set");
  const now = Math.floor(Date.now() / 1000);
  const header = base64url(new TextEncoder().encode(JSON.stringify({ alg: "HS256", typ: "JWT" })));
  const payload: JwtPayload = {
    sub: user.id,
    email: user.email_lc,
    role: user.role,
    jti: newId(),
    aud: AUDIENCE,
    iss: issuer,
    exp: now + ACCESS_TTL_S,
    iat: now,
  };
  const body = base64url(new TextEncoder().encode(JSON.stringify(payload)));
  const signature = base64url(await hmac(secret, `${header}.${body}`));
  return `${header}.${body}.${signature}`;
}

export async function verifyJwt(env: Env, token: string, issuer: string): Promise<JwtPayload | null> {
  const secret = env.SHOP_JWT_SECRET;
  if (!secret) return null;
  const parts = token.split(".");
  if (parts.length !== 3) return null;

  const expected = base64url(await hmac(secret, `${parts[0]}.${parts[1]}`));
  if (!timingSafeEqual(parts[2] ?? "", expected)) return null;

  let payload: JwtPayload;
  try {
    payload = JSON.parse(new TextDecoder().decode(base64urlToBytes(parts[1] ?? ""))) as JwtPayload;
  } catch {
    return null;
  }
  // Every claim is checked: a token minted for another audience or issuer is
  // somebody else's token, however valid its signature.
  if (payload.aud !== AUDIENCE) return null;
  if (payload.iss !== issuer) return null;
  if (typeof payload.exp !== "number" || Date.now() / 1000 > payload.exp) return null;
  if (env.SHOP_KV && (await env.SHOP_KV.get(`jti:${payload.jti}`))) return null; // logged out
  return payload;
}

/** Logging out has to mean something before the token expires, so the id goes
 *  on a deny list for exactly as long as the token would have lived. */
export async function revokeJti(env: Env, jti: string, expSeconds: number): Promise<void> {
  if (!env.SHOP_KV) return;
  const ttl = Math.max(60, Math.floor(expSeconds - Date.now() / 1000));
  await env.SHOP_KV.put(`jti:${jti}`, "1", { expirationTtl: ttl });
}

// ── Refresh tokens ──────────────────────────────────────────────────────────

export const REFRESH_COOKIE = "__Host-shop_refresh";

export function refreshCookie(value: string, maxAgeSeconds: number): string {
  // __Host- requires Secure, Path=/ and no Domain. SameSite=Strict is what
  // stops another site from spending the cookie (S-20).
  return `${REFRESH_COOKIE}=${value}; Path=/; Secure; HttpOnly; SameSite=Strict; Max-Age=${maxAgeSeconds}`;
}

export function readCookie(request: Request, name: string): string {
  const raw = request.headers.get("cookie") ?? "";
  const match = new RegExp(`(?:^|;\\s*)${name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}=([^;]+)`).exec(raw);
  return match?.[1] ?? "";
}

export async function createRefresh(env: Env, adminId: string, rotatedFrom?: string): Promise<string> {
  const token = randomToken(32);
  const hash = await sha256hex(token);
  const expires = new Date(Date.now() + REFRESH_TTL_DAYS * 86400000).toISOString();
  await env.SHOP_DB.prepare(
    `INSERT INTO admin_sessions (jti_hash, admin_id, expires_at, rotated_from, created_at) VALUES (?, ?, ?, ?, ?)`,
  )
    .bind(hash, adminId, expires, rotatedFrom ?? null, nowISO())
    .run();
  return token;
}

export interface RefreshResult {
  ok: boolean;
  adminId?: string;
  /** True when a token that had already been rotated was presented again —
   *  which means someone has a copy, so every session for that admin ends. */
  reuse?: boolean;
}

export async function consumeRefresh(env: Env, token: string): Promise<RefreshResult> {
  if (!token) return { ok: false };
  const hash = await sha256hex(token);
  const row = await env.SHOP_DB.prepare(`SELECT * FROM admin_sessions WHERE jti_hash = ?`)
    .bind(hash)
    .first<{ jti_hash: string; admin_id: string; expires_at: string; used_at: string | null }>();

  // Never issued, or issued so long ago that it has been swept. Either way
  // there is nothing to rotate.
  if (!row) return { ok: false };

  if (Date.parse(row.expires_at) <= Date.now()) {
    await env.SHOP_DB.prepare(`DELETE FROM admin_sessions WHERE jti_hash = ?`).bind(hash).run();
    return { ok: false };
  }

  // A token presented twice means two parties hold it: the owner and someone
  // else. Which of them is asking now cannot be known, so every session for
  // this administrator ends and both have to sign in again (S-18).
  if (row.used_at) {
    await endAllSessions(env, row.admin_id);
    return { ok: false, reuse: true };
  }

  // Rotation: spent the moment it is used, but kept, so the next use of it is
  // recognisable as the replay it is.
  const spent = await env.SHOP_DB.prepare(
    `UPDATE admin_sessions SET used_at = ? WHERE jti_hash = ? AND used_at IS NULL`,
  )
    .bind(nowISO(), hash)
    .run();
  // Two requests raced with the same token. The one that lost is, by the same
  // reasoning as above, a replay.
  if ((spent.meta?.changes ?? 0) === 0) {
    await endAllSessions(env, row.admin_id);
    return { ok: false, reuse: true };
  }

  return { ok: true, adminId: row.admin_id };
}

export async function endAllSessions(env: Env, adminId: string): Promise<void> {
  await env.SHOP_DB.prepare(`DELETE FROM admin_sessions WHERE admin_id = ?`).bind(adminId).run();
}

// ── Login throttling ────────────────────────────────────────────────────────
//
// Per address and per account, because either alone is easy to walk around
// (S-17). Without KV bound there is no counter and the middleware says so
// rather than pretending to be protected.

export async function loginAllowed(env: Env, ip: string, email: string): Promise<boolean> {
  if (!env.SHOP_KV) return true;
  const keys = [`login:ip:${ip}`, `login:em:${email}`];
  const limits = [5, 10];
  for (let i = 0; i < keys.length; i++) {
    const used = Number.parseInt((await env.SHOP_KV.get(keys[i]!)) ?? "0", 10) || 0;
    if (used >= limits[i]!) return false;
  }
  return true;
}

export async function noteLoginFailure(env: Env, ip: string, email: string): Promise<void> {
  if (!env.SHOP_KV) return;
  for (const [key, ttl] of [
    [`login:ip:${ip}`, 900],
    [`login:em:${email}`, 3600],
  ] as const) {
    const used = Number.parseInt((await env.SHOP_KV.get(key)) ?? "0", 10) || 0;
    await env.SHOP_KV.put(key, String(used + 1), { expirationTtl: ttl });
  }
}

export async function clearLoginFailures(env: Env, ip: string, email: string): Promise<void> {
  if (!env.SHOP_KV) return;
  await env.SHOP_KV.delete(`login:ip:${ip}`);
  await env.SHOP_KV.delete(`login:em:${email}`);
}

// ── The gate ────────────────────────────────────────────────────────────────

/** Identifies the caller, or returns the response explaining why not. */
export async function requireAdmin(request: Request, env: Env): Promise<AdminIdentity | Response> {
  const mode = authMode(env);
  if (mode === "none") {
    return fail(
      request,
      "admin_not_configured",
      "Admin access is not set up. Configure Cloudflare Access, or set SHOP_JWT_SECRET.",
      503,
    );
  }

  if (mode === "access") {
    const verdict = await verifyAccessJwt(request, {
      team: env.SHOP_ACCESS_TEAM!,
      aud: env.SHOP_ACCESS_AUD!,
    });
    if (!verdict.ok) return fail(request, verdict.code, verdict.message, verdict.status);
    const owners = (env.SHOP_ACCESS_OWNERS ?? "")
      .split(",")
      .map((s) => s.trim().toLowerCase())
      .filter(Boolean);
    const email = verdict.email.toLowerCase();
    return {
      sub: verdict.sub || email,
      email,
      // With no owner list configured, everyone Access lets through is an
      // owner: the seller has already decided who reaches the application.
      role: owners.length === 0 || owners.includes(email) ? "owner" : "staff",
      via: "access",
    };
  }

  const header = request.headers.get("authorization") ?? "";
  if (!header.startsWith("Bearer ")) {
    return fail(request, "unauthorized", "Sign in to continue.", 401);
  }
  const issuer = new URL(request.url).origin;
  const payload = await verifyJwt(env, header.slice(7), issuer);
  if (!payload) return fail(request, "unauthorized", "That session has expired. Sign in again.", 401);
  return { sub: payload.sub, email: payload.email, role: payload.role, via: "jwt" };
}

/** Owner-only actions: money moves, settings change, things get deleted. The
 *  check is here, on the server, not only hidden in the panel's markup (S-23). */
/** Whether this account may still act. Checked on every admin request, not only
 *  at sign-in: an account suspended at noon must not keep working until its
 *  fifteen-minute token expires. */
export async function accountActive(env: Env, id: string): Promise<boolean> {
  const row = await env.SHOP_DB.prepare(`SELECT disabled FROM admin_users WHERE id = ?`)
    .bind(id)
    .first<{ disabled: number }>();
  // No row means an account under Cloudflare Access, which this table does not
  // hold — those are vouched for by the identity provider instead.
  return !row || row.disabled !== 1;
}

export function requireOwner(request: Request, identity: AdminIdentity): Response | null {
  if (identity.role === "owner") return null;
  return fail(request, "forbidden", "Only an owner can do that.", 403);
}

/** Creates the first owner from a secret, once. The audit entry asks for the
 *  secret to be deleted, because a bootstrap credential left in place is a
 *  password nobody rotates. */
export async function maybeBootstrap(env: Env): Promise<AdminUserRow | null> {
  if (!env.SHOP_ADMIN_BOOTSTRAP) return null;
  const count = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM admin_users`).first<{ n: number }>();
  if ((count?.n ?? 0) > 0) return null;

  const sep = env.SHOP_ADMIN_BOOTSTRAP.indexOf(":");
  if (sep < 1) return null;
  const email = env.SHOP_ADMIN_BOOTSTRAP.slice(0, sep).trim().toLowerCase();
  const password = env.SHOP_ADMIN_BOOTSTRAP.slice(sep + 1);
  if (!email || password.length < 12) return null;

  const user: AdminUserRow = {
    id: newId(),
    email_lc: email,
    pass_hash: await hashPassword(password),
    role: "owner",
    created_at: nowISO(),
    last_login_at: null,
    name: null,
    disabled: 0,
  };
  await env.SHOP_DB.prepare(
    `INSERT OR IGNORE INTO admin_users (id, email_lc, pass_hash, role, created_at, last_login_at)
     VALUES (?, ?, ?, ?, ?, NULL)`,
  )
    .bind(user.id, user.email_lc, user.pass_hash, user.role, user.created_at)
    .run();
  const { audit } = await import("./_audit");
  await audit(env, "system", "admin.bootstrap", user.id, {
    email,
    reminder: "Delete the SHOP_ADMIN_BOOTSTRAP secret now that an owner exists.",
  });
  return user;
}
