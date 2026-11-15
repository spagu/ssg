// What every answer of the worker carries: JSON, the security headers, the
// base path the whole worker lives under, and the limit of one request body.
// Shared by the panel workers: the source is workers/editor-ui/portable/,
// every other template holds a byte-identical copy (GO-185). The template's
// own names (its id, variable prefix, default base) and its Env live in its
// src/http.ts.

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

// MAX_BODY: one request body (a publication, an MCP call) on the Free plan;
// plan.ts BODY_LIMIT holds each plan's. A change at the Free plan's 2 MB
// limit (repo-provider.ts) escaped into JSON, or a theme file as base64 (a
// third larger than its bytes), stays under it; the provider enforces the
// real limits per file and change.
export const MAX_BODY = 8 * 1024 * 1024;

// secure returns the response with the security headers set. Responses from
// fetch (the assets binding) have immutable headers, so it always copies.
export function secure(res: Response): Response {
  const out = new Response(res.body, res);
  for (const [k, v] of Object.entries(SECURITY_HEADERS)) out.headers.set(k, v);
  return out;
}

export const json = (data: unknown, status = 200, headers: Record<string, string> = {}): Response =>
  secure(new Response(JSON.stringify(data), { status, headers: { "content-type": "application/json; charset=utf-8", ...headers } }));

// normalizeBase turns the {PREFIX}_BASE value (unset: `fallback`) into
// "/segment[/segment...]" without a trailing slash ("" = the worker owns the
// whole host). null for a value that is not a plain path, so a typo fails
// closed instead of exposing the panel elsewhere.
export function normalizeBase(value: string | undefined, fallback: string): string | null {
  let base = (value ?? fallback).trim();
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
