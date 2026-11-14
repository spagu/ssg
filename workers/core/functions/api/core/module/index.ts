// Cloudflare Pages Function: core's own module manifest (GO-143).
//   GET /api/core/module → module.json: what core is, its permissions and
//                          endpoints
//
// core is a module like any other, behind the same sign-in as the rest of its
// API. It needs no registry and no database: the manifest is a description.

import { json } from "../_lib";
import { MODULE } from "../_module";
import { route } from "../_route";

export const onRequest = route({ GET: async () => json(MODULE) });
