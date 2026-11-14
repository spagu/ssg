// Cloudflare Pages Function: where am I signed in.
//   GET /api/auth/sessions   → 200 {items: [{id, createdAt, lastUsedAt, expiresAt, current}], nextCursor: null}
//
// The caller's own live sessions, most recently used first; `current` marks
// the one making the request. Never a token, a hash, an IP or a User-Agent.
// 401 not_authenticated without a valid access token.

import { authorize } from "../_admin";
import { listOwnSessions } from "../_account";
import { json } from "../_lib";
import type { Env } from "../_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, null);
  if (caller instanceof Response) return caller;
  return json({ items: await listOwnSessions(env, caller.user.id, caller.session.id, caller.ctx.now), nextCursor: null });
};
