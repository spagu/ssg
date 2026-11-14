// Who may moderate, and what they may do (GO-136). A leading underscore keeps
// this file out of the Pages route table — _lib's requireAdmin imports it.
//
// Three admin modes, in the order of workers/auth/docs/CONTRACT.md:
//   1. Cloudflare Access (COMMENTS_ACCESS_TEAM + COMMENTS_ACCESS_AUD);
//   2. auth — a session cookie issued by the `auth` worker, checked locally by
//      the byte-identical copy of its verifier (_session-verifier.ts), when any
//      COMMENTS_AUTH_* variable is set;
//   3. the shared password (COMMENTS_ADMIN_PASSWORD).
// Access and the password grant every permission, as they always did; in auth
// mode the token's permissions apply, operation by operation (OPERATIONS).

import { json } from "./_lib";
import { can, denied, forbidden, verifySession } from "./_session-verifier";
import type { AuthConfig } from "./_session-verifier";

export type Mode = "access" | "auth" | "password";

// Admin: who passed the gate (user, null when the mode knows no person), what
// they hold (perms, wildcards as written) and the name recorded as the actor of
// what they change (the auth account id, the Access email, or the mode).
export interface Admin {
  mode: Mode;
  user: { id: string; email: string } | null;
  perms: string[];
  actor: string;
}

// The permissions this worker declares (module.json), in its order.
export const PERMISSIONS = ["comments:read", "comments:moderate", "comments:delete", "comments:import"] as const;

// OPERATIONS is the one source of truth: every admin operation and the single
// permission it needs in auth mode. test/permissions.test.ts is generated from it.
export const OPERATIONS = {
  list: "comments:read", //       GET  /api/comments/admin?status=…
  approve: "comments:moderate", // POST /api/comments/admin {id, action: "approve"}
  spam: "comments:moderate", //    POST /api/comments/admin {id, action: "spam"}
  delete: "comments:delete", //    POST /api/comments/admin {id, action: "delete"}
  purge: "comments:delete", //     POST /api/comments/admin {action: "purge"}
  import: "comments:import", //    POST /api/comments/import
  manifest: "comments:read", //    GET  /api/comments/module
  summary: "comments:read", //     GET  /api/comments/module/summary
} as const satisfies Record<string, (typeof PERMISSIONS)[number]>;
export type Operation = keyof typeof OPERATIONS;

// granted: Access and the password, which have always allowed everything.
export function granted(mode: "access" | "password", email: string | null): Admin {
  return { mode, user: email ? { id: email, email } : null, perms: ["comments:*"], actor: email || mode };
}

// permit answers the 403 for an operation the admin may not do, or null.
export function permit(admin: Admin, op: Operation): Response | null {
  return can(admin.perms, OPERATIONS[op]) ? null : forbidden(OPERATIONS[op]);
}

// permissionsOf: the declared permissions the admin holds, wildcards expanded.
export const permissionsOf = (admin: Admin): string[] => PERMISSIONS.filter((p) => can(admin.perms, p));

// CSRF in auth mode. The session rides in a cookie, which the browser attaches
// by itself, so a page elsewhere could make a moderator's browser approve,
// delete or import. SameSite=Lax keeps the cookie off cross-site POSTs, but
// not off a sibling subdomain ("same-site"), and Lax is a browser policy, not
// a check of ours — so a state-changing request must also carry the custom
// header CSRF_HEADER (a page on another origin cannot set it without a CORS
// preflight, which this worker never answers) and no cross-origin evidence:
// an Origin, when sent, is this origin; Sec-Fetch-Site, when sent, is
// "same-origin". The rules and the header are the auth worker's own
// (_request.ts), so one panel script satisfies every worker on the host.
//
// Access mode applies the same check (security review SR-10): Access lets
// through any request that carries a valid CF_Authorization cookie, and the
// browser attaches that cookie by itself. The password mode is unchanged on
// purpose: the password travels in an Authorization header that only the
// panel's script adds — a forged cross-site request cannot carry it. A
// service token (bearer, GO-141) is the same case: never sent by a browser.
export const CSRF_HEADER = "x-ssg-auth";

export function csrfProblem(request: Request): string | null {
  const own = new URL(request.url).origin;
  const origin = request.headers.get("origin");
  const site = request.headers.get("sec-fetch-site");
  if (origin !== null && origin !== own) return "cross_origin";
  if (site !== null && site !== "same-origin") return "cross_origin";
  return request.headers.get(CSRF_HEADER) === "1" ? null : "csrf_check_failed";
}

// authAdmin is mode 2: the verifier's verdict and answers (401 / login
// redirect / 503), then CSRF for a state-changing cookie request, then the
// operation's permission when the caller names one.
export async function authAdmin(request: Request, config: AuthConfig, op?: Operation): Promise<Admin | Response> {
  const v = await verifySession(request, config);
  if (!v.ok) return denied(request, config, v.reason);
  if (!["GET", "HEAD"].includes(request.method) && v.kind === "user") {
    const problem = csrfProblem(request);
    if (problem) return json({ error: problem }, 403);
  }
  const admin: Admin = { mode: "auth", user: v.user, perms: v.perms, actor: v.user.id };
  return op ? (permit(admin, op) ?? admin) : admin;
}
