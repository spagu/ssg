// Cloudflare Pages Function: who am I.
//   GET /api/auth/session
//
// 200 {user: {id, email, name, mustChangePassword}, expiresAt, sessionExpiresAt}
// for a valid access cookie whose session is still live and whose account is
// active; 401 {error: "not_authenticated"} otherwise (an expired access token
// included — the page then calls POST /api/auth/refresh). No token, hash or
// key is ever part of the answer.

import { prepare, whoAmI } from "./_context";
import { json } from "./_lib";
import type { Env } from "./_lib";
import { publicUser } from "./_users";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const ctx = await prepare(request, env);
  if (ctx instanceof Response) return ctx;
  const who = await whoAmI(ctx, false);
  if (!who) return json({ error: "not_authenticated" }, 401);
  return json({ user: publicUser(who.user), expiresAt: who.claims!.exp, sessionExpiresAt: who.session.expires_at });
};
