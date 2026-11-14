// What the admin API treats as sensitive (GO-138), and the audit line every
// administrative change writes. A leading underscore keeps this file out of
// the Pages route table.
//
// Sensitive = needs a RECENT second factor (requireStepUp, GO-140) from a
// caller whose account has one, on top of the permission:
//   - handing out `*` or `auth:*` — a grant, a group membership, an
//     invitation (sent or resent), an Access group mapping, or a role edited
//     to hold one of them (handsOutAdmin) — and taking either away
//     (mayTakeAway, SR-04);
//   - a password reset link for another account (SR-05);
//   - deleting an account; blocking an account that holds `owner`;
//   - ending another account's sessions, one or all, or everyone's;
//   - resetting someone's second factor (mfa/reset.ts, `*` only);
//   - a service token with allowSensitive (`*` only).
// An account without a factor is not asked (it has nothing to show); the
// permission and the escalation rule still apply.

import { escalation } from "./_admin";
import type { Caller } from "./_admin";
import { emitEvent } from "./_audit";
import type { EventKind } from "./_audit";
import { effectivePermissions } from "./_grants";
import { requireStepUp } from "./_mfa-step-up";
import { beyond } from "./_perms";

// ADMIN_WILDCARDS: holding either is holding identity administration whole.
export const ADMIN_WILDCARDS = ["*", "auth:*"];

// handsOutAdmin: does this set of permissions carry `*` or `auth:*`?
export const handsOutAdmin = (perms: readonly string[]): boolean => perms.some((p) => ADMIN_WILDCARDS.includes(p));

// stepUpWhen: the step-up refusal to send when `sensitive`, else null.
export async function stepUpWhen(caller: Caller, sensitive: boolean): Promise<Response | null> {
  return sensitive ? requireStepUp(caller.ctx, caller) : null;
}

// adminEvent records an administrative change: `userId` is the account
// changed (null for a role, a group, everyone), `detail.by` the
// administrator. Never a token, a link or a secret — names and ids only.
export function adminEvent(caller: Caller, kind: EventKind, userId: string | null, detail: Record<string, string | number | boolean | null> = {}): Promise<void> {
  return emitEvent(caller.ctx.env, { kind, userId, detail: { by: caller.user.id, ...detail } }, caller.ctx.now);
}

// mayTakeAway (security review SR-04): taking `*` or `auth:*` away — revoking
// a grant, removing a group member, deleting a role or a group, editing them
// out of a role, dropping an Access mapping — follows the rule for handing
// them out: the caller must cover them (403 escalation) and, with a second
// factor, have used it recently (403 step_up_required). Without it an
// `auth:admin` could strip the co-owners' owner grants one by one and then
// block, reset or sign out the people who were owners. Taking away any other
// permission stays an ordinary `auth:admin` change ("narrowing is fine").
export async function mayTakeAway(caller: Caller, perms: readonly string[]): Promise<Response | null> {
  const admin = perms.filter((p) => ADMIN_WILDCARDS.includes(p));
  const over = beyond(caller.perms, admin);
  if (over.length) return escalation(over);
  return stepUpWhen(caller, admin.length > 0);
}

// mayActOn (SR-04): acting on another account — ending its sessions,
// disabling or deleting it (a service account too), revoking its tokens — is
// refused when it holds `*` or `auth:*` the caller does not (403 escalation):
// a lesser administrator cannot sign the owners out or switch their machines off.
export async function mayActOn(caller: Caller, accountId: string): Promise<Response | null> {
  const admin = (await effectivePermissions(caller.ctx.env, accountId)).filter((p) => ADMIN_WILDCARDS.includes(p));
  const over = beyond(caller.perms, admin);
  return over.length ? escalation(over) : null;
}
