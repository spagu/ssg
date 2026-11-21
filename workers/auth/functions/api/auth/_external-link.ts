// Linking an external issuer's identity to a signed-in account (#435), in
// three steps. A leading underscore keeps this file out of the Pages route
// table; functions/api/auth/jwt/link.ts and functions/auth/jwt/link.ts route
// here.
//
//   1. POST /api/auth/jwt/link   the issuer's page posts a token, as for a
//      sign-in (acceptToken, _external-entry.ts: CSRF incl.
//      AUTH_JWT_FORM_ORIGINS, per-IP lockout, verification, single use) — and
//      AUTH_JWT_ALLOWED_DOMAINS. Nothing is linked: the identity waits five
//      minutes behind a handle cookie (_external-link-store.ts), and the
//      browser goes on to step 2. This request may come from another site and
//      carry no session, so it cannot know the account.
//   2. GET /auth/jwt/link        a live, RECENT session (a recent second
//      factor for an account with one, a recent sign-in otherwise — the rule
//      of every change of sign-in methods) sees "<issuer> says you are …;
//      connect it to your account?". Without a session: to sign in first and
//      back here, the handle still waiting.
//   3. POST /auth/jwt/link       the confirmation, a plain form from THIS
//      origin only: the identity is linked to the session's account and the
//      handle used up. A page elsewhere that posts an attacker's token in
//      step 1 gets no further than a confirmation the person can refuse.
// Refusals: 303 to the account page with ?error=<code>, audited as
// signin_refused (purpose "link") with the reason — never the token.

import { emitEvent } from "./_audit";
import { prepare, preparePost, whoAmI } from "./_context";
import type { Ctx, Who } from "./_context";
import { JWT_ENV_ID } from "./_external-config";
import type { ExternalSetup } from "./_external-config";
import { acceptToken, externalReady } from "./_external-entry";
import { clearLinkCookie, createLinkPending, readLinkPending, takeLinkPending } from "./_external-link-store";
import { JWT_LINK_PAGE, externalConfirmPage } from "./_external-pages";
import { redirect } from "./_lib";
import type { Env } from "./_lib";
import { freshness, stepUpRedirect } from "./_mfa-step-up";
import { withParams } from "./_next";
import { signedIn } from "./_pages";
import type { Identity } from "./_provider";
import { findIdentity, linkIdentity, listIdentities } from "./_provider-identities";
import { domainAllowed, recentSignIn } from "./_provider-policy";

const ACCOUNT = "/auth/account";

type Detail = Record<string, string | boolean>;
const seenOf = (id: Identity): Detail => ({ email: id.email, emailVerified: id.emailVerified });

async function refuseLink(ctx: Ctx, s: ExternalSetup, userId: string | null, error: string, detail: Detail): Promise<Response> {
  await emitEvent(ctx.env, { kind: "signin_refused", userId, method: s.method, detail: { reason: error, purpose: "link", ...detail } }, ctx.now);
  return redirect(withParams(ACCOUNT, { error }), [clearLinkCookie()]);
}

// Step 1: POST /api/auth/jwt/link.
export async function externalLinkArrival(request: Request, env: Env): Promise<Response> {
  const ctx = await prepare(request, env);
  if (ctx instanceof Response) return ctx;
  const a = await acceptToken(ctx, (s, error, cause) => refuseLink(ctx, s, null, error, { cause }));
  if (a instanceof Response) return a;
  if (!domainAllowed(env, JWT_ENV_ID, a.identity)) return refuseLink(ctx, a.setup, null, "domain_not_allowed", seenOf(a.identity));
  return redirect(JWT_LINK_PAGE, [await createLinkPending(env, a.identity, ctx.now)]);
}

interface Gate {
  setup: ExternalSetup;
  who: Who;
  id: Identity;
}

// linkGate: the method on, a live RECENT session and a waiting identity — or
// the answer to send. `take` uses the handle up.
async function linkGate(ctx: Ctx, who: Who | Response, take: boolean): Promise<Gate | Response> {
  const s = await externalReady(ctx.env);
  if (s instanceof Response) return s;
  if (who instanceof Response) return who;
  const fresh = await freshness(ctx, who);
  if (fresh === "step_up") return stepUpRedirect(JWT_LINK_PAGE);
  if (fresh === "no_factor" && !recentSignIn(ctx.env, who.session, ctx.now)) return redirect(withParams(ctx.s.loginUrl, { reauth: "1", next: JWT_LINK_PAGE }));
  const id = await (take ? takeLinkPending : readLinkPending)(ctx.env, ctx.request, ctx.now);
  return id ? { setup: s, who, id } : refuseLink(ctx, s, who.user.id, "jwt_link_expired", {});
}

// Step 2: GET /auth/jwt/link — the confirmation page (a GET never links),
// unless the link could not be made: this account already has an identity of
// this issuer, or the identity opens another account.
export async function externalLinkPage(ctx: Ctx): Promise<Response> {
  const g = await linkGate(ctx, await signedIn(ctx), false);
  if (g instanceof Response) return g;
  const no = (error: string) => refuseLink(ctx, g.setup, g.who.user.id, error, seenOf(g.id));
  if ((await listIdentities(ctx.env, g.who.user.id)).some((i) => i.provider === g.setup.method)) return no("provider_already_linked");
  const other = await findIdentity(ctx.env, g.setup.method, g.id.subject);
  if (other && other.user_id !== g.who.user.id) return no("identity_in_use");
  return externalConfirmPage(ctx, g.who, g.setup.issuer, g.id);
}

// Step 3: POST /auth/jwt/link — the confirmation's plain form (CSRF-checked).
export async function externalLinkConfirm(request: Request, env: Env): Promise<Response> {
  const ctx = await preparePost(request, env, true);
  if (ctx instanceof Response) return ctx;
  const who = await whoAmI(ctx, false);
  const g = await linkGate(ctx, who ?? redirect(withParams(ctx.s.loginUrl, { next: JWT_LINK_PAGE })), true);
  if (g instanceof Response) return g;
  const seen = seenOf(g.id);
  const r = await linkIdentity(env, g.who.user.id, g.setup.method, g.id, ctx.now);
  if (r !== "ok") return refuseLink(ctx, g.setup, g.who.user.id, r === "in_use" ? "identity_in_use" : "provider_already_linked", seen);
  await emitEvent(env, { kind: "method_linked", userId: g.who.user.id, method: g.setup.method, detail: { ...seen, via: "account" } }, ctx.now);
  return redirect(withParams(ACCOUNT, { ok: "method_linked" }), [clearLinkCookie()]);
}
