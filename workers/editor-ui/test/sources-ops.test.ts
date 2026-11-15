// The content operations of GO-170 and GO-172, run on both sources with the
// same assertions: a file in the repository (fake GitHub) and a document in
// MDDB (fake MDDB, no repository configured). Only versions and revision ids
// differ, and they are compared with what the same source answered.
import { test } from "node:test";
import assert from "node:assert/strict";
import { makeEnv } from "./fixtures.ts";
import { mddbOnlyEnv, post, send, seg, world } from "./mddb-fixtures.ts";
import type { World } from "./mddb-fixtures.ts";
import { PERM } from "../src/permissions.ts";

const READ: string[] = [PERM.read];
const WRITE: string[] = [PERM.read, PERM.publish];
// status: publish — a file without it is a draft for ssg, a document without
// it is published (internal/mddb/converter.go); with it both read the same.
const TEXT = "---\nstatus: publish\ntitle: Hello\ntype: post\n---\nBody\n";

interface Backend { name: string; section: string; path(name: string): string; open(): Promise<{ w: World; env: Record<string, unknown> }> }
const BACKENDS: Backend[] = [
  {
    name: "repository",
    section: "content/blog/posts",
    path: (n) => `content/blog/posts/${n}.md`,
    open: async () => ({ w: await world({ files: { "content/blog/posts/hello.md": TEXT } }), env: makeEnv() }),
  },
  {
    name: "MDDB",
    section: "mddb:blog",
    path: (n) => `mddb:blog/en/${n}`,
    open: async () => {
      const w = await world({ git: false });
      w.db.put("blog", "hello", "en", { status: ["publish"], title: ["Hello"], type: ["post"] }, "Body\n");
      return { w, env: mddbOnlyEnv() };
    },
  },
];

for (const b of BACKENDS) {
  const P = b.path("hello");
  const open = async () => {
    const { w, env } = await b.open();
    const json = async (path: string, perms = READ) => {
      const res = await send(w, env, path, perms);
      return { status: res.status, out: await res.json() };
    };
    const publish = async (body: unknown, perms = WRITE) => {
      const res = await post(w, env, "/api/changes", perms, body);
      return { status: res.status, out: await res.json() };
    };
    const read = async (path = P) => (await json(`/api/content/${seg(path)}`)).out;
    return { json, publish, read };
  };

  test(`${b.name}: the list shows the article with its title, type, language and draft state`, async () => {
    const { json } = await open();
    const { status, out } = await json(`/api/content?section=${seg(b.section)}`);
    assert.equal(status, 200);
    const item = out.items.find((i: { path: string }) => i.path === P);
    assert.deepEqual([item.title, item.type, item.language, item.draft, item.loaded], ["Hello", "post", "en", false, true]);
    assert.deepEqual([out.nextCursor, out.pending, out.truncated], [null, 0, false]);
    const q = await json(`/api/content?q=HELL`);
    assert.deepEqual(q.out.items.map((i: { path: string }) => i.path), [P]);
  });

  test(`${b.name}: read gives the frontmatter fields, the raw block, the body and a version`, async () => {
    const { read, json } = await open();
    const a = await read();
    assert.deepEqual([a.path, a.format, a.bom, a.frontmatterRaw, a.fields, a.fixed, a.body], [P, "yaml", false, "status: publish\ntitle: Hello\ntype: post\n", { status: "publish", title: "Hello", type: "post" }, [], "Body\n"]);
    assert.ok(typeof a.version === "string" && a.version.length > 0);
    const denied = await json(`/api/content/${seg(P)}`, []);
    assert.equal(denied.status, 403);
  });

  test(`${b.name}: a form edit changes the field and keeps the body; the answer names the new version`, async () => {
    const { read, publish } = await open();
    const before = await read();
    const r = await publish({ message: "Retitle", changes: [{ op: "put", path: P, version: before.version, fields: { title: "Bye" } }] });
    assert.equal(r.status, 201);
    const after = await read();
    assert.deepEqual([after.fields.title, after.body], ["Bye", "Body\n"]);
    assert.deepEqual(r.out.files[0], { path: P, version: after.version });
    assert.notEqual(after.version, before.version);
    const whole = await publish({ message: "Whole", changes: [{ op: "put", path: P, version: after.version, text: "---\ntitle: W\ntype: post\n---\nNew body\n" }] });
    assert.equal(whole.status, 201);
    assert.deepEqual([(await read()).fields.title, (await read()).body], ["W", "New body\n"]);
    const denied = await publish({ message: "x", changes: [{ op: "put", path: P, version: after.version, text: "x" }] }, READ);
    assert.equal(denied.status, 403);
  });

  test(`${b.name}: a stale version is a 409 conflict with both texts and their version`, async () => {
    const { read, publish } = await open();
    const old = await read();
    assert.equal((await publish({ message: "first", changes: [{ op: "put", path: P, version: old.version, text: "---\ntitle: Theirs\ntype: post\n---\nBody\n" }] })).status, 201);
    const now = await read();
    const r = await publish({ message: "second", changes: [{ op: "put", path: P, version: old.version, text: "---\ntitle: Mine\ntype: post\n---\nBody\n" }] });
    assert.equal(r.status, 409);
    assert.equal(r.out.error, "conflict");
    assert.deepEqual(r.out.files, [{ path: P, yours: "---\ntitle: Mine\ntype: post\n---\nBody\n", theirs: "---\ntitle: Theirs\ntype: post\n---\nBody\n", theirVersion: now.version }]);
    const form = await publish({ message: "form", changes: [{ op: "put", path: P, version: old.version, fields: { title: "Mine" } }] });
    assert.deepEqual([form.status, form.out.files[0].yours], [409, "---\ntitle: Mine\ntype: post\n---\nBody\n"]);
  });

  test(`${b.name}: a new article gets a free name, is created once, and a second create is a conflict`, async () => {
    const { json, read, publish } = await open();
    const name = await json(`/api/content/new-path?section=${seg(b.section)}&title=${seg("A new one")}`);
    assert.deepEqual([name.status, name.out.path, name.out.slug], [200, b.path("a-new-one"), "a-new-one"]);
    const taken = await json(`/api/content/new-path?section=${seg(b.section)}&title=Hello`);
    assert.equal(taken.out.path, b.path("hello-2"));
    const created = await publish({ message: "New", changes: [{ op: "put", path: name.out.path, version: null, fields: { title: "New", type: "post" }, body: "Text\n" }] });
    assert.equal(created.status, 201);
    const a = await read(name.out.path);
    assert.deepEqual([a.fields, a.body], [{ title: "New", type: "post" }, "Text\n"]);
    const again = await publish({ message: "New", changes: [{ op: "put", path: name.out.path, version: null, text: "x" }] });
    assert.equal(again.status, 409);
    assert.deepEqual(again.out.files, [{ path: name.out.path, yours: "x", theirs: "---\ntitle: New\ntype: post\n---\nText\n", theirVersion: a.version }]);
  });

  test(`${b.name}: delete needs editor-ui:delete and the current version; then the article is gone`, async () => {
    const { read, publish, json } = await open();
    const a = await read();
    const denied = await publish({ message: "rm", changes: [{ op: "delete", path: P, version: a.version }] });
    assert.deepEqual([denied.status, denied.out.permission], [403, PERM.delete]);
    const stale = await publish({ message: "rm", changes: [{ op: "delete", path: P, version: "0123456789abcdef0123456789abcdef01234567" }] }, [...WRITE, PERM.delete]);
    assert.deepEqual([stale.status, stale.out.error, stale.out.files[0].theirVersion], [409, "conflict", a.version]);
    const gone = await publish({ message: "rm", changes: [{ op: "delete", path: P, version: a.version }] }, [...WRITE, PERM.delete]);
    assert.equal(gone.status, 201);
    assert.deepEqual(gone.out.files, [{ path: P, version: null }]);
    assert.equal((await json(`/api/content/${seg(P)}`)).status, 404);
  });

  test(`${b.name}: history newest first; an old revision's text; restore and diff`, async () => {
    const { read, publish, json } = await open();
    const v1 = await read();
    await publish({ message: "two", changes: [{ op: "put", path: P, version: v1.version, text: "---\nstatus: publish\ntitle: Hello\ntype: post\n---\nBody\nmore\n" }] });
    await publish({ message: "three", changes: [{ op: "put", path: P, version: (await read()).version, text: "---\nstatus: publish\ntitle: Hello 3\ntype: post\n---\nBody\nmore\n" }] });
    const list = await json(`/api/revisions/${seg(P)}`);
    assert.equal(list.status, 200);
    assert.deepEqual([list.out.path, list.out.items.length, list.out.nextCursor], [P, 3, null]);
    const oldest = list.out.items[2].commitId;
    const rev = await json(`/api/revisions/${seg(P)}/${oldest}`);
    assert.equal(rev.status, 200);
    const now = await read();
    assert.deepEqual([rev.out.text, rev.out.identical, rev.out.current.version, rev.out.restore.change], [TEXT, false, now.version, { op: "put", path: P, version: now.version, text: TEXT }]);
    const newest = await json(`/api/revisions/${seg(P)}/${list.out.items[0].commitId}`);
    assert.equal(newest.out.identical, true);
    const diff = await json(`/api/revisions/${seg(P)}/diff?from=${oldest}`);
    assert.deepEqual([diff.status, diff.out.added, diff.out.removed, diff.out.identical, diff.out.to.current], [200, 2, 1, false, true]);
    assert.equal(diff.out.download.to, `api/content/${seg(P)}`);
    const between = await json(`/api/revisions/${seg(P)}/diff?from=${oldest}&to=${list.out.items[1].commitId}`);
    assert.deepEqual([between.out.added, between.out.removed, between.out.to.current], [1, 0, false]);
    const restored = await publish({ message: rev.out.restore.message, changes: [rev.out.restore.change] });
    assert.equal(restored.status, 201);
    assert.equal((await read()).fields.title, "Hello");
    const bad = await json(`/api/revisions/${seg(P)}/not-an-id`);
    assert.deepEqual([bad.status, bad.out.error], [400, "invalid_commit"]);
  });
}
