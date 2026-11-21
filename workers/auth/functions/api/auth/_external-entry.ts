// The exchange of an external issuer's JWT for a session (#435): the work of
// POST /api/auth/jwt. A leading underscore keeps this file out of the Pages
// route table.
//
// In order, each a refusal:
//   1. the method is configured: absent → 404 not_found (as if the route did
//      not exist); misconfigured → 503 jwt_misconfigured {hint} — also when
//      the roles mapping reaches an administrative role without
//      AUTH_JWT_ALLOW_ADMIN_ROLES=on (mapAdminProblem, _external-roles.ts);
//   2. the CSRF defence of every POST (_request.ts) — a JSON/fetch call with
//      `x-ssg-auth: 1`, or a plain form from this origin — or a plain form
//      from one of AUTH_JWT_FORM_ORIGINS (the issuer's own page posting the
//      token across origins); otherwise 403. A page on any other origin
//      cannot sign a visitor into the account of the attacker's token (login
//      CSRF);
//   3. the per-IP lockout, counted BEFORE the token is checked (SR-02) under
//      its own counter (jwt_ip), given back once a token verifies;
//   4. the token — the Authorization: Bearer header, else the form/JSON field
//      `token` — verified (_external-jwt.ts) and naming a usable identity
//      (_external-identity.ts);
//   5. its `jti` not seen before (_external-schema.ts): a token is spent the
//      moment it verifies, whatever the sign-in then decides;
//   6. who it signs in (_bridge-signin.ts), with the roles mapping applied
//      before the session (_external-roles.ts), and the second factor asked
//      for when a role needs one (startSession) — the issuer is never assumed
//      to have checked one.
// Every answer after step 2 is a 303: to the safe `next` (_next.ts) on
// success, to the login page with ?error=<code> on a refusal, recorded in the
// audit trail with the reason — never the token.

import { bridgeSignIn, refuseBridge } from "./_bridge-signin";
import type { Bridge } from "./_bridge-signin";
import { prepare } from "./_context";
import type { Ctx } from "./_context";
import { JWT_ENV_ID, SKEW_SECONDS, externalLookup } from "./_external-config";
import type { ExternalSetup } from "./_external-config";
import { externalIdentity } from "./_external-identity";
import { verifyExternalToken } from "./_external-jwt";
import type { Claims } from "./_external-jwt";
import { applyExternalRoles, mapAdminProblem } from "./_external-roles";
import { spendToken } from "./_external-schema";
import { clientIP, json } from "./_lib";
import type { Env } from "./_lib";
import { lockKey, recordFailure, releaseAttempt } from "./_lockout";
import { safeNext } from "./_next";
import type { Identity } from "./_provider";
import { csrfProblem, isForm, readBody, str } from "./_request";

// externalCsrf: the usual defence, or a plain form from a configured origin.
export function externalCsrf(request: Request, s: ExternalSetup): string | null {
  const problem = csrfProblem(request, true);
  const origin = request.headers.get("origin");
  return problem && isForm(request) && origin !== null && s.formOrigins.includes(origin) ? null : problem;
}

// tokenOf: the bearer header first, else the `token` field.
export function tokenOf(request: Request, body: Record<string, unknown>): string {
  const m = /^Bearer\s+(\S+)\s*$/i.exec(request.headers.get("authorization") ?? "");
  return m ? m[1] : str(body.token).trim();
}

const bridgeOf = (s: ExternalSetup, env: Env, claims: Claims, now: number): Bridge => ({
  method: s.method,
  domainsOf: JWT_ENV_ID,
  linkByEmail: s.identityClaim === "email",
  invite: s.unknown !== "reject",
  create: s.unknown === "create" ? { roleId: null, roleName: "" } : null,
  codes: { emailUnusable: "jwt_email_unusable", notLinked: "jwt_not_linked" },
  via: "jwt_email",
  before: (user) => applyExternalRoles(env, s, user.id, claims, now),
});

// misconfigured: the 503 of a method an administrator must fix, logged.
function misconfigured(problem: string): Response {
  console.error(`auth: external JWT sign-in is not offered: ${problem}`);
  return json({ error: "jwt_misconfigured", hint: problem }, 503);
}

// externalReady: the configuration, or the 404 / 503 to answer (step 1).
export async function externalReady(env: Env): Promise<ExternalSetup | Response> {
  const l = externalLookup(env);
  if (l.kind === "absent") return json({ error: "not_found" }, 404);
  if (l.kind === "misconfigured") return misconfigured(l.problem);
  const adminProblem = await mapAdminProblem(env, l.setup);
  return adminProblem ? misconfigured(adminProblem) : l.setup;
}

export interface Accepted {
  setup: ExternalSetup;
  identity: Identity;
  claims: Claims;
  next: string;
}

// Refuse answers a refusal of the token or the person after the CSRF check.
export type Refuse = (s: ExternalSetup, error: string, cause: string, next: string) => Promise<Response>;

// acceptToken runs steps 1–5 for any route that takes a token (the sign-in
// here, the link in _external-link.ts): a verified, unspent token with its
// identity — spent now — or the answer to send.
export async function acceptToken(ctx: Ctx, refuse: Refuse): Promise<Accepted | Response> {
  const { request, env } = ctx;
  const s = await externalReady(env);
  if (s instanceof Response) return s;
  const csrf = externalCsrf(request, s);
  if (csrf) return json({ error: csrf }, 403);
  const body = (await readBody(request)) ?? {};
  const next = safeNext(body.next, "");
  const no = (error: string, cause: string) => refuse(s, error, cause, next);

  const entry = { key: await lockKey(env, "jwt_ip", clientIP(request)), kind: "jwt_ip" as const, threshold: ctx.s.lockIP };
  if ((await recordFailure(env, [entry], ctx.s, ctx.now)) > 0) return no("too_many_attempts", "locked");
  const v = await verifyExternalToken(tokenOf(request, body), s, ctx.now);
  if (!v.ok) {
    console.warn(`auth: external JWT refused: ${v.reason}`);
    return no(v.unavailable ? "jwt_unavailable" : "jwt_failed", v.reason);
  }
  const identity = externalIdentity(v.claims, s);
  if ("problem" in identity) return no(identity.problem === "no subject" ? "jwt_failed" : "jwt_email_unusable", identity.problem);
  // Kept until the token could no longer pass anyway: its expiry, or its age limit.
  const until = (Math.min(v.exp, v.iat + s.maxAgeSeconds) + SKEW_SECONDS) * 1000;
  if (!(await spendToken(env, s.issuer, v.jti, until, ctx.now))) return no("jwt_failed", "replayed");
  await releaseAttempt(env, [entry]);
  return { setup: s, identity, claims: v.claims, next };
}

export async function externalExchange(request: Request, env: Env): Promise<Response> {
  const ctx = await prepare(request, env);
  if (ctx instanceof Response) return ctx;
  const a = await acceptToken(ctx, (s, error, cause, next) => refuseBridge(ctx, s.method, error, { cause }, next));
  if (a instanceof Response) return a;
  return bridgeSignIn(ctx, { identity: a.identity, next: a.next, bridge: bridgeOf(a.setup, env, a.claims, ctx.now) });
}
