// What an administrator can narrow about sign-in methods (GO-151), and how
// fresh a sign-in must be to change them. A leading underscore keeps this file
// out of the Pages route table.
//
//   AUTH_<PROVIDER>_ALLOWED_DOMAINS  comma-separated domains: that provider
//       signs in, accepts invitations and links only accounts whose address
//       the provider VERIFIED and whose domain is exactly one of these
//       (sub-domains are separate entries). Unset = any.
//   AUTH_ROLE_METHODS  JSON {"<role name>": ["<method>", …]}: a role listed
//       here counts only in sessions started with one of its methods
//       ("password" or a provider id). Applied at EVERY token issue — login,
//       refresh — from the role grants' provenance: a permission stays when
//       any role giving it allows the session's method. A role not listed
//       allows every method. A value that is not such an object is a
//       configuration error and fails closed: no permission from any role.
//   GO-140: a role that needs a second factor (_mfa-policy.ts) counts only
//       in a session that passed one; roles left out for that reason are
//       listed apart (needsFactor), so the pages can link to enrolment.
//   AUTH_RECENT_SIGNIN_MINUTES  (default 10) adding, removing or setting a
//       sign-in method needs a session started this recently.

import { permissionSources } from "./_grants";
import type { Identity } from "./_provider";
import { envVar } from "./_provider-env";
import { intVar } from "./_lib";
import type { Env } from "./_lib";
import { factorRoles, rolesOf, sessionFactor } from "./_mfa-policy";
import type { Session } from "./_sessions";

export const PASSWORD_METHOD = "password";

// domainAllowed: may this identity use this provider here?
export function domainAllowed(env: Env, providerId: string, id: Identity): boolean {
  const raw = envVar(env, providerId, "ALLOWED_DOMAINS");
  if (!raw) return true;
  const domains = raw.toLowerCase().split(",").map((d) => d.trim()).filter(Boolean);
  const at = id.email.lastIndexOf("@");
  return id.emailVerified && at > 0 && domains.includes(id.email.slice(at + 1));
}

// rolePolicy parses AUTH_ROLE_METHODS: null = no policy, "invalid" = a
// configuration error (fail closed).
export function rolePolicy(env: Env): Map<string, Set<string>> | null | "invalid" {
  const raw = (env.AUTH_ROLE_METHODS ?? "").trim();
  if (!raw) return null;
  let v: unknown;
  try {
    v = JSON.parse(raw);
  } catch {
    return "invalid";
  }
  if (!v || typeof v !== "object" || Array.isArray(v)) return "invalid";
  const out = new Map<string, Set<string>>();
  for (const [role, methods] of Object.entries(v)) {
    if (!Array.isArray(methods) || !methods.every((m) => typeof m === "string")) return "invalid";
    out.set(role, new Set(methods as string[]));
  }
  return out;
}

// sessionMethod: how a session was started (a session without a row is from
// before GO-151 and was a password one).
export async function sessionMethod(env: Env, sessionId: string): Promise<string> {
  const row = await env.AUTH_DB.prepare("SELECT method FROM session_methods WHERE session_id = ?").bind(sessionId).first<{ method: string }>();
  return row?.method ?? PASSWORD_METHOD;
}

export interface Withheld {
  role: string;
  methods: string[];
}

export interface PolicyView {
  perms: string[];
  withheld: Withheld[]; // left out by the method policy
  needsFactor: string[]; // left out because this session has not passed a second factor (GO-140)
}

// policyView: the permissions a session of `method` may carry — `secondFactor`
// says whether it passed one — and the roles it holds whose permissions are
// left out, and why (for the pages to say so).
export async function policyView(env: Env, userId: string, method: string, secondFactor = false): Promise<PolicyView> {
  const sources = await permissionSources(env, userId);
  const roles = rolesOf(sources);
  const factor = secondFactor ? new Set<string>() : factorRoles(env, roles);
  const needsFactor = [...factor].sort();
  const policy = rolePolicy(env);
  if (policy === "invalid") {
    console.error("auth: AUTH_ROLE_METHODS is not a JSON object of role names to method lists; no role counts until it is fixed");
    return { perms: [], withheld: [...roles.keys()].sort().map((role) => ({ role, methods: [] })), needsFactor };
  }
  const methodOk = (role: string) => policy === null || !policy.has(role) || (policy.get(role) as Set<string>).has(method);
  const perms = sources.filter((p) => p.sources.some((s) => methodOk(s.roleName) && !factor.has(s.roleName))).map((p) => p.name);
  const withheld = [...roles.keys()]
    .filter((r) => !methodOk(r))
    .sort()
    .map((role) => ({ role, methods: [...((policy as Map<string, Set<string>>).get(role) as Set<string>)].sort() }));
  return { perms, withheld, needsFactor };
}

// sessionGrant is what goes into an access token for this session: the
// permissions, and when it last passed a second factor (Unix seconds; the
// optional `mfa_at` claim), or null.
export async function sessionGrant(env: Env, userId: string, session: Session): Promise<{ perms: string[]; mfaAt: number | null }> {
  const factor = await sessionFactor(env, session.id);
  const view = await policyView(env, userId, await sessionMethod(env, session.id), factor !== null);
  return { perms: view.perms, mfaAt: factor ? Math.floor(Date.parse(factor.verified_at) / 1000) : null };
}


export const recentMinutes = (env: Env): number => intVar(env.AUTH_RECENT_SIGNIN_MINUTES, 10);

// recentSignIn: was this session started within AUTH_RECENT_SIGNIN_MINUTES?
// A refresh keeps a session alive but does not make its sign-in recent.
export const recentSignIn = (env: Env, session: Session, now: number): boolean => now - Date.parse(session.created_at) <= recentMinutes(env) * 60_000;
