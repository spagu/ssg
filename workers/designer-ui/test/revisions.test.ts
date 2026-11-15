// Revisions: the repository's history of a theme file or the settings file;
// an old theme file as it was; old settings parsed and redacted, never as
// text; the config file's old text never served.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ADVANCED, SETTINGS, site } from "./fixtures.ts";
import { OVERLAY_LITERAL, assertNoSecret, get, publish } from "./helpers.ts";

const q = (p: string) => "?path=" + encodeURIComponent(p);

test("history of a theme file and of the settings, newest first, with a cursor", async () => {
  const gh = await site();
  await gh.store.commitFiles("main", { "templates/mytheme/index.html": "v2\n" }, "second");
  const list = await get(gh, "/api/revisions" + q("templates/mytheme/index.html"));
  assert.equal(list.status, 200);
  assert.deepEqual(list.body.items.map((i: { message: string }) => i.message), ["second", "seed"]);
  assert.equal(list.body.nextCursor, null);
  await publish(gh, SETTINGS, [{ path: ["title"], value: "One" }]);
  const settings = await get(gh, "/api/revisions" + q(".ssg.designer.json"));
  assert.equal(settings.body.items.length, 1);
  assert.equal((await get(gh, "/api/revisions" + q(".ssg.yaml"))).body.items.length, 1, "the config file's history is listed");
  const page = await get(gh, "/api/revisions" + q(".ssg.yaml") + "&cursor=x");
  assert.deepEqual([page.status, page.body.error], [400, "invalid_change"]);
});

test("a theme file at an old commit; the settings at an old commit, redacted", async () => {
  const gh = await site({ ".ssg.designer.json": JSON.stringify({ jwt_secret: OVERLAY_LITERAL, title: "Old", mddb: { api_key: "$K" } }) });
  const first = gh.store.refs.get("main")!;
  await gh.store.commitFiles("main", { "templates/mytheme/index.html": "v2\n" }, "second");
  const old = await get(gh, `/api/revisions/${first}` + q("templates/mytheme/index.html"));
  assert.deepEqual([old.body.kind, old.body.encoding, old.body.text, old.body.commitId], ["theme", "text", "<!doctype html><title>{{ .Site.Title }}</title>\n", first]);
  const settings = await get(gh, `/api/revisions/${first}` + q(".ssg.designer.json"));
  assert.equal(settings.body.kind, "settings");
  assert.deepEqual(settings.body.settings, { jwt_secret: { set: true, reference: null }, title: "Old", mddb: { api_key: { set: true, reference: "$K" } } });
  assert.equal(settings.body.problems.length, 1, "the literal is what ssg would refuse");
  assertNoSecret();
});

test("an old settings file that is not JSON: nothing shown, the problem said", async () => {
  const gh = await site({ ".ssg.designer.json": "{broken" });
  const at = gh.store.refs.get("main")!;
  const r = await get(gh, `/api/revisions/${at}` + q(".ssg.designer.json"));
  assert.deepEqual([r.body.settings, r.body.problems], [null, ["the top level must be an object"]]);
});

test("refusals: the config file's text, a bad commit id, a path outside the policies", async () => {
  const gh = await site();
  const at = gh.store.refs.get("main")!;
  const cfg = await get(gh, `/api/revisions/${at}` + q(".ssg.yaml"));
  assert.deepEqual([cfg.status, cfg.body.error], [400, "not_served"]);
  assert.equal((await get(gh, "/api/revisions/main" + q("templates/mytheme/index.html"))).status, 400);
  for (const p of ["content/blog/posts/hello.md", "templates/mytheme/notes.txt"]) {
    const r = await get(gh, "/api/revisions" + q(p));
    assert.deepEqual([r.status, r.body.error], [400, "path_not_allowed"], p);
  }
  const noPath = await get(gh, "/api/revisions");
  assert.equal(noPath.status, 400);
  const builtIn = await site({ ".ssg.yaml": "template: simple\n" });
  assert.equal((await get(builtIn, "/api/revisions" + q("templates/simple/index.html"))).status, 409);
  assert.equal((await publish(builtIn, ADVANCED, [{ path: ["title"], value: "x" }])).status, 200, "settings still work");
});
