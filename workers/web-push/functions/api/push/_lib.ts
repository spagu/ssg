// Shared helpers for the web-push worker. A leading underscore keeps this file
// out of the Pages route table — it is imported, never served.

export interface Env {
  PUSH_DB: D1Database; // the subscriptions
  PUSH_KV: KVNamespace; // the id of the last feed item announced
  PUSH_VAPID_PUBLIC_KEY?: string; // base64url, uncompressed P-256 point (65 bytes)
  PUSH_VAPID_PRIVATE_KEY?: string; // secret: base64url, the 32-byte private scalar
  PUSH_SUBJECT?: string; // VAPID contact: "mailto:news@example.com" or an https: URL
  PUSH_KEY?: string; // secret: Bearer token for POST /api/push/notify
  PUSH_FEED_URL?: string; // the feed to announce; default /feed.xml on this origin
  PUSH_MAX_ITEMS?: string; // most items announced per notify call; default 3
  PUSH_TTL?: string; // seconds a push service keeps an undelivered message; default 86400
  PUSH_HASH_SALT?: string; // salt for the stored User-Agent hash
  PUSH_ENDPOINT_HOSTS?: string; // push services accepted, comma-separated host suffixes
}

export const json = (data: unknown, status = 200, extra: Record<string, string> = {}): Response =>
  new Response(JSON.stringify(data), {
    status,
    headers: { "content-type": "application/json", "cache-control": "no-store", ...extra },
  });

export async function sha256hex(input: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(input));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

// saltedHash returns sha256(salt + value), or null without a salt or a value.
// Without a salt nothing is stored: an unsalted hash of a User-Agent is a
// dictionary lookup away from the string itself.
export async function saltedHash(env: Env, value: string | null): Promise<string | null> {
  if (!value || !env.PUSH_HASH_SALT) return null;
  return sha256hex(env.PUSH_HASH_SALT + value);
}

export function b64urlEncode(bytes: Uint8Array): string {
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

// b64urlDecode accepts base64url with or without padding, and returns null
// for anything else rather than throwing.
export function b64urlDecode(raw: string): Uint8Array | null {
  if (!/^[A-Za-z0-9_-]*={0,2}$/.test(raw)) return null;
  const b64 = raw.replace(/=+$/, "").replaceAll("-", "+").replaceAll("_", "/");
  try {
    const bin = atob(b64 + "=".repeat((4 - (b64.length % 4)) % 4));
    return Uint8Array.from(bin, (c) => c.charCodeAt(0));
  } catch {
    return null;
  }
}

// timingSafeEqual is a constant-time compare that folds a length difference
// into the accumulator, so the length of the configured secret `b` does not
// leak through timing. Callers guarantee `b` is non-empty.
export function timingSafeEqual(a: string, b: string): boolean {
  let diff = a.length ^ b.length;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i % b.length);
  return diff === 0;
}

// sameOrigin refuses a browser request sent from another site. A missing
// Origin (curl, a server) is let through: the check stops other pages from
// enrolling their visitors' browsers here, not scripts, which can send any
// header they like.
export function sameOrigin(request: Request): boolean {
  const origin = request.headers.get("origin");
  return !origin || origin === new URL(request.url).origin;
}

// readJSON parses a small JSON body, or returns null for anything else.
export async function readJSON(request: Request, limit = 4096): Promise<Record<string, unknown> | null> {
  const raw = await request.text();
  if (raw.length > limit) return null;
  try {
    const v = JSON.parse(raw);
    return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : null;
  } catch {
    return null;
  }
}

// intVar reads a positive integer from a var, or the fallback.
export function intVar(raw: string | undefined, fallback: number): number {
  const n = Number.parseInt(raw || "", 10);
  return Number.isFinite(n) && n > 0 ? n : fallback;
}
