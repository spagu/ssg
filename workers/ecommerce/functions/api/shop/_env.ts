// The shop's bindings and settings, and the one place that decides whether a
// feature is configured at all.
//
// A leading underscore keeps this file out of the Pages route table — it is
// imported, never served.
//
// Design rule: a missing binding is a clean 503 with a sentence that says what
// to bind, never an unhandled exception (a raw Cloudflare 500 tells the shop
// owner nothing). The same rule the comments worker follows.

/** Cloudflare's Workers Rate Limiting binding. */
export interface RateLimitBinding {
  limit(options: { key: string }): Promise<{ success: boolean }>;
}

export interface Env {
  // ── Storage ──────────────────────────────────────────────────────────────
  /** D1: orders, products, invoices, everything durable. Required. */
  SHOP_DB: D1Database;
  /** R2: the product files themselves. Required for digital delivery. */
  SHOP_FILES?: R2Bucket;
  /** KV: download windows and request-budget counters. */
  SHOP_KV?: KVNamespace;
  /** Workers Rate Limiting binding, used for any bucket with no binding of its
   *  own. Shared with the project-wide rate-limit middleware. */
  RATE_LIMITER?: RateLimitBinding;
  /** Per-bucket bindings. The binding's limit is configured in wrangler rather
   *  than in the code, so one binding cannot serve two different budgets —
   *  which is why these exist rather than one for everything (see _ratelimit). */
  RATE_LIMIT_CHECKOUT?: RateLimitBinding;
  RATE_LIMIT_RESEND?: RateLimitBinding;
  RATE_LIMIT_DOWNLOAD?: RateLimitBinding;
  RATE_LIMIT_STATUS?: RateLimitBinding;
  RATE_LIMIT_ADMIN?: RateLimitBinding;

  // ── Shop behaviour (vars) ────────────────────────────────────────────────
  /** Comma-separated gateway names in button order. Default "stripe,paypal":
   *  both are offered, each once its secrets are set (enabledGateways). */
  SHOP_GATEWAYS?: string;
  /** "sandbox" | "live" — picks the PayPal API host. Never a free-form URL. */
  SHOP_PAYPAL_ENV?: string;
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

  // ── Admin authentication (GO-155; _auth.ts) ──────────────────────────────
  // Three modes, the first configured wins: Access → auth → the password.
  /** Cloudflare Access team ("myteam" or "myteam.cloudflareaccess.com"). */
  SHOP_ACCESS_TEAM?: string;
  /** Cloudflare Access application AUD tag. */
  SHOP_ACCESS_AUD?: string;
  /** `auth` sessions (workers/auth/docs/CONTRACT.md): any one set turns the
   *  mode on; an incomplete set is a 503. The `iss` to accept, e.g.
   *  "https://example.com/api/auth". */
  SHOP_AUTH_ISSUER?: string;
  /** Where a browser without a session signs in, e.g. "/auth/login". */
  SHOP_AUTH_LOGIN_URL?: string;
  /** The `auth` public JWK or JWKS (recommended) … */
  SHOP_AUTH_PUBLIC_KEY?: string;
  /** … or "/api/auth/jwks" (exactly one of the two). */
  SHOP_AUTH_JWKS_URL?: string;
  /** The `aud` to accept; default the request's host. */
  SHOP_AUTH_AUDIENCE?: string;
  /** The shared admin password (a secret), sent by the panel as HTTP Basic. */
  SHOP_ADMIN_PASSWORD?: string;
  /** Password mode: wrong passwords per address before it is locked (10). */
  SHOP_ADMIN_MAX_FAILURES?: string;
  /** Password mode: the counting window and the lock, in minutes (10). */
  SHOP_ADMIN_LOCKOUT_MINUTES?: string;

  // ── Selling through AI agents: ACP checkout (GO-163; _acp-*.ts) ──────────
  // Off unless SHOP_ACP_ENABLED is "true": every /api/shop/acp/* route is a
  // 404 then. On but missing a secret below is a 503, never an open endpoint.
  /** "true" turns the Agentic Commerce Protocol checkout on. */
  SHOP_ACP_ENABLED?: string;
  /** Secret: the bearer token the agent platform sends (Authorization). */
  SHOP_ACP_API_KEY?: string;
  /** Secret, optional: when set, every request must carry a valid
   *  Signature + Timestamp pair (see _acp-auth.ts). */
  SHOP_ACP_SIGNING_SECRET?: string;
  /** The platform's order-events receiver, e.g.
   *  "https://agent.example.com/agentic_checkout/webhooks/order_events". */
  SHOP_ACP_WEBHOOK_URL?: string;
  /** Secret: the HMAC key for the Merchant-Signature on those events. */
  SHOP_ACP_WEBHOOK_SECRET?: string;
  /** The Stripe account or profile id the platform scopes payment tokens to
   *  (the payment handler's config.merchant_id). */
  SHOP_ACP_MERCHANT_ID?: string;
  /** Stripe-Version sent with Shared Payment Token calls (a preview API). */
  SHOP_ACP_STRIPE_VERSION?: string;

  // ── Selling through AI agents: UCP checkout (GO-167; _ucp-*.ts, UCP.md) ──
  // Off unless SHOP_UCP_ENABLED is "true": every /api/shop/ucp/* route and
  // /.well-known/ucp are a 404 then. On but missing a secret is a 503.
  /** "true" turns the Universal Commerce Protocol checkout on. */
  SHOP_UCP_ENABLED?: string;
  /** Secret: the API key the platform sends in X-API-Key. */
  SHOP_UCP_API_KEY?: string;
  /** Secret: the private EC P-256 JWK (with a `kid`) that signs order
   *  webhooks; its public half is published in /.well-known/ucp. */
  SHOP_UCP_SIGNING_KEY?: string;
  /** Stripe's publishable key (pk_…), which Google Pay needs to tokenize a
   *  card for Stripe. Not a secret: it is in the public profile. */
  SHOP_UCP_STRIPE_PUBLISHABLE_KEY?: string;
  /** "TEST" (default) or "PRODUCTION" — the Google Pay environment. */
  SHOP_UCP_GPAY_ENVIRONMENT?: string;
  /** The Google Pay & Wallet Console merchant id; required in PRODUCTION. */
  SHOP_UCP_GPAY_MERCHANT_ID?: string;
  /** This shop's id for its Google Pay handler instance (default "gpay"). */
  SHOP_UCP_HANDLER_ID?: string;
  /** The platform's order-webhook URL, without credentials. Unset = no
   *  order events are sent. */
  SHOP_UCP_WEBHOOK_URL?: string;
  /** Secret: the API key for that URL, sent as X-Goog-Api-Key. */
  SHOP_UCP_WEBHOOK_API_KEY?: string;

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

/** The gateways offered by default, in button order (GO-158). */
export const DEFAULT_GATEWAYS = "stripe,paypal";

/** The gateways that are both enabled and actually configured.
 *
 *  A gateway named in SHOP_GATEWAYS but missing its secrets is dropped rather
 *  than offered: a checkout button that always 500s is worse than one button. */
export function enabledGateways(env: Env): string[] {
  const wanted = (env.SHOP_GATEWAYS ?? DEFAULT_GATEWAYS)
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
