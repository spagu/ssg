// The gate in front of the second factor's own routes (GO-140). A leading
// underscore keeps this file out of the Pages route table.
//
// Order: configuration (503) → the CSRF defence (a plain form allowed where
// the route says so) → the second factor is on (404 otherwise, as if the
// route did not exist) → a live session (access cookie, or the refresh cookie
// after a pause; 401) → for a change, freshness: an account WITH a factor
// needs a recent second factor (step-up, 403 step_up_required), one without
// needs a recent sign-in (403 reauth_required) — so a stolen, old session
// cannot add its own authenticator and with it the roles that need one.

import { preparePost, whoAmI } from "./_context";
import type { Ctx, Who } from "./_context";
import { json, redirect } from "./_lib";
import { mfaEnabled } from "./_mfa-crypto";
import { freshness, stepUpRedirect, stepUpRequired } from "./_mfa-step-up";
import { safeNext, withParams } from "./_next";
import { recentMinutes, recentSignIn } from "./_provider-policy";
import { isForm, readBody } from "./_request";
import type { Body } from "./_request";

export interface MfaCall {
  ctx: Ctx;
  who: Who;
  body: Body;
  form: boolean;
  back: string; // where a plain form returns: the safe `next`, or the account page
  fail: (error: string, status: number, extra?: Record<string, unknown>) => Response;
}

export const notFound = (): Response => json({ error: "not_found" }, 404);

// mfaGate: `change` = the request changes the account's factors.
export async function mfaGate(request: Request, env: Ctx["env"], change: boolean, allowForm = true): Promise<MfaCall | Response> {
  const ctx = await preparePost(request, env, allowForm);
  if (ctx instanceof Response) return ctx;
  if (!mfaEnabled(env)) return notFound();
  const form = isForm(request);
  const body = (await readBody(request)) ?? {};
  const back = safeNext(body.next, "/auth/account");
  const who = await whoAmI(ctx, true);
  if (!who) return form ? redirect(withParams(ctx.s.loginUrl, { next: back })) : json({ error: "not_authenticated" }, 401);
  if (change) {
    const fresh = await freshness(ctx, who);
    if (fresh === "step_up") return form ? stepUpRedirect(back) : stepUpRequired(ctx);
    if (fresh === "no_factor" && !recentSignIn(env, who.session, ctx.now)) {
      return form ? redirect(withParams(ctx.s.loginUrl, { reauth: "1", next: back })) : json({ error: "reauth_required", minutes: recentMinutes(env) }, 403);
    }
  }
  const fail = (error: string, status: number, extra: Record<string, unknown> = {}): Response =>
    form ? redirect(withParams(back, { error })) : json({ error, ...extra }, status);
  return { ctx, who, body, form, back, fail };
}
