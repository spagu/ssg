// A request budget for the endpoints a stranger can reach.
//
// The shop's public surface is small but expensive: checkout writes an order
// and calls a payment provider, resend sends email, download streams a file out
// of storage. Turnstile raises the cost of abusing those; it does not cap it,
// and it is optional in the first place.
//
// One budget for the whole API would be wrong here. Three resends an hour is a
// sensible cap and would make the thank-you page — which polls every second
// while it waits for a webhook — unusable. So the budget is per bucket, and the
// buckets are named after what they protect rather than after a route, because
// two routes can share a cost.
//
// Backends, in the order they are used:
//
//   RATE_LIMIT_<BUCKET>  a Workers Rate Limiting binding for one bucket. Exact
//                        per point of presence and free, but its limit is
//                        configured in wrangler.toml rather than here, so the
//                        numbers below are the fallback's, not this one's.
//   RATE_LIMITER         a generic binding, used for any bucket without its own.
//   SHOP_KV              counters. Approximate — KV is eventually consistent,
//                        so a burst arriving at several points of presence can
//                        overshoot — but it honours the per-bucket numbers.
//
// With none of them bound the shop is unchanged and unprotected, which is
// stated in the panel rather than left to be discovered.
//
// This is deliberately not the project-wide `rate-limit` worker
// (workers/rate-limit). That one is middleware with a single budget for
// everything under /api/, which composes with any project; it can sit in front
// of this and they do different jobs. Sharing code between them would mean a
// shared module above both functions/ trees, which Pages cannot deploy — see
// the note in _access.ts.

import type { Env } from "./_env";
import { ipHash } from "./_lib";

/** How much of what, per caller. */
export interface Budget {
  /** Requests allowed per window. */
  max: number;
  /** Window, in seconds. */
  windowS: number;
  /** What to do when the backend itself fails.
   *
   *  Open for reads: losing a buyer's download because KV had a bad minute is
   *  worse than serving one extra. Closed for anything that writes or spends
   *  money, where the failure mode of open is somebody else's bill. */
  onError: "open" | "closed";
}

export const BUDGETS = {
  /** Writes an order and calls a payment provider. */
  checkout: { max: 10, windowS: 60, onError: "closed" },
  /** Sends email to an address the caller chose. */
  resend: { max: 3, windowS: 600, onError: "closed" },
  /** Streams a file. Ranged continuations are already free (see _downloads). */
  download: { max: 60, windowS: 60, onError: "open" },
  /** Polled by the thank-you page while it waits for the webhook. */
  status: { max: 120, windowS: 60, onError: "open" },
  /** The panel, behind authentication already; this is for a stolen token. */
  admin: { max: 300, windowS: 60, onError: "open" },
} as const satisfies Record<string, Budget>;

export type BucketName = keyof typeof BUDGETS;

interface RateLimiter {
  limit(options: { key: string }): Promise<{ success: boolean }>;
}

/** The binding for one bucket, if the deployment provisioned one. */
function bindingFor(env: Env, bucket: BucketName): RateLimiter | null {
  const named = (env as unknown as Record<string, RateLimiter | undefined>)[
    `RATE_LIMIT_${bucket.toUpperCase()}`
  ];
  return named ?? env.RATE_LIMITER ?? null;
}

/** Who the budget is spent against.
 *
 *  The salted hash when the shop has a salt, so a rate-limit key is not a
 *  plaintext address sitting in storage; the address itself otherwise, because
 *  a shop with no salt still deserves a cap and these keys expire in minutes.
 *  A caller Cloudflare gives no address for shares one bucket, which is the
 *  safe direction: they are capped together rather than not at all. */
async function keyFor(env: Env, request: Request, bucket: BucketName): Promise<string> {
  const hashed = await ipHash(request, env);
  const who = hashed ?? request.headers.get("cf-connecting-ip") ?? "unknown";
  return `rl:${bucket}:${who}`;
}

export interface Verdict {
  ok: boolean;
  /** Seconds to wait, for the Retry-After header. */
  retryAfter: number;
}

/** Spends one request from a bucket. */
export async function withinBudget(env: Env, request: Request, bucket: BucketName): Promise<Verdict> {
  const budget: Budget = BUDGETS[bucket];
  const key = await keyFor(env, request, bucket);

  try {
    const binding = bindingFor(env, bucket);
    if (binding) {
      const { success } = await binding.limit({ key });
      return { ok: success, retryAfter: budget.windowS };
    }
    if (!env.SHOP_KV) return { ok: true, retryAfter: 0 }; // nothing to enforce with

    // Bucketed by window, so an entry expires on its own rather than needing a
    // sweep, and so a caller already over the cap does not push their own
    // window forward by knocking again.
    const slot = `${key}:${Math.floor(Date.now() / 1000 / budget.windowS)}`;
    const used = Number.parseInt((await env.SHOP_KV.get(slot)) ?? "0", 10) || 0;
    if (used >= budget.max) return { ok: false, retryAfter: budget.windowS };

    // Counted only when the request is allowed: a bot that keeps knocking must
    // not burn quota belonging to a real visitor sharing its address.
    await env.SHOP_KV.put(slot, String(used + 1), { expirationTtl: budget.windowS * 2 });
    return { ok: true, retryAfter: 0 };
  } catch {
    return { ok: budget.onError === "open", retryAfter: budget.windowS };
  }
}

/** The answer to a caller who is over their budget. Shaped like every other
 *  error the API returns, so a client branches on `error` as it always does. */
export function tooMany(request: Request, retryAfter: number): Response {
  const body: { error: string; message: string; requestId?: string } = {
    error: "rate_limited",
    message: "Too many requests. Wait a moment and try again.",
  };
  const ray = request.headers.get("cf-ray");
  if (ray) body.requestId = ray;
  return new Response(JSON.stringify(body), {
    status: 429,
    headers: {
      "content-type": "application/json; charset=utf-8",
      // So a well-behaved client backs off instead of retrying immediately.
      "retry-after": String(Math.max(1, retryAfter)),
      // A 429 is about this caller at this moment; a cached one would answer
      // somebody else's request.
      "cache-control": "no-store",
      "x-content-type-options": "nosniff",
    },
  });
}

/** Spends one request and returns the refusal to hand back, or null to carry
 *  on. The shape every caller uses:
 *
 *      const limited = await guard(env, request, "checkout");
 *      if (limited) return limited;
 */
export async function guard(env: Env, request: Request, bucket: BucketName): Promise<Response | null> {
  const verdict = await withinBudget(env, request, bucket);
  return verdict.ok ? null : tooMany(request, verdict.retryAfter);
}

/** True when this deployment can enforce anything at all — read by the panel,
 *  which says so rather than letting an owner assume they are covered. */
export function rateLimitingAvailable(env: Env): boolean {
  return Boolean(env.RATE_LIMITER ?? env.SHOP_KV);
}
