// The provider's answer (GO-151): what GET /api/auth/oauth/<provider>/callback
// does before anyone is signed in. A leading underscore keeps this file out of
// the Pages route table. In order, each step a refusal:
//   1. the flow this browser started (cookie handle + state), consumed;
//   2. the provider's own error (cancelled consent, or anything else);
//   3. the redirect URI of this request equals the one the flow used;
//   4. the code exchanged and the identity verified (providers/*);
//   5. the address domain allowed for the provider (_provider-policy.ts);
//   6. the purpose: sign in, accept an invitation, link (_provider-signin.ts).
// Every refusal is a 303 to a page with ?error=<code> — the login page, or
// the account page for a link — and an audit event; the flow cookie is
// cleared in every answer. Why a provider failed goes to the log only, with
// no token, code or secret in it.

import type { Ctx } from "./_context";
import { matchableEmail } from "./_access-identity";
import { normEmail } from "./_lib";
import { redirectUriOf } from "./_provider";
import type { Identity, Provider, Setup } from "./_provider";
import { consumeFlow } from "./_provider-flow";
import { domainAllowed } from "./_provider-policy";
import { acceptWith, linkWith, refuse, signInWith } from "./_provider-signin";

export interface Answer {
  code: string;
  state: string;
  error: string;
  user: string;
}

// The provider errors that mean "the person said no" (RFC 6749 4.1.2.1;
// Apple's own code).
const CANCELLED = new Set(["access_denied", "user_cancelled_authorize"]);

// trusted: the address counts as verified only when the provider said so AND
// it can be matched without being mistaken for another one — printable ASCII
// that looks like an address (matchableEmail, the rule Access already used).
// Lower-casing maps some non-ASCII letters onto ASCII (U+212A KELVIN SIGN →
// "k"), so a "verified" "\u212Aate@example.com" would otherwise accept the
// invitation for kate@example.com (security review SR-06).
// The providers hand the address over as given; this is its one normalisation
// (normEmail, as for every stored address) — after the ASCII check, never before.
const trusted = (id: Identity): Identity => {
  const matchable = matchableEmail(id.email);
  return { ...id, email: matchable || normEmail(id.email).slice(0, 254), emailVerified: id.emailVerified && matchable !== "", name: [...id.name].slice(0, 200).join("") };
};

export async function callback(ctx: Ctx, p: Provider, setup: Setup, a: Answer): Promise<Response> {
  const flow = await consumeFlow(ctx, p.id, a.state);
  if (!flow) return refuse(ctx, p, null, "oauth_state_invalid");
  if (a.error) return refuse(ctx, p, flow, CANCELLED.has(a.error) ? "oauth_cancelled" : "provider_failed", { providerError: a.error.slice(0, 64) });
  if (!a.code) return refuse(ctx, p, flow, "provider_failed", { cause: "no code" });
  if (flow.redirect_uri !== redirectUriOf(ctx.request, p.id)) return refuse(ctx, p, flow, "provider_failed", { cause: "redirect URI mismatch" });
  const got = await p.identify(setup, { code: a.code, redirectUri: flow.redirect_uri, verifier: flow.verifier, nonce: flow.nonce, now: ctx.now, user: a.user });
  if (!got.ok) {
    console.warn(`auth: ${p.id} sign-in refused: ${got.detail}`);
    return refuse(ctx, p, flow, "provider_failed", { cause: got.detail });
  }
  const id = trusted(got.identity);
  if (!domainAllowed(ctx.env, p.id, id)) return refuse(ctx, p, flow, "domain_not_allowed", { email: id.email, emailVerified: id.emailVerified });
  if (flow.purpose === "invite") return acceptWith(ctx, p, id, flow);
  if (flow.purpose === "link") return linkWith(ctx, p, id, flow);
  return signInWith(ctx, p, id, flow);
}
