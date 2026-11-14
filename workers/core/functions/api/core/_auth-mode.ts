// The `auth` sign-in mode: the session of the optional `auth` worker. A
// leading underscore keeps this file out of the Pages route table.
//
// core does not verify the session token itself: _session-verifier.ts does —
// a byte-identical copy of workers/auth/portable/session-verifier.ts (the
// copies test keeps it equal), which checks auth's ES256 token locally and
// never asks `auth`. Its contract (workers/auth/docs/CONTRACT.md) fixes the
// variables, read here with the prefix CORE:
//   CORE_AUTH_ISSUER, CORE_AUTH_LOGIN_URL, and exactly one of
//   CORE_AUTH_PUBLIC_KEY (recommended) / CORE_AUTH_JWKS_URL; CORE_AUTH_AUDIENCE
//   optional (default: the request's host).
// None set → this mode is off. Some but not all → every request is answered
// 503 not_configured naming what is missing: never a weaker mode.

import { configFrom, denied, verifySession } from "./_session-verifier";
import type { AuthConfig } from "./_session-verifier";
import type { Env } from "./_lib";

export type { AuthConfig };
export type AuthOutcome = { ok: true; user: { id: string; email: string }; perms: string[] } | { ok: false; response: Response };

// authConfig is the mode's configuration, or null when no CORE_AUTH_* is set.
export const authConfig = (env: Env): AuthConfig | null => configFrom(env, "CORE");

// verifyAuth checks the caller's session. A refusal is the contract's answer:
// 503 when misconfigured or the keys cannot be had, 401 JSON for an API call,
// a 302 to the login page for a page.
export async function verifyAuth(request: Request, config: AuthConfig): Promise<AuthOutcome> {
  const v = await verifySession(request, config);
  if (!v.ok) return { ok: false, response: denied(request, config, v.reason) };
  return { ok: true, user: v.user, perms: v.perms };
}
