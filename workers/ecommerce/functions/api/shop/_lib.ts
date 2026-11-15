// Shared helpers: responses, errors, ids, hashing, validation.
//
// A leading underscore keeps this file out of the Pages route table.

import type { Env } from "./_env";

/** One error shape for the whole API, so a client can branch on `error` and
 *  show `message`. Never a stack trace, never SQL (S-29). */
export interface ApiError {
  error: string;
  message: string;
  requestId?: string;
}

export const json = (data: unknown, status = 200, headers: Record<string, string> = {}): Response =>
  new Response(JSON.stringify(data), {
    status,
    headers: {
      "content-type": "application/json; charset=utf-8",
      "cache-control": "no-store",
      "x-content-type-options": "nosniff",
      "referrer-policy": "no-referrer",
      ...headers,
    },
  });

/** fail builds the standard error body. `requestId` is the Cloudflare ray, so a
 *  support email can be matched to a log line without asking for a screenshot. */
export function fail(request: Request, code: string, message: string, status: number): Response {
  const body: ApiError = { error: code, message };
  const ray = request.headers.get("cf-ray");
  if (ray) body.requestId = ray;
  return json(body, status);
}

/** Cacheable GET responses (the two public product reads) opt in explicitly. */
export const cached = (data: unknown, seconds: number): Response =>
  json(data, 200, { "cache-control": `public, max-age=${seconds}` });

export const newId = (): string => crypto.randomUUID();

export function nowISO(): string {
  return new Date().toISOString();
}

export async function sha256hex(input: string | ArrayBuffer): Promise<string> {
  const data = typeof input === "string" ? new TextEncoder().encode(input) : input;
  const digest = await crypto.subtle.digest("SHA-256", data);
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

/** A URL-safe random secret: download tokens, order keys, refresh tokens. */
export function randomToken(bytes = 32): string {
  const raw = crypto.getRandomValues(new Uint8Array(bytes));
  return base64url(raw);
}

export function base64url(bytes: Uint8Array): string {
  let binary = "";
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

export function base64urlToBytes(s: string): Uint8Array {
  let b = s.replaceAll("-", "+").replaceAll("_", "/");
  while (b.length % 4) b += "=";
  const bin = atob(b);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

/** Constant-time string compare. A length difference is folded into the
 *  accumulator rather than returned early, so the secret's length does not leak
 *  through timing — the same shape the comments worker uses. */
export function timingSafeEqual(a: string, b: string): boolean {
  if (b.length === 0) return false;
  let diff = a.length ^ b.length;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i % b.length);
  return diff === 0;
}

/** The visitor's IP, kept only as a salted hash (S-28). Without a salt
 *  configured nothing is recorded: an unsalted hash of an IPv4 address is
 *  reversible by brute force in seconds, so it would be PII wearing a hat. */
export async function ipHash(request: Request, env: Env): Promise<string | null> {
  const ip = request.headers.get("cf-connecting-ip");
  if (!ip || !env.SHOP_IP_SALT) return null;
  return sha256hex(`${env.SHOP_IP_SALT}:${ip}`);
}

/** The country Cloudflare resolved for this request. Not PII, and one of the
 *  two pieces of evidence the VAT rules need (S-28, tax evidence). */
export function requestCountry(request: Request): string | null {
  const cf = (request as Request & { cf?: { country?: string } }).cf;
  const country = cf?.country;
  return typeof country === "string" && /^[A-Z]{2}$/.test(country) ? country : null;
}

export function userAgent(request: Request): string {
  return (request.headers.get("user-agent") ?? "").slice(0, 512);
}

// ── Validation ──────────────────────────────────────────────────────────────
//
// Every field has a pattern and a length. We do not "sanitise" input — it is
// stored as given and escaped on the way out (S-25), because sanitising input
// silently changes what someone typed and still gets the escaping wrong.

export const SKU_RE = /^[A-Za-z0-9._-]{1,64}$/;
export const EMAIL_RE = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;
export const COUNTRY_RE = /^[A-Za-z]{2}$/;
export const CURRENCY_RE = /^[A-Za-z]{3}$/;

export function str(value: unknown, max: number): string {
  return typeof value === "string" ? value.trim().slice(0, max) : "";
}

export function isValidEmail(value: string): boolean {
  return value.length <= 254 && EMAIL_RE.test(value);
}

/** A VAT id with spaces, dots and dashes removed, upper-cased. Format checking
 *  per country lives in _tax; this only normalises so two spellings of the same
 *  number compare equal. */
export function normaliseVatId(raw: string): string {
  return raw.replace(/[\s.-]/g, "").toUpperCase().slice(0, 20);
}

/** Reads a JSON body, or returns null when it is not JSON. Callers turn that
 *  into a 400 — an unparseable body is a client bug, not a server error. */
export async function readJson<T>(request: Request): Promise<T | null> {
  try {
    return (await request.json()) as T;
  } catch {
    return null;
  }
}

/** Accepts both JSON and form-encoded bodies, so the storefront works without
 *  JavaScript (the checkout page posts a plain form) exactly as the
 *  contact-form worker does. */
export async function readBody<T extends object>(request: Request): Promise<T | null> {
  const type = request.headers.get("content-type") ?? "";
  if (type.includes("application/json")) return readJson<T>(request);
  if (type.includes("form")) {
    try {
      const form = await request.formData();
      const out: Record<string, unknown> = {};
      for (const [k, v] of form.entries()) out[k] = String(v);
      return out as T;
    } catch {
      return null;
    }
  }
  return readJson<T>(request);
}

/** Escapes text for HTML interpolation (invoices, emails). The panel uses
 *  textContent instead (S-22); this is for the places that build markup. */
export function escapeHtml(value: unknown): string {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

/** Turnstile verification, shared by checkout and resend. Optional by design:
 *  with no secret configured the shop still works (S-27). */
export async function verifyTurnstile(secret: string, token: string, ip: string | null): Promise<boolean> {
  const body = new FormData();
  body.append("secret", secret);
  body.append("response", token);
  if (ip) body.append("remoteip", ip);
  try {
    const res = await fetch("https://challenges.cloudflare.com/turnstile/v0/siteverify", {
      method: "POST",
      body,
    });
    const out = (await res.json()) as { success?: boolean };
    return out.success === true;
  } catch {
    return false;
  }
}

/** A CSV cell that cannot become a formula when the accountant opens the export
 *  in a spreadsheet (S-25). */
export function csvCell(value: unknown): string {
  const s = String(value ?? "");
  const guarded = /^[=+\-@\t\r]/.test(s) ? `'${s}` : s;
  return `"${guarded.replaceAll('"', '""')}"`;
}

/** Strips the characters that would let a filename break out of a
 *  Content-Disposition header (S-13). */
export function safeFilename(name: string): string {
  return name.replace(/[\r\n";\\]/g, "_").slice(0, 200) || "download";
}
