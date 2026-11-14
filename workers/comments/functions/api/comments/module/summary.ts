// Cloudflare Pages Function: the module summary (GO-148).
//   GET /api/comments/module/summary → {contract, generatedAt, metrics,
//       attention, problems}: counts by status, the moderation queue,
//       configuration problems (workers/module-summary.schema.json)
//
// Behind the same gate as moderation (requireAdmin: Cloudflare Access, an
// `auth` session with comments:read, or HTTP Basic), so it refuses exactly as
// GET /api/comments/admin does. Aggregates only: one statement, never a
// comment's author, address or text.

import { json, requireAdmin } from "../_lib";
import type { Env } from "../_lib";
import { commentsSummary } from "../_summary";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const admin = await requireAdmin(request, env, "summary");
  if (admin instanceof Response) return admin;
  return json(await commentsSummary(env));
};
