// Cloudflare Pages Function: this module's manifest (GO-143).
//   GET /api/auth/module   → the manifest as JSON
//
// Behind `auth:admin`, like the other modules' manifests behind their admin
// gate: 401 without a session, 403 {error: "forbidden", permission} without
// the permission. The manifest comes from ../_module.ts, the generated copy of
// module.json (a Pages Function cannot read files at runtime).

import { authorize } from "../_admin";
import { json } from "../_lib";
import type { Env } from "../_lib";
import { MODULE } from "../_module";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  return json(MODULE);
};
