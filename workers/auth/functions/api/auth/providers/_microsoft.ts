// Microsoft identity platform (Entra ID and personal Microsoft accounts) —
// OpenID Connect v2.0, authorization code with PKCE (GO-151). A leading
// underscore keeps this file out of the Pages route table.
//   AUTH_MICROSOFT_CLIENT_ID (var), AUTH_MICROSOFT_CLIENT_SECRET (secret),
//   AUTH_MICROSOFT_TENANT (var): common (default) | organizations | consumers
//   | one tenant id (a GUID) — the last restricts sign-in to that tenant.
//
// The issuer is per tenant: https://login.microsoftonline.com/{tid}/v2.0,
// where {tid} is the token's own `tid` claim. A multi-tenant app therefore
// cannot compare `iss` to one string; it checks that `tid` is a tenant id,
// that `iss` is exactly the template filled with it, and — when the site
// restricts — that `tid` is the allowed one (consumers = the fixed tenant of
// personal accounts; organizations = anything but it).
//
// Microsoft's `email` claim is NOT verified by default (an administrator of
// any tenant can set any address). It counts as verified only when the token
// carries the optional claim xms_edov ("email domain owner verified") set to
// true; without it an address never matches an invitation by itself — the
// invitation link still works (docs/PROVIDERS.md says how to add the claim).

import type { Provider } from "../_provider";
import { clientSetup, envVar } from "../_provider-env";
import { exchangeCode } from "../_provider-exchange";
import { idTokenOf, truthy, verifyIdToken } from "../_provider-oidc";

const HOST = "login.microsoftonline.com";
const HOSTS = [HOST] as const;
export const CONSUMERS_TENANT = "9188040d-6c67-4c5b-b112-36a304b66dad";
const GUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const KEYWORDS = ["common", "organizations", "consumers"];

// tenantAllows: may a token from tenant `tid` sign in, for the configured tenant?
export function tenantAllows(tenant: string, tid: string): boolean {
  if (!GUID.test(tid)) return false;
  if (tenant === "common") return true;
  if (tenant === "consumers") return tid === CONSUMERS_TENANT;
  if (tenant === "organizations") return tid !== CONSUMERS_TENANT;
  return tid === tenant;
}

export const microsoft: Provider = {
  id: "microsoft",
  name: "Microsoft",
  label: "signIn",
  pkce: true,
  formPost: false,
  setup(env) {
    const tenant = (envVar(env, "microsoft", "TENANT") || "common").toLowerCase();
    const s = clientSetup(env, "microsoft", { tenant });
    if (!s) return null;
    if (!KEYWORDS.includes(tenant) && !GUID.test(tenant)) return { problem: "AUTH_MICROSOFT_TENANT must be common, organizations, consumers or a tenant id (GUID)" };
    return s;
  },
  authorizeOrigin: () => `https://${HOST}`,
  authorizeUrl(s, a) {
    const u = new URL(`https://${HOST}/${s.extra.tenant}/oauth2/v2.0/authorize`);
    const q = { client_id: s.clientId, redirect_uri: a.redirectUri, response_type: "code", response_mode: "query", scope: "openid email profile", state: a.state, nonce: a.nonce, prompt: "select_account" };
    for (const [k, v] of Object.entries(q)) u.searchParams.set(k, v);
    if (a.challenge) u.searchParams.set("code_challenge", a.challenge);
    if (a.challenge) u.searchParams.set("code_challenge_method", "S256");
    return u.href;
  },
  async identify(s, x) {
    const tenant = s.extra.tenant;
    const got = await exchangeCode(`https://${HOST}/${tenant}/oauth2/v2.0/token`, HOSTS, s.clientId, s.clientSecret, x);
    if (!got.ok) return got;
    const idToken = idTokenOf(got.data);
    if (!idToken) return { ok: false, detail: "Microsoft answered without an ID token" };
    const issuer = (iss: unknown, c: Record<string, unknown>): boolean =>
      typeof c.tid === "string" && tenantAllows(tenant, c.tid) && iss === `https://${HOST}/${c.tid}/v2.0`;
    const jwksUrl = `https://${HOST}/${GUID.test(tenant) ? tenant : "common"}/discovery/v2.0/keys`;
    const v = await verifyIdToken(idToken, { jwksUrl, hosts: HOSTS, algs: ["RS256"], audience: s.clientId, nonce: x.nonce, now: x.now, issuer });
    if (!v.ok) return v;
    const c = v.claims;
    const email = typeof c.email === "string" ? c.email : "";
    return { ok: true, identity: { subject: c.sub as string, email, emailVerified: truthy(c.xms_edov) || c.xms_edov === 1, name: typeof c.name === "string" ? c.name : "" } };
  },
};
