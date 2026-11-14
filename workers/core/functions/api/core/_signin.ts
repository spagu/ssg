// Who is calling, and how did they sign in. A leading underscore keeps this
// file out of the Pages route table.
//
// Three modes, in the house order — the first one configured wins, the others
// are not consulted:
//   1. Cloudflare Access  CORE_ACCESS_TEAM + CORE_ACCESS_AUD
//   2. `auth` worker      any CORE_AUTH_* variable (_auth-mode.ts)
//   3. shared password    CORE_ADMIN_PASSWORD (HTTP Basic, _password.ts)
// None configured, or a mode configured by halves → 503. Never open.
//
// Only the `auth` mode knows permissions; with Access or a password every
// signed-in caller sees every module (perms: null).

import { verifyAccess } from "./_access";
import { authConfig, verifyAuth } from "./_auth-mode";
import { problem } from "./_lib";
import type { Env } from "./_lib";
import { verifyPassword } from "./_password";

export type Mode = "access" | "auth" | "password";

export interface Caller {
  mode: Mode;
  user: { id?: string; email: string } | null; // null: a shared password names nobody
  perms: string[] | null; // null: this mode has no permissions, everything is visible
}

export type SignIn = { ok: true; caller: Caller } | { ok: false; response: Response };

const set = (v: string | undefined): boolean => Boolean(v?.trim());

export async function signIn(request: Request, env: Env): Promise<SignIn> {
  const team = set(env.CORE_ACCESS_TEAM);
  const aud = set(env.CORE_ACCESS_AUD);
  if (team !== aud) {
    const missing = team ? "CORE_ACCESS_AUD" : "CORE_ACCESS_TEAM";
    return { ok: false, response: problem("access_misconfigured", 503, `Cloudflare Access needs both CORE_ACCESS_TEAM and CORE_ACCESS_AUD; ${missing} is not set`) };
  }
  if (team) {
    const got = await verifyAccess(request, env.CORE_ACCESS_TEAM!.trim(), env.CORE_ACCESS_AUD!.trim());
    return got.ok ? { ok: true, caller: { mode: "access", user: got.email ? { email: got.email } : null, perms: null } } : got;
  }
  const auth = authConfig(env);
  if (auth) {
    const got = await verifyAuth(request, auth);
    return got.ok ? { ok: true, caller: { mode: "auth", user: { id: got.user.id, email: got.user.email }, perms: got.perms } } : got;
  }
  const password = env.CORE_ADMIN_PASSWORD;
  if (password) {
    const got = await verifyPassword(request, env, password);
    return got.ok ? { ok: true, caller: { mode: "password", user: null, perms: null } } : got;
  }
  return {
    ok: false,
    response: problem("signin_not_configured", 503, "set CORE_ACCESS_TEAM + CORE_ACCESS_AUD, the CORE_AUTH_* variables, or the CORE_ADMIN_PASSWORD secret"),
  };
}
