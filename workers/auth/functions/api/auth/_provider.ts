// Login providers (GO-151): the one interface every provider implements, and
// the registry of them. A leading underscore keeps this file out of the Pages
// route table. One file per provider lives in providers/; adding one is a new
// file there and one line in PROVIDERS below — nothing else changes.
//
// A provider is ENABLED only when its variables are present: the client id as
// a var, the client secret as a wrangler secret (Apple: team id, key id and a
// private key). Absent → the provider does not exist for this site (no button,
// its routes answer 404). Present but unusable (a malformed tenant, a key that
// is not PEM) → no button, its routes answer 503 naming the variable: a
// configuration error is reported, never silently opened.
//
// What a provider hands back is an Identity: the stable subject at the
// provider (the account's identity here is the pair provider + subject, never
// the address), the address, whether the provider ASSERTS it verified that
// address, and a display name. Provider tokens never leave identify().

import { ACCESS_METHOD, ACCESS_NAME } from "./_access-config";
import { json } from "./_lib";
import type { Env } from "./_lib";
export { clientSetup, envVar } from "./_provider-env";
import { apple } from "./providers/_apple";
import { facebook } from "./providers/_facebook";
import { github } from "./providers/_github";
import { google } from "./providers/_google";
import { microsoft } from "./providers/_microsoft";

export interface Identity {
  subject: string;
  email: string; // "" when the provider gave none; lower-cased
  emailVerified: boolean;
  name: string;
}

// Setup is a provider's configuration for one request.
export interface Setup {
  clientId: string;
  clientSecret: string; // "" for Apple: its secret is a JWT signed per exchange
  extra: Record<string, string>;
}

export interface AuthParams {
  redirectUri: string;
  state: string;
  nonce: string;
  challenge: string | null; // PKCE S256, null for a provider without PKCE
}

export interface Exchange {
  code: string;
  redirectUri: string;
  verifier: string | null;
  nonce: string;
  now: number;
  user: string; // Apple's one-time `user` form field (name on first consent); "" elsewhere
}

export type Identified = { ok: true; identity: Identity } | { ok: false; detail: string };

export interface Provider {
  id: string; // lower-case, the URL segment and the stored method name
  name: string; // the brand name, never translated
  label: "signIn" | "continue"; // the wording the provider's own guidelines use
  pkce: boolean;
  formPost: boolean; // the provider POSTs its answer to the callback (Apple)
  setup(env: Env): Setup | { problem: string } | null;
  authorizeUrl(s: Setup, a: AuthParams): string;
  authorizeOrigin(s: Setup): string; // for the pages' CSP form-action
  identify(s: Setup, x: Exchange): Promise<Identified>;
}

// Display order on the pages.
export const PROVIDERS: readonly Provider[] = [google, microsoft, apple, github, facebook];

export type Lookup = { kind: "enabled"; provider: Provider; setup: Setup } | { kind: "absent" } | { kind: "misconfigured"; problem: string };

// lookupProvider resolves a provider id from a URL for this site.
export function lookupProvider(env: Env, id: unknown): Lookup {
  const provider = PROVIDERS.find((p) => p.id === id);
  if (!provider) return { kind: "absent" };
  const s = provider.setup(env);
  if (!s) return { kind: "absent" };
  if ("problem" in s) return { kind: "misconfigured", problem: s.problem };
  return { kind: "enabled", provider, setup: s };
}

// providerOr is lookupProvider for a route: the provider, or the answer —
// 404 not_found for one this site does not offer (as if the route did not
// exist), 503 provider_misconfigured naming what to fix.
export function providerOr(env: Env, id: unknown): { provider: Provider; setup: Setup } | Response {
  const l = lookupProvider(env, id);
  if (l.kind === "absent") return json({ error: "not_found" }, 404);
  if (l.kind === "misconfigured") return json({ error: "provider_misconfigured", hint: l.problem }, 503);
  return { provider: l.provider, setup: l.setup };
}

// enabledProviders: the providers this site offers, in display order.
export function enabledProviders(env: Env): { provider: Provider; setup: Setup }[] {
  const out: { provider: Provider; setup: Setup }[] = [];
  for (const p of PROVIDERS) {
    const l = lookupProvider(env, p.id);
    if (l.kind === "enabled") out.push({ provider: l.provider, setup: l.setup });
    else if (l.kind === "misconfigured") console.error(`auth: login provider ${p.id} is not offered: ${l.problem}`);
  }
  return out;
}

// providerName: the brand name of any known provider id (enabled or not), or
// the id itself — an identity linked before a provider was turned off still
// shows on the account page. Cloudflare Access (GO-139) is not a provider of
// this list (no OAuth flow) but is named the same way.
export const providerName = (id: string): string => PROVIDERS.find((p) => p.id === id)?.name ?? (id === ACCESS_METHOD ? ACCESS_NAME : id);

// callbackPath: the one redirect URI per provider, on this host.
export const callbackPath = (id: string): string => `/api/auth/oauth/${id}/callback`;
export const redirectUriOf = (request: Request, id: string): string => new URL(request.url).origin + callbackPath(id);
