// Cloudflare Pages Function: the module settings as a file (GO-149).
//   GET /api/core/manage/export → core-settings.json (an attachment):
//     {kind: "ssg-core-settings", format: 1, exportedAt,
//      modules: [{id, hidden, position, labels, note}]}
//
// Every module of the site, in bar order; POST /api/core/manage/import on
// another environment of the same site reads it back. Who changed what is
// not exported: the change log stays where the changes happened.

import { json, problem } from "../_lib";
import type { Env } from "../_lib";
import { knownIds, NO_DATABASE, reading } from "../_manage";
import type { Registry } from "../_registry";
import { route } from "../_route";
import { readSettings } from "../_settings";
import type { Caller } from "../_signin";
import { exportDoc } from "../_transfer";

async function download(_request: Request, env: Env, _caller: Caller, registry: Registry): Promise<Response> {
  if (!env.CORE_DB) return problem("no_database", 503, NO_DATABASE);
  const doc = exportDoc(knownIds(registry), await readSettings(env.CORE_DB), Date.now());
  return json(doc, 200, { "content-disposition": 'attachment; filename="core-settings.json"' });
}

export const onRequest = route({ GET: reading(download) });
