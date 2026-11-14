// Cloudflare Pages Function: the module manifest (GO-143).
//   GET /api/newsletter/module → module.json: what this module is, its
//                                permissions and endpoints
//
// Behind the same gate as the admin endpoint (requireAdmin: Cloudflare Access,
// an `auth` session holding newsletter:read, or HTTP Basic), so it refuses
// exactly as /api/newsletter/admin does. It needs no database: the manifest is a description, never data or
// configuration.

import { READ, json, requireAdmin } from "../_lib";
import type { Env } from "../_lib";
import { MODULE } from "../_module";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const denied = await requireAdmin(request, env, READ);
  if (denied) return denied;
  return json(MODULE);
};
