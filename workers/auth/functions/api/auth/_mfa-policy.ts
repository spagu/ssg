// Which roles need a second factor, and whether a session has passed one
// (GO-140). A leading underscore keeps this file out of the Pages route table.
// _provider-policy.ts applies it at every token issue, next to the per-role
// method policy; for people nothing applies while the second factor is off
// (no AUTH_MFA_KEY, _mfa-crypto.ts). Service tokens read the same policy
// whatever the switch says (policyRoles, _service-policy.ts).
//
// Default — a role needs a second factor when it holds a permission that
// isSensitive() flags (`*` — so `owner` —, `auth:…`, `…:erase`, a worker
// wildcard `…:*`) or one that sends (`…:send`). The same definition of
// "sensitive" as service tokens (_service-perms.ts), plus sending: one mistake
// there reaches a whole mailing list.
//
// AUTH_MFA_ROLES  JSON {"<role name>": true|false} overrides the default per
//     role: true = needs one, false = does not. A role not listed keeps the
//     default. A value that is not such an object fails closed: every role
//     needs a second factor until it is fixed.

import type { Provenance } from "./_grants";
import { iso, intVar } from "./_lib";
import type { Env } from "./_lib";
import { mfaEnabled } from "./_mfa-crypto";
import { isSensitive } from "./_service-perms";

export type Overrides = Map<string, boolean> | null | "invalid";

// mfaOverrides parses AUTH_MFA_ROLES.
export function mfaOverrides(env: Env): Overrides {
  const raw = (env.AUTH_MFA_ROLES ?? "").trim();
  if (!raw) return null;
  let v: unknown;
  try {
    v = JSON.parse(raw);
  } catch {
    return "invalid";
  }
  if (!v || typeof v !== "object" || Array.isArray(v)) return "invalid";
  const out = new Map<string, boolean>();
  for (const [role, needs] of Object.entries(v)) {
    if (typeof needs !== "boolean") return "invalid";
    out.set(role, needs);
  }
  return out;
}

export const sensitiveForPeople = (perm: string): boolean => isSensitive(perm) || perm.endsWith(":send");

// roleNeedsFactor: does a role with these permissions need a second factor?
export function roleNeedsFactor(role: string, perms: readonly string[], o: Overrides): boolean {
  if (o === "invalid") return true;
  const set = o?.get(role);
  return set ?? perms.some(sensitiveForPeople);
}

// rolesOf: the roles behind a permission list with provenance, as role name →
// the permissions it gives — the shape factorRoles takes.
export function rolesOf(sources: readonly Provenance[]): Map<string, string[]> {
  const roles = new Map<string, string[]>();
  for (const p of sources) for (const s of p.sources) roles.set(s.roleName, [...(roles.get(s.roleName) ?? []), p.name]);
  return roles;
}

// factorRoles: of the roles given (name → permissions), the ones that need a
// second factor here; none while the second factor is off.
export function factorRoles(env: Env, roles: Map<string, string[]>): Set<string> {
  return mfaEnabled(env) ? policyRoles(env, roles) : new Set();
}

// policyRoles: the roles the policy (defaults + AUTH_MFA_ROLES) marks as
// needing a second factor, whether or not the second factor is switched on.
// People get factorRoles (inert while it is off); service tokens get this
// (_service-policy.ts) — the switch exists so people are not locked out, not
// to widen what a machine may carry.
export function policyRoles(env: Env, roles: Map<string, string[]>): Set<string> {
  const o = mfaOverrides(env);
  if (o === "invalid") console.error("auth: AUTH_MFA_ROLES is not a JSON object of role names to true/false; every role needs a second factor until it is fixed");
  return new Set([...roles].filter(([name, perms]) => roleNeedsFactor(name, perms, o)).map(([name]) => name));
}

export interface SessionFactor {
  factor: string; // totp | backup | passkey
  verified_at: string;
}

// sessionFactor: the second factor this session passed, and when last.
export function sessionFactor(env: Env, sessionId: string): Promise<SessionFactor | null> {
  return env.AUTH_DB.prepare("SELECT factor, verified_at FROM session_factors WHERE session_id = ?").bind(sessionId).first<SessionFactor>();
}

// markSessionFactor records a passed second factor for the session (at
// sign-in, enrolment or step-up).
export async function markSessionFactor(env: Env, sessionId: string, factor: string, now: number): Promise<void> {
  await env.AUTH_DB.prepare(
    `INSERT INTO session_factors (session_id, factor, verified_at) VALUES (?, ?, ?)
     ON CONFLICT (session_id) DO UPDATE SET factor = excluded.factor, verified_at = excluded.verified_at`,
  )
    .bind(sessionId, factor, iso(now))
    .run();
}

// stepUpMinutes: how recent a second factor must be for a sensitive change.
export const stepUpMinutes = (env: Env): number => intVar(env.AUTH_STEP_UP_MINUTES, 10);
