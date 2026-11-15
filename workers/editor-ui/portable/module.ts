// GET {base}/api/module — the module manifest: identity, panel, navigation,
// permissions, endpoints, from module.json at the template root. wrangler
// bundles the JSON at deploy time; Node's test runner reads the same file, so
// there is no copy to keep in step. Behind the same gate and permission as
// every /api/ route (docs/MODULE-REQUIREMENTS.md, level 2).

import manifest from "../module.json" with { type: "json" };
import type { Op } from "./ops";
import { PERM } from "../src/permissions";

export const moduleOp: Op<void, typeof manifest> = {
  name: "module_manifest",
  permission: PERM.read,
  run: () => manifest,
};
