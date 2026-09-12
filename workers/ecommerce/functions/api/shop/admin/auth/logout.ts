// POST /api/shop/admin/auth/logout — end this session now, not in 15 minutes.

import type { Env } from "../../_env";
import { consumeRefresh, readCookie, refreshCookie, REFRESH_COOKIE, revokeJti, verifyJwt } from "../../_auth";
import { json } from "../../_lib";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const cookie = readCookie(request, REFRESH_COOKIE);
  if (cookie) await consumeRefresh(env, cookie);

  // The access token stays valid until it expires unless we say otherwise, so
  // its id goes on the deny list for exactly that long.
  const header = request.headers.get("authorization") ?? "";
  if (header.startsWith("Bearer ")) {
    const payload = await verifyJwt(env, header.slice(7), new URL(request.url).origin);
    if (payload) await revokeJti(env, payload.jti, payload.exp);
  }

  return json({ ok: true }, 200, { "set-cookie": refreshCookie("", 0) });
};
