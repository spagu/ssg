// Facebook Login — OAuth 2.0, authorization code with PKCE (GO-151). A
// leading underscore keeps this file out of the Pages route table.
//   AUTH_FACEBOOK_CLIENT_ID (var: the App ID), AUTH_FACEBOOK_CLIENT_SECRET
//   (secret: the App Secret), AUTH_FACEBOOK_API_VERSION (var, default below)
//
// The identity comes from the Graph API (/me) with the access token; the
// subject is the app-scoped user id. Facebook does NOT assert that the
// address is verified, so its address is always treated as unverified: it is
// shown, never matched — Facebook accepts an invitation only through the
// invitation link. Graph calls carry appsecret_proof (an HMAC of the token
// with the App Secret), which Facebook can be told to require.

import type { Provider } from "../_provider";
import { clientSetup, envVar } from "../_provider-env";
import { bearer, exchangeCode } from "../_provider-exchange";
import { okJson, providerFetch } from "../_provider-fetch";

export const FACEBOOK_DEFAULT_VERSION = "v24.0";
const HOSTS = ["graph.facebook.com"] as const;

async function appSecretProof(token: string, secret: string): Promise<string> {
  const key = await crypto.subtle.importKey("raw", new TextEncoder().encode(secret), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  const mac = new Uint8Array(await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(token)));
  return [...mac].map((b) => b.toString(16).padStart(2, "0")).join("");
}

export const facebook: Provider = {
  id: "facebook",
  name: "Facebook",
  label: "continue",
  pkce: true,
  formPost: false,
  setup(env) {
    const version = envVar(env, "facebook", "API_VERSION") || FACEBOOK_DEFAULT_VERSION;
    const s = clientSetup(env, "facebook", { version });
    if (!s) return null;
    return /^v\d{1,3}\.\d{1,3}$/.test(version) ? s : { problem: "AUTH_FACEBOOK_API_VERSION must look like v24.0" };
  },
  authorizeOrigin: () => "https://www.facebook.com",
  authorizeUrl(s, a) {
    const u = new URL(`https://www.facebook.com/${s.extra.version}/dialog/oauth`);
    const q = { client_id: s.clientId, redirect_uri: a.redirectUri, response_type: "code", scope: "public_profile,email", state: a.state };
    for (const [k, v] of Object.entries(q)) u.searchParams.set(k, v);
    if (a.challenge) u.searchParams.set("code_challenge", a.challenge);
    if (a.challenge) u.searchParams.set("code_challenge_method", "S256");
    return u.href;
  },
  async identify(s, x) {
    const base = `https://graph.facebook.com/${s.extra.version}`;
    const got = await exchangeCode(`${base}/oauth/access_token`, HOSTS, s.clientId, s.clientSecret, x);
    if (!got.ok) return got;
    const auth = bearer(got.data);
    if (!auth) return { ok: false, detail: "Facebook answered without an access token" };
    const proof = await appSecretProof(auth.slice("Bearer ".length), s.clientSecret);
    const me = okJson(await providerFetch(`${base}/me?fields=id,name,email&appsecret_proof=${proof}`, HOSTS, { headers: { authorization: auth } }), "Facebook /me");
    if (!me.ok) return me;
    const id = me.data.id;
    if (typeof id !== "string" || !/^\d{1,32}$/.test(id)) return { ok: false, detail: "Facebook /me: no id" };
    const email = typeof me.data.email === "string" ? me.data.email : "";
    return { ok: true, identity: { subject: id, email, emailVerified: false, name: typeof me.data.name === "string" ? me.data.name : "" } };
  },
};
