// The shared admin password (SHOP_ADMIN_PASSWORD) over HTTP Basic, with a
// failed-attempt lockout (GO-155). A leading underscore keeps this file out of
// the Pages route table.
//
// The lockout is per client address — SHOP_ADMIN_MAX_FAILURES failures (default
// 10) within SHOP_ADMIN_LOCKOUT_MINUTES (default 10) lock that address for one
// window — and lives in D1 (auth_attempts), the same pattern as the
// newsletter-ui worker. It is checked BEFORE the password is compared: a locked
// address gets 429 even for the right password, otherwise the lockout would
// only change the error code of a wrong guess and slow nobody down. The address
// is stored hashed, salted with SHOP_IP_SALT when it is set; a row is deleted on
// the next successful sign-in from that address.
//
// No WWW-Authenticate header on the 401: it would make the browser pop its own
// Basic dialog on top of the panel's password form. curl -u sends the
// credential without being asked, so it does not need the challenge.

import type { Env } from "./_env";
import { fail, sha256hex, timingSafeEqual } from "./_lib";

// intVar reads a whole, positive number of something, or the default.
function intVar(raw: string | undefined, fallback: number): number {
  const n = Number.parseInt(raw ?? "", 10);
  return Number.isFinite(n) && n > 0 ? n : fallback;
}

// passwordGate: null when the request carries the password, otherwise the 401
// or 429 to send. `expected` is non-empty (the caller checked).
export async function passwordGate(request: Request, env: Env, expected: string): Promise<Response | null> {
  const header = request.headers.get("authorization") ?? "";
  // No credentials yet: the panel's first probe. Not an attempt, not counted.
  if (!header) return unauthorized(request);

  const key = await sha256hex(`${env.SHOP_IP_SALT ?? ""}:${request.headers.get("cf-connecting-ip") ?? "unknown"}`);
  const windowMs = intVar(env.SHOP_ADMIN_LOCKOUT_MINUTES, 10) * 60_000;
  const max = intVar(env.SHOP_ADMIN_MAX_FAILURES, 10);
  const now = Date.now();
  const locked = await lockedFor(env, key, now);
  if (locked > 0) return tooMany(request, locked);

  const given = basicPassword(header);
  if (given !== null && timingSafeEqual(given, expected)) {
    // A burst of parallel guesses all pass the read above before any failure
    // is written: the right password is accepted only while the lock is still
    // clear after the comparison.
    const late = await lockedFor(env, key, Date.now());
    if (late > 0) return tooMany(request, late);
    await env.SHOP_DB.prepare("DELETE FROM auth_attempts WHERE ip_hash = ?").bind(key).run();
    return null;
  }
  const failures = await recordFailure(env, key, now, windowMs, max);
  return failures >= max ? tooMany(request, Math.ceil(windowMs / 1000)) : unauthorized(request);
}

const unauthorized = (request: Request): Response =>
  fail(request, "unauthorized", "Enter the admin password to continue.", 401);

function tooMany(request: Request, waitSeconds: number): Response {
  const res = fail(request, "too_many_attempts", "Too many wrong passwords. Wait a few minutes and try again.", 429);
  res.headers.set("retry-after", String(waitSeconds));
  return res;
}

// lockedFor: the seconds this address is still locked for, or 0.
async function lockedFor(env: Env, key: string, now: number): Promise<number> {
  const row = await env.SHOP_DB.prepare("SELECT locked_until FROM auth_attempts WHERE ip_hash = ?")
    .bind(key)
    .first<{ locked_until: string | null }>();
  const until = row?.locked_until ? Date.parse(row.locked_until) : 0;
  return until > now ? Math.ceil((until - now) / 1000) : 0;
}

// basicPassword extracts the password from a Basic header (the user name is
// ignored), or null for another scheme or an undecodable value. The bytes are
// read as UTF-8, as curl -u and the panel send them, so a password outside
// ASCII compares equal to the secret it was copied from.
function basicPassword(header: string): string | null {
  if (!header.startsWith("Basic ")) return null;
  try {
    const decoded = new TextDecoder().decode(Uint8Array.from(atob(header.slice(6)), (c) => c.charCodeAt(0)));
    return decoded.slice(decoded.indexOf(":") + 1);
  } catch {
    return null;
  }
}

// recordFailure counts one failure in ONE statement — the arithmetic reads the
// row's own values, so parallel failures all count — starts a new window when
// the old one has passed, locks the address once the count reaches `max`, and
// returns the count in the current window.
async function recordFailure(env: Env, key: string, now: number, windowMs: number, max: number): Promise<number> {
  const at = new Date(now).toISOString();
  const stale = new Date(now - windowMs).toISOString();
  const until = new Date(now + windowMs).toISOString();
  const row = await env.SHOP_DB.prepare(
    `INSERT INTO auth_attempts (ip_hash, failures, window_start, locked_until)
       VALUES (?1, 1, ?2, CASE WHEN 1 >= ?4 THEN ?5 ELSE NULL END)
     ON CONFLICT (ip_hash) DO UPDATE SET
       failures = CASE WHEN auth_attempts.window_start <= ?3 THEN 1 ELSE auth_attempts.failures + 1 END,
       window_start = CASE WHEN auth_attempts.window_start <= ?3 THEN ?2 ELSE auth_attempts.window_start END,
       locked_until = CASE WHEN (CASE WHEN auth_attempts.window_start <= ?3 THEN 1 ELSE auth_attempts.failures + 1 END) >= ?4
                           THEN ?5 ELSE auth_attempts.locked_until END
     RETURNING failures`,
  )
    .bind(key, at, stale, max, until)
    .first<{ failures: number }>();
  return (row as { failures: number }).failures;
}
