// Cloudflare Pages Function: confirm a second factor again within a session (GO-140).
//   POST /api/auth/mfa/step-up   {code | backupCode, next?} — the step-up page's forms or JSON with x-ssg-auth: 1
//                                {passkey: <assertion>} — JSON, from the page's script
//
// Sensitive changes ask for a second factor passed within
// AUTH_STEP_UP_MINUTES (_mfa-step-up.ts); this is where it is passed. Same
// checks and lockout as the sign-in step (_mfa-verify.ts); a pass is
// recorded for this session and a new access cookie goes out (its `mfa_at`
// and, if this session had none before, the roles that need a second factor).
// JSON: 200 {ok: true, verifiedAt, minutes}; a form gets a 303 to the safe
// `next` (default /auth/account). 409 no_factor (the account has none: there
// is nothing to step up with); 401 not_authenticated | invalid_code; 429
// too_many_attempts; 503 mfa_unavailable; 404 not_found while the second
// factor is off.

import { issueCookies } from "../_context";
import { iso, json, redirect } from "../_lib";
import type { Env } from "../_lib";
import { listFactors } from "../_mfa-factors";
import { mfaGate } from "../_mfa-gate";
import { markSessionFactor, stepUpMinutes } from "../_mfa-policy";
import { STEP_UP_PAGE } from "../_mfa-signin";
import { checkSecondFactor } from "../_mfa-verify";
import { withParams } from "../_next";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const g = await mfaGate(request, env, false);
  if (g instanceof Response) return g;
  const { ctx, who, body } = g;
  if (!(await listFactors(env, who.user.id)).length) return g.fail("no_factor", 409);
  const checked = await checkSecondFactor(ctx, who.user, body, "step-up", who.session.id);
  if (!checked.ok) {
    const headers: Record<string, string> = checked.error === "too_many_attempts" ? { "retry-after": String(checked.retryAfter) } : {};
    return g.form ? redirect(withParams(STEP_UP_PAGE, { next: g.back, error: checked.error, use: body.backupCode ? "backup" : "" })) : json({ error: checked.error }, checked.status, headers);
  }
  await markSessionFactor(env, who.session.id, checked.factor, ctx.now);
  const { cookies } = await issueCookies(ctx, who.user, who.session, null);
  if (g.form) return redirect(g.back, cookies);
  return json({ ok: true, verifiedAt: iso(ctx.now), minutes: stepUpMinutes(env) }, 200, {}, cookies);
};
