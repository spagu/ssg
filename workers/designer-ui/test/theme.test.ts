// The theme: listing and reading for designer-ui:read, publishing for
// designer-ui:theme, the path policy (the theme folder and the extension
// list, nothing else — never .ssg.yaml, never the overlay), and a theme that
// is not in the repository explained instead of guessed.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ADVANCED, READ, SETTINGS, SITE_YAML, THEME, call, site } from "./fixtures.ts";
import { changedFiles, fileAt, get } from "./helpers.ts";
import { themeLocation } from "../src/theme.ts";
import type { ConfigState } from "../src/config-read.ts";

const file = (p: string) => "/api/theme/files/" + encodeURIComponent(p);
const themePublish = (gh: Awaited<ReturnType<typeof site>>, perms: string[], changes: unknown, message: unknown = "Theme") =>
  call(gh, "/api/theme/publish", perms, { json: { message, changes } });

test("the list: the theme folder's files the policy allows, sorted, with versions", async () => {
  const gh = await site();
  const { status, body } = await get(gh, "/api/theme");
  assert.equal(status, 200);
  assert.deepEqual([body.available, body.template, body.dir, body.truncated], [true, "mytheme", "templates/mytheme", false]);
  assert.deepEqual(body.files.map((f: { path: string; text: boolean }) => [f.path, f.text]), [
    ["templates/mytheme/css/style.css", true],
    ["templates/mytheme/img/logo.png", false],
    ["templates/mytheme/index.html", true],
  ]);
  assert.equal(body.files[2].blobId, gh.store.blobIdAt("main", "templates/mytheme/index.html"));
});

test("reading a file: text as UTF-8, images and non-UTF-8 text as base64, anything outside the policy refused", async () => {
  const gh = await site({ "templates/mytheme/latin1.css": "café" });
  gh.store.blobs.set(gh.store.blobIdAt("main", "templates/mytheme/latin1.css")!, new Uint8Array([0x63, 0xe9]));
  const html = await get(gh, file("templates/mytheme/index.html"));
  assert.deepEqual([html.body.encoding, html.body.text, html.body.head], ["text", "<!doctype html><title>{{ .Site.Title }}</title>\n", gh.store.refs.get("main")]);
  const png = await get(gh, file("templates/mytheme/img/logo.png"));
  assert.equal(png.body.encoding, "base64");
  assert.ok(Buffer.from(png.body.base64, "base64").equals(Buffer.from("\u0089PNG-not-really")), "the bytes as stored");
  assert.equal((await get(gh, file("templates/mytheme/latin1.css"))).body.base64, "Y+k=");
  for (const p of ["templates/mytheme/notes.txt", ".ssg.yaml", ".ssg.designer.json", "content/blog/posts/hello.md", "templates/mytheme/../../.ssg.yaml"]) {
    const r = await get(gh, file(p));
    assert.deepEqual([r.status, r.body.error], [400, "path_not_allowed"], p);
  }
});

test("publishing needs designer-ui:theme; one commit with the theme files only", async () => {
  const gh = await site();
  const blob = gh.store.blobIdAt("main", "templates/mytheme/css/style.css");
  const changes = [
    { op: "put", path: "templates/mytheme/css/style.css", content: { text: "body { color: #000; }\n" }, baseBlobId: blob },
    { op: "put", path: "templates/mytheme/partials/footer.html", content: { text: "<footer></footer>\n" }, baseBlobId: null },
    { op: "move", from: "templates/mytheme/img/logo.png", to: "templates/mytheme/img/brand.png", baseBlobId: gh.store.blobIdAt("main", "templates/mytheme/img/logo.png") },
  ];
  for (const perms of [READ, SETTINGS, ADVANCED]) {
    const r = await themePublish(gh, perms, changes);
    assert.deepEqual([r.status, r.body], [403, { error: "forbidden", permission: "designer-ui:theme" }]);
  }
  const ok = await themePublish(gh, THEME, changes);
  assert.equal(ok.status, 200, ok.text);
  assert.deepEqual(changedFiles(gh), ["templates/mytheme/css/style.css", "templates/mytheme/img/brand.png", "templates/mytheme/img/logo.png", "templates/mytheme/partials/footer.html"]);
  assert.equal(fileAt(gh, "templates/mytheme/css/style.css"), "body { color: #000; }\n");
  const del = await themePublish(gh, THEME, [{ op: "delete", path: "templates/mytheme/partials/footer.html", baseBlobId: ok.body.blobIds["templates/mytheme/partials/footer.html"] }]);
  assert.equal(del.status, 200, del.text);
  assert.equal(fileAt(gh, "templates/mytheme/partials/footer.html"), null);
});

test("the theme route can write neither .ssg.yaml nor the settings overlay, nor anything outside the theme", async () => {
  const gh = await site();
  const head = gh.store.refs.get("main");
  const put = (path: string) => ({ op: "put", path, content: { text: "x" }, baseBlobId: null });
  for (const p of [".ssg.yaml", ".ssg.designer.json", "templates/mytheme/../../.ssg.yaml", "templates/mytheme/notes.txt", "templates/other/x.html", ".github/workflows/x.js", "templates/mytheme/.hidden.html", "templates/mytheme/wrangler.json"]) {
    const r = await themePublish(gh, ["*"], [put(p)]);
    assert.deepEqual([r.status, r.body.error], [400, "path_not_allowed"], p);
  }
  const move = await themePublish(gh, ["*"], [{ op: "move", from: "templates/mytheme/index.html", to: ".ssg.yaml", baseBlobId: "a".repeat(40) }]);
  assert.equal(move.status, 400);
  for (const changes of [[], "x", [7], [{ op: "chmod", path: "templates/mytheme/index.html" }]]) {
    assert.deepEqual((await themePublish(gh, THEME, changes)).body.error, "invalid_change", JSON.stringify(changes));
  }
  assert.equal(gh.store.refs.get("main"), head);
  assert.equal((await themePublish(gh, THEME, [put("templates/mytheme/new.css")], "")).body.error, "invalid_message");
});

test("a file changed since it was read: 409 with the conflicting path", async () => {
  const gh = await site();
  const stale = gh.store.blobIdAt("main", "templates/mytheme/index.html");
  await gh.store.commitFiles("main", { "templates/mytheme/index.html": "changed elsewhere\n" });
  const r = await themePublish(gh, THEME, [{ op: "put", path: "templates/mytheme/index.html", content: { text: "mine" }, baseBlobId: stale }]);
  assert.equal(r.status, 409);
  assert.deepEqual(r.body.conflicts.map((c: { path: string }) => c.path), ["templates/mytheme/index.html"]);
});

test("a theme that is not in the repository: explained, with the way to bring it in; settings still work", async () => {
  const builtIn = await site({ ".ssg.yaml": SITE_YAML.replace("template: mytheme", "template: simple") });
  const list = (await get(builtIn, "/api/theme")).body;
  assert.deepEqual([list.available, list.reason, list.dir], [false, "theme_not_in_repository", "templates/simple"]);
  assert.match(list.detail, /ssg uses its built-in copy[\s\S]*commit that folder/);
  const read = await get(builtIn, file("templates/simple/index.html"));
  assert.deepEqual([read.status, read.body.error, read.body.template], [409, "theme_not_in_repository", "simple"]);
  assert.equal((await themePublish(builtIn, THEME, [{ op: "put", path: "templates/simple/a.css", content: { text: "" }, baseBlobId: null }])).status, 409);
  assert.equal((await get(builtIn, "/api/settings")).status, 200);
  const online = await site({ ".ssg.yaml": SITE_YAML + "online_theme: https://example.com/theme.zip\n" });
  assert.equal((await get(online, "/api/theme")).body.reason, "online_theme");
  const none = await site({ ".ssg.yaml": "title: X\n" });
  assert.deepEqual([(await get(none, "/api/theme")).body.reason, (await get(none, "/api/theme")).body.template], ["no_template", null]);
});

test("themeLocation: templates_dir forms, unreadable keys, a config the worker cannot read", () => {
  const state = (over: Partial<ConfigState> = {}): ConfigState => ({ head: "h", file: ".ssg.yaml", format: "yaml", values: {}, unread: {}, fileProblem: null, include: false, overlay: null, overlayBlobId: null, overlayProblem: null, ...over });
  assert.deepEqual(themeLocation(state(), { template: "t", templates_dir: "./themes/" }), { ok: true, dir: "themes/t", template: "t" });
  assert.deepEqual(themeLocation(state(), { template: "t", templates_dir: "." }), { ok: true, dir: "t", template: "t" });
  assert.deepEqual(themeLocation(state(), { template: 7 }), { ok: true, dir: "templates/7", template: "7" });
  assert.equal((themeLocation(state(), { template: "t", templates_dir: "$THEMES" }) as { reason: string }).reason, "theme_dir_unknown");
  assert.match((themeLocation(state(), { template: "../t", templates_dir: "" }) as { detail: string }).detail, /traversal/);
  assert.match((themeLocation(state({ unread: { templates_dir: "line 3 uses a tag" } }), {}) as { detail: string }).detail, /templates_dir is set, but its value could not be read \(line 3 uses a tag\)/);
  assert.equal((themeLocation(state({ format: "toml", fileProblem: "TOML" }), {}) as { detail: string }).detail, "TOML");
  assert.equal((themeLocation(state({ format: null, fileProblem: "none" }), { template: ["x"] }) as { reason: string }).reason, "no_template");
});
