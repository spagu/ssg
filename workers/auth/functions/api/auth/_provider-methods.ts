// The sign-in methods a site allows (GO-151): the password and every enabled
// login provider, in display order. A leading underscore keeps this file out
// of the Pages route table. _signin.ts re-exports these — it is where the
// pages and routes look — but they live here because _context.ts needs them
// too (an empty list is a configuration error) and _signin.ts imports
// _context.ts.
//
// The password is a method like the others: AUTH_PASSWORD_LOGIN=off turns it
// off. Turning EVERYTHING off is refused: every route that needs the database
// answers 503 no_sign_in_method naming the variables, rather than a site
// nobody can enter. GO-139 (Cloudflare Access) adds its entry here, last:
// enabled by AUTH_ACCESS_TEAM + AUTH_ACCESS_AUD (_access-config.ts); a half or
// unusable configuration offers no entry and is logged, like a provider's.

import { ACCESS_METHOD, accessIssuer, accessLookup } from "./_access-config";
import type { Env } from "./_lib";
import { enabledProviders } from "./_provider";
import type { Provider, Setup } from "./_provider";
import { PASSWORD_METHOD } from "./_provider-policy";

export interface SignInMethod {
  id: string; // "password" or a provider id
  provider?: Provider;
  setup?: Setup;
  // An origin the pages' CSP form-action must allow: Cloudflare Access sends
  // a signed-out browser on to its team host (logout.ts), and browsers apply
  // form-action to the redirect of a form too.
  formOrigin?: string;
}

export const passwordEnabled = (env: Env): boolean => (env.AUTH_PASSWORD_LOGIN ?? "").trim().toLowerCase() !== "off";

// signInMethods lists the methods this site allows, in display order.
export function signInMethods(env: Env): SignInMethod[] {
  const out: SignInMethod[] = passwordEnabled(env) ? [{ id: PASSWORD_METHOD }] : [];
  for (const { provider, setup } of enabledProviders(env)) out.push({ id: provider.id, provider, setup });
  const access = accessLookup(env);
  if (access.kind === "enabled") out.push({ id: ACCESS_METHOD, formOrigin: accessIssuer(access.setup) });
  else if (access.kind === "misconfigured") console.error(`auth: Cloudflare Access is not offered: ${access.problem}`);
  return out;
}

export const methodEnabled = (env: Env, id: unknown): boolean => signInMethods(env).some((m) => m.id === id);

export const NO_METHOD_HINT =
  "AUTH_PASSWORD_LOGIN is off and no login provider is configured, so nobody could sign in: remove AUTH_PASSWORD_LOGIN=off or configure a provider (docs/PROVIDERS.md) or Cloudflare Access (docs/ACCESS.md)";
