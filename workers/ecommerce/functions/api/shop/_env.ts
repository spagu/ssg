// The shop's bindings and settings, and the one place that decides whether a
// feature is configured at all.
//
// A leading underscore keeps this file out of the Pages route table — it is
// imported, never served.
//
// Design rule: a missing binding is a clean 503 with a sentence that says what
// to bind, never an unhandled exception (a raw Cloudflare 500 tells the shop
// owner nothing). The same rule the comments worker follows.

export interface Env {
  // ── Storage ──────────────────────────────────────────────────────────────
  /** D1: orders, products, invoices, everything durable. Required. */
  SHOP_DB: D1Database;
  /** R2: the product files themselves. Required for digital delivery. */
  SHOP_FILES?: R2Bucket;
  /** KV: sessions, token blacklists, download windows, login counters. */
  SHOP_KV?: KVNamespace;
  /** Workers Rate Limiting binding, shared with the rate-limit middleware. */
  RATE_LIMITER?: { limit(o: { key: string }): Promise<{ success: boolean }> };

  // ── Shop behaviour (vars) ────────────────────────────────────────────────
  /** Comma-separated gateway names in button order, e.g. "stripe,paypal". */
  SHOP_GATEWAYS?: string;
  /** "sandbox" | "live" — picks the PayPal API host. Never a free-form URL. */
  SHOP_PAYPAL_ENV?: string;
  /** Fallback currency when a request names none. */
  SHOP_BASE_CURRENCY?: string;
  /** Upload cap for product files. */
  SHOP_MAX_FILE_MB?: string;
  /** 0 = never purge personal data automatically. */
  SHOP_RETENTION_DAYS?: string;
  /** Set by tests so the rate limiter can be absent without failing closed. */
  SHOP_TEST?: string;

  // ── Payments (secrets) ───────────────────────────────────────────────────
  STRIPE_SECRET_KEY?: string;
  STRIPE_WEBHOOK_SECRET?: string;
  PAYPAL_CLIENT_ID?: string;
  PAYPAL_CLIENT_SECRET?: string;
  PAYPAL_WEBHOOK_ID?: string;

  // ── Admin authentication ─────────────────────────────────────────────────
  /** Cloudflare Access team ("myteam" or "myteam.cloudflareaccess.com"). */
  SHOP_ACCESS_TEAM?: string;
  /** Cloudflare Access application AUD tag. */
  SHOP_ACCESS_AUD?: string;
  /** Comma-separated emails that get the owner role under Access. */
  SHOP_ACCESS_OWNERS?: string;
  /** HS256 key for the built-in JWT mode. At least 32 bytes. */
  SHOP_JWT_SECRET?: string;
  /** "email:password" — creates the first owner, then should be deleted. */
  SHOP_ADMIN_BOOTSTRAP?: string;

  // ── Anti-abuse ───────────────────────────────────────────────────────────
  TURNSTILE_SECRET?: string;
  /** Salt for the stored IP hash. Without it no IP is recorded at all. */
  SHOP_IP_SALT?: string;

  // ── Outgoing mail ────────────────────────────────────────────────────────
  SHOP_MAIL_URL?: string;
  SHOP_MAIL_KEY?: string;
  SHOP_MAIL_FROM?: string;
  SHOP_MAIL_ADMIN?: string;

  // ── Server-side tracking ─────────────────────────────────────────────────
  GA4_MEASUREMENT_ID?: string;
  GA4_API_SECRET?: string;
  META_PIXEL_ID?: string;
  META_ACCESS_TOKEN?: string;
}

/** Which admin authentication mode this deployment is in. The two are
 *  mutually exclusive: with Access configured there is no password to guess,
 *  so the login endpoint stops existing (S-15). */
export type AuthMode = "access" | "jwt" | "none";

export function authMode(env: Env): AuthMode {
  if (env.SHOP_ACCESS_TEAM && env.SHOP_ACCESS_AUD) return "access";
  if (env.SHOP_JWT_SECRET && env.SHOP_JWT_SECRET.length >= 32) return "jwt";
  return "none";
}

/** The gateways that are both enabled and actually configured.
 *
 *  A gateway named in SHOP_GATEWAYS but missing its secrets is dropped rather
 *  than offered: a checkout button that always 500s is worse than one button. */
export function enabledGateways(env: Env): string[] {
  const wanted = (env.SHOP_GATEWAYS ?? "stripe")
    .split(",")
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean);
  return wanted.filter((name) => {
    if (name === "stripe") return Boolean(env.STRIPE_SECRET_KEY);
    if (name === "paypal") return Boolean(env.PAYPAL_CLIENT_ID && env.PAYPAL_CLIENT_SECRET);
    return false;
  });
}

/** True when the keys in use are test/sandbox keys, so the admin panel and the
 *  storefront can say so loudly. A shop that takes real money while the owner
 *  thinks it is in test mode is the expensive direction of this mistake. */
export function isTestMode(env: Env): boolean {
  if (env.STRIPE_SECRET_KEY?.startsWith("sk_test_")) return true;
  if ((env.SHOP_PAYPAL_ENV ?? "sandbox") === "sandbox" && env.PAYPAL_CLIENT_ID) return true;
  return false;
}

export function paypalBase(env: Env): string {
  return (env.SHOP_PAYPAL_ENV ?? "sandbox") === "live"
    ? "https://api-m.paypal.com"
    : "https://api-m.sandbox.paypal.com";
}

export function maxFileBytes(env: Env): number {
  const mb = Number.parseInt(env.SHOP_MAX_FILE_MB ?? "100", 10);
  return (Number.isFinite(mb) && mb > 0 ? mb : 100) * 1024 * 1024;
}
