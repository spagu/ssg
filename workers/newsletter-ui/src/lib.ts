// Shared helpers for the newsletter-ui worker: the environment it is given,
// JSON answers, the security headers every response carries, and the base path
// the whole worker lives under.
//
// This is a standalone Worker, not Pages Functions: it is deployed with its own
// `wrangler deploy` and shares only the D1 database with the `newsletter`
// template (binding NEWSLETTER_DB). Pages Functions have no cron trigger and no
// queue consumer, and a campaign cannot be sent or resumed reliably without both.

import type { MailEnv } from "./mail/_transport";

// The mail settings (sender, Resend / MailChannels key, SMTP) come from
// MailEnv, under the same names as the `newsletter` template, so one
// configuration serves both workers.
export interface Env extends MailEnv {
  NEWSLETTER_DB: D1Database; // the same database the `newsletter` template writes
  // The site's origin, e.g. "https://example.com": where the `newsletter`
  // template's unsubscribe endpoint lives. Required to preview, test or
  // schedule a campaign (every mail links back to it).
  NEWSLETTER_SITE_URL?: string;
  NEWSLETTER_QUEUE?: Queue; // send jobs (producer + consumer in wrangler.toml)
  RATE_LIMITER?: RateLimit; // per-IP request volume, optional
  ASSETS?: Fetcher; // the panel files in public/, served behind admin auth
  NEWSLETTER_UI_BASE?: string; // path prefix, default /newsletter-admin
  // Admin auth — the SAME names as the `newsletter` admin, so one configuration
  // serves both workers. Access wins when both of its values are set.
  NEWSLETTER_ACCESS_TEAM?: string; // "myteam" or "myteam.cloudflareaccess.com"
  NEWSLETTER_ACCESS_AUD?: string; // the Access application's AUD tag
  NEWSLETTER_ADMIN_PASSWORD?: string; // HTTP Basic password
  NEWSLETTER_HASH_SALT?: string; // salt for the short-lived failed-login records
  NEWSLETTER_AUTH_MAX_FAILURES?: string; // failed Basic logins per IP per window (default 10)
  NEWSLETTER_AUTH_WINDOW_MINUTES?: string; // the window and the lockout length (default 10)
  // `auth` sessions (GO-137), read by the portable verifier (src/session-verifier.ts):
  // any one set turns the mode on, an incomplete set is a 503. Shared with the
  // `newsletter` template. Not the two lines above: those are Basic's lockout.
  NEWSLETTER_AUTH_ISSUER?: string; // the `iss` to accept, e.g. "https://example.com/api/auth"
  NEWSLETTER_AUTH_AUDIENCE?: string; // the `aud` to accept; default the request's host
  NEWSLETTER_AUTH_PUBLIC_KEY?: string; // the `auth` public JWK or JWKS (recommended) …
  NEWSLETTER_AUTH_JWKS_URL?: string; // … or "/api/auth/jwks" (exactly one of the two)
  NEWSLETTER_AUTH_LOGIN_URL?: string; // where a page without a session is sent, e.g. "/auth/login"
  NEWSLETTER_AUDIT_RETENTION_DAYS?: string; // audit rows kept this many days (default 365, 0 = forever)
  NEWSLETTER_IMPORT_MAX_ROWS?: string; // rows per CSV import (default 20000); the byte cap is 1 KiB per row
  // Send engine (GO-126); defaults and reasoning in src/engine-config.ts.
  NEWSLETTER_SEND_RATE?: string; // messages per minute, all campaigns together (default 60)
  NEWSLETTER_SEND_BATCH?: string; // messages per queue job (default 10, safe for the free plan)
  NEWSLETTER_SEND_STALE_MINUTES?: string; // when a claim / a queued batch counts as abandoned (default 30)
  NEWSLETTER_SEND_MAX_ATTEMPTS?: string; // tries per recipient before `failed` (default 3)
  NEWSLETTER_DEAD_LETTER_QUEUE?: string; // the dead-letter queue's name (default ssg-newsletter-send-dlq)
  NEWSLETTER_BOUNCE_BREAKER_MIN?: string; // hard bounces in one batch before the campaign pauses (default 5) …
  NEWSLETTER_BOUNCE_BREAKER_PERCENT?: string; // … if they are also this share of its messages (default 50)
  // Bounces (GO-127); see src/bounce-*.ts and src/hook-*.ts.
  NEWSLETTER_SOFT_BOUNCE_LIMIT?: string; // soft bounces in a row before a suppression (default 3)
  NEWSLETTER_RESEND_WEBHOOK_SECRET?: string; // "whsec_…" from Resend's webhook page; unset → the hook answers 503
  NEWSLETTER_MAILCHANNELS_WEBHOOK_KEY?: string; // MailChannels' Ed25519 public key (PEM); unset → 503
  // Campaign from a post (GO-130): the site feed, absolute https:// or a path
  // on NEWSLETTER_SITE_URL (default /feed.xml, what ssg's `feed: true` writes).
  NEWSLETTER_FEED_URL?: string;
  NEWSLETTER_CONFIRM_BATCH?: string; // confirmation mails per call to /api/subscribers/confirmations (default 10)
  // Campaign statistics (GO-129).
  NEWSLETTER_TRACK_CLICKS?: string; // "true" rewrites links through {base}/hooks/c/… (default off)
  NEWSLETTER_UNSUBSCRIBE_WINDOW_DAYS?: string; // unsubscribe attribution window after a send (default 14)
}

export const DEFAULT_BASE = "/newsletter-admin";

// One policy for JSON and for the panel's HTML: nothing inline, nothing from
// another origin. A JSON answer that is somehow rendered as a page can then run
// nothing either.
export const CSP = [
  "default-src 'none'",
  "script-src 'self'",
  "style-src 'self'",
  "img-src 'self' data:",
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
  secure(
    new Response(JSON.stringify(data), {
      status,
      headers: { "content-type": "application/json; charset=utf-8", ...headers },
    }),
  );

// basePath normalises NEWSLETTER_UI_BASE to "/segment[/segment…]" without a
// trailing slash ("" means the worker owns the whole host). Returns null for a
// value that is not a plain path, so a typo fails closed instead of exposing
// the panel somewhere unexpected.
export function basePath(env: Env): string | null {
  let base = (env.NEWSLETTER_UI_BASE ?? DEFAULT_BASE).trim();
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

// intVar reads a positive integer variable, falling back to def.
export function intVar(raw: string | undefined, def: number): number {
  const n = Number.parseInt(raw ?? "", 10);
  return Number.isFinite(n) && n > 0 ? n : def;
}

export async function sha256hex(input: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(input));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

// clientIP is the address Cloudflare saw. Behind Cloudflare the header is always
// set; "unknown" only happens in local tests and buckets them together.
export function clientIP(request: Request): string {
  return request.headers.get("cf-connecting-ip") || "unknown";
}
