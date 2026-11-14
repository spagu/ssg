// Sign in with Apple — OpenID Connect (GO-151). A leading underscore keeps
// this file out of the Pages route table.
//   AUTH_APPLE_CLIENT_ID (var: the Services ID), AUTH_APPLE_TEAM_ID (var),
//   AUTH_APPLE_KEY_ID (var), AUTH_APPLE_PRIVATE_KEY (secret: the .p8 key, PEM)
//
// Apple's quirks:
//   - there is no static client secret: each token request carries a JWT
//     signed with the private key (ES256), naming the team, the key and the
//     Services ID, valid for 5 minutes here (Apple allows up to 6 months);
//   - asking for the name or the address requires response_mode=form_post:
//     Apple POSTs the answer to the callback from its own origin (the callback
//     bounces it to a same-site GET, see oauth/[provider]/callback.ts);
//   - the name arrives ONCE, on the first consent, in a `user` form field
//     that Apple does not sign — used only as the display name of a new
//     account, never for anything that decides access;
//   - PKCE is not part of Apple's documented token request, so it is not sent;
//     `state` (bound to the browser) and `nonce` (in the signed ID token) are.

import type { Provider } from "../_provider";
import { envVar } from "../_provider-env";
import { b64urlDecode, b64urlEncode } from "../_lib";
import { exchangeCode } from "../_provider-exchange";
import { idTokenOf, truthy, verifyIdToken } from "../_provider-oidc";

const ORIGIN = "https://appleid.apple.com";
const HOSTS = ["appleid.apple.com"] as const;
const ID10 = /^[A-Z0-9]{10}$/;

const pemBody = (pem: string): string | null => {
  const m = /^-----BEGIN PRIVATE KEY-----([A-Za-z0-9+/=\s\\n]+)-----END PRIVATE KEY-----$/.exec(pem.trim());
  return m ? m[1].replace(/\\n|\s/g, "") : null;
};

const enc = (v: unknown): string => b64urlEncode(new TextEncoder().encode(JSON.stringify(v)));

// clientSecret signs Apple's per-request client secret (ES256 with WebCrypto;
// WebCrypto's ECDSA signature is already the raw r||s form JWS wants).
export async function clientSecret(extra: Record<string, string>, clientId: string, now: number): Promise<string | null> {
  const der = b64urlDecode((extra.pem as string).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, ""));
  let key: CryptoKey;
  try {
    key = await crypto.subtle.importKey("pkcs8", der as Uint8Array, { name: "ECDSA", namedCurve: "P-256" }, false, ["sign"]);
  } catch {
    return null;
  }
  const iat = Math.floor(now / 1000);
  const input = `${enc({ alg: "ES256", kid: extra.keyId })}.${enc({ iss: extra.teamId, iat, exp: iat + 300, aud: ORIGIN, sub: clientId })}`;
  const sig = new Uint8Array(await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, key, new TextEncoder().encode(input)));
  return `${input}.${b64urlEncode(sig)}`;
}

// nameOf reads the one-time `user` field: {"name":{"firstName","lastName"}}.
export function nameOf(user: string): string {
  try {
    const n = (JSON.parse(user) as { name?: { firstName?: unknown; lastName?: unknown } }).name;
    return [n?.firstName, n?.lastName].filter((s) => typeof s === "string" && s.trim()).join(" ").slice(0, 200);
  } catch {
    return "";
  }
}

export const apple: Provider = {
  id: "apple",
  name: "Apple",
  label: "signIn",
  pkce: false,
  formPost: true,
  setup(env) {
    const clientId = envVar(env, "apple", "CLIENT_ID");
    const teamId = envVar(env, "apple", "TEAM_ID");
    const keyId = envVar(env, "apple", "KEY_ID");
    const key = envVar(env, "apple", "PRIVATE_KEY");
    if (!clientId || !teamId || !keyId || !key) return null;
    if (!ID10.test(teamId) || !ID10.test(keyId)) return { problem: "AUTH_APPLE_TEAM_ID and AUTH_APPLE_KEY_ID are the 10-character ids from the Apple developer account" };
    const pem = pemBody(key);
    if (!pem) return { problem: "AUTH_APPLE_PRIVATE_KEY must be the .p8 key file's text (-----BEGIN PRIVATE KEY----- …)" };
    return { clientId, clientSecret: "", extra: { teamId, keyId, pem } };
  },
  authorizeOrigin: () => ORIGIN,
  authorizeUrl(s, a) {
    const u = new URL(`${ORIGIN}/auth/authorize`);
    const q = { client_id: s.clientId, redirect_uri: a.redirectUri, response_type: "code", response_mode: "form_post", scope: "name email", state: a.state, nonce: a.nonce };
    for (const [k, v] of Object.entries(q)) u.searchParams.set(k, v);
    return u.href;
  },
  async identify(s, x) {
    const secret = await clientSecret(s.extra, s.clientId, x.now);
    if (!secret) return { ok: false, detail: "AUTH_APPLE_PRIVATE_KEY is not a P-256 private key" };
    const got = await exchangeCode(`${ORIGIN}/auth/token`, HOSTS, s.clientId, secret, x);
    if (!got.ok) return got;
    const idToken = idTokenOf(got.data);
    if (!idToken) return { ok: false, detail: "Apple answered without an ID token" };
    const v = await verifyIdToken(idToken, { jwksUrl: `${ORIGIN}/auth/keys`, hosts: HOSTS, algs: ["RS256"], audience: s.clientId, nonce: x.nonce, now: x.now, issuer: (iss) => iss === ORIGIN });
    if (!v.ok) return v;
    const c = v.claims;
    return { ok: true, identity: { subject: c.sub as string, email: typeof c.email === "string" ? c.email : "", emailVerified: truthy(c.email_verified), name: nameOf(x.user) } };
  },
};
