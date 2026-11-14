// GET {base}/api/module — the module manifest (GO-143): what this module is,
// its panel, navigation, permissions and endpoints, from module.json at the
// template root. Behind admin auth like every /api/ route; it is a
// description, never data or configuration, so it needs no database.
//
// wrangler bundles the JSON into the Worker at deploy time (a Worker cannot
// read files at runtime); Node's test runner reads the same file, so there is
// no copy to keep in step.

import manifest from "../module.json" with { type: "json" };
import { json } from "./lib";

export function moduleManifest(): Response {
  return json(manifest);
}
