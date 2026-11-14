// Admin authentication for everything under the base path except hooks/.
//
// Three modes, under the same variable names as the `newsletter` admin so one
// configuration serves both workers, checked in this order (the `auth`
// contract's, workers/auth/docs/CONTRACT.md):
//   1. Cloudflare Access when NEWSLETTER_ACCESS_TEAM + AUD are set;
//   2. `auth` sessions when any NEWSLETTER_AUTH_{ISSUER,AUDIENCE,PUBLIC_KEY,
//      JWKS_URL,LOGIN_URL} is set (GO-137): the session cookie of the `auth`
//      worker, checked locally by the portable verifier. An incomplete set is
//      a 503 naming what is missing — never a fall-through to the password,
//      and with a complete set the password is NOT accepted either: it would
//      bypass the per-action permissions;
//   3. HTTP Basic with NEWSLETTER_ADMIN_PASSWORD.
// None set → 503. The panel is never open by default. Modes 1 and 3 grant every
// permission, as before; mode 2 grants what the token's `perms` say.
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
import { clientIP, intVar, json, secure, sha256hex } from "./lib";
import type { Env } from "./lib";
import { configFrom, denied, verifySession } from "./session-verifier";

export const REALM = 'Basic realm="newsletter-admin", charset="UTF-8"';
// AUTH_PREFIX: the NEWSLETTER_AUTH_* variables, shared with the `newsletter`
// template like the Access ones.
export const AUTH_PREFIX = "NEWSLETTER";

export type Mode = "access" | "auth" | "password";

// Admin is who is calling. `user` is known in `auth` mode only (Access's
// identity is not read here; the password names nobody); `perms` is "all" in
// the modes that grant everything, else the token's permissions as issued.
export interface Admin {
  mode: Mode;
  user: { id: string; email: string } | null;
  perms: string[] | "all";
}

// modeOf names the mode the configuration selects (health, /api/me).
export function modeOf(env: Env): Mode {
  if (env.NEWSLETTER_ACCESS_TEAM && env.NEWSLETTER_ACCESS_AUD) return "access";
  return configFrom(env, AUTH_PREFIX) ? "auth" : "password";
}

// authenticate returns who is calling, or the Response to send instead.
export async function authenticate(request: Request, env: Env): Promise<Admin | Response> {
  if (env.NEWSLETTER_ACCESS_TEAM && env.NEWSLETTER_ACCESS_AUD) {
    const refused = await verifyAccess(request, env.NEWSLETTER_ACCESS_TEAM, env.NEWSLETTER_ACCESS_AUD);
    return refused ?? { mode: "access", user: null, perms: "all" };
  }
  const config = configFrom(env, AUTH_PREFIX);
  if (config) {
    const v = await verifySession(request, config);
    if (!v.ok) return secure(denied(request, config, v.reason));
    return { mode: "auth", user: v.user, perms: v.perms };
  }
  const refused = await passwordGate(request, env);
  return refused ?? { mode: "password", user: null, perms: "all" };
}

// requireAdmin returns null when the request is authorised, or the Response to
// send instead — the question without the answer "who", kept for callers that
// only need a yes or no.
export async function requireAdmin(request: Request, env: Env): Promise<Response | null> {
  const who = await authenticate(request, env);
  return who instanceof Response ? who : null;
}

// passwordGate: HTTP Basic with the lockout described above.
async function passwordGate(request: Request, env: Env): Promise<Response | null> {
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
    // A burst of parallel guesses all pass the read above before any failure
    // is written (security review SR-12): the right password is accepted only
    // while the lock is still clear after the comparison.
    const late = await lockedFor(env, key, Date.now());
    if (late > 0) return tooMany(late);
    if (row) await env.NEWSLETTER_DB.prepare("DELETE FROM auth_attempts WHERE ip_hash = ?").bind(key).run();
    return null;
  }
  const max = intVar(env.NEWSLETTER_AUTH_MAX_FAILURES, 10);
  return (await recordFailure(env, key, now, windowMs, max)) > max ? tooMany(Math.ceil(windowMs / 1000)) : unauthorized();
}

const tooMany = (wait: number): Response => json({ error: "too many failed logins, try later" }, 429, { "retry-after": String(wait) });

async function lockedFor(env: Env, key: string, now: number): Promise<number> {
  const row = await env.NEWSLETTER_DB.prepare("SELECT locked_until FROM auth_attempts WHERE ip_hash = ?").bind(key).first<{ locked_until: string | null }>();
  const until = row?.locked_until ? Date.parse(row.locked_until) : 0;
  return until > now ? Math.ceil((until - now) / 1000) : 0;
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
// recordFailure counts one failure in ONE statement — the arithmetic reads the
// row's own values, so parallel failures all count (SR-12; it used to read the
// row first and write back a computed number, losing concurrent failures) —
// and returns the count in the current window.
async function recordFailure(env: Env, key: string, now: number, windowMs: number, max: number): Promise<number> {
  const at = new Date(now).toISOString();
  const fresh = new Date(now - windowMs).toISOString();
  const until = new Date(now + windowMs).toISOString();
  const row = await env.NEWSLETTER_DB.prepare(
    `INSERT INTO auth_attempts (ip_hash, failures, window_start, locked_until) VALUES (?1, 1, ?2, CASE WHEN 1 >= ?4 THEN ?5 ELSE NULL END)
     ON CONFLICT (ip_hash) DO UPDATE SET
       failures = CASE WHEN auth_attempts.window_start <= ?3 THEN 1 ELSE auth_attempts.failures + 1 END,
       window_start = CASE WHEN auth_attempts.window_start <= ?3 THEN ?2 ELSE auth_attempts.window_start END,
       locked_until = CASE WHEN (CASE WHEN auth_attempts.window_start <= ?3 THEN 1 ELSE auth_attempts.failures + 1 END) >= ?4 THEN ?5 ELSE auth_attempts.locked_until END
     RETURNING failures`,
  )
    .bind(key, at, fresh, max, until)
    .first<{ failures: number }>();
  return (row as { failures: number }).failures;
}

// Constant-time compare that folds a length difference into the accumulator, so
// the length of the configured secret `b` does not leak through timing. Callers
// guarantee `b` is non-empty.
export function timingSafeEqual(a: string, b: string): boolean {
  let diff = a.length ^ b.length;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i % b.length);
  return diff === 0;
}
