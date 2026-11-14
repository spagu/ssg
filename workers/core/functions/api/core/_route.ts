// The one path every core route takes: sign-in, the CSRF header rule, the
// method, then the handler — and the security headers on whatever comes out,
// errors included. A leading underscore keeps this file out of the route table.

import { CSRF_HEADER, QUIET_HEADER, problem, secure } from "./_lib";
import type { Env } from "./_lib";
import { signIn } from "./_signin";
import type { Caller } from "./_signin";

export type Handler = (request: Request, env: Env, caller: Caller) => Promise<Response>;
export type Methods = Partial<Record<"GET" | "POST" | "PUT" | "PATCH" | "DELETE", Handler>>;
type Context = { request: Request; env: Env };

const SAFE = new Set(["GET", "HEAD"]);

// quiet drops the challenge from a refusal (see QUIET_HEADER).
function quiet(res: Response): Response {
  const out = new Response(res.body, res);
  out.headers.delete("www-authenticate");
  return out;
}

async function dispatch(request: Request, env: Env, methods: Methods): Promise<Response> {
  const signedIn = await signIn(request, env);
  if (!signedIn.ok) return request.headers.get(QUIET_HEADER) === "1" ? quiet(signedIn.response) : signedIn.response;
  if (!SAFE.has(request.method) && request.headers.get(CSRF_HEADER) !== "1") {
    return problem("csrf_header_missing", 403, `send the ${CSRF_HEADER}: 1 header with every request that changes something`);
  }
  const handler = methods[(request.method === "HEAD" ? "GET" : request.method) as keyof Methods];
  if (!handler) return problem("method_not_allowed", 405, undefined, { allow: Object.keys(methods).join(", ") });
  return handler(request, env, signedIn.caller);
}

// route builds a Pages handler (export it as onRequest). A thrown error is a
// 500 without detail: nothing internal leaks to the caller.
export function route(methods: Methods): (ctx: Context) => Promise<Response> {
  return async ({ request, env }) => {
    try {
      return secure(await dispatch(request, env, methods));
    } catch {
      return problem("internal_error", 500);
    }
  };
}
