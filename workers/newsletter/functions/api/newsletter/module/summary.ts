// Cloudflare Pages Function: the module summary (GO-148).
//   GET /api/newsletter/module/summary → {contract, generatedAt, metrics,
//       attention, problems}: subscribers by status and the configuration
//       problems that refuse sign-ups (workers/module-summary.schema.json)
//
// Behind the same gate as the admin endpoint (requireAdmin: Cloudflare Access,
// an `auth` session holding newsletter:read, or HTTP Basic), so it refuses
// exactly as /api/newsletter/admin does.
// Aggregates only: one GROUP BY, never an address.

import { READ, json, requireAdmin } from "../_lib";
import type { Env } from "../_lib";
import { newsletterSummary } from "../_summary";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const denied = await requireAdmin(request, env, READ);
  if (denied) return denied;
  return json(await newsletterSummary(env));
};
