// POST /api/shop/admin/auth/login — the built-in password mode only.
//
// With Cloudflare Access configured this endpoint answers 404: there is no
// password, so there is nothing here to attack (S-15).

import { authMode, type Env } from "../../_env";
import { audit } from "../../_audit";
import {
  clearLoginFailures,
  createRefresh,
  loginAllowed,
  maybeBootstrap,
  noteLoginFailure,
  refreshCookie,
  signJwt,
  verifyPassword,
} from "../../_auth";
import { fail, isValidEmail, json, nowISO, readJson, str } from "../../_lib";
import type { AdminUserRow } from "../../_types";

interface LoginBody {
  email?: string;
  password?: string;
}

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  if (authMode(env) !== "jwt") {
    return fail(request, "not_found", "This shop signs in through Cloudflare Access.", 404);
  }

  const body = await readJson<LoginBody>(request);
  if (!body) return fail(request, "invalid_body", "The request body could not be read.", 400);

  const email = str(body.email, 254).toLowerCase();
  const password = typeof body.password === "string" ? body.password : "";
  if (!isValidEmail(email) || !password) {
    return fail(request, "invalid_credentials", "Enter an email address and a password.", 422);
  }

  const ip = request.headers.get("cf-connecting-ip") ?? "unknown";
  if (!(await loginAllowed(env, ip, email))) {
    return fail(request, "too_many_attempts", "Too many attempts. Wait a few minutes and try again.", 429);
  }

  // The first owner comes from a secret, once. After that this is a no-op.
  await maybeBootstrap(env);

  const user = await env.SHOP_DB.prepare(`SELECT * FROM admin_users WHERE email_lc = ?`)
    .bind(email)
    .first<AdminUserRow>();

  // The same answer whether the address is unknown or the password is wrong, so
  // this cannot be used to find out who has an account (S-17).
  if (!user || !(await verifyPassword(password, user.pass_hash))) {
    await noteLoginFailure(env, ip, email);
    return fail(request, "invalid_credentials", "Those details do not match.", 401);
  }

  await clearLoginFailures(env, ip, email);
  await env.SHOP_DB.prepare(`UPDATE admin_users SET last_login_at = ? WHERE id = ?`)
    .bind(nowISO(), user.id)
    .run();
  await audit(env, `admin:${user.id}`, "admin.login", user.id, { email });

  const issuer = new URL(request.url).origin;
  const accessToken = await signJwt(env, user, issuer);
  const refresh = await createRefresh(env, user.id);

  return json(
    { accessToken, role: user.role, email: user.email_lc, expiresIn: 900 },
    200,
    { "set-cookie": refreshCookie(refresh, 7 * 86400) },
  );
};
