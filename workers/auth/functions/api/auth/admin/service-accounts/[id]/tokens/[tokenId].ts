// Cloudflare Pages Function: revoke one service token (GO-141).
//   DELETE /api/auth/admin/service-accounts/:id/tokens/:tokenId   → 200 {ok: true}
//
// Requires `auth:admin`, never on an account holding more than the caller (403
// escalation, SR-04). The next exchange of the token fails (401
// invalid_token, like an unknown one); an access token it was already
// exchanged for keeps working in other workers until it expires — at most
// AUTH_SERVICE_ACCESS_MINUTES (default 10) + 30 s leeway. Revoking twice is
// fine (the first time and author are kept). 404 token_not_found, also for a
// token of another account.

import { authorize, notFound, param } from "../../../../_admin";
import { adminEvent, mayActOn } from "../../../../_admin-sensitive";
import { json } from "../../../../_lib";
import type { Env } from "../../../../_lib";
import { revokeToken } from "../../../../_service-tokens";

export const onRequestDelete: PagesFunction<Env> = async ({ request, env, params }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  // SR-04: never the tokens of an account holding more than the caller.
  const refused = await mayActOn(caller, param(params, "id"));
  if (refused) return refused;
  const r = await revokeToken(env, param(params, "id"), param(params, "tokenId"), caller.user.id, caller.ctx.now);
  if (r !== "ok") return notFound("token");
  // GO-138: audit service_token_revoked.
  await adminEvent(caller, "service_token_revoked", null, { serviceAccount: param(params, "id"), token: param(params, "tokenId") });
  return json({ ok: true });
};
