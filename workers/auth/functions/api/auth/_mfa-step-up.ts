// Step-up (GO-140): a sensitive change asks for a RECENT second factor, not
// just a live session — someone at an unlocked computer, or holding a stolen
// refresh cookie, has the session but not the phone or the key. A leading
// underscore keeps this file out of the Pages route table.
//
// The rule: while the second factor is on and the account HAS a factor, the
// session must have passed one (sign-in, enrolment or POST
// /api/auth/mfa/step-up) within AUTH_STEP_UP_MINUTES (default 10). An account
// without a factor is not asked (it has nothing to show; the routes keep
// their own "recent sign-in" rule for it).
//
// Applied to: adding or removing a sign-in method or a factor, regenerating
// backup codes, a service token with allowSensitive, handing out `*` (an
// owner grant, a member of a group holding it, an invitation carrying it),
// ending every session of ANOTHER account, resetting someone's factors.
// requireStepUp is the one call for a JSON route — GO-138's panel uses it
// the same way; stepUpRedirect is the plain-form answer.

import type { Ctx } from "./_context";
import { json, redirect } from "./_lib";
import { mfaEnabled } from "./_mfa-crypto";
import { hasFactor } from "./_mfa-factors";
import { sessionFactor, stepUpMinutes } from "./_mfa-policy";
import { STEP_UP_PAGE } from "./_mfa-signin";
import { withParams } from "./_next";

export interface Holder {
  user: { id: string };
  session: { id: string };
}

// recentFactor: did this session pass a second factor within the window?
export async function recentFactor(ctx: Ctx, sessionId: string): Promise<boolean> {
  const f = await sessionFactor(ctx.env, sessionId);
  return f !== null && ctx.now - Date.parse(f.verified_at) <= stepUpMinutes(ctx.env) * 60_000;
}

// freshness: "no_factor" (nothing to ask for: the second factor is off or the
// account has none), "recent" (passed one within the window) or "step_up".
export async function freshness(ctx: Ctx, who: Holder): Promise<"no_factor" | "recent" | "step_up"> {
  if (!mfaEnabled(ctx.env) || !(await hasFactor(ctx.env, who.user.id))) return "no_factor";
  return (await recentFactor(ctx, who.session.id)) ? "recent" : "step_up";
}

// stepUpNeeded: must this caller step up before a sensitive change?
export const stepUpNeeded = async (ctx: Ctx, who: Holder): Promise<boolean> => (await freshness(ctx, who)) === "step_up";

// stepUpRequired is the JSON refusal: where to step up and the window.
export const stepUpRequired = (ctx: Ctx): Response => json({ error: "step_up_required", stepUpUrl: STEP_UP_PAGE, minutes: stepUpMinutes(ctx.env) }, 403);

// requireStepUp: null when the change may go ahead, otherwise the 403 to send.
export async function requireStepUp(ctx: Ctx, who: Holder): Promise<Response | null> {
  return (await stepUpNeeded(ctx, who)) ? stepUpRequired(ctx) : null;
}

// stepUpRedirect: a plain form goes to the step-up page, which comes back to `next`.
export const stepUpRedirect = (next: string): Response => redirect(withParams(STEP_UP_PAGE, { next }));
