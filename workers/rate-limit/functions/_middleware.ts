// Cloudflare Pages Function middleware: a request budget for everything under
// functions/.
//
// `ssg new worker contact-form` scaffolds a public, unauthenticated POST that
// sends email, and nothing in it bounds how often it can be called. Turnstile
// raises the cost of abuse without capping it — a solved token can be replayed
// inside its validity window, and Turnstile is optional in the first place. The
// same shape applies to comments and to consent logging: every scaffolded
// Function that writes something was uncapped.
//
// `rate_limit` / `rate_burst` in .ssg.yaml bound the built-in preview server
// only. This is the deployed equivalent, and it is middleware rather than a
// library so it composes with whatever else the project already has: drop it in
// and every route under functions/ is covered, including ones added later.
//
// Bindings / config — all optional, and the middleware is a no-op without a
// backend, so an unconfigured project behaves exactly as it did:
//
//   RATE_LIMITER      Workers Rate Limiting binding. Exact per point of
//                     presence, free, nothing to provision. The default.
//   RATE_LIMIT_KV     KV namespace, used only when RATE_LIMITER is absent.
//                     Approximate: KV is eventually consistent, so a burst
//                     arriving at several points of presence at once can
//                     overshoot the cap — which is the shape a spam run takes,
//                     so prefer the binding where you can.
//   RATE_LIMIT_MAX    requests per key per window (default 20)
//   RATE_LIMIT_WINDOW window in seconds (default 60)
//   RATE_LIMIT_BY     "ip" (default) or "header:<name>"
//   RATE_LIMIT_PATHS  comma-separated path prefixes to cover; default "/api/"
//   RATE_LIMIT_FAIL   "open" (default) or "closed" — what to do when the
//                     backend itself errors

interface RateLimiter {
  limit(options: { key: string }): Promise<{ success: boolean }>;
}

interface Env {
  RATE_LIMITER?: RateLimiter;
  RATE_LIMIT_KV?: KVNamespace;
  RATE_LIMIT_MAX?: string;
  RATE_LIMIT_WINDOW?: string;
  RATE_LIMIT_BY?: string;
  RATE_LIMIT_PATHS?: string;
  RATE_LIMIT_FAIL?: string;
}

const DEFAULT_MAX = 20;
const DEFAULT_WINDOW = 60;

/** Reads a positive integer from the environment, falling back on anything else. */
const num = (raw: string | undefined, fallback: number): number => {
  const n = Number.parseInt(raw ?? "", 10);
  return Number.isFinite(n) && n > 0 ? n : fallback;
};

/** The paths this middleware guards. Static assets are not routed through
 *  functions/ at all, but a project may add a page-serving Function, and
 *  rate-limiting a page is a different decision from rate-limiting an API. */
const covers = (path: string, env: Env): boolean =>
  (env.RATE_LIMIT_PATHS ?? "/api/")
    .split(",")
    .map((p) => p.trim())
    .filter(Boolean)
    .some((prefix) => path.startsWith(prefix));

/** The identity the budget is spent against.
 *
 *  cf-connecting-ip is the obvious default and a blunt one: everyone behind a
 *  single CGNAT or one office shares a bucket. RATE_LIMIT_BY="header:X" keys on
 *  something better when the site has it — an authenticated user id, say. */
const keyFor = (request: Request, env: Env): string => {
  const by = env.RATE_LIMIT_BY ?? "ip";
  if (by.startsWith("header:")) {
    const header = by.slice("header:".length).trim();
    const value = request.headers.get(header);
    if (value) return `h:${header}:${value}`;
  }
  return `ip:${request.headers.get("cf-connecting-ip") ?? "unknown"}`;
};

/** Answers whether this request fits in the budget, using the binding when it
 *  is there and KV when it is not. */
async function withinBudget(key: string, env: Env): Promise<boolean | null> {
  if (env.RATE_LIMITER) {
    const { success } = await env.RATE_LIMITER.limit({ key });
    return success;
  }
  if (!env.RATE_LIMIT_KV) return null; // no backend: nothing to enforce

  const window = num(env.RATE_LIMIT_WINDOW, DEFAULT_WINDOW);
  const max = num(env.RATE_LIMIT_MAX, DEFAULT_MAX);
  // Bucketed by window so an entry expires on its own rather than needing a
  // sweep, and so a caller already over the cap does not push their own window
  // forward by knocking again.
  const bucket = Math.floor(Date.now() / 1000 / window);
  const slot = `${key}:${bucket}`;

  const used = Number.parseInt((await env.RATE_LIMIT_KV.get(slot)) ?? "0", 10) || 0;
  if (used >= max) return false;
  // Counted only when the request is being allowed: a bot that keeps knocking
  // must not burn quota that belongs to a real visitor sharing its address.
  await env.RATE_LIMIT_KV.put(slot, String(used + 1), { expirationTtl: window * 2 });
  return true;
}

const tooMany = (window: number): Response =>
  new Response(JSON.stringify({ error: "rate limit exceeded" }), {
    status: 429,
    headers: {
      "content-type": "application/json",
      // So a well-behaved client backs off instead of retrying immediately.
      "retry-after": String(window),
      // A 429 is about this caller at this moment; a cached one would answer
      // somebody else's request.
      "cache-control": "no-store",
    },
  });

export const onRequest: PagesFunction<Env> = async ({ request, env, next }) => {
  const url = new URL(request.url);
  if (!covers(url.pathname, env)) return next();

  const window = num(env.RATE_LIMIT_WINDOW, DEFAULT_WINDOW);
  let allowed: boolean | null;
  try {
    allowed = await withinBudget(keyFor(request, env), env);
  } catch {
    // The backend itself failed. Open is right for a contact form — losing a
    // real enquiry costs more than letting one extra message through — and
    // wrong for anything that moves money, which is why it is a setting rather
    // than a default nobody sees.
    allowed = (env.RATE_LIMIT_FAIL ?? "open") !== "closed";
  }
  if (allowed === null) return next(); // unconfigured: unchanged behaviour
  return allowed ? next() : tooMany(window);
};
