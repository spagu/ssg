// GO-183 (security review): the sensitive permissions of the worker templates
// shipped with ssg are sensitive whatever AUTH_KNOWN_PERMISSIONS says. Before
// the review, the flag came only from that variable: an operator who listed
// the permissions as bare names (the format allows it, and the 5 KB cap of a
// Cloudflare variable pushes towards it), or left the module out, made
// designer-ui:theme an ordinary permission — a service token then carried it
// without an owner's allowSensitive.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync, existsSync } from "node:fs";
import { registryOf } from "../functions/api/auth/_registry.ts";
import { BUNDLED_SENSITIVE } from "../functions/api/auth/_bundled-sensitive.ts";
import { tokenPermissions } from "../functions/api/auth/_service-perms.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const WORKERS = new URL("../../", import.meta.url);

// declaredSensitive: every `"sensitive": true` in the module.json files of the templates.
function declaredSensitive(): string[] {
  const out: string[] = [];
  for (const dir of readdirSync(WORKERS)) {
    const file = new URL(`${dir}/module.json`, WORKERS);
    if (!existsSync(file)) continue;
    const m = JSON.parse(readFileSync(file, "utf8")) as { permissions?: { name?: string; sensitive?: boolean }[] };
    for (const p of m.permissions ?? []) if (p && p.sensitive === true && typeof p.name === "string") out.push(p.name);
  }
  return out.sort();
}

test("BUNDLED_SENSITIVE is exactly what the templates' module.json files declare sensitive", () => {
  assert.deepEqual([...BUNDLED_SENSITIVE].sort(), declaredSensitive());
  for (const name of ["designer-ui:advanced", "designer-ui:theme", "editor-ui:publish"]) assert.ok(BUNDLED_SENSITIVE.includes(name), name);
});

test("a template's sensitive permission stays sensitive when AUTH_KNOWN_PERMISSIONS lists it as a bare name, or not at all", () => {
  const bare = registryOf({ AUTH_KNOWN_PERMISSIONS: JSON.stringify([{ id: "designer-ui", permissions: ["designer-ui:read", "designer-ui:theme"] }]) } as unknown as Env);
  const none = registryOf({} as Env);
  for (const reg of [bare, none]) {
    for (const p of BUNDLED_SENSITIVE) assert.ok(reg.sensitive.has(p), p);
    assert.equal(reg.sensitive.has("designer-ui:read"), false);
    const carried = tokenPermissions(["designer-ui:read", "designer-ui:theme", "editor-ui:publish"], null, [], undefined, reg.sensitive);
    assert.deepEqual(carried, ["designer-ui:read"], "without allowSensitive a service token gets neither");
  }
  assert.equal(none.known.has("designer-ui:theme"), false, "sensitive does not make a name known");
});
