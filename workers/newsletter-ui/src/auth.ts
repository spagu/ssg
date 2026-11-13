// Admin authentication for everything under the base path except hooks/.
//
// Two modes, under the same variable names as the `newsletter` admin so one
// configuration serves both workers:
//   - Cloudflare Access (recommended) when NEWSLETTER_ACCESS_TEAM + AUD are set;
//   - HTTP Basic with NEWSLETTER_ADMIN_PASSWORD otherwise.
// Neither set → 503. The panel is never open by default.
//
// Basic mode adds a failed-login lockout per IP (NEWSLETTER_AUTH_MAX_FAILURES
// within NEWSLETTER_AUTH_WINDOW_MINUTES, default 10 in 10). It lives in D1
// (auth_attempts) and is checked BEFORE the password is compared: a locked IP
// gets 429 even for the right password, otherwise the lockout would only change
// the error code of a wrong guess and slow nobody down. The Workers rate-limit
// binding cannot serve here — it has no way to ask "is this key over?" without
// counting the request — so RATE_LIMITER bounds request volume instead (app.ts).
// With Access the lockout is off: Access stops anonymous requests first.

import { verifyAccess } from "./access";
import { clientIP, intVar, json, sha256hex } from "./lib";
import type { Env } from "./lib";

export const REALM = 'Basic realm="newsletter-admin", charset="UTF-8"';

// requireAdmin returns null when the request is authorised, or the Response to
// send instead.
export async function requireAdmin(request: Request, env: Env): Promise<Response | null> {
  if (env.NEWSLETTER_ACCESS_TEAM && env.NEWSLETTER_ACCESS_AUD) {
    return verifyAccess(request, env.NEWSLETTER_ACCESS_TEAM, env.NEWSLETTER_ACCESS_AUD);
  }
  const expected = env.NEWSLETTER_ADMIN_PASSWORD;
  if (!expected) {
    return json(
      {
        error: "admin not configured",
        hint: "set NEWSLETTER_ACCESS_TEAM + NEWSLETTER_ACCESS_AUD, or the NEWSLETTER_ADMIN_PASSWORD secret",
      },
      503,
    );
  }
  const header = request.headers.get("authorization") || "";
  // No credentials yet: the browser's first request. Not an attempt, not counted.
  if (!header) return unauthorized();

  const key = await sha256hex((env.NEWSLETTER_HASH_SALT || "") + clientIP(request));
  const windowMs = intVar(env.NEWSLETTER_AUTH_WINDOW_MINUTES, 10) * 60_000;
  const now = Date.now();
  const row = await env.NEWSLETTER_DB.prepare("SELECT failures, window_start, locked_until FROM auth_attempts WHERE ip_hash = ?")
    .bind(key)
    .first<{ failures: number; window_start: string; locked_until: string | null }>();
  if (row?.locked_until && Date.parse(row.locked_until) > now) {
    const wait = Math.ceil((Date.parse(row.locked_until) - now) / 1000);
    return json({ error: "too many failed logins, try later" }, 429, { "retry-after": String(wait) });
  }

  const given = basicPassword(header);
  if (given !== null && timingSafeEqual(given, expected)) {
    if (row) await env.NEWSLETTER_DB.prepare("DELETE FROM auth_attempts WHERE ip_hash = ?").bind(key).run();
    return null;
  }
  await recordFailure(env, key, row, now, windowMs);
  return unauthorized();
}

function unauthorized(): Response {
  return json({ error: "unauthorized" }, 401, { "www-authenticate": REALM });
}

// basicPassword extracts the password from a Basic header, or null.
function basicPassword(header: string): string | null {
  if (!header.startsWith("Basic ")) return null;
  try {
    const decoded = atob(header.slice(6));
    return decoded.slice(decoded.indexOf(":") + 1);
  } catch {
    return null;
  }
}

// recordFailure counts a failed login in a fixed window and locks the IP for
// one window once the count reaches the limit.
async function recordFailure(
  env: Env,
  key: string,
  row: { failures: number; window_start: string } | null,
  now: number,
  windowMs: number,
): Promise<void> {
  const fresh = !row || Date.parse(row.window_start) + windowMs <= now;
  const failures = fresh ? 1 : (row as { failures: number }).failures + 1;
  const windowStart = fresh ? new Date(now).toISOString() : (row as { window_start: string }).window_start;
  const max = intVar(env.NEWSLETTER_AUTH_MAX_FAILURES, 10);
  const lockedUntil = failures >= max ? new Date(now + windowMs).toISOString() : null;
  await env.NEWSLETTER_DB.prepare(
    `INSERT INTO auth_attempts (ip_hash, failures, window_start, locked_until) VALUES (?, ?, ?, ?)
     ON CONFLICT (ip_hash) DO UPDATE SET failures = excluded.failures,
       window_start = excluded.window_start, locked_until = excluded.locked_until`,
  )
    .bind(key, failures, windowStart, lockedUntil)
    .run();
}

// Constant-time compare that folds a length difference into the accumulator, so
// the length of the configured secret `b` does not leak through timing. Callers
// guarantee `b` is non-empty.
export function timingSafeEqual(a: string, b: string): boolean {
  let diff = a.length ^ b.length;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i % b.length);
  return diff === 0;
}
