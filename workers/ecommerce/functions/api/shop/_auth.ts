// Who may get into the shop's admin API (GO-155). A leading underscore keeps
// this file out of the Pages route table; admin/_middleware.ts calls it for
// every admin request.
//
// Three modes, the same in every worker of this repository and checked in this
// order (workers/auth/docs/CONTRACT.md, "Admin modes"):
//   1. Cloudflare Access, when SHOP_ACCESS_TEAM and SHOP_ACCESS_AUD are set;
//   2. `auth`, when any SHOP_AUTH_{ISSUER,AUDIENCE,PUBLIC_KEY,JWKS_URL,LOGIN_URL}
//      is set: the `auth` worker's session cookie, checked here with the
//      byte-identical copy of its verifier (_session-verifier.ts). An
//      incomplete set is a 503 naming what is missing — never a fall-through to
//      the password — and with a complete set the password is refused too: it
//      would let anyone who knows it past every permission, unrecorded;
//   3. the shared password, SHOP_ADMIN_PASSWORD, over HTTP Basic
//      (_admin-password.ts). A first-class mode: one person needs no accounts.
// Nothing configured → 503. The admin API is never open by default.
//
// Access and the password grant every shop permission; in `auth` mode the
// token's permissions apply, route by route (_permissions.ts).

import { verifyAccessJwt } from "./_access";
import { passwordGate } from "./_admin-password";
import type { Env } from "./_env";
import { fail, json } from "./_lib";
import { PERMISSIONS, SESSION } from "./_permissions";
import type { Need } from "./_permissions";
import { can, configFrom, denied, loginRedirect, verifySession } from "./_session-verifier";
import type { AuthConfig } from "./_session-verifier";

export type Mode = "access" | "auth" | "password";

// Admin: who passed the gate — `user` is null when the mode knows no person
// (the password) — what they hold (`perms`, wildcards as written) and the name
// recorded as the actor of what they change in the shop's audit log: the
// `auth` account ("auth:<id> <email>"), an `auth` service account
// ("service:<id>"), the Access address ("access:<email>") or "password".
// `service` marks an `auth` service token (GO-157): a machine, which the gate
// lets through only to the routes listed for machines (_gate.ts).
export interface Admin {
  mode: Mode;
  user: { id: string; email: string } | null;
  perms: string[];
  actor: string;
  service?: true;
}

// AUTH_PREFIX: the SHOP_AUTH_* variables. Every worker of a Pages project
// shares one environment, hence a prefix of the shop's own.
export const AUTH_PREFIX = "SHOP";

// modeOf names the mode the configuration selects, or "none".
export function modeOf(env: Env): Mode | "none" {
  if (env.SHOP_ACCESS_TEAM && env.SHOP_ACCESS_AUD) return "access";
  if (configFrom(env, AUTH_PREFIX)) return "auth";
  return env.SHOP_ADMIN_PASSWORD ? "password" : "none";
}

// granted: the modes that have always allowed everything.
function granted(mode: "access" | "password", email: string | null): Admin {
  return {
    mode,
    user: email ? { id: email, email } : null,
    perms: ["ecommerce:*"],
    actor: email ? `access:${email}` : "password",
  };
}

// CSRF_HEADER: the header the panel sends on every request. A page on another
// origin cannot set it without a CORS preflight, which this API never answers.
export const CSRF_HEADER = "x-ssg-auth";

// csrfProblem: the check for a state-changing request whose credential the
// browser attaches by itself — the `auth` session cookie, or Access's
// CF_Authorization cookie. SameSite=Lax keeps a cookie off cross-site POSTs
// but not off a sibling subdomain, and it is a browser policy rather than a
// check of ours. So the request must carry CSRF_HEADER and no cross-origin
// evidence: an Origin, when sent, is this origin; Sec-Fetch-Site, when sent, is
// "same-origin". The rules are the auth worker's own, so one panel script
// satisfies every worker on the host. The password mode needs none of this:
// its credential travels in a header only the panel's script adds.
export function csrfProblem(request: Request): string | null {
  const own = new URL(request.url).origin;
  const origin = request.headers.get("origin");
  const site = request.headers.get("sec-fetch-site");
  if (origin !== null && origin !== own) return "cross_origin";
  if (site !== null && site !== "same-origin") return "cross_origin";
  return request.headers.get(CSRF_HEADER) === "1" ? null : "csrf_check_failed";
}

const changes = (request: Request): boolean => !["GET", "HEAD"].includes(request.method);

function csrfRefusal(request: Request): Response | null {
  const problem = changes(request) ? csrfProblem(request) : null;
  return problem ? fail(request, problem, "Send this from the shop's own admin panel.", 403) : null;
}

// authenticate returns who is calling, or the Response to send instead.
export async function authenticate(request: Request, env: Env): Promise<Admin | Response> {
  if (env.SHOP_ACCESS_TEAM && env.SHOP_ACCESS_AUD) {
    const verdict = await verifyAccessJwt(request, { team: env.SHOP_ACCESS_TEAM, aud: env.SHOP_ACCESS_AUD });
    if (!verdict.ok) return fail(request, verdict.code, verdict.message, verdict.status);
    return csrfRefusal(request) ?? granted("access", verdict.email.toLowerCase());
  }
  const config = configFrom(env, AUTH_PREFIX);
  if (config) return authSession(request, config);
  if (!env.SHOP_ADMIN_PASSWORD) {
    return fail(
      request,
      "admin_not_configured",
      "Admin access is not set up. Set SHOP_ACCESS_TEAM and SHOP_ACCESS_AUD, the SHOP_AUTH_* variables, or the SHOP_ADMIN_PASSWORD secret.",
      503,
    );
  }
  return (await passwordGate(request, env, env.SHOP_ADMIN_PASSWORD)) ?? granted("password", null);
}

// authSession is mode 2: the verifier's verdict and its answers (401, a login
// redirect for a page, 503), then CSRF for a state-changing cookie request.
// A service token (GO-157) arrives as `Authorization: Bearer` — the verifier
// accepts kind "service" only there and kind "user" only from the cookie — so
// it is no ambient credential a browser could be tricked into sending, and
// needs no CSRF header. Which routes it may call is the gate's question.
async function authSession(request: Request, config: AuthConfig): Promise<Admin | Response> {
  const v = await verifySession(request, config);
  if (!v.ok) return denied(request, config, v.reason);
  if (v.kind === "service") {
    return { mode: "auth", user: v.user, perms: v.perms, actor: `service:${v.user.id}`, service: true };
  }
  const refused = csrfRefusal(request);
  if (refused) return refused;
  return { mode: "auth", user: v.user, perms: v.perms, actor: `auth:${v.user.id} ${v.user.email}`.trim() };
}

// permit answers the 403 for a route the admin may not call, or null. The body
// names the permission, so the panel can say what is missing.
export function permit(admin: Admin, need: Need): Response | null {
  if (need === SESSION || can(admin.perms, need)) return null;
  const message = `This needs the permission ${need}, which this account does not have.`;
  return json({ error: "forbidden", permission: need, message }, 403);
}

// permissionsOf: the shop permissions the admin holds, wildcards expanded and
// other workers' permissions left out.
export const permissionsOf = (admin: Admin): string[] => PERMISSIONS.filter((p) => can(admin.perms, p));

// LOGOUT_URL, REFRESH_URL: the `auth` worker's endpoints, on this host.
export const LOGOUT_URL = "/api/auth/logout";
export const REFRESH_URL = "/api/auth/refresh";
// PANEL: where the panel lives — the page a login sends the browser back to.
export const PANEL = "/ecommerce-admin/";

// loginUrlFor: the login URL as the verifier builds it for a browser opening
// the panel, from configuration only — the panel never composes one from input.
export function loginUrlFor(request: Request, config: AuthConfig): string | null {
  const page = new Request(new URL(PANEL, request.url), { headers: { accept: "text/html" } });
  return loginRedirect(page, config).headers.get("location");
}

// withLogin adds {mode: "auth", loginUrl, refreshUrl} to the 401 an
// `auth`-mode caller without a session gets, so the panel can refresh the
// session once and otherwise knows where to send the browser.
export async function withLogin(request: Request, env: Env, refused: Response): Promise<Response> {
  const config = refused.status === 401 && modeOf(env) === "auth" ? configFrom(env, AUTH_PREFIX) : null;
  if (!config) return refused;
  const body = { ...((await refused.json()) as Record<string, unknown>), mode: "auth", loginUrl: loginUrlFor(request, config), refreshUrl: REFRESH_URL };
  return new Response(JSON.stringify(body), { status: 401, headers: refused.headers });
}
