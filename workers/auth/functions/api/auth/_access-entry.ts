// The Cloudflare Access routes' work (GO-139): signing in at /auth/access and
// linking Access to a signed-in account at /auth/access/link. A leading
// underscore keeps this file out of the Pages route table; functions/auth/
// access/ only routes here.
//
// Both paths sit behind the Access application whose AUD is AUTH_ACCESS_AUD:
// Access signs the person in before the request arrives. The token is still
// verified here (_access-jwt.ts), so a request that reaches the worker some
// other way — or with a token for another application — gets nothing.
//
// Answers: Access not configured → 404 not_found (as if absent); configured
// wrongly, or AUTH_ACCESS_AUTO_CREATE with a default role that could make an
// administrator → 503 access_misconfigured {hint}; any refusal of a person →
// 303 to the login page (the account page for a link) with ?error=<code>,
// recorded in the audit trail with the reason — never the token.

import { ACCESS_METHOD, accessLookup } from "./_access-config";
import type { AccessSetup } from "./_access-config";
import { identityOf } from "./_access-identity";
import { verifyAccessToken } from "./_access-jwt";
import type { AccessVerified } from "./_access-jwt";
import { confirmPage } from "./_access-pages";
import { defaultRole } from "./_access-provision";
import { accessSignIn, refuseAccess } from "./_access-signin";
import { emitEvent } from "./_audit";
import { preparePost, whoAmI } from "./_context";
import type { Ctx, Who } from "./_context";
import { json, redirect } from "./_lib";
import type { Env } from "./_lib";
import { safeNext, withParams } from "./_next";
import { signedIn } from "./_pages";
import type { Identity } from "./_provider";
import { findIdentity, linkIdentity, listIdentities } from "./_provider-identities";
import { domainAllowed, recentSignIn } from "./_provider-policy";
import { freshness, stepUpRedirect } from "./_mfa-step-up";
import { formOrigins } from "./_provider-pages";
import { signInMethods } from "./_provider-methods";

export interface Ready {
  setup: AccessSetup;
  roleId: string | null;
}

// accessReady: the configuration, or the 404 / 503 to answer.
export async function accessReady(ctx: Ctx): Promise<Ready | Response> {
  const l = accessLookup(ctx.env);
  if (l.kind === "absent") return json({ error: "not_found" }, 404);
  if (l.kind === "misconfigured") return json({ error: "access_misconfigured", hint: l.problem }, 503);
  if (!l.setup.autoCreate) return { setup: l.setup, roleId: null };
  const role = await defaultRole(ctx.env, l.setup);
  if ("problem" in role) {
    console.error(`auth: Cloudflare Access sign-in refused to start: ${role.problem}`);
    return json({ error: "access_misconfigured", hint: role.problem }, 503);
  }
  return { setup: l.setup, roleId: role.roleId };
}

// failure: the ?error= code of a token that did not verify; logged (the
// reason only).
function failure(v: Extract<AccessVerified, { ok: false }>): string {
  console.warn(`auth: Cloudflare Access token refused: ${v.reason}`);
  return v.unavailable ? "access_unavailable" : "access_failed";
}

// GET /auth/access?next=
export async function accessEntry(ctx: Ctx): Promise<Response> {
  const ready = await accessReady(ctx);
  if (ready instanceof Response) return ready;
  const next = safeNext(new URL(ctx.request.url).searchParams.get("next"), "");
  const v = await verifyAccessToken(ctx.request, ready.setup, ctx.now);
  if (!v.ok) return refuseAccess(ctx, failure(v), { cause: v.reason }, next);
  return accessSignIn(ctx, { setup: ready.setup, person: v.person, next, roleId: ready.roleId });
}

const ACCOUNT = "/auth/account";

async function refuseLink(ctx: Ctx, who: Who, error: string, detail: Record<string, string | boolean>): Promise<Response> {
  await emitEvent(ctx.env, { kind: "signin_refused", userId: who.user.id, method: ACCESS_METHOD, detail: { reason: error, purpose: "link", ...detail } }, ctx.now);
  return redirect(withParams(ACCOUNT, { error }));
}

// linkGate: a live, RECENT session of an active account, and a verified
// Access token whose address domain is allowed — or the answer to send.
// "Recent" is the rule every change of sign-in methods follows
// (_provider-account.ts changeGate): a recent second factor (step-up) for an
// account that has one (GO-140), a recent sign-in otherwise.
async function linkGate(ctx: Ctx, who: Who | Response): Promise<{ who: Who; id: Identity } | Response> {
  const ready = await accessReady(ctx);
  if (ready instanceof Response) return ready;
  if (who instanceof Response) return who;
  const fresh = await freshness(ctx, who);
  if (fresh === "step_up") return stepUpRedirect(ACCOUNT);
  if (fresh === "no_factor" && !recentSignIn(ctx.env, who.session, ctx.now)) return redirect(withParams(ctx.s.loginUrl, { reauth: "1", next: ACCOUNT }));
  const v = await verifyAccessToken(ctx.request, ready.setup, ctx.now);
  if (!v.ok) return refuseLink(ctx, who, failure(v), { cause: v.reason });
  const id = identityOf(v.person);
  if (!domainAllowed(ctx.env, ACCESS_METHOD, id)) return refuseLink(ctx, who, "domain_not_allowed", { email: id.email, emailVerified: id.emailVerified });
  return { who, id };
}

// GET /auth/access/link — the confirmation page (a GET never links).
export async function linkPage(ctx: Ctx): Promise<Response> {
  const g = await linkGate(ctx, await signedIn(ctx));
  if (g instanceof Response) return g;
  if ((await listIdentities(ctx.env, g.who.user.id)).some((i) => i.provider === ACCESS_METHOD)) return redirect(withParams(ACCOUNT, { error: "provider_already_linked" }));
  const other = await findIdentity(ctx.env, ACCESS_METHOD, g.id.subject);
  if (other && other.user_id !== g.who.user.id) return refuseLink(ctx, g.who, "identity_in_use", { email: g.id.email });
  return confirmPage(ctx, g.who, g.id.email, formOrigins(signInMethods(ctx.env)));
}

// POST /auth/access/link — the confirmation's plain form (CSRF-checked).
export async function linkPost(request: Request, env: Env): Promise<Response> {
  const ctx = await preparePost(request, env, true);
  if (ctx instanceof Response) return ctx;
  const who = await whoAmI(ctx, false);
  const g = await linkGate(ctx, who ?? redirect(withParams(ctx.s.loginUrl, { next: ACCOUNT })));
  if (g instanceof Response) return g;
  const r = await linkIdentity(env, g.who.user.id, ACCESS_METHOD, g.id, ctx.now);
  if (r === "in_use") return refuseLink(ctx, g.who, "identity_in_use", { email: g.id.email });
  if (r === "provider_linked") return refuseLink(ctx, g.who, "provider_already_linked", { email: g.id.email });
  await emitEvent(env, { kind: "method_linked", userId: g.who.user.id, method: ACCESS_METHOD, detail: { email: g.id.email, emailVerified: g.id.emailVerified, via: "account" } }, ctx.now);
  return redirect(withParams(ACCOUNT, { ok: "method_linked" }));
}
