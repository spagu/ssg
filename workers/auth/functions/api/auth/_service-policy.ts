// The second-factor policy as service tokens read it (GO-141 × GO-140). A
// leading underscore keeps this file out of the Pages route table.
//
// A role that people may use only after a second factor (_mfa-policy.ts: the
// default — isSensitive or `…:send` — plus AUTH_MFA_ROLES) gives a machine
// nothing unless the token's allowSensitive covers it: a machine never
// presents a factor. A permission the account also holds through a role that
// needs no factor stays free.
//
// Unlike for people, this applies whether or not the second factor is
// switched on (AUTH_MFA_KEY). The switch exists so a site without the key
// does not lock its people out; turning it off must not widen what a stolen
// service secret opens. AUTH_MFA_ROLES still counts while it is off — an
// owner who marks a role `false` has said, for people and machines alike,
// that it needs no factor — and an invalid value fails closed here too: no
// role's permissions are free.
//
// Applied at every exchange (token.ts), from the grants as they are then, so
// a role that later gains `newsletter:send` does not widen an existing token.

import { permissionSources } from "./_grants";
import type { Env } from "./_lib";
import { policyRoles, rolesOf } from "./_mfa-policy";

export interface ServiceGrant {
  perms: string[]; // every permission the account holds, sorted
  free: string[]; // of those, the ones given by at least one role that needs no second factor
}

// serviceGrant reads a service account's permissions and which of them a
// machine may carry without an owner's allowSensitive (besides isSensitive).
export async function serviceGrant(env: Env, accountId: string): Promise<ServiceGrant> {
  const sources = await permissionSources(env, accountId);
  const bound = policyRoles(env, rolesOf(sources));
  return {
    perms: sources.map((p) => p.name),
    free: sources.filter((p) => p.sources.some((s) => !bound.has(s.roleName))).map((p) => p.name),
  };
}
