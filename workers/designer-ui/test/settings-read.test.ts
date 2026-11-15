// GET api/settings: every key of the schema, where its value comes from, the
// secrets as {set, reference}, and the configuration files the reader cannot
// follow reported — never guessed.
import { test } from "node:test";
import assert from "node:assert/strict";
import { SITE_YAML, site } from "./fixtures.ts";
import { OVERLAY_LITERAL, assertNoSecret, get } from "./helpers.ts";
import { SCHEMA, namedNodes } from "../src/schema.ts";

const entry = (body: { entries: { path: string }[] }, path: string) => body.entries.find((e) => e.path === path) as Record<string, any>;

test("completeness: one entry for every key the schema names, in the schema's order", async () => {
  const gh = await site();
  const { status, body } = await get(gh, "/api/settings");
  assert.equal(status, 200);
  const want: string[] = [];
  const walk = (nodes: { path: string; type: string; fields?: unknown[] }[]) => {
    for (const n of nodes) {
      want.push(n.path);
      if (n.type === "object") walk((n.fields ?? []) as never);
    }
  };
  walk(SCHEMA.keys as never);
  assert.deepEqual(body.entries.map((e: { path: string }) => e.path), want);
  assert.equal(new Set(want).size, want.length);
  assert.ok(want.length > SCHEMA.keys.length);
  for (const top of SCHEMA.keys) assert.ok(want.includes(top.path), top.path);
  assert.deepEqual(body.groups, SCHEMA.groups);
  assert.equal(body.schemaVersion, SCHEMA.schemaVersion);
  assert.equal(namedNodes().length, want.length);
});

test("sources and values: file, default, secrets redacted, alternate form, element shapes kept", async () => {
  const gh = await site();
  const { body } = await get(gh, "/api/settings");
  assert.deepEqual([body.configFile, body.configFormat, body.overlayBlobId, body.overlayKeys, body.problems], [".ssg.yaml", "yaml", null, [], []]);
  assert.deepEqual([entry(body, "title").source, entry(body, "title").value], ["file", "Example"]);
  assert.deepEqual(entry(body, "languages").value, ["en", "pl"]);
  assert.deepEqual(entry(body, "mddb.api_key").value, { set: true, reference: "$MDDB_KEY" });
  assert.deepEqual(entry(body, "jwt_secret").value, { set: true, reference: null });
  assert.equal(entry(body, "jwt_secret").class, "secret");
  assert.deepEqual(entry(body, "hooks").value, { pre_build: ["echo hi"] });
  assert.equal(entry(body, "hooks").class, "executes");
  assert.deepEqual([entry(body, "story").source, entry(body, "story").value], ["file", true], "an object in its alternate form shows its value");
  assert.equal(entry(body, "mddb").value, undefined, "an object's value is in its fields' entries");
  assert.equal(entry(body, "mddb").source, "file");
  const def = entry(body, "domain");
  assert.deepEqual([def.source, def.value], ["default", ""]);
  assert.deepEqual(entry(body, "tts.api_key").value, { set: false, reference: null }, "a default secret is redacted too");
  assert.equal(entry(body, "story.enabled").source, "default");
  assert.equal(entry(body, "story.enabled").value, undefined, "no default, not set");
  assert.ok(entry(body, "workers").node.items, "the element shape rides along");
  assert.equal(entry(body, "mddb").node.fields, undefined, "fields are entries of their own");
  assertNoSecret();
});

test("the overlay: source overlay, merged over the file, its keys listed, a literal in it never shown", async () => {
  const overlay = { title: "From the panel", mddb: { lang: "pl" }, jwt_secret: OVERLAY_LITERAL, server_users: ["admin:$PW", OVERLAY_LITERAL], story: { enabled: true, extra: 1 }, variables: { x: { y: 1 } } };
  const gh = await site({ ".ssg.designer.json": JSON.stringify(overlay) });
  const { body } = await get(gh, "/api/settings");
  assert.equal(typeof body.overlayBlobId, "string");
  assert.deepEqual(body.overlayKeys, ["jwt_secret", "mddb.lang", "server_users", "story.enabled", "story.extra", "title", "variables.x.y"]);
  assert.deepEqual([entry(body, "title").source, entry(body, "title").value], ["overlay", "From the panel"]);
  assert.equal(entry(body, "mddb.lang").source, "overlay");
  assert.equal(entry(body, "mddb.enabled").source, "file");
  assert.equal(entry(body, "mddb").source, "overlay");
  assert.deepEqual(entry(body, "jwt_secret").value, { set: true, reference: null });
  assert.deepEqual(entry(body, "server_users").value, [{ set: true, reference: "admin:$PW" }, { set: true, reference: null }]);
  assert.deepEqual(entry(body, "story").value, { enabled: true, extra: 1 }, "an open object shows its value whole");
  assert.deepEqual(entry(body, "variables").value, { x: { y: 1 } });
  assertNoSecret();
});

test("a key the reader cannot follow: set in the file, value not read, the others still read", async () => {
  const yaml = SITE_YAML + "feeds: &f\n  - name: x\nsitemap_exclude: *f\n";
  const gh = await site({ ".ssg.yaml": yaml });
  const { body } = await get(gh, "/api/settings");
  const feeds = entry(body, "feeds");
  assert.equal(feeds.source, "unknown");
  assert.equal(feeds.value, undefined);
  assert.match(feeds.unread, /\.ssg\.yaml: feeds is set, but its value could not be read \(line 14 uses an anchor, alias or tag\)/);
  assert.equal(entry(body, "title").value, "Example");
  assert.deepEqual(body.problems.map((p: { code: string }) => p.code), ["value_unread", "value_unread"]);
});

test("problems: include, an unknown top-level key, an unreadable overlay", async () => {
  const gh = await site({ ".ssg.yaml": "title: X\ninclude: [more.yaml]\ntitel: typo\n", ".ssg.designer.json": "[1]" });
  const { body } = await get(gh, "/api/settings");
  assert.deepEqual(body.problems.map((p: { code: string }) => p.code), ["include_not_followed", "overlay_unreadable", "unknown_key"]);
  assert.match(body.problems[2].detail, /titel is not a configuration key/);
});

test("other configuration files: JSON read, TOML reported, none at all reported", async () => {
  const json = await site({ ".ssg.yaml": null, "ssg.json": JSON.stringify({ title: "J", include: ["x.json"] }) });
  const j = (await get(json, "/api/settings")).body;
  assert.deepEqual([j.configFile, j.configFormat, entry(j, "title").value], ["ssg.json", "json", "J"]);
  assert.equal(j.problems[0].code, "include_not_followed");
  const bad = await site({ ".ssg.yaml": null, ".ssg.json": "[]" });
  assert.match((await get(bad, "/api/settings")).body.problems[0].detail, /\.ssg\.json is not a JSON object/);
  const toml = await site({ ".ssg.yaml": null, ".ssg.toml": "title = 'T'\n" });
  const t = (await get(toml, "/api/settings")).body;
  assert.deepEqual([t.configFormat, t.problems[0].code], ["toml", "config_unreadable"]);
  const none = await site({ ".ssg.yaml": null });
  const n = (await get(none, "/api/settings")).body;
  assert.equal(n.configFile, null);
  assert.match(n.problems[0].detail, /no configuration file/);
});
