// Cloudflare Pages Function: everyone's live sessions (GO-138).
//   GET /api/auth/admin/sessions?userId=&cursor=&limit=   → 200 {items: [session], nextCursor}
//
// Requires `auth:admin`. Newest use first; `userId` narrows to one account;
// `limit` 1–200 (100). A session is {id, userId, email, name, method,
// secondFactor, createdAt, lastUsedAt, expiresAt}: `method` how it started
// (password, a provider, access), `secondFactor` the factor it passed (totp,
// backup, passkey) or null. Never a token or a hash. 400 invalid_request.

import { authorize, invalid } from "../../_admin";
import { liveSessions } from "../../_admin-sessions";
import { json } from "../../_lib";
import type { Env } from "../../_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const q = new URL(request.url).searchParams;
  const limit = q.has("limit") ? Number(q.get("limit")) : 100;
  if (!Number.isInteger(limit) || limit < 1 || limit > 200) return invalid("limit: 1–200");
  return json(await liveSessions(env, caller.ctx.now, q.get("userId") ?? "", q.get("cursor") ?? "", limit));
};
