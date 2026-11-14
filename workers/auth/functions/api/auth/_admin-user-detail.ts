// One account as the admin panel's person page shows it (GO-138). A leading
// underscore keeps this file out of the Pages route table.
//
// Everything about the account an administrator needs to decide — and
// nothing that could sign anyone in: no password hash (only whether there is
// one), no provider subject, no factor secret or credential, no session
// token or IP hash.

import { permissionsAnswer } from "./_admin";
import { liveSessions } from "./_admin-sessions";
import { listEvents } from "./_admin-events";
import { listGrants } from "./_grants";
import type { Env } from "./_lib";
import { mfaEnabled } from "./_mfa-crypto";
import { holdsOwner } from "./_mfa-reset";
import { backupCodesLeft, listFactors, view } from "./_mfa-factors";
import { listIdentities } from "./_provider-identities";
import type { User } from "./_users";

export const DETAIL_SESSIONS = 50;
export const DETAIL_EVENTS = 20;

// accountOf: the account's own fields as the admin API shows them.
export const accountOf = (u: User) => ({
  id: u.id,
  email: u.email,
  name: u.display_name,
  status: u.status,
  mustChangePassword: u.must_change_password === 1,
  createdAt: u.created_at,
  updatedAt: u.updated_at,
  lastLoginAt: u.last_login_at,
});

// memberships: the groups the account is in.
async function memberships(env: Env, userId: string): Promise<{ id: string; name: string; addedAt: string }[]> {
  const { results } = await env.AUTH_DB.prepare(
    "SELECT g.id, g.name, m.added_at FROM group_members m JOIN groups g ON g.id = m.group_id WHERE m.user_id = ? ORDER BY g.name",
  )
    .bind(userId)
    .all<{ id: string; name: string; added_at: string }>();
  return results.map((r) => ({ id: r.id, name: r.name, addedAt: r.added_at }));
}

// userDetail is the answer of GET /api/auth/admin/users/:id.
export async function userDetail(env: Env, u: User, now: number) {
  const identities = await listIdentities(env, u.id);
  const factors = await listFactors(env, u.id);
  return {
    user: { ...accountOf(u), owner: await holdsOwner(env, u.id) },
    methods: {
      password: u.password_hash !== null,
      identities: identities.map((i) => ({ id: i.id, provider: i.provider, email: i.email, linkedAt: i.linked_at, lastUsedAt: i.last_used_at })),
    },
    secondFactor: {
      enabled: mfaEnabled(env),
      factors: factors.map(view),
      backupCodesLeft: await backupCodesLeft(env, u.id),
    },
    groups: await memberships(env, u.id),
    grants: await listGrants(env, { userId: u.id }),
    permissions: (await permissionsAnswer(env, u.id)).permissions,
    sessions: (await liveSessions(env, now, u.id, "", DETAIL_SESSIONS)).items,
    events: (await listEvents(env, { kind: "", user: u.id, from: "", to: "", cursor: "" }, DETAIL_EVENTS)).items,
  };
}
