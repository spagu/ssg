// Shared helpers for the newsletter worker. A leading underscore keeps this file
// out of the Pages route table — it is imported, never served.

import { verifyAccess } from "./_access";

export interface Env {
  NEWSLETTER_DB: D1Database;
  NEWSLETTER_TURNSTILE_SECRET?: string; // this worker's own Turnstile secret
  TURNSTILE_SECRET?: string; // shared fallback, read only when the prefixed one is unset
  NEWSLETTER_HASH_SALT?: string; // salt for the stored IP / User-Agent hashes
  NEWSLETTER_ADMIN_PASSWORD?: string; // admin via HTTP Basic (fallback when Access is not set)
  NEWSLETTER_ACCESS_TEAM?: string; // Cloudflare Access team ("myteam" or "myteam.cloudflareaccess.com")
  NEWSLETTER_ACCESS_AUD?: string; // Cloudflare Access application AUD tag
  NEWSLETTER_THANKS_URL?: string; // where a plain form POST is sent: "/{lang}/thanks/" or a JSON map
  NEWSLETTER_RETENTION_DAYS?: string; // purge pending/unsubscribed rows after N days (0/unset = keep)
  // Double opt-in. Set a sender and ONE provider key and every new sign-up is
  // `pending` until the confirmation link is followed; without them it is
  // single opt-in. Deliberately prefixed with no unprefixed fallback: a shared
  // RESEND_API_KEY set for contact-form must not switch this flow on (#325).
  NEWSLETTER_MAIL_FROM?: string; // verified sender, e.g. "news@example.com"
  NEWSLETTER_MAIL_FROM_NAME?: string; // optional display name
  NEWSLETTER_RESEND_API_KEY?: string; // Resend
  NEWSLETTER_MAILCHANNELS_API_KEY?: string; // MailChannels Email API
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
// provider key. The answer is global, so returning it to every caller says
// nothing about whether a particular address was already subscribed.
export function doubleOptIn(env: Env): boolean {
  return !!env.NEWSLETTER_MAIL_FROM && !!(env.NEWSLETTER_RESEND_API_KEY || env.NEWSLETTER_MAILCHANNELS_API_KEY);
}

// retentionDays reads NEWSLETTER_RETENTION_DAYS; 0 (the default) keeps rows.
export function retentionDays(env: Env): number {
  const days = Number.parseInt(env.NEWSLETTER_RETENTION_DAYS || "0", 10);
  return Number.isFinite(days) && days > 0 ? days : 0;
}

// requireAdmin gates the admin endpoint. Two mutually-exclusive modes, as in
// the comments worker:
//   - Cloudflare Access (recommended) when NEWSLETTER_ACCESS_TEAM + AUD are set;
//   - HTTP Basic with NEWSLETTER_ADMIN_PASSWORD otherwise.
// Returns null when authorised, or a Response to return otherwise.
export async function requireAdmin(request: Request, env: Env): Promise<Response | null> {
  if (env.NEWSLETTER_ACCESS_TEAM && env.NEWSLETTER_ACCESS_AUD) {
    return verifyAccess(request, env.NEWSLETTER_ACCESS_TEAM, env.NEWSLETTER_ACCESS_AUD);
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

// Constant-time compare that folds a length difference into the accumulator, so
// the length of the configured secret `b` does not leak through timing. Callers
// guarantee `b` is non-empty.
function timingSafeEqual(a: string, b: string): boolean {
  let diff = a.length ^ b.length;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i % b.length);
  return diff === 0;
}
