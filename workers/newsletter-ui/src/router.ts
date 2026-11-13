// A method + path router small enough to read in one sitting. Routes are
// declared in routes.ts, one line each; paths are relative to the base path
// ("/api/campaigns/:id", not "/newsletter-admin/api/…") and a ":name" segment
// captures one path segment into ctx.params.
//
// Two prefixes, two trust levels (decided in app.ts, not per route):
//   /api/…    admin auth + CSRF header + per-IP volume limit;
//   /hooks/…  none of those — mail providers call them from many addresses and
//             without a browser; each hook verifies the provider's signature.

import type { Env } from "./lib";

export interface RouteContext {
  request: Request;
  env: Env;
  ctx: ExecutionContext;
  url: URL;
  params: Record<string, string>;
}

export type Handler = (c: RouteContext) => Response | Promise<Response>;

export interface Route {
  method: string; // "GET", "POST", "PUT", "PATCH", "DELETE"
  path: string; // relative to the base path, e.g. "/api/campaigns/:id"
  handler: Handler;
  // Whether the route reads the `newsletter` template's subscribers table. When
  // true (the default) and the table is missing, the route answers 503 with an
  // instruction instead of failing on SQL. /api/health opts out to report it.
  needsSubscribers?: boolean;
}

export type Match =
  | { kind: "found"; route: Route; params: Record<string, string> }
  | { kind: "method"; allow: string[] } // the path exists, not with this method
  | { kind: "none" };

// matchPath returns the captured params when pattern matches path, else null.
export function matchPath(pattern: string, path: string): Record<string, string> | null {
  const want = pattern.split("/");
  const got = path.split("/");
  if (want.length !== got.length) return null;
  const params: Record<string, string> = {};
  for (let i = 0; i < want.length; i++) {
    if (want[i].startsWith(":")) {
      if (!got[i]) return null; // an empty segment is not a value
      try {
        params[want[i].slice(1)] = decodeURIComponent(got[i]);
      } catch {
        return null; // malformed %-escape: no such resource
      }
    } else if (want[i] !== got[i]) {
      return null;
    }
  }
  return params;
}

// match finds the route for method + path in the given table.
export function match(routes: Route[], method: string, path: string): Match {
  const allow: string[] = [];
  for (const route of routes) {
    const params = matchPath(route.path, path);
    if (!params) continue;
    if (route.method === method) return { kind: "found", route, params };
    allow.push(route.method);
  }
  return allow.length ? { kind: "method", allow } : { kind: "none" };
}
