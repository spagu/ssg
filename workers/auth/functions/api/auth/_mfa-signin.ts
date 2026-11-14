// The second factor's place in signing in (GO-140). A leading underscore
// keeps this file out of the Pages route table. _signin.ts startSession()
// calls secondFactorStep() for EVERY first factor — the password, each login
// provider, Cloudflare Access, an accepted invitation — so none of them can
// skip it, and none is assumed to have checked a second factor of its own.
//
//   the account has a factor → no session yet: a pending handle
//       (_mfa-pending.ts) and the challenge page /auth/mfa;
//   it has none, but a role it holds needs one → a session WITHOUT those
//       roles' permissions (the token policy leaves them out) and, for a
//       browser, straight to enrolment (/auth/mfa/enrol). Enrolling proves
//       the new factor, which counts for that session. This is the grace that
//       lets the first owner in: it adds nothing an account without a factor
//       did not already have — the password alone still opens only the roles
//       that need no second factor, and once a factor exists, adding or
//       removing one needs a recent second factor (_mfa-step-up.ts);
//   neither → a plain session.
// While the second factor is off (no AUTH_MFA_KEY) every account takes the
// last path.

import type { Ctx } from "./_context";
import { mfaEnabled } from "./_mfa-crypto";
import { listFactors } from "./_mfa-factors";
import { createPending } from "./_mfa-pending";
import { policyView } from "./_provider-policy";
import type { User } from "./_users";

export const CHALLENGE_PAGE = "/auth/mfa";
export const ENROL_PAGE = "/auth/mfa/enrol";
export const STEP_UP_PAGE = "/auth/mfa/step-up";

export interface Challenge {
  ok: "second_factor";
  cookie: string; // the pending handle
  expiresAt: string;
  factors: string[]; // what the account can answer with: totp, passkey, backup
}

// secondFactorStep: the challenge to send instead of a session, or null.
export async function secondFactorStep(ctx: Ctx, user: User, method: string, next: string): Promise<Challenge | null> {
  if (!mfaEnabled(ctx.env)) return null;
  const factors = await listFactors(ctx.env, user.id);
  if (!factors.length) return null;
  const { cookie, expiresAt } = await createPending(ctx.env, user.id, method, next, ctx.now);
  const kinds = [...new Set(factors.map((f) => f.kind as string)), "backup"];
  return { ok: "second_factor", cookie, expiresAt, factors: kinds };
}

// mustEnrol: the second factor is on, the account has none, and a role it
// holds needs one.
export async function mustEnrol(ctx: Ctx, user: User): Promise<boolean> {
  if (!mfaEnabled(ctx.env) || (await listFactors(ctx.env, user.id)).length) return false;
  return (await policyView(ctx.env, user.id, "", false)).needsFactor.length > 0;
}
