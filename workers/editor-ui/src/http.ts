// The environment editor-ui is given, and what every answer carries: JSON,
// the security headers, the base path the whole worker lives under.
//
// A standalone Worker (`wrangler deploy` from workers/editor-ui). It has no
// database: the history of changes is the repository's history, drafts live
// in the browser (README of the round, "Konfiguracja").

export interface Env {
  ASSETS?: Fetcher; // the panel files in public/, served behind the gate
  RATE_LIMITER?: RateLimit; // per-IP request volume, optional
  EDITOR_BASE?: string; // path prefix, default /editor
  // The repository (workers/editor-ui/portable/repo-config.ts reads them).
  EDITOR_GIT_REPO?: string; // "github.com/<owner>/<repo>"
  EDITOR_GIT_BRANCH?: string; // empty = the repository's default branch
  EDITOR_GIT_TOKEN?: string; // a secret, never a [vars] entry
  // `auth` sessions, read by the portable verifier (src/session-verifier.ts).
  // Required: an incomplete or missing set answers 503 everywhere.
  EDITOR_AUTH_ISSUER?: string;
  EDITOR_AUTH_AUDIENCE?: string;
  EDITOR_AUTH_PUBLIC_KEY?: string;
  EDITOR_AUTH_JWKS_URL?: string;
  EDITOR_AUTH_LOGIN_URL?: string;
  // Overrides for a site whose .ssg.yaml the worker cannot read (site-model.ts).
  EDITOR_CONTENT_DIRS?: string; // comma-separated repository folders
  EDITOR_MEDIA_DIR?: string; // the folder pasted images go to
  EDITOR_COMPONENTS_URL?: string; // where the site publishes components.json (same origin; default /components.json)
}

// PREFIX: the variable prefix of this worker — EDITOR_AUTH_*, EDITOR_GIT_*.
export const PREFIX = "EDITOR";
export const DEFAULT_BASE = "/editor";

// One policy for JSON and for the panel's HTML: nothing inline, nothing from
// another origin. A JSON answer somehow rendered as a page can run nothing.
export const CSP = [
  "default-src 'none'",
  "script-src 'self'",
  "style-src 'self'",
  "img-src 'self' data: blob:",
  "font-src 'self'",
  "connect-src 'self'",
  "manifest-src 'self'",
  "base-uri 'none'",
  "form-action 'self'",
  "frame-ancestors 'none'",
].join("; ");

const SECURITY_HEADERS: Record<string, string> = {
  "cache-control": "no-store", // admin data must never sit in a shared cache
  "x-robots-tag": "noindex, nofollow",
  "content-security-policy": CSP,
  "referrer-policy": "no-referrer",
  "x-content-type-options": "nosniff",
  "x-frame-options": "DENY",
};

// secure returns the response with the security headers set. Responses from
// fetch (the assets binding) have immutable headers, so it always copies.
export function secure(res: Response): Response {
  const out = new Response(res.body, res);
  for (const [k, v] of Object.entries(SECURITY_HEADERS)) out.headers.set(k, v);
  return out;
}

export const json = (data: unknown, status = 200, headers: Record<string, string> = {}): Response =>
  secure(new Response(JSON.stringify(data), { status, headers: { "content-type": "application/json; charset=utf-8", ...headers } }));

// basePath normalises EDITOR_BASE to "/segment[/segment…]" without a trailing
// slash ("" = the worker owns the whole host). null for a value that is not a
// plain path, so a typo fails closed instead of exposing the panel elsewhere.
export function basePath(env: Env): string | null {
  let base = (env.EDITOR_BASE ?? DEFAULT_BASE).trim();
  if (!base.startsWith("/")) base = "/" + base;
  base = base.replace(/\/+$/, "");
  return /^(\/[A-Za-z0-9._~-]+)*$/.test(base) && !/\/\.\.?(\/|$)/.test(base) ? base : null;
}

// underBase returns the path relative to base ("" for the base itself, else
// starting with "/"), or null when the request is outside it.
export function underBase(pathname: string, base: string): string | null {
  if (pathname === base) return "";
  return pathname.startsWith(base + "/") ? pathname.slice(base.length) : null;
}

// clientIP is the address Cloudflare saw ("unknown" only in local tests).
export const clientIP = (request: Request): string => request.headers.get("cf-connecting-ip") || "unknown";
