// The manifest reader (module-manifest.ts) on its own: the schema subset it
// interprets, the contract rule, and the checks a schema cannot express.
// Run from the repository root: node --test 'workers/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { MANIFEST_CONTRACT, readManifest, validateSchema } from "./module-manifest.ts";

const schema = JSON.parse(readFileSync(new URL("./module.schema.json", import.meta.url), "utf8"));
const text = (s: string) => ({ en: s, pl: s, hi: s, de: s, fr: s });
const valid = () => ({
  id: "example",
  contract: 1,
  version: "1.0.0",
  kind: "pages-function",
  title: text("Example"),
  description: text("An example module."),
  panel: "/example-admin.html",
  nav: [{ label: text("Queue"), path: "/example-admin.html", permission: "example:read" }],
  permissions: [{ name: "example:read", description: text("Read.") }],
  endpoints: { manifest: "/api/example/module" },
  disable: text("Remove it."),
});
const errorsOf = (m: unknown) => {
  const r = readManifest(m, schema);
  return r.ok ? [] : r.errors;
};

test("reader: a valid manifest is returned as is; unknown fields are ignored", () => {
  const m = { ...valid(), later: { anything: true }, endpoints: { manifest: "/api/x/module", future: "/api/x/future" }, title: { ...text("T"), es: "T" } };
  const r = readManifest(m, schema);
  assert.ok(r.ok);
  assert.equal(r.manifest, m);
  assert.equal(MANIFEST_CONTRACT, 1);
});

test("reader: a newer contract is refused first, with what to do", () => {
  assert.deepEqual(errorsOf({ contract: 2, id: "x" }), [
    'module "x" uses manifest contract 2, but this reader understands contract 1 at most — upgrade ssg (and the core worker) to read it',
  ]);
  assert.match(errorsOf({ contract: 9 })[0], /^this module uses manifest contract 9/);
  assert.match(errorsOf([])[0], /manifest: must be object, not array/);
});

test("reader: schema violations name the field", () => {
  const cases: [Record<string, unknown>, RegExp][] = [
    [{ id: "Bad_Id" }, /manifest\.id: "Bad_Id" does not match/],
    [{ contract: 0 }, /manifest\.contract: must be at least 1/],
    [{ contract: 1.5 }, /manifest\.contract: must be integer, not number/],
    [{ kind: "lambda" }, /manifest\.kind: must be one of "pages-function", "worker"/],
    [{ title: { en: "x" } }, /manifest\.title\.pl: is required/],
    [{ title: { ...text("x"), de: "" } }, /manifest\.title\.de: must not be empty/],
    [{ description: { ...text("x"), fr: "x".repeat(601) } }, /manifest\.description\.fr: longer than 600 characters/],
    [{ panel: "//evil.example.com" }, /manifest\.panel: .* does not match/],
    [{ panel: "/../up" }, /manifest\.panel: .* does not match/],
    [{ panel: 5 }, /manifest\.panel: must be string or null, not integer/],
    [{ nav: new Array(13).fill(valid().nav[0]) }, /manifest\.nav: at most 12 entries/],
    [{ permissions: [{ name: "Example:*", description: text("x") }] }, /manifest\.permissions\[0\]\.name: .* does not match/],
    [{ endpoints: { health: "/api/x#frag" } }, /manifest\.endpoints\.health: .* does not match/],
    [{ endpoints: { other: "relative" } }, /manifest\.endpoints\.other: .* does not match/],
  ];
  for (const [patch, want] of cases) assert.match(errorsOf({ ...valid(), ...patch }).join("\n"), want, JSON.stringify(patch));
  const missing = valid() as Record<string, unknown>;
  delete missing.disable;
  assert.deepEqual(errorsOf(missing), ["manifest.disable: is required"]);
});

test("reader: paths — a base, files, directories and hash views pass", () => {
  for (const panel of ["/", "/a", "/a/", "/a/b.html", "/#/", "/#/campaigns?tab=x", "/admin/#/x"]) {
    assert.deepEqual(errorsOf({ ...valid(), panel }), [], panel);
  }
  for (const panel of ["", "a", "/a//b", "/.hidden", "https://example.com/", "/a b"]) {
    assert.notDeepEqual(errorsOf({ ...valid(), panel }), [], panel);
  }
});

test("reader: cross-field rules", () => {
  const p = valid().permissions[0];
  const n = valid().nav[0];
  assert.deepEqual(errorsOf({ ...valid(), permissions: [p, p] }), ['manifest.permissions: "example:read" is declared twice']);
  assert.deepEqual(errorsOf({ ...valid(), nav: [n, n] }), ['manifest.nav: path "/example-admin.html" appears twice']);
  assert.deepEqual(errorsOf({ ...valid(), nav: [{ ...n, permission: "example:write" }] }), ['manifest.nav[0].permission: "example:write" is not declared in permissions']);
  assert.deepEqual(errorsOf({ ...valid(), panel: null }), ["manifest.nav: a module without a panel has no navigation entries"]);
  assert.deepEqual(errorsOf({ ...valid(), panel: null, nav: [] }), []);
});

test("validateSchema: the keyword subset, and a loud failure outside it", () => {
  assert.deepEqual(validateSchema({ a: 1, b: 2 }, { type: "object", properties: { a: {} }, additionalProperties: false }), ["manifest.b: is not allowed"]);
  assert.deepEqual(validateSchema({ a: 1 }, { additionalProperties: true }), []);
  assert.deepEqual(validateSchema(3, { type: "number", minimum: 1 }), []);
  assert.deepEqual(validateSchema("x", { required: ["a"], properties: {}, additionalProperties: {}, items: {}, maxItems: 0 }), []);
  assert.deepEqual(validateSchema([1], { minLength: 2, maxLength: 0, pattern: "^z$", minimum: 5 }), []);
  assert.throws(() => validateSchema({}, { oneOf: [] }), /unsupported keyword "oneOf" at manifest/);
  assert.throws(() => validateSchema({}, { $ref: "other.json#/x" }), /only local #\/\$defs references/);
  assert.throws(() => validateSchema({}, { $ref: "#/$defs/none" }), /#\/\$defs\/none is not defined/);
  assert.throws(() => validateSchema({}, { $ref: "#/$defs/none" }, {}), /is not defined/);
});

test("module.schema.json: every pattern also compiles as RE2 would read it (no lookaround, no backreference)", () => {
  const patterns = JSON.stringify(schema).match(/"pattern":"((?:[^"\\]|\\.)*)"/g) ?? [];
  assert.ok(patterns.length >= 5);
  for (const p of patterns) assert.doesNotMatch(p, /\(\?[=!<]|\\[1-9]/, p);
});
