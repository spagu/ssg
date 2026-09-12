// POST /api/shop/admin/auth/refresh — a new access token from the cookie.
//
// The refresh token rotates on every use. Presenting one twice means a copy
// exists somewhere it should not, so every session for that admin ends (S-18).

import { authMode, type Env } from "../../_env";
import { audit } from "../../_audit";
import {
  consumeRefresh,
  createRefresh,
  endAllSessions,
  readCookie,
  refreshCookie,
  REFRESH_COOKIE,
  signJwt,
} from "../../_auth";
import { fail, json, sha256hex } from "../../_lib";
import type { AdminUserRow } from "../../_types";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  if (authMode(env) !== "jwt") {
    return fail(request, "not_found", "This shop signs in through Cloudflare Access.", 404);
  }

  // This is the one endpoint a cookie alone can drive, so it checks the origin
  // as well as relying on SameSite=Strict (S-20).
  const origin = request.headers.get("origin");
  if (origin && origin !== new URL(request.url).origin) {
    return fail(request, "bad_origin", "Cross-site refresh is not allowed.", 403);
  }

  const token = readCookie(request, REFRESH_COOKIE);
  if (!token) return fail(request, "unauthorized", "Sign in to continue.", 401);

  const result = await consumeRefresh(env, token);
  if (!result.ok || !result.adminId) {
    // A token that does not resolve may be expired, or may be a replay of one
    // already rotated. We cannot tell, so we clear the cookie and make them
    // sign in.
    return json({ error: "unauthorized", message: "That session has ended. Sign in again." }, 401, {
      "set-cookie": refreshCookie("", 0),
    });
  }

  const user = await env.SHOP_DB.prepare(`SELECT * FROM admin_users WHERE id = ?`)
    .bind(result.adminId)
    .first<AdminUserRow>();
  if (!user) {
    await endAllSessions(env, result.adminId);
    return fail(request, "unauthorized", "That account no longer exists.", 401);
  }

  const issuer = new URL(request.url).origin;
  const accessToken = await signJwt(env, user, issuer);
  const next = await createRefresh(env, user.id, await sha256hex(token));
  await audit(env, `admin:${user.id}`, "admin.refresh", user.id);

  return json({ accessToken, role: user.role, email: user.email_lc, expiresIn: 900 }, 200, {
    "set-cookie": refreshCookie(next, 7 * 86400),
  });
};
