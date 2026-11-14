// Cloudflare Pages Function: the module summary (GO-148 format, GO-138).
//   GET /api/auth/module/summary → {contract, generatedAt, metrics, attention, problems}
//
// Requires `auth:admin`, like the manifest route beside it. Aggregates only:
// counts of accounts, open invitations, live sessions and accounts without a
// second factor their roles need; a signing key past the rotation advice;
// configuration problems — never an address or a secret
// (workers/module-summary.schema.json).

import { authorize } from "../_admin";
import { authSummary } from "../_admin-summary";
import { json } from "../_lib";
import type { Env } from "../_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  return json(await authSummary(caller.ctx));
};
