// The fetch pipeline. Every request goes through the same gates, in this
// order, so a route added to routes.ts gets all of them without asking:
//
//   1. outside the base path            → 404 (the worker owns nothing else)
//   2. schema (own tables only)         → 503 when the database is unreachable
//   3. {base}/hooks/…                   → the hook route, no auth, no IP limit
//   4. RATE_LIMITER bound and exceeded  → 429
//   5. admin auth                       → 401 / 403 / 429 / 503
//   6. CSRF: a state-changing method without `x-newsletter-admin: 1` → 403
//   7. {base}/api/…                     → the route (503 without subscribers)
//   8. anything else under base         → the panel files (ASSETS binding)

import { requireAdmin } from "./auth";
import { MISSING_SUBSCRIBERS } from "./health";
import { basePath, clientIP, json, secure, underBase } from "./lib";
import type { Env } from "./lib";
import { match } from "./router";
import type { Route } from "./router";
import { ROUTES } from "./routes";
import { ensureSchema, subscribersReady } from "./schema";

// CSRF_HEADER must accompany every non-GET/HEAD request. A form on another site
// can POST here with the admin's Basic credentials or Access cookie attached,
// but it cannot set a custom header, and a cross-site fetch() that does would
// need a CORS preflight this worker never grants.
export const CSRF_HEADER = "x-newsletter-admin";

export async function handleFetch(request: Request, env: Env, ctx: ExecutionContext, routes: Route[] = ROUTES): Promise<Response> {
  const base = basePath(env);
  if (base === null) return json({ error: "NEWSLETTER_UI_BASE is not a valid path" }, 503);
  const url = new URL(request.url);
  const rel = underBase(url.pathname, base);
  if (rel === null) return json({ error: "not found" }, 404);

  try {
    await ensureSchema(env);
  } catch (e) {
    console.error("newsletter-ui: schema", e);
    return json({ error: "database unavailable", hint: "bind the D1 database as NEWSLETTER_DB in wrangler.toml" }, 503);
  }

  if (rel.startsWith("/hooks/")) {
    return dispatch(request, env, ctx, url, rel, routes.filter((r) => r.path.startsWith("/hooks/")));
  }

  if (env.RATE_LIMITER) {
    const { success } = await env.RATE_LIMITER.limit({ key: "newsletter-ui:" + clientIP(request) });
    if (!success) return json({ error: "too many requests" }, 429, { "retry-after": "60" });
  }

  const denied = await requireAdmin(request, env);
  if (denied) return denied;

  if (request.method !== "GET" && request.method !== "HEAD" && request.headers.get(CSRF_HEADER) !== "1") {
    return json({ error: `missing ${CSRF_HEADER}: 1 header` }, 403);
  }

  // The panel uses relative URLs, so it must be loaded from "{base}/".
  if (rel === "") return secure(new Response(null, { status: 308, headers: { location: base + "/" + url.search } }));

  if (rel === "/api" || rel.startsWith("/api/")) {
    return dispatch(request, env, ctx, url, rel, routes.filter((r) => r.path.startsWith("/api/")));
  }

  if (!env.ASSETS) return json({ error: "not found" }, 404);
  const assetURL = new URL(request.url);
  assetURL.pathname = rel;
  return secure(await env.ASSETS.fetch(new Request(assetURL, request)));
}

// dispatch runs the matching route, or answers 404/405. Handler failures are
// logged and answered with a generic 500: the error text may carry SQL or an
// address and the panel has no use for it.
async function dispatch(request: Request, env: Env, ctx: ExecutionContext, url: URL, rel: string, routes: Route[]): Promise<Response> {
  const m = match(routes, request.method, rel);
  if (m.kind === "none") return json({ error: "not found" }, 404);
  if (m.kind === "method") return json({ error: "method not allowed" }, 405, { allow: m.allow.join(", ") });
  try {
    if (m.route.needsSubscribers !== false && !(await subscribersReady(env))) {
      return json({ error: "newsletter not installed", hint: MISSING_SUBSCRIBERS }, 503);
    }
    return secure(await m.route.handler({ request, env, ctx, url, params: m.params }));
  } catch (e) {
    console.error("newsletter-ui:", request.method, rel, e);
    return json({ error: "internal error" }, 500);
  }
}
