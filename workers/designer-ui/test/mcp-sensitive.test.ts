// GO-181 end to end, without a database: this worker's module.json declares
// designer-ui:theme and designer-ui:advanced sensitive; `auth` reads the
// declaration from its registry (AUTH_KNOWN_PERMISSIONS, built here from
// module.json as scripts/auth-permissions.mjs prints it) and leaves both out
// of a service token unless an owner allowed them (allowSensitive). A token
// with what auth then gives cannot write a theme file nor an advanced key —
// by MCP or REST — and the allowed one can. The exchange itself, with D1, is
// workers/auth/test/sensitive-declared.test.ts. Skipped when this template
// was scaffolded without auth beside it.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import manifest from "../module.json" with { type: "json" };
import { site } from "./fixtures.ts";
import { ALL, rest, tool } from "./mcp-fixtures.ts";

const AUTH = new URL("../../auth/functions/api/auth/", import.meta.url);
const alone = !existsSync(AUTH) && "workers/auth is not next to this template (scaffolded alone)";
const CSS = "templates/mytheme/css/style.css";

test("module.json declares exactly theme and advanced sensitive", () => {
  assert.deepEqual(manifest.permissions.filter((p) => (p as { sensitive?: boolean }).sensitive === true).map((p) => p.name), ["designer-ui:advanced", "designer-ui:theme"]);
});

test("a service token without the owner's allowSensitive cannot write a theme file or an advanced key; with it, it can", { skip: alone }, async () => {
  const { buildRegistry } = await import(new URL("_registry.ts", AUTH).href);
  const { tokenPermissions } = await import(new URL("_service-perms.ts", AUTH).href);
  const declared = buildRegistry([{ id: manifest.id, permissions: manifest.permissions }]).sensitive as Set<string>;
  const perms = (allow: string[]): string[] => tokenPermissions(ALL, null, allow, ALL, declared);
  const plain = perms([]);
  assert.deepEqual(plain, ["designer-ui:read", "designer-ui:settings"]);

  const gh = await site();
  const head = gh.store.refs.get("main");
  const blob = gh.store.blobIdAt("main", CSS);
  const write = (p: string[]) => tool(gh, p, "designer_write", { message: "m", path: CSS, content: "body{}\n", baseBlobId: blob });
  const hooks = (p: string[]) => tool(gh, p, "designer_config_set", { message: "m", key: "hooks.pre_build", value: ["make"] });
  assert.deepEqual((await write(plain)).structuredContent, { error: "forbidden", permission: "designer-ui:theme", status: 403 });
  assert.equal((await hooks(plain)).structuredContent.permission, "designer-ui:advanced");
  const viaRest = await rest(gh, plain, "/api/theme/publish", { message: "m", changes: [{ op: "put", path: CSS, content: { text: "x" }, baseBlobId: blob }] });
  assert.equal(viaRest.status, 403);
  assert.equal(gh.store.refs.get("main"), head, "nothing written");

  const allowed = perms(["designer-ui:theme", "designer-ui:advanced"]);
  assert.deepEqual(allowed, [...ALL].sort());
  assert.equal((await write(allowed)).isError, false);
  assert.equal((await hooks(allowed)).isError, false);
});
