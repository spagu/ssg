// GET {base}/api/me — who the panel is talking for (GO-137). The panel reads it
// once at start: it hides what the account cannot use, shows who is signed in
// and, in `auth` mode, where to sign in again and out.
//
//   {mode: "password" | "access", user: null, permissions: "all"}
//   {mode: "auth", user: {id, email}, permissions: ["newsletter:read", …],
//    loginUrl, logoutUrl, refreshUrl}
//
// Needs a session only (permission SESSION): an account holding no
// newsletter permission at all still learns that, instead of a bare 403.
// `permissions` lists this module's permissions the token grants — wildcards
// expanded, other workers' permissions left out.

import { AUTH_PREFIX } from "./auth";
import type { Admin } from "./auth";
import { json } from "./lib";
import { held } from "./permissions";
import type { RouteContext } from "./router";
import { configFrom } from "./session-verifier";
import type { AuthConfig } from "./session-verifier";

// LOGOUT_URL, REFRESH_URL: the `auth` worker's endpoints. It lives on the same
// host (the session cookie is per host) under the fixed /api/auth/ prefix.
// Logout takes a plain form POST with `next`; the panel calls refresh once
// when an API call answers 401 (the access token lives minutes), then retries.
export const LOGOUT_URL = "/api/auth/logout";
export const REFRESH_URL = "/api/auth/refresh";

export function me({ env, admin }: RouteContext): Response {
  const who = admin as Admin; // never null on an /api/ route
  const body: Record<string, unknown> = { mode: who.mode, user: who.user, permissions: held(who) };
  if (who.mode === "auth") {
    // authenticate() let the request through, so the configuration is complete.
    body.loginUrl = (configFrom(env, AUTH_PREFIX) as AuthConfig).loginUrl;
    body.logoutUrl = LOGOUT_URL;
    body.refreshUrl = REFRESH_URL;
  }
  return json(body);
}
