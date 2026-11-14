// The fetch pipeline. Every request goes through the same gates, in this
// order, so a route added to routes.ts gets all of them without asking:
//
//   1. outside the base path            → 404 (the worker owns nothing else)
//   2. schema (own tables only)         → 503 when the database is unreachable
//   3. {base}/hooks/…                   → the hook route, no auth, no IP limit
//   4. RATE_LIMITER bound and exceeded  → 429
//   5. admin auth (Access → auth → password) → 302 / 401 / 403 / 429 / 503
//   6. CSRF: a state-changing method without `x-newsletter-admin: 1` → 403;
//      in `auth` mode also a cross-origin Origin / Sec-Fetch-Site → 403
//   7. {base}/api/…                     → the route's permission (403
//      {error: "forbidden", permission}), then the route (503 without
//      subscribers), then the audit row of a state-changing route
//   8. anything else under base         → the panel files (ASSETS binding)

import { authenticate } from "./auth";
import type { Admin } from "./auth";
import { recordAudit } from "./audit";
import { MISSING_SUBSCRIBERS } from "./health";
import { basePath, clientIP, json, secure, underBase } from "./lib";
import type { Env } from "./lib";
import { missing } from "./permissions";
import { match } from "./router";
import type { Route } from "./router";
import { ROUTES } from "./routes";
import { ensureSchema, subscribersReady } from "./schema";
import { forbidden } from "./session-verifier";

// CSRF_HEADER must accompany every non-GET/HEAD request. A form on another site
// can POST here with the admin's Basic credentials or Access cookie attached,
// but it cannot set a custom header, and a cross-site fetch() that does would
// need a CORS preflight this worker never grants.
export const CSRF_HEADER = "x-newsletter-admin";

// crossOrigin: in `auth` mode the session is a cookie any page may cause the
// browser to send, so a state-changing request also needs no cross-origin
// evidence, as the `auth` contract recommends: an Origin, when sent, is this
// origin, and Sec-Fetch-Site, when sent, is "same-origin" (a sibling subdomain
// may be someone else's). A script with a bearer token sends neither.
export function crossOrigin(request: Request): boolean {
  const origin = request.headers.get("origin");
  const site = request.headers.get("sec-fetch-site");
  return (origin !== null && origin !== new URL(request.url).origin) || (site !== null && site !== "same-origin");
}

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
    return dispatch(request, env, ctx, url, rel, routes.filter((r) => r.path.startsWith("/hooks/")), null);
  }

  if (env.RATE_LIMITER) {
    const { success } = await env.RATE_LIMITER.limit({ key: "newsletter-ui:" + clientIP(request) });
    if (!success) return json({ error: "too many requests" }, 429, { "retry-after": "60" });
  }

  const admin = await authenticate(request, env);
  if (admin instanceof Response) return admin;

  if (request.method !== "GET" && request.method !== "HEAD") {
    if (request.headers.get(CSRF_HEADER) !== "1") return json({ error: `missing ${CSRF_HEADER}: 1 header` }, 403);
    if (admin.mode === "auth" && crossOrigin(request)) return json({ error: "cross-origin request refused" }, 403);
  }

  // The panel uses relative URLs, so it must be loaded from "{base}/".
  if (rel === "") return secure(new Response(null, { status: 308, headers: { location: base + "/" + url.search } }));

  if (rel === "/api" || rel.startsWith("/api/")) {
    return dispatch(request, env, ctx, url, rel, routes.filter((r) => r.path.startsWith("/api/")), admin);
  }

  if (!env.ASSETS) return json({ error: "not found" }, 404);
  const assetURL = new URL(request.url);
  assetURL.pathname = rel;
  return secure(await env.ASSETS.fetch(new Request(assetURL, request)));
}

// dispatch runs the matching route, or answers 404/405. `admin` is null on
// the public hooks; otherwise the route's permission is checked first and a
// state-changing route's answer is recorded in the audit trail. Handler
// failures are logged and answered with a generic 500: the error text may
// carry SQL or an address and the panel has no use for it.
async function dispatch(request: Request, env: Env, ctx: ExecutionContext, url: URL, rel: string, routes: Route[], admin: Admin | null): Promise<Response> {
  const m = match(routes, request.method, rel);
  if (m.kind === "none") return json({ error: "not found" }, 404);
  if (m.kind === "method") return json({ error: "method not allowed" }, 405, { allow: m.allow.join(", ") });
  const lacking = admin && missing(admin, m.route.permission);
  if (lacking) return secure(forbidden(lacking));
  const res = await run(request, env, ctx, url, rel, m.route, m.params, admin);
  if (admin && m.route.audit) await recordAudit(env, admin, m.route, m.params, res);
  return res;
}

async function run(request: Request, env: Env, ctx: ExecutionContext, url: URL, rel: string, route: Route, params: Record<string, string>, admin: Admin | null): Promise<Response> {
  try {
    if (route.needsSubscribers !== false && !(await subscribersReady(env))) {
      return json({ error: "newsletter not installed", hint: MISSING_SUBSCRIBERS }, 503);
    }
    return secure(await route.handler({ request, env, ctx, url, params, admin }));
  } catch (e) {
    console.error("newsletter-ui:", request.method, rel, e);
    return json({ error: "internal error" }, 500);
  }
}
