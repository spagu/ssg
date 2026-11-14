// Cloudflare Pages Function: the module manifest (GO-143).
//   GET /api/comments/module → module.json: what this module is, its panel,
//                              navigation, permissions and endpoints
//
// Behind the same gate as moderation (requireAdmin: Cloudflare Access, an
// `auth` session with comments:read, or HTTP Basic), so it refuses exactly as
// GET /api/comments/admin does. It needs no database: the manifest is a
// description, never data or configuration.

import { json, requireAdmin } from "../_lib";
import type { Env } from "../_lib";
import { MODULE } from "../_module";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const admin = await requireAdmin(request, env, "manifest");
  if (admin instanceof Response) return admin;
  return json(MODULE);
};
