// Every designer operation by both ways in (README of the round, decision 11):
// the MCP tool and the REST route give the same answer, write the same
// commit, and refuse with the same body — the tool adds only the status.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ADVANCED, READ, SETTINGS, THEME, site } from "./fixtures.ts";
import { ALL, published, refusalOf, rest, tool, tree } from "./mcp-fixtures.ts";

const CSS = "templates/mytheme/css/style.css";
const file = (p: string) => "/api/theme/files/" + encodeURIComponent(p);

test("reads: designer_list, designer_read, designer_config_read and the revisions equal their routes", async () => {
  const gh = await site({ ".ssg.designer.json": '{"title": "From the panel"}\n' });
  const same = async (name: string, args: Record<string, unknown>, path: string) => {
    const out = await tool(gh, READ, name, args);
    const r = await rest(gh, READ, path);
    assert.equal(r.status, 200, name);
    assert.equal(out.isError, false, name);
    assert.deepEqual(out.structuredContent, r.body, name);
    assert.deepEqual(JSON.parse(out.content[0].text), r.body, `${name}: the same JSON as text`);
    return r.body;
  };
  await same("designer_list", {}, "/api/theme");
  await same("designer_read", { path: CSS }, file(CSS));
  const settings = await same("designer_config_read", {}, "/api/settings");
  const history = await same("revisions_list", { path: ".ssg.designer.json" }, "/api/revisions?path=.ssg.designer.json");
  const commit = history.items[0].commitId;
  await same("revision_read", { path: ".ssg.designer.json", commit }, `/api/revisions/${commit}?path=.ssg.designer.json`);
  await same("revision_read", { path: CSS, commit }, `/api/revisions/${commit}?path=${encodeURIComponent(CSS)}`);
  // With a key: that key's entry (and its fields) out of the same view.
  const mddb = (await tool(gh, READ, "designer_config_read", { key: "mddb" })).structuredContent;
  assert.deepEqual(mddb.entries, settings.entries.filter((e: { segments: string[] }) => e.segments[0] === "mddb"));
  assert.deepEqual([mddb.key, mddb.overlayBlobId, mddb.head], ["mddb", settings.overlayBlobId, settings.head]);
  const title = (await tool(gh, READ, "designer_config_read", { key: ["title"] })).structuredContent;
  assert.deepEqual(title.entries.map((e: { path: string; source: string; value: unknown }) => [e.path, e.source, e.value]), [["title", "overlay", "From the panel"]]);
});

test("reads refused alike: a path outside the theme, the config file's old text, a bad commit id", async () => {
  const gh = await site();
  const commit = (await rest(gh, READ, "/api/revisions?path=.ssg.yaml")).body.items[0].commitId;
  const pairs: [string, Record<string, unknown>, string][] = [
    ["designer_read", { path: ".ssg.yaml" }, file(".ssg.yaml")],
    ["revision_read", { path: ".ssg.yaml", commit }, `/api/revisions/${commit}?path=.ssg.yaml`],
    ["revision_read", { path: CSS, commit: "abc" }, `/api/revisions/abc?path=${encodeURIComponent(CSS)}`],
    ["revisions_list", { path: "content/blog/posts/hello.md" }, "/api/revisions?path=content%2Fblog%2Fposts%2Fhello.md"],
  ];
  for (const [name, args, path] of pairs) {
    const out = await tool(gh, READ, name, args);
    const r = await rest(gh, READ, path);
    assert.ok(r.status >= 400, name);
    assert.equal(out.isError, true, name);
    assert.deepEqual(out.structuredContent, refusalOf(r), name);
  }
});

test("designer_write = POST api/theme/publish: the same commit, several files in one", async () => {
  const [a, b] = [await site(), await site()];
  const blob = a.store.blobIdAt("main", CSS);
  const out = await tool(a, THEME, "designer_write", {
    message: "Darker text",
    changes: [
      { path: CSS, content: "body { color: #000; }\n", baseBlobId: blob },
      { path: "templates/mytheme/img/dot.png", content_base64: "iVBORw0KGgo=", baseBlobId: null },
    ],
  });
  const r = await rest(b, THEME, "/api/theme/publish", {
    message: "Darker text",
    changes: [
      { op: "put", path: CSS, content: { text: "body { color: #000; }\n" }, baseBlobId: blob },
      { op: "put", path: "templates/mytheme/img/dot.png", content: { base64: "iVBORw0KGgo=" }, baseBlobId: null },
    ],
  });
  assert.equal(out.isError, false, JSON.stringify(out.structuredContent));
  assert.deepEqual(published(out.structuredContent), published(r.body));
  assert.deepEqual(tree(a), tree(b));
  assert.equal(a.store.commits.get(a.store.refs.get("main")!)!.parents.length, 1, "one commit");
  const one = await tool(a, THEME, "designer_write", { message: "One file", path: CSS, content: "body {}\n", baseBlobId: out.structuredContent.blobIds[CSS] });
  assert.equal(one.isError, false, "one file at the top level, as ssg mcp takes it");
});

test("designer_write refused alike: no designer-ui:theme, a path outside the theme, a stale version", async () => {
  const gh = await site();
  const before = gh.store.refs.get("main");
  const blob = gh.store.blobIdAt("main", CSS);
  const cases: [string[], Record<string, unknown>][] = [
    [ADVANCED, { path: CSS, content: "x", baseBlobId: blob }],
    [THEME, { path: ".ssg.yaml", content: "x", baseBlobId: null }],
    [THEME, { path: ".ssg.designer.json", content: "{}", baseBlobId: null }],
    [THEME, { path: CSS, content: "x", baseBlobId: "0".repeat(40) }],
    [THEME, { path: CSS, content: "x" }],
  ];
  for (const [perms, f] of cases) {
    const out = await tool(gh, perms, "designer_write", { message: "m", ...f });
    const body = { op: "put", path: f.path, content: { text: f.content }, baseBlobId: f.baseBlobId };
    const r = await rest(gh, perms, "/api/theme/publish", { message: "m", changes: [body] });
    assert.ok(r.status >= 400, JSON.stringify(f));
    assert.deepEqual(out.structuredContent, refusalOf(r), JSON.stringify(f));
  }
  assert.equal(gh.store.refs.get("main"), before, "nothing written");
});

test("designer_config_set = POST api/settings/publish: several keys, one commit; null removes; the base read when omitted", async () => {
  const [a, b] = [await site(), await site()];
  const out = await tool(a, SETTINGS, "designer_config_set", { message: "Settings", changes: [{ key: "title", value: "New" }, { key: "mddb.lang", value: "pl" }] });
  const r = await rest(b, SETTINGS, "/api/settings/publish", { message: "Settings", baseBlobId: null, changes: [{ path: ["title"], value: "New" }, { path: ["mddb", "lang"], value: "pl" }] });
  assert.equal(out.isError, false, JSON.stringify(out.structuredContent));
  assert.deepEqual(published(out.structuredContent), published(r.body));
  assert.deepEqual(out.structuredContent.changed, ["mddb.lang", "title"]);
  assert.deepEqual(tree(a), tree(b));
  const base = out.structuredContent.blobId;
  const drop = await tool(a, SETTINGS, "designer_config_set", { message: "Back", key: ["mddb", "lang"], value: null, baseBlobId: base });
  const dropRest = await rest(b, SETTINGS, "/api/settings/publish", { message: "Back", baseBlobId: base, changes: [{ path: ["mddb", "lang"], value: null }] });
  assert.deepEqual(published(drop.structuredContent), published(dropRest.body));
  assert.deepEqual(tree(a), tree(b));
});

test("designer_config_set refused alike, naming each key and why: unknown key, a secret's value, designer-ui:advanced, a stale base", async () => {
  const gh = await site();
  const before = gh.store.refs.get("main");
  const cases: [string[], { key: string; value: unknown }[], unknown, { key: string; reason: RegExp }[]][] = [
    [ADVANCED, [{ key: "no_such_key", value: 1 }, { key: "mddb.nope", value: true }], null, [{ key: "mddb.nope", reason: /unknown configuration key/ }, { key: "no_such_key", reason: /unknown configuration key/ }]],
    [ADVANCED, [{ key: "jwt_secret", value: "plain-literal" }], null, [{ key: "jwt_secret", reason: /environment reference/ }]],
    [SETTINGS, [{ key: "hooks.pre_build", value: ["rm -rf /"] }, { key: "title", value: "ok" }], null, [{ key: "hooks.pre_build", reason: /^needs designer-ui:advanced$/ }]],
    [SETTINGS, [{ key: "mddb.api_key", value: "$OTHER_KEY" }], null, [{ key: "mddb.api_key", reason: /^needs designer-ui:advanced$/ }]],
    [SETTINGS, [{ key: "title", value: "x" }], "0".repeat(40), []],
  ];
  for (const [perms, changes, baseBlobId, refused] of cases) {
    const out = await tool(gh, perms, "designer_config_set", { message: "m", changes, baseBlobId });
    const r = await rest(gh, perms, "/api/settings/publish", { message: "m", baseBlobId, changes: changes.map((c) => ({ path: c.key.split("."), value: c.value })) });
    assert.ok(r.status >= 400, JSON.stringify(changes));
    assert.equal(out.isError, true);
    const got = (out.structuredContent.refused ?? []) as { key: string; reason: string }[];
    assert.deepEqual(out.structuredContent, refusalOf(r), JSON.stringify(changes));
    assert.equal(got.length, refused.length, JSON.stringify(got));
    refused.forEach((w, i) => {
      assert.equal(got[i].key, w.key);
      assert.match(got[i].reason, w.reason);
    });
    assert.ok(!JSON.stringify(out).includes("plain-literal"), "a refused value is never echoed");
  }
  assert.equal(gh.store.refs.get("main"), before, "nothing written");
});

test("designer_publish = POST api/publish: settings and theme files in one commit; refused alike without both permissions", async () => {
  const [a, b] = [await site(), await site()];
  const blob = a.store.blobIdAt("main", CSS);
  const out = await tool(a, ALL, "designer_publish", {
    message: "Together",
    settings: { changes: [{ key: "title", value: "Both" }] },
    theme: { changes: [{ path: CSS, content: "body{}\n", baseBlobId: blob }] },
  });
  const body = {
    message: "Together",
    settings: { baseBlobId: null, changes: [{ path: ["title"], value: "Both" }] },
    theme: { changes: [{ op: "put", path: CSS, content: { text: "body{}\n" }, baseBlobId: blob }] },
  };
  const r = await rest(b, ALL, "/api/publish", body);
  assert.equal(out.isError, false, JSON.stringify(out.structuredContent));
  assert.deepEqual(published(out.structuredContent), published(r.body));
  assert.deepEqual(tree(a), tree(b));
  for (const perms of [SETTINGS, THEME]) {
    const refused = await tool(b, perms, "designer_publish", { message: "m", settings: { baseBlobId: r.body.settings.blobId, changes: [{ key: "title", value: "x" }] }, theme: { changes: [{ path: CSS, content: "x", baseBlobId: blob }] } });
    const restRefused = await rest(b, perms, "/api/publish", { ...body, settings: { ...body.settings, baseBlobId: r.body.settings.blobId } });
    assert.deepEqual(refused.structuredContent, refusalOf(restRefused));
    assert.equal(restRefused.status, 403);
  }
  const theme = await tool(a, THEME, "designer_publish", { message: "Theme only", theme: { changes: [{ path: CSS, content: "a{}\n", baseBlobId: out.structuredContent.theme.blobIds[CSS] }] } });
  assert.equal(theme.isError, false, "one part needs only its own permission");
  const settings = await tool(a, SETTINGS, "designer_publish", { message: "Settings only", settings: { baseBlobId: out.structuredContent.settings.blobId, changes: [{ key: "title", value: "Again" }] } });
  assert.equal(settings.isError, false, JSON.stringify(settings.structuredContent));
});
