// Google — OpenID Connect, authorization code with PKCE (GO-151). A leading
// underscore keeps this file out of the Pages route table.
//   AUTH_GOOGLE_CLIENT_ID (var), AUTH_GOOGLE_CLIENT_SECRET (secret)
// Google's ID token carries `email_verified`; the subject is `sub`.

import type { Provider } from "../_provider";
import { clientSetup } from "../_provider-env";
import { exchangeCode } from "../_provider-exchange";
import { idTokenOf, truthy, verifyIdToken } from "../_provider-oidc";

const AUTHORIZE = "https://accounts.google.com/o/oauth2/v2/auth";
const TOKEN = "https://oauth2.googleapis.com/token";
const JWKS = "https://www.googleapis.com/oauth2/v3/certs";
const HOSTS = ["oauth2.googleapis.com", "www.googleapis.com"] as const;
const ISSUERS = ["https://accounts.google.com", "accounts.google.com"];

export const google: Provider = {
  id: "google",
  name: "Google",
  label: "signIn",
  pkce: true,
  formPost: false,
  setup: (env) => clientSetup(env, "google"),
  authorizeOrigin: () => "https://accounts.google.com",
  authorizeUrl(s, a) {
    const u = new URL(AUTHORIZE);
    const q = { client_id: s.clientId, redirect_uri: a.redirectUri, response_type: "code", scope: "openid email profile", state: a.state, nonce: a.nonce, prompt: "select_account" };
    for (const [k, v] of Object.entries(q)) u.searchParams.set(k, v);
    if (a.challenge) u.searchParams.set("code_challenge", a.challenge);
    if (a.challenge) u.searchParams.set("code_challenge_method", "S256");
    return u.href;
  },
  async identify(s, x) {
    const got = await exchangeCode(TOKEN, HOSTS, s.clientId, s.clientSecret, x);
    if (!got.ok) return got;
    const idToken = idTokenOf(got.data);
    if (!idToken) return { ok: false, detail: "Google answered without an ID token" };
    const v = await verifyIdToken(idToken, { jwksUrl: JWKS, hosts: HOSTS, algs: ["RS256"], audience: s.clientId, nonce: x.nonce, now: x.now, issuer: (iss) => ISSUERS.includes(iss as string) });
    if (!v.ok) return v;
    const c = v.claims;
    return { ok: true, identity: { subject: c.sub as string, email: typeof c.email === "string" ? c.email : "", emailVerified: truthy(c.email_verified), name: typeof c.name === "string" ? c.name : "" } };
  },
};
