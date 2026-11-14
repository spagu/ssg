// GitHub — OAuth 2.0 (an OAuth App), authorization code with PKCE (GO-151).
// A leading underscore keeps this file out of the Pages route table.
//   AUTH_GITHUB_CLIENT_ID (var), AUTH_GITHUB_CLIENT_SECRET (secret)
//
// No ID token: the identity comes from the API with the access token. The
// subject is the numeric user id (a login can be renamed and reused, the id
// cannot). The profile's `email` is whatever the person made public, so the
// address comes from GET /user/emails (scope user:email): the PRIMARY address,
// and it counts as verified only when GitHub says so.

import type { Provider } from "../_provider";
import { clientSetup } from "../_provider-env";
import { bearer, exchangeCode } from "../_provider-exchange";
import { okJson, providerFetch } from "../_provider-fetch";

const HOSTS = ["github.com", "api.github.com"] as const;
const API = { "x-github-api-version": "2022-11-28", "user-agent": "ssg-auth-worker" };

interface Email {
  email?: unknown;
  primary?: unknown;
  verified?: unknown;
}

export const github: Provider = {
  id: "github",
  name: "GitHub",
  label: "signIn",
  pkce: true,
  formPost: false,
  setup: (env) => clientSetup(env, "github"),
  authorizeOrigin: () => "https://github.com",
  authorizeUrl(s, a) {
    const u = new URL("https://github.com/login/oauth/authorize");
    const q = { client_id: s.clientId, redirect_uri: a.redirectUri, scope: "read:user user:email", state: a.state, allow_signup: "false" };
    for (const [k, v] of Object.entries(q)) u.searchParams.set(k, v);
    if (a.challenge) u.searchParams.set("code_challenge", a.challenge);
    if (a.challenge) u.searchParams.set("code_challenge_method", "S256");
    return u.href;
  },
  async identify(s, x) {
    const got = await exchangeCode("https://github.com/login/oauth/access_token", HOSTS, s.clientId, s.clientSecret, x);
    if (!got.ok) return got;
    const auth = bearer(got.data);
    if (!auth) return { ok: false, detail: "GitHub answered without an access token" };
    const headers = { ...API, authorization: auth, accept: "application/vnd.github+json" };
    const user = okJson(await providerFetch("https://api.github.com/user", HOSTS, { headers }), "GitHub /user");
    if (!user.ok) return user;
    const id = user.data.id;
    if (typeof id !== "number" || !Number.isSafeInteger(id) || id <= 0) return { ok: false, detail: "GitHub /user: no numeric id" };
    const emails = await providerFetch("https://api.github.com/user/emails", HOSTS, { headers });
    if (!emails.ok) return emails;
    if (emails.status !== 200 || !Array.isArray(emails.data)) return { ok: false, detail: `GitHub /user/emails: status ${emails.status}` };
    const primary = (emails.data as Email[]).find((e) => e && e.primary === true && typeof e.email === "string");
    const name = typeof user.data.name === "string" && user.data.name ? user.data.name : typeof user.data.login === "string" ? user.data.login : "";
    return { ok: true, identity: { subject: String(id), email: primary ? (primary.email as string) : "", emailVerified: primary?.verified === true, name } };
  },
};
