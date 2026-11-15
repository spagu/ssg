// The lower layers on their own: validation against the schema (every kind of
// problem), redaction of secrets in every shape, the strict reference forms,
// the change list and its classes, reading the files of a site that are not
// UTF-8, and the named keys of a schema.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ADVANCED, SETTINGS, THEME, call, site } from "./fixtures.ts";
import { fileAt, publish } from "./helpers.ts";
import { ENV_REF, secretRefOK, validateOverlay } from "../src/overlay-validate.ts";
import { redact, secretState, strictReference } from "../src/redact.ts";
import { ROOT, child, classesIn, namedNodes, strongest } from "../src/schema.ts";
import type { SchemaNode } from "../src/schema.ts";
import { changedKeys, hasReference, needsAdvanced, overlayLeaves, secretShapeProblems } from "../src/settings-diff.ts";
import { applyChanges, canonicalJSON, serializeOverlay } from "../src/overlay-write.ts";
import { lookup, merge, presentKeys, readYaml, utf8 } from "../src/config-read.ts";

const node = (path: string): SchemaNode => {
  let n: SchemaNode | null = ROOT;
  for (const k of path.split(".")) n = n && child(n, k);
  return n!;
};

test("validateOverlay: every kind of value in the wrong place, named by path, never by value", () => {
  assert.deepEqual(validateOverlay([]), ["the top level must be an object"]);
  assert.deepEqual(validateOverlay({ minify_html: 1, title: ["x"], port: 1.5, timezone: null, languages: ["en", null], colors: { primary: 3, x: null }, mddb: "on", story: { anything: [1] } }), [
    "colors.primary: expected text, got a number",
    "languages[1]: null is not a list element",
    "mddb: expected an object, got text",
    "minify_html: expected true or false, got a number",
    "port: expected a whole number, got a number",
    "title: expected text, got a list",
  ]);
  assert.deepEqual(validateOverlay({ port: true, colors: [], hooks: { pre_build: "x" }, languages: "en" }), [
    "colors: expected an object, got a list",
    "hooks.pre_build: expected a list, got text",
    "languages: expected a list (or list), got text",
    "port: expected a number, got true/false",
  ]);
  assert.deepEqual(validateOverlay({ pretty_urls: "maybe", story: false, cook_mode: { enabled: true } }), ['pretty_urls: expected one of "", "auto", "on", "off" (or bool), got text'].filter(() => false).concat(validateOverlay({ pretty_urls: "maybe" })));
  assert.deepEqual(validateOverlay({ title: { a: 1 } }), ["title: expected text, got an object"]);
});

test("secret references: the formats ssg accepts and the strict forms this worker writes", () => {
  assert.equal(secretRefOK("whole", "$A_1"), true);
  assert.equal(secretRefOK("whole", "${A}"), true);
  assert.equal(secretRefOK("whole", "x$A"), false);
  assert.equal(secretRefOK("login", "admin:$PW"), true);
  assert.equal(secretRefOK("login", "$PW"), false);
  assert.equal(secretRefOK("inline", "Basic abc $X"), true, "ssg accepts any text holding one");
  assert.equal(strictReference("inline", "Basic abc $X"), false, "this worker does not");
  assert.equal(strictReference("inline", "$X"), true);
  assert.equal(strictReference("login", "a b:$X"), false);
  assert.ok(ENV_REF.test("Bearer ${TOKEN}"));
  assert.deepEqual(secretState("whole", ""), { set: false, reference: null });
  assert.deepEqual(secretState("whole", null), { set: false, reference: null });
  assert.deepEqual(secretState("whole", 5), { set: true, reference: null });
});

test("redact: every value inside a secret key, in lists, maps and objects; unknown keys dropped", () => {
  assert.deepEqual(redact({ jwt_secret: "lit", titel: "x", title: "T", ai: { models: { m: { url: "u", key: "k" } } } }, ROOT), {
    jwt_secret: { set: true, reference: null },
    title: "T",
    ai: { models: { m: { url: "u", key: { set: true, reference: null } } } },
  });
  assert.deepEqual(redact(["admin:$P", "lit"], node("server_users")), [{ set: true, reference: "admin:$P" }, { set: true, reference: null }]);
  assert.deepEqual(redact({ a: { nested: "x" } }, node("jwt_secret")), {}, "an object where a secret belongs shows nothing");
  assert.deepEqual(redact([["$X"]], node("jwt_secret")), [[{ set: true, reference: "$X" }]], "a list inside a non-list secret: still redacted");
  assert.deepEqual(redact({ x: { y: [1] } }, node("variables")), { x: { y: [1] } }, "free-form values pass");
  const languages = node("languages");
  assert.deepEqual(redact([{ code: "en", nope: 1 }], languages), [{ code: "en" }], "the alternate's element shape");
  assert.deepEqual(redact({ enabled: true }, node("cook_mode")), { enabled: true });
  const alt: SchemaNode = { path: "x", type: "bool", alternate: { path: "x", type: "object", fields: [{ path: "x.a", key: "a", type: "string" }] } };
  assert.deepEqual(redact({ a: "1", b: "2" }, alt), { a: "1" }, "an object in the alternate form");
});

test("schema helpers: named keys of an object without fields, classes, the strongest", () => {
  const keys: SchemaNode[] = [{ path: "o", key: "o", type: "object" }];
  assert.deepEqual(namedNodes(keys).map((n) => n.segments), [["o"]]);
  assert.equal(child(node("title"), "x"), null);
  assert.equal(child(node("story"), "anything")!.type, "any", "an open object's unknown key");
  assert.equal(child(node("story"), "anything"), child(node("story"), "anything"), "the same node each time");
  assert.deepEqual([...classesIn([{ auth: { token: "$T" } }], node("workers"))].sort(), ["executes", "secret"]);
  assert.equal(strongest(["plain", "secret", "executes"]), "secret");
  assert.equal(strongest([]), "plain");
});

test("settings-diff helpers: leaves, references, unknown old keys, secret shapes anywhere", () => {
  assert.deepEqual(overlayLeaves({ b: { c: 1, d: {} }, a: [] }), ["a", "b.c", "b.d"]);
  assert.equal(hasReference({ a: ["x", { b: "$Y" }] }), true);
  assert.equal(hasReference(7), false);
  const file = { values: {}, unread: {} };
  const unknown = changedKeys({ titel: "x" }, {}, file);
  assert.deepEqual(unknown, [{ path: "titel", classes: ["executes"], reference: false }]);
  assert.deepEqual(needsAdvanced(unknown), ["titel"]);
  assert.deepEqual(changedKeys({ colors: { a: "1" } }, { colors: { a: "1", b: "2" } }, { values: { colors: { a: "0" } }, unread: {} }).map((c) => c.path), ["colors.b"]);
  assert.deepEqual(secretShapeProblems({ variables: { x: [["$A"]] }, server_users: ["a:$X", "bad"] }, ROOT, ""), ['server_users[1]: a secret is written only as an environment reference, e.g. "$NAME"']);
  assert.deepEqual(secretShapeProblems({ titel: "lit" }, ROOT, ""), [], "unknown keys are validation's to refuse");
});

test("overlay-write and config-read helpers", () => {
  assert.equal(canonicalJSON(undefined), "undefined");
  assert.equal(canonicalJSON({ b: [{ d: 1, c: 2 }], a: 1 }), '{"a":1,"b":[{"c":2,"d":1}]}');
  assert.equal(serializeOverlay({}), "{}\n");
  const before = { story: true, a: { b: { c: 1 } } };
  assert.deepEqual(applyChanges(before, [{ path: ["story", "enabled"], value: false }, { path: ["a", "b", "c"], value: null }, { path: ["story", "x", "y"], value: null }]), { story: { enabled: false } });
  assert.deepEqual(before, { story: true, a: { b: { c: 1 } } }, "the input is not modified");
  assert.deepEqual(merge({ a: { b: 1, c: 2 }, d: 1 }, { a: { b: null, e: 3 }, d: null, __proto__: { x: 1 } } as never), { a: { c: 2, e: 3 } });
  assert.deepEqual(lookup({ a: { b: 1 } }, ["a", "toString"]), { has: false, value: undefined });
  assert.deepEqual(presentKeys("﻿a: 1\n  b: 2\nc:\n# d: 1\ne :x\n"), ["a", "c"]);
  assert.deepEqual(readYaml("a: [1\nb: 2\n"), { values: { b: 2 }, unread: { a: "line 1 uses a flow collection that continues on the next line" } });
  assert.equal(utf8(new Uint8Array([0xff])), undefined);
});

test("files that are not UTF-8: the config file, the overlay, an old overlay", async () => {
  const gh = await site({ ".ssg.yaml": "x", ".ssg.designer.json": "x" });
  gh.store.blobs.set(gh.store.blobIdAt("main", ".ssg.yaml")!, new Uint8Array([0xff]));
  const r = await call(gh, "/api/settings", ADVANCED);
  assert.deepEqual(r.body.problems.map((p: { detail: string }) => p.detail), [".ssg.yaml is not UTF-8 text", ".ssg.designer.json is not a JSON object"]);
  const at = gh.store.refs.get("main")!;
  const old = await call(gh, `/api/revisions/${at}?path=.ssg.designer.json`, ADVANCED);
  assert.deepEqual(old.body.problems, ["the top level must be an object"]);
});

test("removing a key ssg does not know needs designer-ui:advanced; a move with new content", async () => {
  const gh = await site({ ".ssg.designer.json": '{"titel": "typo"}' });
  const r = await publish(gh, SETTINGS, [{ path: ["titel"], value: null }]);
  assert.deepEqual([r.status, r.body.keys], [403, ["titel"]]);
  assert.equal((await publish(gh, ADVANCED, [{ path: ["titel"], value: null }])).status, 200);
  const blob = gh.store.blobIdAt("main", "templates/mytheme/index.html");
  const move = await call(gh, "/api/theme/publish", THEME, { json: { message: "rename", changes: [{ op: "move", from: "templates/mytheme/index.html", to: "templates/mytheme/home.html", baseBlobId: blob, content: { text: "new\n" } }] } });
  assert.equal(move.status, 200, move.text);
  assert.equal(fileAt(gh, "templates/mytheme/home.html"), "new\n");
});
