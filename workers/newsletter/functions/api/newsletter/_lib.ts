// Shared helpers for the newsletter worker. A leading underscore keeps this file
// out of the Pages route table — it is imported, never served.

import { verifyAccess } from "./_access";
import { can, configFrom, denied, forbidden, verifySession } from "./_session-verifier";
import { transportOf, transportProblem } from "./_transport";
import type { MailEnv } from "./_transport";

export interface Env extends MailEnv {
  NEWSLETTER_DB: D1Database;
  NEWSLETTER_TURNSTILE_SECRET?: string; // this worker's own Turnstile secret
  TURNSTILE_SECRET?: string; // shared fallback, read only when the prefixed one is unset
  NEWSLETTER_HASH_SALT?: string; // salt for the stored IP / User-Agent hashes
  NEWSLETTER_ADMIN_PASSWORD?: string; // admin via HTTP Basic (fallback when Access is not set)
  NEWSLETTER_ACCESS_TEAM?: string; // Cloudflare Access team ("myteam" or "myteam.cloudflareaccess.com")
  NEWSLETTER_ACCESS_AUD?: string; // Cloudflare Access application AUD tag
  // `auth` sessions (GO-137), shared with newsletter-ui: any one set turns the
  // mode on, an incomplete set is a 503 (see requireAdmin).
  NEWSLETTER_AUTH_ISSUER?: string; // the `iss` to accept, e.g. "https://example.com/api/auth"
  NEWSLETTER_AUTH_AUDIENCE?: string; // the `aud` to accept; default the request's host
  NEWSLETTER_AUTH_PUBLIC_KEY?: string; // the `auth` public JWK or JWKS (recommended) …
  NEWSLETTER_AUTH_JWKS_URL?: string; // … or "/api/auth/jwks" (exactly one of the two)
  NEWSLETTER_AUTH_LOGIN_URL?: string; // where a page without a session is sent, e.g. "/auth/login"
  NEWSLETTER_THANKS_URL?: string; // where a plain form POST is sent: "/{lang}/thanks/" or a JSON map
  NEWSLETTER_RETENTION_DAYS?: string; // purge pending/unsubscribed rows after N days (0/unset = keep)
  // Double opt-in: the NEWSLETTER_MAIL_* / _RESEND_ / _MAILCHANNELS_ / _SMTP_
  // settings come from MailEnv (_transport.ts). Set a sender and ONE transport
  // and every new sign-up is `pending` until the confirmation link is
  // followed; without them it is single opt-in. Deliberately prefixed with no
  // unprefixed fallback: a shared RESEND_API_KEY set for contact-form must not
  // switch this flow on (#325).
}

export const json = (data: unknown, status = 200): Response =>
  new Response(JSON.stringify(data), {
    status,
    headers: { "content-type": "application/json", "cache-control": "no-store" },
  });

export async function sha256hex(input: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(input));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

// saltedHash returns sha256(salt + value), or null without a salt or a value.
// An unsalted hash of an IPv4 address is a 2^32 lookup away from the address,
// so storing one would only pretend to protect it — no salt, nothing stored.
export async function saltedHash(env: Env, value: string | null): Promise<string | null> {
  if (!value || !env.NEWSLETTER_HASH_SALT) return null;
  return sha256hex(env.NEWSLETTER_HASH_SALT + value);
}

// turnstileSecret picks the secret the sign-up token is verified with. Every
// worker in one Pages project shares one environment (#325), so the prefixed
// name is this worker's own and the shared TURNSTILE_SECRET only a fallback.
export function turnstileSecret(env: Env): string | undefined {
  return env.NEWSLETTER_TURNSTILE_SECRET || env.TURNSTILE_SECRET;
}

export async function verifyTurnstile(secret: string, token: string, ip: string | null): Promise<boolean> {
  const body = new FormData();
  body.append("secret", secret);
  body.append("response", token);
  if (ip) body.append("remoteip", ip);
  const res = await fetch("https://challenges.cloudflare.com/turnstile/v0/siteverify", {
    method: "POST",
    body,
  });
  const out = (await res.json()) as { success: boolean };
  return out.success === true;
}

// newToken is the per-subscriber secret behind the confirm and unsubscribe
// links: 32 random bytes, base64url (43 characters). It is random rather than
// derived from the id or the email, so a link reveals nothing and cannot be
// forged, and there is no extra signing key to manage or rotate.
export function newToken(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

// isToken checks the shape before a token reaches the database, so arbitrary
// query strings never become a lookup.
export function isToken(raw: unknown): raw is string {
  return typeof raw === "string" && /^[A-Za-z0-9_-]{43}$/.test(raw);
}

// doubleOptIn reports whether confirmation mail can be sent: a sender plus one
// usable transport (a Resend or MailChannels key, or an SMTP host whose
// settings can work). The answer is global, so returning it to every caller
// says nothing about whether a particular address was already subscribed.
export function doubleOptIn(env: Env): boolean {
  return !!env.NEWSLETTER_MAIL_FROM && transportOf(env) !== null && transportProblem(env) === null;
}

// mailConfigProblem: double opt-in was asked for (a sender and a transport are
// set) but the transport can never send. Such an install must not fall back
// to single opt-in — that would confirm addresses with no proof of consent —
// nor accept sign-ups that stay `pending` forever: the sign-up endpoint
// refuses them as not configured. null when there is nothing wrong (or no mail
// configuration at all, which is single opt-in as it always was).
export function mailConfigProblem(env: Env): string | null {
  return env.NEWSLETTER_MAIL_FROM ? transportProblem(env) : null;
}

// retentionDays reads NEWSLETTER_RETENTION_DAYS; 0 (the default) keeps rows.
export function retentionDays(env: Env): number {
  const days = Number.parseInt(env.NEWSLETTER_RETENTION_DAYS || "0", 10);
  return Number.isFinite(days) && days > 0 ? days : 0;
}

// The permissions of this template's admin routes (module.json).
export const READ = "newsletter:read"; // counts, the manifest, the summary
export const EXPORT = "newsletter:export"; // the CSV export
export const SETTINGS = "newsletter:settings"; // the retention purge

// requireAdmin gates the admin routes, in the order of the `auth` contract
// (workers/auth/docs/CONTRACT.md), the same as newsletter-ui:
//   1. Cloudflare Access when NEWSLETTER_ACCESS_TEAM + AUD are set; a
//      state-changing request must come from this origin (CSRF, SR-10);
//   2. `auth` sessions when any NEWSLETTER_AUTH_* is set (GO-137): the token
//      must hold `permission`. An incomplete set is a 503 naming what is
//      missing, and with a complete set the password is NOT accepted: it
//      would bypass the per-action permissions. A state-changing request
//      carried by the cookie must also come from this origin (CSRF);
//   3. HTTP Basic with NEWSLETTER_ADMIN_PASSWORD.
// Modes 1 and 3 grant every permission, as before. Returns null when
// authorised, or a Response to return otherwise.
export async function requireAdmin(request: Request, env: Env, permission: string): Promise<Response | null> {
  if (env.NEWSLETTER_ACCESS_TEAM && env.NEWSLETTER_ACCESS_AUD) {
    const refused = await verifyAccess(request, env.NEWSLETTER_ACCESS_TEAM, env.NEWSLETTER_ACCESS_AUD);
    // Access lets through any request with a valid CF_Authorization cookie,
    // which the browser attaches by itself: a write must come from this
    // origin, as in mode 2 (security review SR-10).
    if (!refused && !["GET", "HEAD"].includes(request.method) && !sameOrigin(request)) return json({ error: "cross-origin request refused" }, 403);
    return refused;
  }
  const auth = configFrom(env, "NEWSLETTER");
  if (auth) {
    const v = await verifySession(request, auth);
    if (!v.ok) return denied(request, auth, v.reason);
    if (!can(v.perms, permission)) return forbidden(permission);
    if (v.kind === "user" && !["GET", "HEAD"].includes(request.method) && !sameOrigin(request)) {
      return json({ error: "cross-origin request refused" }, 403);
    }
    return null;
  }
  const expected = env.NEWSLETTER_ADMIN_PASSWORD;
  if (!expected) return json({ error: "admin not configured" }, 503);
  const header = request.headers.get("authorization") || "";
  if (header.startsWith("Basic ")) {
    try {
      const decoded = atob(header.slice(6));
      const pass = decoded.slice(decoded.indexOf(":") + 1);
      if (timingSafeEqual(pass, expected)) return null;
    } catch {
      /* malformed header falls through to a 401 */
    }
  }
  return json({ error: "unauthorized" }, 401);
}

// sameOrigin: the CSRF rule for a state-changing request authenticated by the
// session cookie, as the `auth` worker applies it to its own forms — no
// cross-origin evidence (an Origin, when sent, is this origin; Sec-Fetch-Site,
// when sent, is "same-origin") and positive proof of one of the two. Every
// current browser sends Origin on a POST; a script uses a bearer token.
export function sameOrigin(request: Request): boolean {
  const own = new URL(request.url).origin;
  const origin = request.headers.get("origin");
  const site = request.headers.get("sec-fetch-site");
  if ((origin !== null && origin !== own) || (site !== null && site !== "same-origin")) return false;
  return origin === own || site === "same-origin";
}

// Constant-time compare that folds a length difference into the accumulator, so
// the length of the configured secret `b` does not leak through timing. Callers
// guarantee `b` is non-empty.
function timingSafeEqual(a: string, b: string): boolean {
  let diff = a.length ^ b.length;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i % b.length);
  return diff === 0;
}
