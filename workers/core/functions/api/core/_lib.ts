// Shared helpers for the core worker: the environment it is given, JSON
// answers and the security headers every response carries. A leading
// underscore keeps this file out of the Pages route table.

// Env: every variable is optional; what is missing decides the sign-in mode
// (see _signin.ts) or falls back to a default. The one database, CORE_DB, is
// optional too (GO-149): without it core keeps no state of its own and the
// module settings are read-only.
export interface Env {
  // Cloudflare Access — both values, or neither.
  CORE_ACCESS_TEAM?: string; // "myteam" or "myteam.cloudflareaccess.com"
  CORE_ACCESS_AUD?: string; // the Access application's AUD tag
  // The `auth` worker's session (see _auth-mode.ts).
  CORE_AUTH_ISSUER?: string;
  CORE_AUTH_AUDIENCE?: string;
  CORE_AUTH_PUBLIC_KEY?: string;
  CORE_AUTH_JWKS_URL?: string;
  CORE_AUTH_LOGIN_URL?: string;
  // A shared password (HTTP Basic), with a failed-login lockout.
  CORE_ADMIN_PASSWORD?: string;
  CORE_LOGIN_MAX_FAILURES?: string; // failed logins per IP per window (default 10)
  CORE_LOGIN_WINDOW_MINUTES?: string; // the window and the lockout length (default 10)
  CORE_LOGIN_LIMITER?: RateLimit; // optional Workers rate-limit binding, see _password.ts
  // Asking modules how they are (_status.ts).
  CORE_PROBE_TIMEOUT_MS?: string; // per module (default 2000)
  CORE_STATUS_DEADLINE_MS?: string; // for the whole answer (default 4000)
  CORE_STATUS_CACHE_SECONDS?: string; // per caller and module (default 15)
  // The site's build report, when build_report names another file (_build.ts).
  CORE_BUILD_REPORT_PATH?: string; // default /build.json
  // Module settings, status history, change log and a durable lockout (GO-149).
  CORE_DB?: D1Database;
  CORE_HISTORY_DAYS?: string; // status history kept for (default 90)
  CORE_CHANGES_DAYS?: string; // settings change log kept for (default 365)
}

// The D1 surface core uses (declared here so the template type-checks
// without the workers-types package).
export interface D1Statement {
  bind(...values: unknown[]): D1Statement;
  first<T = Record<string, unknown>>(): Promise<T | null>;
  all<T = Record<string, unknown>>(): Promise<{ results: T[] }>;
  run(): Promise<unknown>;
}
export interface D1Database {
  prepare(sql: string): D1Statement;
  batch(statements: D1Statement[]): Promise<unknown[]>;
}

// The Workers rate-limit binding's surface (declared here so the template
// type-checks without the workers-types package).
export interface RateLimit {
  limit(options: { key: string }): Promise<{ success: boolean }>;
}

// CSRF_HEADER must accompany every request that is not GET or HEAD. A form on
// another site cannot set a custom header, and a script there cannot send one
// cross-origin without a CORS preflight this worker never grants. The routes
// that change state are the module settings under /api/core/manage (GO-149).
export const CSRF_HEADER = "x-core-admin";

// QUIET_HEADER: sent by the shared navigation (public/core/nav.js). A refusal
// to such a request carries no WWW-Authenticate challenge, so a browser never
// pops up a password dialog because a panel tried to draw the navigation: the
// navigation simply does not appear. The home page leaves it out and is asked
// for the password as usual.
export const QUIET_HEADER = "x-core-quiet";

// One policy for every answer: nothing inline, nothing from another origin. A
// JSON answer that is somehow rendered as a page can then run nothing.
export const CSP = [
  "default-src 'none'",
  "script-src 'self'",
  "style-src 'self'",
  "img-src 'self' data:",
  "font-src 'self'",
  "connect-src 'self'",
  "base-uri 'none'",
  "form-action 'self'",
  "frame-ancestors 'none'",
].join("; ");

export const SECURITY_HEADERS: Record<string, string> = {
  "cache-control": "no-store", // who sees which module is per caller: never in a shared cache
  "x-robots-tag": "noindex, nofollow",
  "content-security-policy": CSP,
  "referrer-policy": "no-referrer",
  "x-content-type-options": "nosniff",
  "x-frame-options": "DENY",
};

// secure returns the response with the security headers set. Headers of a
// response may be immutable, so it always copies.
export function secure(res: Response): Response {
  const out = new Response(res.body, res);
  for (const [k, v] of Object.entries(SECURITY_HEADERS)) out.headers.set(k, v);
  return out;
}

export const json = (data: unknown, status = 200, headers: Record<string, string> = {}): Response =>
  secure(new Response(JSON.stringify(data), { status, headers: { "content-type": "application/json; charset=utf-8", ...headers } }));

// problem is the house error shape: {error: "<snake_case>", detail?}.
export const problem = (error: string, status: number, detail?: string, headers: Record<string, string> = {}): Response =>
  json(detail === undefined ? { error } : { error, detail }, status, headers);

// intVar reads a positive integer variable, falling back to def.
export function intVar(raw: string | undefined, def: number): number {
  const n = Number.parseInt(raw ?? "", 10);
  return Number.isFinite(n) && n > 0 ? n : def;
}

export async function sha256hex(input: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(input));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

// clientIP is the address Cloudflare saw; "unknown" only in local tests.
export function clientIP(request: Request): string {
  return request.headers.get("cf-connecting-ip") || "unknown";
}

// timingSafeEqual compares in constant time and folds a length difference into
// the result, so the configured secret's length does not leak through timing.
// Callers guarantee `b` is non-empty.
export function timingSafeEqual(a: string, b: string): boolean {
  let diff = a.length ^ b.length;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i % b.length);
  return diff === 0;
}
