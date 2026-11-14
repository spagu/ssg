// What the admin panel's Status view shows (GO-138): how this worker is set
// up, read from its configuration and its database — never a secret's value,
// only whether one is set and what follows from it. A leading underscore
// keeps this file out of the Pages route table.

import { accessLookup } from "./_access-config";
import { keyAges, ROTATION_DAYS } from "./_admin-keys";
import { auditRetentionDays } from "./_audit";
import type { Ctx } from "./_context";
import { iso } from "./_lib";
import type { Env } from "./_lib";
import { mailState } from "./_mail";
import { mfaState } from "./_mfa-crypto";
import { factorRoles, mfaOverrides, stepUpMinutes } from "./_mfa-policy";
import { SCHEME } from "./_password";
import { passwordEnabled, signInMethods } from "./_provider-methods";
import { registryOf } from "./_registry";
import { listRoles } from "./_roles";
import { serviceSettings } from "./_service-tokens";

// LEEWAY_SECONDS: the clock leeway every verifier allows (docs/CONTRACT.md).
export const LEEWAY_SECONDS = 30;

// rolesNeedingFactor: the role names the second-factor policy covers here.
export async function rolesNeedingFactor(env: Env): Promise<string[]> {
  const roles = new Map((await listRoles(env)).map((r) => [r.name, r.permissions]));
  return [...factorRoles(env, roles)].sort();
}

// missingFactor: active accounts holding (directly or through a group) a
// role that needs a second factor, without having one — they sign in
// without that role's permissions until they enrol.
export async function missingFactor(env: Env, roleNames: readonly string[]): Promise<number> {
  if (!roleNames.length) return 0;
  const row = await env.AUTH_DB.prepare(
    `SELECT COUNT(*) AS n FROM users u
      WHERE u.status = 'active'
        AND NOT EXISTS (SELECT 1 FROM mfa_factors f WHERE f.user_id = u.id AND f.confirmed_at IS NOT NULL)
        AND EXISTS (SELECT 1 FROM grants g JOIN roles r ON r.id = g.role_id
                     WHERE r.name IN (SELECT value FROM json_each(?))
                       AND (g.user_id = u.id OR EXISTS (SELECT 1 FROM group_members m WHERE m.group_id = g.group_id AND m.user_id = u.id)))`,
  )
    .bind(JSON.stringify(roleNames))
    .first<{ n: number }>();
  return (row as { n: number }).n;
}

// pendingInvitations: invitations still open (neither used, withdrawn nor expired).
export async function pendingInvitations(env: Env, now: number): Promise<number> {
  const row = await env.AUTH_DB.prepare("SELECT COUNT(*) AS n FROM invitations WHERE accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?")
    .bind(iso(now))
    .first<{ n: number }>();
  return (row as { n: number }).n;
}

// adminStatus is the answer of GET /api/auth/admin/status.
export async function adminStatus(ctx: Ctx) {
  const { env, s, keys, now } = ctx;
  const mail = mailState(env);
  const overrides = mfaOverrides(env);
  const needing = await rolesNeedingFactor(env);
  const reg = registryOf(env);
  const modules = new Map<string, number>();
  for (const p of reg.known.values()) modules.set(p.module, (modules.get(p.module) ?? 0) + 1);
  return {
    keys: await keyAges(env, keys.jwks.map((k) => k.kid), keys.signing.kid, now),
    rotationDays: ROTATION_DAYS,
    tokens: {
      accessMinutes: s.tokenSeconds / 60,
      // A changed grant reaches a session at its next refresh: at most the
      // token's lifetime plus the verifiers' leeway (docs/PERMISSIONS.md).
      stalenessSeconds: s.tokenSeconds + LEEWAY_SECONDS,
      sessionDays: s.sessionSeconds / 86_400,
      serviceAccessMinutes: serviceSettings(env).accessSeconds / 60,
    },
    hashing: { scheme: SCHEME, iterations: s.iterations, passwordMin: s.passwordMin, passwordMax: s.passwordMax },
    signIn: { methods: signInMethods(env).map((m) => m.id), password: passwordEnabled(env) },
    access: accessLookup(env).kind,
    mfa: {
      state: mfaState(env),
      stepUpMinutes: stepUpMinutes(env),
      policy: overrides === "invalid" ? "invalid" : overrides ? "custom" : "default",
      rolesNeedingFactor: needing,
      accountsMissingFactor: await missingFactor(env, needing),
    },
    mail: mail.kind === "ready" ? { state: "ready", transport: mail.transport } : mail.kind === "broken" ? { state: "broken", problem: mail.problem } : { state: "none" },
    audit: { retentionDays: auditRetentionDays(env) },
    invitations: { pending: await pendingInvitations(env, now) },
    modules: [...modules].map(([id, permissions]) => ({ id, permissions })).sort((a, b) => (a.id < b.id ? -1 : 1)),
    registryProblems: reg.problems,
  };
}
