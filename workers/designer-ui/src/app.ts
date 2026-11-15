// The fetch pipeline. Every request goes through the same steps, in order, so
// a route added to routes.ts gets all of them without asking:
//
//   1. DESIGNER_BASE not a plain path      → 503 (fail closed)
//   2. outside the base path               → 404 (the worker owns nothing else)
//   3. RATE_LIMITER bound and exceeded     → 429
//   4. the gate (gate.ts)                  → 503 / 302 / 401 — the panel's files too
//   5. CSRF for a cookie session's write   → 403 cross_origin
//   6. {base}/api/…, {base}/mcp            → the route's operation, which checks
//                                            its own permission (ops.ts) → 403
//   7. anything else under base            → the panel files, for designer-ui:read

import { authenticate, csrfRefusal } from "./gate";
import type { Caller } from "./gate";
import { MODULE_ID, basePath, clientIP, json, secure, underBase } from "./http";
import type { Env } from "./http";
import { errorResponse, perform } from "./ops";
import type { Deps, OpContext } from "./ops";
import { PERM } from "./permissions";
import { match } from "./router";
import type { Route } from "./router";
import { ROUTES } from "./routes";
import { can, forbidden } from "./session-verifier";

export async function handleFetch(request: Request, env: Env, deps: Deps = {}, routes: Route[] = ROUTES): Promise<Response> {
  const base = basePath(env);
  if (base === null) return json({ error: "bad_base", detail: "DESIGNER_BASE is not a valid path" }, 503);
  const url = new URL(request.url);
  const rel = underBase(url.pathname, base);
  if (rel === null) return json({ error: "not_found" }, 404);

  if (env.RATE_LIMITER) {
    const { success } = await env.RATE_LIMITER.limit({ key: `${MODULE_ID}:${clientIP(request)}` });
    if (!success) return json({ error: "rate_limited" }, 429, { "retry-after": "60" });
  }

  const gate = await authenticate(request, env, deps);
  if (!gate.ok) return gate.response;
  const refused = csrfRefusal(request, gate.caller);
  if (refused) return refused;

  // The panel uses relative URLs, so it must be loaded from "{base}/".
  if (rel === "") return secure(new Response(null, { status: 308, headers: { location: base + "/" + url.search } }));

  const ctx: OpContext = { env, caller: gate.caller, auth: gate.config, deps, request, url };
  if (rel === "/api" || rel.startsWith("/api/") || rel === "/mcp") return dispatch(ctx, rel, routes);
  return panel(ctx, rel);
}

// dispatch runs the matching route's operation, or answers 404/405.
async function dispatch(ctx: OpContext, rel: string, routes: Route[]): Promise<Response> {
  const m = match(routes, ctx.request.method, rel);
  if (m.kind === "none") return json({ error: "not_found" }, 404);
  if (m.kind === "method") return json({ error: "method_not_allowed" }, 405, { allow: m.allow.join(", ") });
  const { route, params } = m;
  // The operation's own permission before its body is read (GO-183): an
  // account that may not run it never makes the Worker parse its input.
  // perform() checks it again; this only moves the refusal earlier.
  if (!can(ctx.caller.perms, route.op.permission)) return secure(forbidden(route.op.permission));
  try {
    const input = route.input ? await route.input(ctx, params) : undefined;
    const out = await perform(route.op, ctx, input);
    return route.respond ? secure(route.respond(out)) : json(out);
  } catch (err) {
    return errorResponse(err, `${ctx.request.method} ${rel}`);
  }
}

// panel serves the static files. They are code, not data, but an account
// that may not read the settings has no use for the designer, so the same
// read permission applies (the verifier's forbidden() names it).
async function panel(ctx: OpContext, rel: string): Promise<Response> {
  if (!mayRead(ctx.caller)) return secure(forbidden(PERM.read));
  if (!ctx.env.ASSETS) return json({ error: "not_found" }, 404);
  const assetURL = new URL(ctx.url);
  assetURL.pathname = rel;
  return secure(await ctx.env.ASSETS.fetch(new Request(assetURL, ctx.request)));
}

const mayRead = (caller: Caller): boolean => can(caller.perms, PERM.read);
