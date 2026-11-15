// Routes: method + path (relative to the base path, ":name" captures one
// segment) → an operation. A route only translates HTTP to the operation's
// input and its output back to HTTP; the permission lives in the operation
// (ops.ts), which is why a Route has no permission field of its own.

import type { Op, OpContext } from "./ops";

export interface Route<I = any, O = any> {
  method: string; // "GET", "POST", "PUT", "PATCH", "DELETE"
  path: string; // e.g. "/api/content/:path"
  op: Op<I, O>;
  // input builds the operation's input from the request (default: undefined).
  input?: (ctx: OpContext, params: Record<string, string>) => I | Promise<I>;
  // respond turns the output into a Response (default: 200 JSON).
  respond?: (out: O) => Response;
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
        return null; // a malformed %-escape: no such resource
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
