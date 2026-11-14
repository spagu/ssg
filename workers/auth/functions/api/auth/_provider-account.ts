// The signed-in person's own sign-in methods (GO-151): what GET
// /api/auth/methods and the account page show, and the gate every change to
// them passes. A leading underscore keeps this file out of the Pages route
// table.

import { ACCESS_METHOD, ACCESS_NAME, accessLookup } from "./_access-config";
import type { Ctx, Who } from "./_context";
import { preparePost, whoAmI } from "./_context";
import { json, redirect } from "./_lib";
import type { Env } from "./_lib";
import { freshness, stepUpRedirect, stepUpRequired } from "./_mfa-step-up";
import { withParams } from "./_next";
import { enabledProviders, providerName } from "./_provider";
import { listIdentities } from "./_provider-identities";
import { recentMinutes, recentSignIn, sessionMethod } from "./_provider-policy";
import { isForm, readBody } from "./_request";
import type { Body } from "./_request";

export interface MethodsView {
  password: boolean;
  identities: { id: string; provider: string; name: string; email: string; linkedAt: string; lastUsedAt: string | null }[];
  available: { id: string; name: string }[]; // enabled providers not linked yet
  sessionMethod: string;
  recentSignIn: boolean;
  recentMinutes: number;
}

export async function methodsOf(ctx: Ctx, who: Who): Promise<MethodsView> {
  const identities = await listIdentities(ctx.env, who.user.id);
  const linked = new Set(identities.map((i) => i.provider));
  return {
    password: who.user.password_hash !== null,
    identities: identities.map((i) => ({ id: i.id, provider: i.provider, name: providerName(i.provider), email: i.email, linkedAt: i.linked_at, lastUsedAt: i.last_used_at })),
    available: [
      ...enabledProviders(ctx.env)
        .filter(({ provider }) => !linked.has(provider.id))
        .map(({ provider }) => ({ id: provider.id, name: provider.name })),
      // GO-139: Cloudflare Access, when on and not linked yet.
      ...(accessLookup(ctx.env).kind === "enabled" && !linked.has(ACCESS_METHOD) ? [{ id: ACCESS_METHOD, name: ACCESS_NAME }] : []),
    ],
    sessionMethod: await sessionMethod(ctx.env, who.session.id),
    recentSignIn: recentSignIn(ctx.env, who.session, ctx.now),
    recentMinutes: recentMinutes(ctx.env),
  };
}

export interface Change {
  ctx: Ctx;
  who: Who;
  body: Body;
  // done answers a success: JSON 200 with `data`, a form 303 to the account
  // page with ?ok=<ok>; fail answers an error the same two ways.
  done: (ok: string, data: Record<string, unknown>) => Response;
  fail: (error: string, status: number) => Response;
}

// changeGate: a state-changing request about the caller's own methods —
// CSRF-checked (a form allowed), signed in, and signed in RECENTLY; otherwise
// the answer to send (a form goes to the login page, with reauth=1 when the
// sign-in is too old). GO-140: an account with a second factor needs a recent
// second factor instead (a step-up, _mfa-step-up.ts) — a form goes to the
// step-up page, JSON gets 403 step_up_required.
export async function changeGate(request: Request, env: Env): Promise<Change | Response> {
  const ctx = await preparePost(request, env, true);
  if (ctx instanceof Response) return ctx;
  const form = isForm(request);
  const body = (await readBody(request)) ?? {};
  const back = "/auth/account";
  const fail = (error: string, status: number): Response => (form ? redirect(withParams(back, { error })) : json({ error }, status));
  const who = await whoAmI(ctx, true);
  if (!who) return form ? redirect(withParams(ctx.s.loginUrl, { next: back })) : json({ error: "not_authenticated" }, 401);
  const fresh = await freshness(ctx, who);
  if (fresh === "step_up") return form ? stepUpRedirect(back) : stepUpRequired(ctx);
  if (fresh === "no_factor" && !recentSignIn(env, who.session, ctx.now)) {
    return form ? redirect(withParams(ctx.s.loginUrl, { reauth: "1", next: back })) : json({ error: "reauth_required", minutes: recentMinutes(env) }, 403);
  }
  const done = (ok: string, data: Record<string, unknown>): Response => (form ? redirect(withParams(back, { ok })) : json(data));
  return { ctx, who, body, done, fail };
}
