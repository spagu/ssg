// The registry of known permissions (GO-133): its own manifest first, then
// AUTH_KNOWN_PERMISSIONS in the manifest's shape; malformed entries skipped
// and reported; built once per configuration.
import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { buildRegistry, isKnown, registryOf, unknownOf } from "../functions/api/auth/_registry.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const warnings = (fn: () => void): string[] => {
  const out: string[] = [];
  const real = console.warn;
  console.warn = (...a: unknown[]) => out.push(a.map(String).join(" "));
  try {
    fn();
  } finally {
    console.warn = real;
  }
  return out;
};

test("buildRegistry: first declaration wins; names, bare strings and descriptions; bad entries reported", () => {
  const r = buildRegistry([
    { id: "a", permissions: [{ name: "a:read", description: { en: "Read.", pl: "Czytaj.", de: "", x: 3 } }, "a:write", { name: "a:x", description: "" }] },
    { id: "b", permissions: [{ name: "a:read", description: "other" }, { name: "b:go", description: "Go." }, { name: "b:list", description: [] }, { name: "b:none", description: { de: "" } }] },
    { id: "c", permissions: ["c:*", "C:x", "c:" + "x".repeat(63), null, { description: "no name" }] },
    { id: "Bad", permissions: [] },
    { id: "d" },
    null,
    "e",
  ]);
  assert.deepEqual([...r.known.values()], [
    { name: "a:read", module: "a", description: { en: "Read.", pl: "Czytaj." } },
    { name: "a:write", module: "a", description: null },
    { name: "a:x", module: "a", description: null },
    { name: "b:go", module: "b", description: { en: "Go." } },
    { name: "b:list", module: "b", description: null },
    { name: "b:none", module: "b", description: null },
  ]);
  assert.deepEqual(r.problems, [
    'c: "c:*" is not a permission name',
    'c: "C:x" is not a permission name',
    `c: "c:${"x".repeat(63)}" is not a permission name`,
    "c: null is not a permission name",
    "c: null is not a permission name",
    'entry 3: expected {"id": "<module>", "permissions": [...]}',
    'entry 4: expected {"id": "<module>", "permissions": [...]}',
    'entry 5: expected {"id": "<module>", "permissions": [...]}',
    'entry 6: expected {"id": "<module>", "permissions": [...]}',
  ]);
  assert.ok(isKnown(r, "*"));
  assert.ok(isKnown(r, "a:*"));
  assert.ok(!isKnown(r, "z:*"));
  assert.ok(!isKnown(r, "a:delete"));
  assert.deepEqual(unknownOf(r, ["a:read", "z:y", "*"]), ["z:y"]);
});

test("registryOf: own manifest plus the variable — a JSON string or an array — cached per value, problems logged once", () => {
  const plain = registryOf({} as Env);
  assert.deepEqual([...plain.known.keys()], ["auth:admin", "auth:invite"]);
  assert.equal(plain.known.get("auth:admin")?.description?.pl?.startsWith("Zarządzanie"), true);
  assert.equal(registryOf({} as Env), plain, "cached");
  const json = JSON.stringify([{ id: "newsletter-ui", permissions: ["newsletter:send"] }]);
  assert.ok(registryOf({ AUTH_KNOWN_PERMISSIONS: json } as Env).known.has("newsletter:send"));
  const asArray = [{ id: "comments", permissions: ["comments:read", "auth:admin"] }];
  const r = registryOf({ AUTH_KNOWN_PERMISSIONS: asArray } as never);
  assert.ok(r.known.has("comments:read"));
  assert.equal(r.known.get("auth:admin")?.module, "auth", "the worker's own declaration wins");
  let logged = warnings(() => assert.deepEqual(registryOf({ AUTH_KNOWN_PERMISSIONS: "{oops" } as Env).problems, ["AUTH_KNOWN_PERMISSIONS is not valid JSON"]));
  assert.equal(logged.length, 1);
  logged = warnings(() => assert.deepEqual(registryOf({ AUTH_KNOWN_PERMISSIONS: '{"id":"x"}' } as Env).problems, ["AUTH_KNOWN_PERMISSIONS must be a JSON array of modules"]));
  assert.match(logged[0], /auth: known permissions: AUTH_KNOWN_PERMISSIONS must be/);
  assert.equal(warnings(() => registryOf({ AUTH_KNOWN_PERMISSIONS: '{"id":"x"}' } as Env)).length, 0, "once per configuration");
  assert.equal(registryOf({ AUTH_KNOWN_PERMISSIONS: "" } as Env).problems.length, 0);
});

test("scripts/auth-permissions.mjs prints a value the registry reads back", () => {
  const root = new URL("../../", import.meta.url).pathname;
  const files = ["newsletter-ui", "comments"].map((m) => `${root}${m}/module.json`);
  const out = execFileSync(process.execPath, [new URL("../scripts/auth-permissions.mjs", import.meta.url).pathname, ...files], { encoding: "utf8" });
  const line = out.split("\n").find((l) => l.startsWith("AUTH_KNOWN_PERMISSIONS=")) as string;
  const value = line.slice("AUTH_KNOWN_PERMISSIONS=".length);
  assert.ok(value.length < 5000, `${value.length} bytes: fits Cloudflare's 5 KB per variable`);
  const r = registryOf({ AUTH_KNOWN_PERMISSIONS: value } as Env);
  assert.deepEqual(r.problems, []);
  assert.ok(r.known.has("newsletter:erase") && r.known.has("comments:moderate"));
  assert.deepEqual(Object.keys(r.known.get("newsletter:send")?.description ?? {}), ["en"], "English by default, to stay under the limit");
  const all = execFileSync(process.execPath, [new URL("../scripts/auth-permissions.mjs", import.meta.url).pathname, "--all-languages", files[1]], { encoding: "utf8" });
  assert.match(all, /"pl":/);
  assert.throws(() => execFileSync(process.execPath, [new URL("../scripts/auth-permissions.mjs", import.meta.url).pathname], { stdio: "pipe" }), /usage/);
});

test("scripts/auth-permissions.mjs carries titles and panels, so Your panels can link each module", async () => {
  const root = new URL("../../", import.meta.url).pathname;
  const script = new URL("../scripts/auth-permissions.mjs", import.meta.url).pathname;
  const run = (...a: string[]) => {
    const r = spawnSync(process.execPath, [script, ...a], { encoding: "utf8" });
    return { status: r.status, err: r.stderr, value: JSON.parse((r.stdout.split("\n").find((l) => l.startsWith("AUTH_KNOWN_PERMISSIONS=")) ?? "AUTH_KNOWN_PERMISSIONS=null").slice(23)) };
  };
  const pages = run(`${root}comments/module.json`);
  assert.equal(pages.value[0].panel, "/comments-admin.html", "a Pages template's panel is site-relative already");
  assert.deepEqual(pages.value[0].title, { en: "Comments" });
  const alone = run(`${root}newsletter-ui/module.json`);
  assert.equal(alone.value[0].panel, undefined, "a standalone Worker's panel needs its base");
  assert.match(alone.err, /--base newsletter-ui=/);
  const based = run("--base", "newsletter-ui=/newsletter-admin/", `${root}newsletter-ui/module.json`);
  assert.equal(based.value[0].panel, "/newsletter-admin/");
  for (const bad of ["newsletter-ui=https://example.com/x", "newsletter-ui=//evil.example.com", "newsletter-ui=/a/../b", "newsletter-ui", "=/x"]) {
    assert.equal(run("--base", bad, `${root}newsletter-ui/module.json`).status, 2, bad);
  }
  const { panelsFor } = await import("../functions/api/auth/_panels.ts");
  assert.equal(typeof panelsFor, "function");
});
