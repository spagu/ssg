// Cloudflare Pages Function: the module manifest (GO-156, the GO-143 contract).
//   GET /api/shop/module → module.json: what this module is, its panel,
//                          navigation, permissions and endpoints
//
// Behind the same gate as the admin API (_gate.ts: Cloudflare Access, an
// `auth` session or service token holding ecommerce:read, or the shared
// password), so it refuses exactly as GET /api/shop/admin/stats does. The
// manifest is a description, never data or configuration.

import type { Env } from "../_env";
import { adminGate } from "../_gate";
import { json } from "../_lib";
import { MODULE } from "../_module";
import { needFor } from "../_permissions";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const admin = await adminGate(request, env, needFor("GET", new URL(request.url).pathname));
  if (admin instanceof Response) return admin;
  return json(MODULE);
};
