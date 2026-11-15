// The environment designer-ui is given, and what every answer carries: JSON,
// the security headers, the base path the whole worker lives under.
//
// A standalone Worker (`wrangler deploy` from workers/designer-ui). It has no
// database: the history of changes is the repository's history (README of the
// round, "Konfiguracja"). Everything about the site is read from the branch.

export interface Env {
  ASSETS?: Fetcher; // the panel files in public/, served behind the gate
  RATE_LIMITER?: RateLimit; // per-IP request volume, optional
  DESIGNER_BASE?: string; // path prefix, default /designer
  // The repository (portable/repo-config.ts reads them).
  DESIGNER_GIT_REPO?: string; // "github.com/<owner>/<repo>"
  DESIGNER_GIT_BRANCH?: string; // empty = the repository's default branch
  DESIGNER_GIT_TOKEN?: string; // a secret, never a [vars] entry
  // `auth` sessions, read by the portable verifier (src/session-verifier.ts).
  // Required: an incomplete or missing set answers 503 everywhere.
  DESIGNER_AUTH_ISSUER?: string;
  DESIGNER_AUTH_AUDIENCE?: string;
  DESIGNER_AUTH_PUBLIC_KEY?: string;
  DESIGNER_AUTH_JWKS_URL?: string;
  DESIGNER_AUTH_LOGIN_URL?: string;
}

// PREFIX: the variable prefix of this worker — DESIGNER_AUTH_*, DESIGNER_GIT_*.
export const PREFIX = "DESIGNER";
export const MODULE_ID = "designer-ui";
export const DEFAULT_BASE = "/designer";

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

// basePath normalises DESIGNER_BASE to "/segment[/segment…]" without a
// trailing slash ("" = the worker owns the whole host). null for a value that
// is not a plain path, so a typo fails closed instead of exposing the panel.
export function basePath(env: Env): string | null {
  let base = (env.DESIGNER_BASE ?? DEFAULT_BASE).trim();
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
