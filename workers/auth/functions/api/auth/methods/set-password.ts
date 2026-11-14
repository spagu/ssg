// Cloudflare Pages Function: give a password to an account that has none
// (GO-151 — an account made through a login provider).
//   POST /api/auth/methods/set-password   JSON {password} with x-ssg-auth: 1,
//                                         or the account page's plain form
//
// Needs a live session started within AUTH_RECENT_SIGNIN_MINUTES, and the
// password login turned on. The password must pass the usual rules. An
// account that has a password changes it through POST /api/auth/password
// (with the current one) instead. JSON: 200 {ok: true}; 401
// not_authenticated; 403 reauth_required; 404 method_disabled (password login
// is off); 409 password_exists; 400 <password rule>; 503
// hashing_unavailable. A form gets a 303 to /auth/account with
// ?ok=password_set or ?error=<code> (the login page when not signed in or not
// recently).

import { emitEvent } from "../_audit";
import { hashingUnavailable } from "../_context";
import type { Env } from "../_lib";
import { hashPassword, passwordProblem } from "../_password";
import { changeGate } from "../_provider-account";
import { setFirstPassword } from "../_provider-identities";
import { PASSWORD_METHOD } from "../_provider-policy";
import { str } from "../_request";
import { passwordEnabled } from "../_signin";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const g = await changeGate(request, env);
  if (g instanceof Response) return g;
  const { ctx, who } = g;
  if (!passwordEnabled(env)) return g.fail("method_disabled", 404);
  if (who.user.password_hash !== null) return g.fail("password_exists", 409);
  const problem = passwordProblem(g.body.password, who.user.email, ctx.s);
  if (problem) return g.fail(problem, 400);
  let hash: string;
  try {
    hash = await hashPassword(str(g.body.password), ctx.s.iterations);
  } catch {
    return hashingUnavailable();
  }
  if (!(await setFirstPassword(env, who.user.id, hash, ctx.now))) return g.fail("password_exists", 409);
  await emitEvent(env, { kind: "password_set", userId: who.user.id, method: PASSWORD_METHOD }, ctx.now);
  return g.done("password_set", { ok: true });
};
