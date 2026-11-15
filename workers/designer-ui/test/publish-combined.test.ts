// POST api/publish: settings and theme files in ONE commit; a missing
// permission or a conflict in either part refuses the whole, nothing written.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ADVANCED, READ, SETTINGS, THEME, call, site } from "./fixtures.ts";
import { changedFiles, fileAt, get } from "./helpers.ts";

const ALL = [...ADVANCED, "designer-ui:theme"];
const BOTH = [...SETTINGS, "designer-ui:theme"];
const CSS = "templates/mytheme/css/style.css";

async function body(gh: Awaited<ReturnType<typeof site>>, over: Record<string, unknown> = {}) {
  const s = (await get(gh, "/api/settings")).body;
  const f = (await get(gh, "/api/theme/files/" + encodeURIComponent(CSS))).body;
  return {
    message: "Colours and name",
    settings: { baseBlobId: s.overlayBlobId, changes: [{ path: ["title"], value: "New name" }] },
    theme: { changes: [{ op: "put", path: CSS, content: { text: "body { color: #111; }\n" }, baseBlobId: f.blobId }] },
    ...over,
  };
}
const commits = (gh: Awaited<ReturnType<typeof site>>) => gh.store.commits.size;

test("one commit holds the overlay and the theme files", async () => {
  const gh = await site();
  const before = commits(gh);
  const r = await call(gh, "/api/publish", BOTH, { json: await body(gh) });
  assert.equal(r.status, 200, r.text);
  assert.equal(commits(gh), before + 1, "exactly one commit");
  assert.deepEqual(changedFiles(gh), [".ssg.designer.json", CSS]);
  assert.deepEqual(r.body.settings.changed, ["title"]);
  assert.equal(r.body.settings.blobId, gh.store.blobIdAt("main", ".ssg.designer.json"));
  assert.deepEqual(Object.keys(r.body.theme.blobIds), [CSS]);
  assert.match(fileAt(gh, ".ssg.designer.json")!, /"title": "New name"/);
  assert.equal(fileAt(gh, CSS), "body { color: #111; }\n");
  assert.equal(gh.store.commits.get(gh.store.refs.get("main")!)!.message, "Colours and name");
});

test("one part alone works, with that part's permission only", async () => {
  const gh = await site();
  const { settings, theme, message } = await body(gh);
  const s = await call(gh, "/api/publish", SETTINGS, { json: { message, settings } });
  assert.equal(s.status, 200, s.text);
  assert.equal(s.body.theme, undefined);
  assert.deepEqual(changedFiles(gh), [".ssg.designer.json"]);
  const t = await call(gh, "/api/publish", THEME, { json: { message, theme } });
  assert.equal(t.status, 200, t.text);
  assert.equal(t.body.settings, undefined);
  assert.deepEqual(changedFiles(gh), [CSS]);
});

test("a missing permission refuses the whole publication; nothing is written", async () => {
  const gh = await site();
  const before = commits(gh);
  const json = await body(gh);
  for (const [perms, missing] of [[SETTINGS, "designer-ui:theme"], [THEME, "designer-ui:settings"], [READ, "designer-ui:settings"]] as const) {
    const r = await call(gh, "/api/publish", [...perms], { json });
    assert.equal(r.status, 403, r.text);
    assert.equal(r.body.permission, missing);
  }
  // a key that needs designer-ui:advanced, beside a theme part the caller may publish
  const hooks = { ...json, settings: { ...json.settings, changes: [{ path: ["hooks"], value: { pre_build: ["echo x"] } }] } };
  const adv = await call(gh, "/api/publish", BOTH, { json: hooks });
  assert.equal(adv.status, 403);
  assert.deepEqual([adv.body.permission, adv.body.keys], ["designer-ui:advanced", ["hooks.pre_build"]]);
  assert.equal(commits(gh), before, "no commit");
  assert.equal((await call(gh, "/api/publish", ALL, { json: hooks })).status, 200);
});

test("a conflict in either part refuses the whole", async () => {
  const gh = await site();
  const json = await body(gh);
  await gh.store.commitFiles("main", { ".ssg.designer.json": '{"toc": true}\n' }, "someone else's settings");
  const before = commits(gh);
  const s = await call(gh, "/api/publish", BOTH, { json });
  assert.equal(s.status, 409);
  assert.equal(s.body.conflicts[0].path, ".ssg.designer.json");
  assert.equal(commits(gh), before);

  const gh2 = await site();
  const json2 = await body(gh2);
  await gh2.store.commitFiles("main", { [CSS]: "body { color: red; }\n" }, "someone else's theme");
  const before2 = commits(gh2);
  const t = await call(gh2, "/api/publish", BOTH, { json: json2 });
  assert.equal(t.status, 409, t.text);
  assert.equal(t.body.error, "conflict");
  assert.ok(t.body.conflicts.some((c: { path: string }) => c.path === CSS));
  assert.equal(commits(gh2), before2);
  assert.doesNotMatch(fileAt(gh2, ".ssg.designer.json") ?? "", /New name/, "the settings part was not written either");
});

test("shape: no part, or a part that is not an object", async () => {
  const gh = await site();
  for (const json of [{ message: "x" }, { message: "x", settings: [] }, { message: "x", theme: "css" }]) {
    const r = await call(gh, "/api/publish", ALL, { json });
    assert.equal(r.status, 400, JSON.stringify(json));
    assert.equal(r.body.error, "invalid_change");
  }
  const bad = await call(gh, "/api/publish", ALL, { json: { message: "x", theme: { changes: [{ op: "put", path: ".ssg.designer.json", content: { text: "{}" } }] } } });
  assert.equal(bad.status, 400, "the overlay is never written as a theme file");
});
