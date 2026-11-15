// POST api/settings/publish — the ticket's acceptance criteria: only
// .ssg.designer.json is ever written; every key of the class executes or
// secret in the schema is refused without designer-ui:advanced (iterated over
// the schema itself); a literal secret is refused; a mixed publication is
// refused whole, naming the keys; the file is sorted with a newline at the end.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ADVANCED, READ, SETTINGS, SITE_YAML, call, site } from "./fixtures.ts";
import { OVERLAY_LITERAL, addressOf, assertNoSecret, changedFiles, fileAt, publish, sample } from "./helpers.ts";
import { ROOT, clsOf } from "../src/schema.ts";
import type { SchemaNode } from "../src/schema.ts";

// every schema node (fields, list elements, map values — not alternates).
function allNodes(): SchemaNode[] {
  const out: SchemaNode[] = [];
  const walk = (n: SchemaNode) => {
    out.push(n);
    for (const f of n.fields ?? []) walk(f);
    for (const c of [n.items, n.values]) if (c) walk(c);
  };
  for (const k of ROOT.fields!) walk(k);
  return out;
}

test("a plain key with designer-ui:settings: one commit changing only .ssg.designer.json, sorted, newline at the end", async () => {
  const gh = await site();
  const yaml = fileAt(gh, ".ssg.yaml");
  const r = await publish(gh, SETTINGS, [{ path: ["title"], value: "New" }, { path: ["mddb", "lang"], value: "pl" }, { path: ["colors", "primary"], value: "#00695c" }]);
  assert.equal(r.status, 200, r.text);
  assert.deepEqual(r.body.changed, ["colors.primary", "mddb.lang", "title"]);
  assert.deepEqual(changedFiles(gh), [".ssg.designer.json"]);
  assert.equal(fileAt(gh, ".ssg.yaml"), yaml);
  assert.equal(fileAt(gh, ".ssg.designer.json"), '{\n  "colors": {\n    "primary": "#00695c"\n  },\n  "mddb": {\n    "lang": "pl"\n  },\n  "title": "New"\n}\n');
  assert.equal(r.body.blobId, gh.store.blobIdAt("main", ".ssg.designer.json"));
  const commit = gh.store.commits.get(r.body.commitId)!;
  assert.deepEqual([commit.message, commit.author.name, commit.author.email], ["Change settings", "ada@example.com", "ada@example.com"]);
});

test("every key of the class executes or secret in the schema: refused without designer-ui:advanced, accepted with it", async () => {
  const gh = await site();
  let checked = 0;
  for (const n of allNodes()) {
    if (clsOf(n) === "plain" || n.type === "object") continue; // an object's fields are checked one by one
    const { keys, value } = addressOf(n.path, sample(n));
    const without = await publish(gh, SETTINGS, [{ path: keys, value }]);
    assert.equal(without.status, 403, `${n.path}: ${without.text}`);
    assert.equal(without.body.permission, "designer-ui:advanced");
    const at = keys.join(".");
    // The key named is the one changed, a key inside it, or the map entry it creates.
    assert.ok(without.body.keys.some((k: string) => k === at || k.startsWith(at + ".") || at.startsWith(k + ".")), `${n.path}: ${without.body.keys}`);
    const before = gh.store.refs.get("main");
    const withIt = await publish(gh, ADVANCED, [{ path: keys, value }]);
    assert.equal(withIt.status, 200, `${n.path}: ${withIt.text}`);
    assert.notEqual(gh.store.refs.get("main"), before);
    assert.deepEqual(changedFiles(gh), [".ssg.designer.json"], n.path);
    await publish(gh, ADVANCED, [{ path: [keys[0]], value: null }]); // back to the file's value
    checked++;
  }
  assert.ok(checked > 150, `only ${checked} keys checked`);
});

test("an object written where the file holds another form replaces it whole: its own class counts", async () => {
  const gh = await site({ ".ssg.yaml": SITE_YAML.replace("mddb:\n  enabled: false\n  api_key: $MDDB_KEY\n", "mddb: true\n") });
  const r = await publish(gh, SETTINGS, [{ path: ["mddb", "lang"], value: "en" }]);
  assert.deepEqual([r.status, r.body.keys], [403, ["mddb"]]);
  const unread = await site({ ".ssg.yaml": SITE_YAML.replace("mddb:\n  enabled: false\n  api_key: $MDDB_KEY\n", "mddb: &m {}\n") });
  const u = await publish(unread, SETTINGS, [{ path: ["mddb", "lang"], value: "en" }]);
  assert.deepEqual([u.status, u.body.keys], [403, ["mddb"]], "a value the reader could not follow is replaced whole too");
  const merged = await site();
  assert.equal((await publish(merged, SETTINGS, [{ path: ["mddb", "lang"], value: "en" }])).status, 200, "the file's mapping is merged into: mddb.lang alone is plain");
});

test("a mixed publication without designer-ui:advanced is refused whole, naming every such key; nothing is committed", async () => {
  const gh = await site();
  const head = gh.store.refs.get("main");
  const r = await publish(gh, SETTINGS, [
    { path: ["title"], value: "Fine" },
    { path: ["hooks", "post_build"], value: ["make deploy"] },
    { path: ["description"], value: "Reads $HOME at build time" },
    { path: ["ai", "models", "gpt-4.1"], value: { url: "https://example.com/v1", key: "$AI_KEY" } },
  ]);
  assert.equal(r.status, 403);
  const keys = ["ai.models.gpt-4.1", "description", "hooks.post_build"];
  assert.deepEqual(r.body, { error: "forbidden", permission: "designer-ui:advanced", keys, refused: keys.map((key) => ({ key, reason: "needs designer-ui:advanced" })) });
  assert.equal(gh.store.refs.get("main"), head);
});

test("secrets: only a reference, in the strict form; a literal is refused even with designer-ui:advanced", async () => {
  const gh = await site();
  for (const [path, value] of [[["jwt_secret"], "plain-literal"], [["server_users"], ["admin:pw"]], [["notifications"], [{ url: "https://example.com", headers: { Authorization: "Basic dXNlcjpwYXNz $X" } }]]] as const) {
    const r = await publish(gh, ADVANCED, [{ path, value }]);
    assert.equal(r.status, 422, `${path}: ${r.text}`);
    assert.equal(r.body.error, "invalid_settings");
    assert.ok(!r.text.includes("plain-literal") && !r.text.includes("dXNlcjpwYXNz"), "a refused value is never echoed");
  }
  const ok = await publish(gh, ADVANCED, [{ path: ["jwt_secret"], value: "${JWT_SECRET}" }, { path: ["server_users"], value: ["admin:$PW"] }]);
  assert.equal(ok.status, 200, ok.text);
  const read = await call(gh, "/api/settings", READ);
  const jwt = read.body.entries.find((e: { path: string }) => e.path === "jwt_secret");
  assert.deepEqual([jwt.source, jwt.value], ["overlay", { set: true, reference: "${JWT_SECRET}" }]);
});

test("a hand-written literal already in the overlay stops publishing until it is removed — and is never echoed", async () => {
  const gh = await site({ ".ssg.designer.json": JSON.stringify({ jwt_secret: OVERLAY_LITERAL }) });
  const r = await publish(gh, SETTINGS, [{ path: ["title"], value: "X" }]);
  assert.equal(r.status, 422);
  assert.deepEqual(r.body.problems, ["jwt_secret: a secret must be written as an environment reference (\"$NAME\" or \"${NAME}\"), never as a literal value"]);
  const fixed = await publish(gh, ADVANCED, [{ path: ["jwt_secret"], value: null }, { path: ["title"], value: "X" }]);
  assert.equal(fixed.status, 200, fixed.text);
  assert.equal(fileAt(gh, ".ssg.designer.json"), '{\n  "title": "X"\n}\n');
  assertNoSecret();
});

test("schema validation: unknown key, wrong type, enum, null in a list — 422 listing every problem by path", async () => {
  const gh = await site();
  const r = await publish(gh, ADVANCED, [
    { path: ["titel"], value: "typo" },
    { path: ["port"], value: "eighty" },
    { path: ["mddb", "protocol"], value: "ftp" },
    { path: ["languages"], value: ["en", null] },
    { path: ["languages", "0"], value: "x" },
  ]);
  assert.equal(r.status, 422);
  assert.deepEqual(r.body.problems, [
    "languages: expected a list (or list), got an object",
    "mddb.protocol: not one of \"\", \"http\", \"grpc\"",
    "port: expected a number, got text",
    "titel: unknown configuration key",
  ]);
});

test("null removes a key from the overlay (the file's value applies again) and prunes emptied objects", async () => {
  const gh = await site();
  assert.equal((await publish(gh, SETTINGS, [{ path: ["mddb", "lang"], value: "pl" }, { path: ["title"], value: "T" }])).status, 200);
  const r = await publish(gh, SETTINGS, [{ path: ["mddb", "lang"], value: null }, { path: ["nothing", "here"], value: null }]);
  assert.equal(r.status, 200, r.text);
  assert.deepEqual(r.body.changed, ["mddb.lang"]);
  assert.equal(fileAt(gh, ".ssg.designer.json"), '{\n  "title": "T"\n}\n');
  const same = await publish(gh, SETTINGS, [{ path: ["title"], value: "T" }]);
  assert.deepEqual([same.status, same.body.error], [400, "no_change"]);
});

test("conflict: a stale baseBlobId is a 409 naming the file and its current version", async () => {
  const gh = await site();
  assert.equal((await publish(gh, SETTINGS, [{ path: ["title"], value: "A" }], null)).status, 200);
  const stale = await publish(gh, SETTINGS, [{ path: ["title"], value: "B" }], null);
  assert.equal(stale.status, 409);
  assert.equal(stale.body.error, "conflict");
  assert.deepEqual(stale.body.conflicts[0].path, ".ssg.designer.json");
  assert.equal(stale.body.conflicts[0].currentBlobId, gh.store.blobIdAt("main", ".ssg.designer.json"));
  // A race lost inside the commit: the provider's own conflict.
  gh.beforePatch = async () => void (await gh.store.commitFiles("main", { ".ssg.designer.json": '{"title": "C"}\n' }));
  const raced = await publish(gh, SETTINGS, [{ path: ["title"], value: "D" }]);
  assert.equal(raced.status, 409, raced.text);
});

test("refusals of the site: a config that is not YAML, an unreadable overlay", async () => {
  const toml = await site({ ".ssg.yaml": null, ".ssg.toml": "title = 'T'\n" });
  const t = await publish(toml, SETTINGS, [{ path: ["title"], value: "X" }]);
  assert.deepEqual([t.status, t.body.error], [409, "overlay_needs_yaml"]);
  const none = await site({ ".ssg.yaml": null });
  assert.match((await publish(none, SETTINGS, [{ path: ["title"], value: "X" }])).body.detail, /no configuration file/);
  const broken = await site({ ".ssg.designer.json": "{not json" });
  const b = await publish(broken, SETTINGS, [{ path: ["title"], value: "X" }]);
  assert.deepEqual([b.status, b.body.error], [409, "overlay_unreadable"]);
});

test("bad input: changes, paths, baseBlobId, body, message; permission before anything", async () => {
  const gh = await site();
  for (const changes of [undefined, [], [{ path: ["title"] }], [{ path: [], value: 1 }], [{ path: ["a", ""], value: 1 }], [{ path: ["__proto__"], value: 1 }], [7], Array(201).fill({ path: ["title"], value: "x" })]) {
    const r = await publish(gh, SETTINGS, changes, null);
    assert.deepEqual([r.status, r.body.error], [400, "invalid_change"], JSON.stringify(changes)?.slice(0, 60));
  }
  const base = await publish(gh, SETTINGS, [{ path: ["title"], value: "x" }], 5 as never);
  assert.match(base.body.detail, /baseBlobId/);
  const noMessage = await publish(gh, SETTINGS, [{ path: ["title"], value: "x" }], undefined, "  ");
  assert.deepEqual([noMessage.status, noMessage.body.error], [400, "invalid_message"]);
  for (const body of ["[]", "{not json"]) {
    const r = await call(gh, "/api/settings/publish", SETTINGS, { method: "POST", body, headers: { origin: "https://example.com" } });
    assert.deepEqual([r.status, r.body.error], [400, "invalid_json"]);
  }
  const reader = await publish(gh, READ, [{ path: ["title"], value: "x" }]);
  assert.deepEqual([reader.status, reader.body], [403, { error: "forbidden", permission: "designer-ui:settings" }]);
  assert.equal((await publish(gh, ["designer-ui:read", "designer-ui:advanced"], [{ path: ["hooks"], value: {} }])).status, 403, "advanced alone is not settings");
});

test("a service account publishes under its own name", async () => {
  const gh = await site();
  const r = await call(gh, "/api/settings/publish", SETTINGS, { kind: "service", json: { message: "nightly", baseBlobId: null, changes: [{ path: ["title"], value: "S" }] } });
  assert.equal(r.status, 200, r.text);
  assert.deepEqual(gh.store.commits.get(r.body.commitId)!.author.email, "u-ada@service-account.invalid");
});

test("a map entry: a plain field of an existing entry needs designer-ui:settings; a new entry is a whole thing", async () => {
  const yaml = SITE_YAML + "external_sources:\n  sources:\n    blog:\n      url: https://example.com/feed.json\n";
  const gh = await site({ ".ssg.yaml": yaml });
  const edit = await publish(gh, SETTINGS, [{ path: ["external_sources", "sources", "blog", "timeout"], value: "5s" }]);
  assert.equal(edit.status, 200, edit.text);
  const add = await publish(gh, SETTINGS, [{ path: ["external_sources", "sources", "shop", "timeout"], value: "5s" }]);
  assert.deepEqual([add.status, add.body.keys], [403, ["external_sources.sources.shop"]]);
  const back = await publish(gh, SETTINGS, [{ path: ["external_sources", "sources", "blog"], value: null }]);
  assert.equal(back.status, 200, "the file's entry stays: only the panel's plain field goes");
  assert.equal((await publish(gh, ADVANCED, [{ path: ["ai", "models", "m"], value: { url: "https://example.com/v1" } }])).status, 200);
  const drop = await publish(gh, SETTINGS, [{ path: ["ai", "models", "m"], value: null }]);
  assert.deepEqual([drop.status, drop.body.keys], [403, ["ai.models.m"]], "removing an entry only the panel made counts its class too");
});
