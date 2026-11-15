// Publishing to MDDB through the whole pipeline: one article per publication
// and one source, no rename, images to the repository (or refused without
// one), conflicts when MDDB moved on in between, the site's filters kept, the
// optional rebuild call — and neither key in any answer, log or foreign request.
import { test } from "node:test";
import assert from "node:assert/strict";
import { MDDB_KEY } from "./fake-mddb.ts";
import { ORIGIN } from "./fixtures.ts";
import { REBUILD_KEY, bothEnv, mddbOnlyEnv, post, send, seg, world } from "./mddb-fixtures.ts";

const META = { type: ["post"] };
import type { World } from "./mddb-fixtures.ts";
import { PERM } from "../src/permissions.ts";
import { SAVED_NOTICE, STARTED_NOTICE } from "../src/rebuild.ts";

const WRITE = [PERM.read, PERM.publish];
const ALL = [PERM.read, PERM.publish, PERM.media, PERM.delete];
const P = "mddb:blog/en/x";
const PNG = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52, 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0]);

const call = async (w: World, env: Record<string, unknown>, changes: unknown[], perms: string[] = WRITE) => {
  const res = await post(w, env, "/api/changes", perms, { message: "m", changes });
  return { status: res.status, out: await res.json() };
};
const version = async (w: World, env: Record<string, unknown>, path = P) => (await (await send(w, env, `/api/content/${seg(path)}`, [PERM.read])).json()).version;
async function seededWorld(opts: { git?: boolean } = {}) {
  const w = await world(opts);
  w.db.put("blog", "x", "en", { type: ["post"], title: ["X"] }, "x\n");
  w.db.put("kb", "faq", "en", { visibility: ["public"] }, "q\n");
  w.db.put("kb", "secret", "en", { visibility: ["private"] }, "s\n");
  return w;
}

test("one source, one article, no rename", async () => {
  const w = await seededWorld();
  const env = bothEnv();
  const v = await version(w, env);
  const mixed = await call(w, env, [{ op: "put", path: P, version: v, text: "a" }, { op: "put", path: "docs/guide.md", version: null, text: "b" }]);
  assert.deepEqual([mixed.status, mixed.out.error], [400, "invalid_change"]);
  const two = await call(w, env, [{ op: "put", path: P, version: v, text: "a" }, { op: "put", path: "mddb:blog/en/y", version: null, text: "b" }]);
  assert.deepEqual([two.status, two.out.error], [400, "invalid_change"]);
  const move = await call(w, env, [{ op: "move", from: P, to: "mddb:blog/en/z", version: v }]);
  assert.deepEqual([move.status, move.out.detail], [400, "an MDDB article is not renamed: its key is its address; change its slug to change its URL"]);
  const git = await call(w, env, [{ op: "put", path: "docs/new.md", version: null, text: "n" }]);
  assert.deepEqual([git.status, git.out.origin, git.out.url.startsWith("https://github.com/")], [201, undefined, true]);
});

test("the answer: the new revision, the collection, the notice that the site shows it after the next build", async () => {
  const w = await seededWorld({ git: false });
  const env = mddbOnlyEnv();
  const before = await version(w, env);
  const r = await call(w, env, [{ op: "put", path: P, version: before, text: "---\ntitle: Y\ntype: post\n---\ny\n" }]);
  assert.equal(r.status, 201);
  assert.deepEqual(r.out, {
    commitId: await version(w, env), url: "", branch: "blog", files: [{ path: P, version: await version(w, env) }], origin: "mddb",
    rebuild: { triggered: false, reason: "not_configured" }, notice: SAVED_NOTICE,
  });
  assert.deepEqual(w.db.get("blog", "x", "en")!.meta, { title: ["Y"], type: ["post"] });
  assert.equal(w.db.requests.find((q) => q.path === "/v1/update")!.body.expectedRevision, before, "a conditional write");
});

test("the rebuild: on the site's own origin, with EDITOR_REBUILD_KEY, never the caller's cookie", async () => {
  const w = await seededWorld({ git: false });
  const env = mddbOnlyEnv({ EDITOR_REBUILD_URL: "/api/republish", EDITOR_REBUILD_KEY: REBUILD_KEY });
  const edit = async (e: Record<string, unknown>) => call(w, e, [{ op: "put", path: P, version: await version(w, e), text: `---\ntype: post\n---\n${Math.random()}\n` }]);
  const ok = await edit(env);
  assert.deepEqual([ok.out.rebuild, ok.out.notice], [{ triggered: true, status: 202 }, STARTED_NOTICE]);
  assert.equal(w.rebuilds.length, 1);
  assert.deepEqual([w.rebuilds[0].url, w.rebuilds[0].headers.get("authorization"), w.rebuilds[0].headers.get("cookie")], [`${ORIGIN}/api/republish`, `Bearer ${REBUILD_KEY}`, null]);
  w.rebuildStatus = 429;
  assert.deepEqual((await edit(env)).out.rebuild, { triggered: false, status: 429, reason: "refused" });
  w.rebuildStatus = "throw";
  assert.deepEqual((await edit(env)).out.rebuild, { triggered: false, reason: "unreachable" });
  const before = w.rebuilds.length;
  for (const url of ["https://elsewhere.example.com/api/republish", "http://example.com/api/republish", "https://u:p@example.com/x", "http://[bad"]) {
    const r = await edit({ ...env, EDITOR_REBUILD_URL: url });
    assert.deepEqual([r.status, r.out.rebuild.reason, r.out.notice], [201, "bad_url", SAVED_NOTICE], url);
  }
  assert.deepEqual((await edit({ ...env, EDITOR_REBUILD_KEY: "" })).out.rebuild, { triggered: false, reason: "not_configured" });
  assert.equal(w.rebuilds.length, before, "no call to another origin or without the key");
});

test("images go to the repository first, in their own commit; without a repository they are refused", async () => {
  const w = await seededWorld();
  const env = bothEnv();
  const name = await (await send(w, env, `/api/media/new-path?name=a.png&type=image/png&hash=${"0".repeat(64)}`, [PERM.read])).json();
  const hash = Buffer.from(await crypto.subtle.digest("SHA-256", PNG)).toString("hex");
  const path = (await (await send(w, env, `/api/media/new-path?name=a.png&type=image/png&hash=${hash}`, [PERM.read])).json()).path;
  assert.ok(name.path);
  const r = await call(w, env, [{ op: "put", path, version: null, base64: PNG.toString("base64") }, { op: "put", path: P, version: await version(w, env), text: `---\ntype: post\n---\n![a](/${path})\n` }], ALL);
  assert.equal(r.status, 201);
  assert.equal(r.out.files.length, 2);
  assert.deepEqual([r.out.files[0].path, r.out.files[1].path, r.out.url.startsWith("https://github.com/acme/site/commit/")], [P, path, true]);
  assert.ok(w.gh!.store.blobIdAt("main", path));
  const only = await seededWorld({ git: false });
  const refused = await call(only, mddbOnlyEnv(), [{ op: "put", path: "content/blog/media/a.png", version: null, base64: PNG.toString("base64") }, { op: "put", path: P, version: await version(only, mddbOnlyEnv()), text: "t" }], ALL);
  assert.deepEqual([refused.status, refused.out.error], [503, "media_needs_repository"]);
  assert.equal(only.db.get("blog", "x", "en")!.contentMd, "x\n", "nothing written");
});

test("the site's filter holds for writes: no article may leave it, none may be created outside it", async () => {
  const w = await seededWorld();
  const env = bothEnv();
  const faq = "mddb:kb/en/faq";
  const leave = await call(w, env, [{ op: "put", path: faq, version: await version(w, env, faq), fields: { visibility: "private" } }]);
  assert.deepEqual([leave.status, leave.out.error], [400, "invalid_change"]);
  const outside = await call(w, env, [{ op: "put", path: "mddb:kb/en/new", version: null, text: "no visibility" }]);
  assert.equal(outside.status, 400);
  const inside = await call(w, env, [{ op: "put", path: "mddb:kb/en/new", version: null, fields: { visibility: "public" }, body: "b" }]);
  assert.equal(inside.status, 201);
  const hidden = await call(w, env, [{ op: "put", path: "mddb:kb/en/secret", version: null, text: "mine" }]);
  assert.deepEqual([hidden.status, hidden.out.files], [409, [{ path: "mddb:kb/en/secret", yours: "mine", theirs: null, theirVersion: null, theirsUnread: "unavailable" }]]);
  const over = await call(w, env, [{ op: "put", path: "mddb:kb/en/secret", version: "abc", text: "mine" }]);
  assert.deepEqual([over.status, over.out.files[0].theirs], [409, null]);
  const nested = await call(w, env, [{ op: "put", path: faq, version: await version(w, env, faq), text: "---\nvisibility: public\nauthor:\n  name: Ada\n---\n" }]);
  assert.deepEqual([nested.status, nested.out.error, nested.out.refused], [400, "fields_refused", [{ key: "author", reason: "not_flat" }]]);
  const badKey = await call(w, env, [{ op: "put", path: faq, version: await version(w, env, faq), fields: { "a b": "x" } }]);
  assert.deepEqual([badKey.status, badKey.out.error], [400, "fields_refused"]);
  const schema = await seededWorld({ git: false });
  schema.db.override = (p) => (p === "/v1/update" ? Response.json({ error: "schema: title is required" }, { status: 400 }) : null);
  const refused = await call(schema, mddbOnlyEnv(), [{ op: "put", path: P, version: await version(schema, mddbOnlyEnv()), text: "---\ntype: post\n---\n" }]);
  assert.deepEqual([refused.status, refused.out.error, refused.out.detail], [422, "mddb_refused", "MDDB refused the request: schema: title is required"]);
});

test("a write that lands between the editor's read and its conditional write is a conflict, as is a delete in between", async () => {
  const w = await seededWorld({ git: false });
  const env = mddbOnlyEnv();
  const v = await version(w, env);
  w.db.beforeWrite = () => { w.db.beforeWrite = null; w.db.put("blog", "x", "en", { type: ["post"] }, "theirs\n"); };
  const race = await call(w, env, [{ op: "put", path: P, version: v, fields: { title: "Mine" } }]);
  assert.deepEqual([race.status, race.out.files[0].theirs, race.out.files[0].yours], [409, "---\ntype: post\n---\ntheirs\n", "---\ntype: post\ntitle: Mine\n---\ntheirs\n"]);
  const v2 = await version(w, env);
  w.db.beforeWrite = () => { w.db.beforeWrite = null; w.db.docs.clear(); };
  const gone = await call(w, env, [{ op: "put", path: P, version: v2, text: "---\ntype: post\n---\nt\n" }]);
  assert.deepEqual([gone.status, gone.out.files[0].theirs, gone.out.files[0].theirVersion], [409, null, null]);
  w.db.put("blog", "x", "en", { type: ["post"] }, "x");
  const del = await call(w, env, [{ op: "delete", path: P, version: await version(w, env) }], ALL);
  assert.deepEqual([del.status, del.out.commitId, del.out.files], [201, "", [{ path: P, version: null }]]);
  const fresh = await call(w, env, [{ op: "put", path: P, version: null, fields: { "a b": "x" } }]);
  assert.deepEqual([fresh.status, fresh.out.error], [400, "fields_refused"]);
  const lost = await call(w, env, [{ op: "put", path: P, version: "nope", fields: { "a b": "x" } }]);
  assert.deepEqual([lost.status, lost.out.files[0].yours], [409, null]);
});

test("names: ten taken names in a row is 409 name_taken", async () => {
  const w = await world({ git: false });
  for (const k of ["a", ...Array.from({ length: 9 }, (_, i) => `a-${i + 2}`)]) w.db.put("blog", k, "en", META, "");
  const r = await send(w, mddbOnlyEnv(), `/api/content/new-path?section=${seg("mddb:blog/pl")}&title=a`, [PERM.read]);
  assert.equal(r.status, 200, "another language is another name");
  const taken = await send(w, mddbOnlyEnv(), `/api/content/new-path?section=${seg("mddb:blog")}&title=a`, [PERM.read]);
  assert.deepEqual([taken.status, (await taken.json()).error], [409, "name_taken"]);
});

test("neither key is in any answer or log, and the MDDB key goes to MDDB only", async () => {
  const logs: string[] = [];
  const saved = { error: console.error, log: console.log, warn: console.warn };
  for (const k of ["error", "log", "warn"] as const) console[k] = (...a: unknown[]) => { logs.push(a.map(String).join(" ")); };
  const bodies: string[] = [];
  try {
    const w = await seededWorld();
    const env = bothEnv({ EDITOR_REBUILD_URL: "/api/republish", EDITOR_REBUILD_KEY: REBUILD_KEY });
    const keep = async (res: Response) => { bodies.push(JSON.stringify([...res.headers]) + (await res.text())); };
    await keep(await send(w, env, "/api/content", [PERM.read]));
    await keep(await send(w, env, `/api/content/${seg(P)}`, [PERM.read]));
    await keep(await post(w, env, "/api/changes", WRITE, { message: "m", changes: [{ op: "put", path: P, version: await version(w, env), text: "---\ntype: post\n---\n" }] }));
    await keep(await send(w, env, `/api/revisions/${seg(P)}`, [PERM.read]));
    w.db.key = "another";
    await keep(await send(w, env, `/api/content/${seg(P)}`, [PERM.read]));
    w.db.override = () => { throw new Error(`boom ${MDDB_KEY}`); };
    await keep(await send(w, env, `/api/content/${seg(P)}`, [PERM.read]));
    w.db.override = () => Response.json({ error: `echo ${MDDB_KEY}` }, { status: 500 });
    await keep(await send(w, env, `/api/content/${seg(P)}`, [PERM.read]));
    const all = bodies.join("\n") + logs.join("\n");
    assert.ok(bodies.length === 7 && !all.includes(MDDB_KEY) && !all.includes(REBUILD_KEY));
    assert.ok(w.gh!.requests.every((r) => !JSON.stringify(r).includes(MDDB_KEY)));
    assert.ok(w.rebuilds.every((r) => ![...r.headers].some(([, v]) => v.includes(MDDB_KEY))));
  } finally {
    Object.assign(console, saved);
  }
});

test("the rebuild without an injected fetch uses the global one", async () => {
  const { triggerRebuild } = await import("../src/rebuild.ts");
  const saved = globalThis.fetch;
  const seen: string[] = [];
  globalThis.fetch = (async (u: string) => { seen.push(u); return new Response(null, { status: 204 }); }) as typeof fetch;
  try {
    const ctx = { env: { EDITOR_REBUILD_URL: "/api/republish", EDITOR_REBUILD_KEY: "k" }, deps: {}, url: new URL(`${ORIGIN}/editor/api/changes`) };
    assert.deepEqual((await triggerRebuild(ctx as never)).rebuild, { triggered: true, status: 204 });
    assert.deepEqual(seen, [`${ORIGIN}/api/republish`]);
  } finally {
    globalThis.fetch = saved;
  }
});
