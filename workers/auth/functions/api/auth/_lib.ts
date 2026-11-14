// Shared helpers for the auth worker: the environment, JSON answers with the
// security headers, base64url, hashing, and the clock. A leading underscore
// keeps this file out of the Pages route table — it is imported, never served.

export interface Env {
  AUTH_DB: D1Database; // users, sessions, refresh history, login attempts
  AUTH_SIGNING_KEY?: string; // secret: the P-256 private key as a JWK (scripts/auth-keys.mjs)
  AUTH_PREVIOUS_PUBLIC_KEY?: string; // var: the retired public JWK, still listed in the JWKS while tokens it signed live
  AUTH_BOOTSTRAP_TOKEN?: string; // secret: one-time token for POST /api/auth/bootstrap
  AUTH_HASH_SALT?: string; // secret: salt for the stored IP / User-Agent hashes
  AUTH_TURNSTILE_SECRET?: string; // secret: Turnstile after repeated failures
  TURNSTILE_SECRET?: string; // shared fallback, read only when the prefixed one is unset
  AUTH_ISSUER?: string; // token `iss`; default <site origin>/api/auth
  AUTH_AUDIENCE?: string; // token `aud`; default the site host
  AUTH_TOKEN_MINUTES?: string; // access token lifetime (default 10)
  AUTH_SESSION_DAYS?: string; // session (refresh token) lifetime (default 14)
  AUTH_REFRESH_GRACE_SECONDS?: string; // two-tab refresh race window (default 30)
  AUTH_PBKDF2_ITERATIONS?: string; // password hash cost (default 100000, the production cap)
  AUTH_PASSWORD_MIN?: string; // minimum password length (default 12)
  AUTH_PASSWORD_MAX?: string; // maximum password length (default 256)
  AUTH_LOCK_ACCOUNT_FAILURES?: string; // failures per account before a lock (default 5)
  AUTH_LOCK_IP_FAILURES?: string; // failures per IP before a lock (default 20)
  AUTH_LOCK_BASE_SECONDS?: string; // first lock length, doubled per further failure (default 30)
  AUTH_LOCK_MAX_SECONDS?: string; // longest lock (default 3600)
  AUTH_TURNSTILE_AFTER?: string; // failures before Turnstile is required (default 3)
  AUTH_LOGIN_URL?: string; // where a failed form login is sent back (default /auth/login)
  AUTH_AFTER_LOGIN_URL?: string; // where a form login goes without `next` (default /)
  AUTH_KNOWN_PERMISSIONS?: string; // var: JSON array of modules and their permissions (see _registry.ts)
  // GO-134: invitations, reset links and the mail that carries them (_mail.ts).
  AUTH_INVITE_HOURS?: string; // invitation link lifetime (default 72)
  AUTH_RESET_MINUTES?: string; // password reset link lifetime (default 60)
  AUTH_MAIL_FROM?: string; // verified sender; without it no mail is sent
  AUTH_MAIL_FROM_NAME?: string; // optional display name of the sender
  AUTH_RESEND_API_KEY?: string; // secret: Resend
  AUTH_MAILCHANNELS_API_KEY?: string; // secret: MailChannels Email API
  AUTH_SMTP_HOST?: string; // your own mail server
  AUTH_SMTP_PORT?: string; // 465 = TLS from the start, 587 (default) = STARTTLS
  AUTH_SMTP_USER?: string;
  AUTH_SMTP_PASSWORD?: string; // secret
  AUTH_SMTP_HELO?: string; // EHLO name; default the sender's domain
  // GO-141: service accounts and their tokens (_service-tokens.ts).
  AUTH_SERVICE_TOKEN_MAX_DAYS?: string; // longest expiry a service token may have (default 365)
  AUTH_SERVICE_ACCESS_MINUTES?: string; // lifetime of the access token POST /api/auth/token issues (default 10)
  // GO-151: sign-in methods (_signin.ts) and login providers (_provider.ts,
  // providers/). A provider is enabled by its client id (var) and secret
  // (secret): AUTH_<GOOGLE|MICROSOFT|GITHUB|FACEBOOK>_CLIENT_ID / _CLIENT_SECRET;
  // Apple: AUTH_APPLE_CLIENT_ID, _TEAM_ID, _KEY_ID and the secret _PRIVATE_KEY.
  // Each may also have AUTH_<P>_ALLOWED_DOMAINS (comma-separated).
  AUTH_PASSWORD_LOGIN?: string; // "off" disables sign-in with a password
  AUTH_ROLE_METHODS?: string; // JSON {"<role>": ["<method>", …]}: a role counts only in sessions started so
  AUTH_RECENT_SIGNIN_MINUTES?: string; // how fresh a sign-in must be to change sign-in methods (default 10)
  AUTH_TURNSTILE_SITE_KEY?: string; // with AUTH_TURNSTILE_SECRET: the login page shows the Turnstile widget
  AUTH_MICROSOFT_TENANT?: string; // common (default) | organizations | consumers | <tenant id>
  AUTH_FACEBOOK_API_VERSION?: string; // Graph API version, e.g. v24.0
  // GO-139: Cloudflare Access as a sign-in method (_access-config.ts).
  AUTH_ACCESS_TEAM?: string; // the team name (<team>.cloudflareaccess.com); with AUTH_ACCESS_AUD
  AUTH_ACCESS_AUD?: string; // the Access application's AUD tag
  AUTH_ACCESS_LINK_BY_EMAIL?: string; // "on": an Access identity may enter the existing account with its address
  AUTH_ACCESS_AUTO_CREATE?: string; // "on": an unknown Access identity gets an account …
  AUTH_ACCESS_DEFAULT_ROLE?: string; // … with this one role (never owner, *, or a sensitive permission)
  AUTH_ACCESS_ALLOWED_DOMAINS?: string; // like AUTH_<PROVIDER>_ALLOWED_DOMAINS
  // GO-140: the second factor (docs/SECOND-FACTOR.md). Off until AUTH_MFA_KEY is set.
  AUTH_MFA_KEY?: string; // secret: encrypts the TOTP secrets; setting it switches the second factor on
  AUTH_MFA_KEY_PREVIOUS?: string; // secret: the key before a rotation, still read
  AUTH_MFA_ROLES?: string; // JSON {"<role>": true|false}: overrides which roles need a second factor
  AUTH_MFA_RESET_TOKEN?: string; // secret: one-time break-glass for an owner who lost every factor
  AUTH_STEP_UP_MINUTES?: string; // how recent a second factor must be for sensitive changes (default 10)
  // GO-138: the admin panel and its audit log (_audit.ts).
  AUTH_AUDIT_RETENTION_DAYS?: string; // how long auth_events rows are kept (default 365), purged with every write
}

// The tests replace `now` with a fake clock; nothing else should.
export const clock = { now: (): number => Date.now() };

export const iso = (ms: number): string => new Date(ms).toISOString();

// One policy for every answer: nothing inline, nothing from elsewhere, never
// framed, never cached by a shared cache, never indexed. JWKS overrides
// cache-control (public keys are meant to be cached).
const SECURITY_HEADERS: Record<string, string> = {
  "cache-control": "no-store",
  "x-robots-tag": "noindex, nofollow",
  "content-security-policy": "default-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'",
  "referrer-policy": "no-referrer",
  "x-content-type-options": "nosniff",
  "x-frame-options": "DENY",
};

// secure returns res with the security headers set; `extra` wins over them.
export function secure(res: Response, extra: Record<string, string> = {}): Response {
  const out = new Response(res.body, res);
  for (const [k, v] of Object.entries({ ...SECURITY_HEADERS, ...extra })) out.headers.set(k, v);
  return out;
}

export function json(data: unknown, status = 200, headers: Record<string, string> = {}, cookies: string[] = []): Response {
  const res = secure(new Response(JSON.stringify(data), { status, headers: { "content-type": "application/json; charset=utf-8" } }), headers);
  for (const c of cookies) res.headers.append("set-cookie", c);
  return res;
}

// redirect is a 303 for a plain HTML form: the browser follows it with a GET.
export function redirect(location: string, cookies: string[] = []): Response {
  const res = secure(new Response(null, { status: 303 }), { location });
  for (const c of cookies) res.headers.append("set-cookie", c);
  return res;
}

// intVar reads a positive integer variable, falling back to def.
export function intVar(raw: string | undefined, def: number): number {
  const n = Number.parseInt(raw ?? "", 10);
  return Number.isFinite(n) && n > 0 ? n : def;
}

export function b64urlEncode(bytes: Uint8Array): string {
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

// b64urlDecode accepts unpadded base64url only, and returns null for anything
// else rather than throwing: token parts are attacker-controlled.
export function b64urlDecode(raw: string): Uint8Array | null {
  if (!/^[A-Za-z0-9_-]*$/.test(raw) || raw.length % 4 === 1) return null;
  const bin = atob(raw.replaceAll("-", "+").replaceAll("_", "/") + "=".repeat((4 - (raw.length % 4)) % 4));
  return Uint8Array.from(bin, (c) => c.charCodeAt(0));
}

export async function sha256(input: string): Promise<Uint8Array> {
  return new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(input)));
}

export async function sha256hex(input: string): Promise<string> {
  return [...(await sha256(input))].map((b) => b.toString(16).padStart(2, "0")).join("");
}

// saltedHash is sha256(salt + value), or null without a salt or a value. An
// unsalted hash of an IPv4 address is a 2^32 lookup away from the address, so
// without AUTH_HASH_SALT nothing about the visitor is stored with a session.
export async function saltedHash(env: Env, value: string | null): Promise<string | null> {
  if (!value || !env.AUTH_HASH_SALT) return null;
  return sha256hex(env.AUTH_HASH_SALT + value);
}

// bytesEqual compares in time that depends only on the lengths, which are
// public here (both sides are digests or fixed-size derived keys).
export function bytesEqual(a: Uint8Array, b: Uint8Array): boolean {
  let diff = a.length ^ b.length;
  for (let i = 0; i < a.length; i++) diff |= a[i] ^ b[i % b.length];
  return diff === 0;
}

// secretEqual compares two strings through their SHA-256 digests, so neither
// the content nor the length of the configured secret leaks through timing.
export async function secretEqual(given: string, expected: string): Promise<boolean> {
  return bytesEqual(await sha256(given), await sha256(expected));
}

// randomToken is 32 random bytes, base64url (43 characters).
export function randomToken(): string {
  return b64urlEncode(crypto.getRandomValues(new Uint8Array(32)));
}

export const isToken = (raw: unknown): raw is string => typeof raw === "string" && /^[A-Za-z0-9_-]{43}$/.test(raw);

// clientIP is the address Cloudflare saw; "unknown" only in local tests.
export function clientIP(request: Request): string {
  return request.headers.get("cf-connecting-ip") || "unknown";
}

// normEmail is the stored form of an address: trimmed and lower-cased. It is
// not a validator; looksLikeEmail is the (deliberately loose) shape check.
export const normEmail = (raw: unknown): string => (typeof raw === "string" ? raw.trim().toLowerCase() : "");
export const looksLikeEmail = (e: string): boolean => e.length <= 254 && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(e);
