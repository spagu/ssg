// Decision 11 of the round, tested in pairs: every operation called by its
// REST route and by its MCP tool gives the same answer, the same effect on
// the repository, and the same refusal ({…the route's error body, status}).
import { test } from "node:test";
import assert from "node:assert/strict";
import { site } from "./fixtures.ts";
import type { FakeGitHub } from "./fake-github.ts";
import { b64, jpeg, png } from "./media-fixtures.ts";
import { ALL, published, refusalOf, rest, tool, tree } from "./mcp-fixtures.ts";
import { PERM } from "../src/permissions.ts";
import { sha256Hex } from "../src/media-name.ts";

const P = "content/blog/posts/hello.md";
const SEG = encodeURIComponent(P);
const READ: string[] = [PERM.read];
const v = (gh: FakeGitHub, path: string) => gh.store.blobIdAt("main", path);

// same: the REST body equals the tool's structured content.
async function same(gh: FakeGitHub, path: string, name: string, args: Record<string, unknown>, perms = READ) {
  const r = await rest(gh, perms, path);
  const t = await tool(gh, perms, name, args);
  assert.equal(t.isError, r.status >= 400, name);
  assert.deepEqual(t.structuredContent, r.status >= 400 ? refusalOf(r) : r.body, name);
  assert.deepEqual(JSON.parse(t.content[0].text), t.structuredContent, "the text block is the same JSON");
  return r;
}

test("reading tools answer as their routes, and refuse as they do", async () => {
  const gh = await site();
  assert.equal((await same(gh, "/api/content?limit=1", "content_list", { limit: 1 })).status, 200);
  assert.equal((await same(gh, "/api/content?q=hello&section=content%2Fblog%2Fposts", "content_list", { q: "hello", section: "content/blog/posts" })).status, 200);
  assert.equal((await same(gh, "/api/content?section=..%2Fx", "content_list", { section: "../x" })).status, 400);
  assert.equal((await same(gh, `/api/content/${SEG}`, "content_read", { path: P })).status, 200);
  assert.equal((await same(gh, "/api/content/" + encodeURIComponent("../etc.md"), "content_read", { path: "../etc.md" })).status, 400);
  assert.equal((await same(gh, "/api/media?limit=5", "media_list", { limit: "5" })).status, 200);
  assert.equal((await same(gh, "/api/media?cursor=%25%25", "media_list", { cursor: "%%" })).status, 400);
  assert.equal((await same(gh, `/api/revisions/${SEG}`, "revisions_list", { path: P })).status, 200);
  const head = gh.store.refs.get("main")!;
  assert.equal((await same(gh, `/api/revisions/${SEG}/${head}`, "revision_read", { path: P, commit: head })).status, 200);
  assert.equal((await same(gh, `/api/revisions/${SEG}/nothex`, "revision_read", { path: P, commit: "nothex" })).status, 400);
});

// twin: the same change published by REST on one site and by MCP on its twin.
async function twin(body: { message: string; changes: unknown[] }, name: string, args: Record<string, unknown>, perms = ALL, seed: Record<string, string> = {}) {
  const [a, b] = [await site(seed), await site(seed)];
  const r = await rest(a, perms, "/api/changes", body);
  const t = await tool(b, perms, name, args);
  return { a, b, r, t };
}

test("content_create = a put with version null; content_update = a put with the version; both one commit", async () => {
  const created = await twin({ message: "New", changes: [{ op: "put", path: "docs/n.md", version: null, text: "# N\n" }, { op: "put", path: "docs/m.md", version: null, text: "# M\n" }] },
    "content_create", { message: "New", changes: [{ path: "docs/n.md", content: "# N\n" }, { path: "docs/m.md", content: "# M\n" }] });
  assert.equal(created.r.status, 201);
  assert.equal(created.t.isError, false);
  assert.deepEqual(published(created.t.structuredContent), published(created.r.body));
  assert.deepEqual(tree(created.b), tree(created.a));

  const gh = await site();
  const update = await twin({ message: "Edit", changes: [{ op: "put", path: P, version: v(gh, P), text: "# Hi\n" }] }, "content_update", { message: "Edit", path: P, version: v(gh, P), content: "# Hi\n" });
  assert.equal(update.r.status, 201);
  assert.deepEqual(tree(update.b), tree(update.a));
  const moved = await twin({ message: "Move", changes: [{ op: "move", from: P, to: "content/blog/posts/hi.md", version: v(gh, P) }] }, "content_update", { message: "Move", path: P, to: "content/blog/posts/hi.md", version: v(gh, P) });
  assert.equal(moved.r.status, 201);
  assert.deepEqual(tree(moved.b), tree(moved.a));
  const movedText = await twin({ message: "Move", changes: [{ op: "move", from: P, to: "docs/hi.md", version: v(gh, P), text: "x" }] }, "content_update", { message: "Move", path: P, to: "docs/hi.md", version: v(gh, P), content: "x" });
  assert.deepEqual(tree(movedText.b), tree(movedText.a));
});

test("content_create from section + title: the path content_new_path names, as the panel publishes it", async () => {
  const [a, b] = [await site(), await site()];
  const named = await rest(a, ALL, "/api/content/new-path?section=content%2Fblog%2Fposts&title=Hello");
  assert.equal(named.body.path, "content/blog/posts/hello-2.md");
  await rest(a, ALL, "/api/changes", { message: "New", changes: [{ op: "put", path: named.body.path, version: null, text: "# Hello\n" }] });
  const t = await tool(b, ALL, "content_create", { message: "New", section: "content/blog/posts", title: "Hello", content: "# Hello\n" });
  assert.deepEqual((t.structuredContent.files as { path: string }[]).map((f) => f.path), ["content/blog/posts/hello-2.md"]);
  assert.deepEqual(tree(b), tree(a));
  const noSection = await tool(b, ALL, "content_create", { message: "New", title: "Hello", content: "x" });
  assert.deepEqual(noSection.structuredContent, refusalOf(await rest(b, ALL, "/api/content/new-path?title=Hello")));
});

test("write refusals are the route's: permission, conflict, message, path", async () => {
  const gh = await site();
  const pairs: [unknown, string, Record<string, unknown>, string[]][] = [
    [{ message: "x", changes: [{ op: "put", path: "docs/n.md", version: null, text: "n" }] }, "content_create", { message: "x", path: "docs/n.md", content: "n" }, READ],
    [{ message: "x", changes: [{ op: "put", path: P, version: "0".repeat(40), text: "n" }] }, "content_update", { message: "x", path: P, version: "0".repeat(40), content: "n" }, ALL],
    [{ message: "x", changes: [{ op: "delete", path: P, version: v(gh, P) }] }, "content_delete", { message: "x", path: P, version: v(gh, P) }, [PERM.read, PERM.publish]],
    [{ changes: [{ op: "delete", path: P, version: v(gh, P) }] }, "content_delete", { path: P, version: v(gh, P) }, ALL],
    [{ message: "x", changes: [{ op: "put", path: "static/x.md", version: null, text: "n" }] }, "content_create", { message: "x", path: "static/x.md", content: "n" }, ALL],
    [{ message: "x", changes: [{ op: "put", path: P, version: v(gh, P) }] }, "content_update", { message: "x", path: P, version: v(gh, P) }, ALL],
  ];
  for (const [body, name, args, perms] of pairs) {
    const { a, b, r, t } = await twin(body as never, name, args, perms);
    assert.ok(r.status >= 400, name);
    assert.equal(t.isError, true, name);
    assert.deepEqual(t.structuredContent, refusalOf(r), name);
    assert.deepEqual(tree(b), tree(a), "nothing written on either side");
  }
});

test("content_delete and media_delete remove what the route removes", async () => {
  const gh = await site({ "content/blog/media/2026/01/a-1a2b3c4d.png": "x" });
  const img = "content/blog/media/2026/01/a-1a2b3c4d.png";
  const seed = { [img]: "x" };
  const del = await twin({ message: "Del", changes: [{ op: "delete", path: P, version: v(gh, P) }] }, "content_delete", { message: "Del", changes: [{ path: P, version: v(gh, P) }] }, ALL, seed);
  assert.equal(del.r.status, 201);
  assert.deepEqual(tree(del.b), tree(del.a));
  const media = await twin({ message: "Del", changes: [{ op: "delete", path: img, version: v(gh, img) }] }, "media_delete", { message: "Del", path: img, version: v(gh, img) }, ALL, seed);
  assert.equal(media.r.status, 201);
  assert.equal(v(media.b, img), null);
  assert.deepEqual(tree(media.b), tree(media.a));
});

test("media_upload names the bytes as media/new-path does and publishes them as /api/changes does", async () => {
  const [a, b] = [await site(), await site()];
  const bytes = [png("first"), jpeg("second")];
  const paths: string[] = [];
  for (const [i, x] of bytes.entries()) {
    const type = i === 0 ? "png" : "jpg";
    paths.push((await rest(a, ALL, `/api/media/new-path?name=Cat.png&type=${type}&hash=${await sha256Hex(x)}`)).body.path);
  }
  const r = await rest(a, ALL, "/api/changes", { message: "Cats", changes: bytes.map((x, i) => ({ op: "put", path: paths[i], version: null, base64: b64(x) })) });
  assert.equal(r.status, 201);
  const t = await tool(b, ALL, "media_upload", { message: "Cats", changes: bytes.map((x) => ({ name: "Cat.png", content_base64: b64(x) })) });
  assert.deepEqual(published(t.structuredContent), published(r.body));
  assert.deepEqual(tree(b), tree(a));
  const again = await tool(b, ALL, "media_upload", { message: "Again", path: paths[0], content_base64: b64(bytes[0]) });
  assert.equal(again.isError, false, "a canonical path may be given");
});

test("media_upload refusals: no editor-ui:media, markup, not base64 - each as the route answers", async () => {
  const gh = await site();
  const writer = [PERM.read, PERM.publish];
  const svg = new TextEncoder().encode("<svg onload=alert(1)>");
  const noMedia = await tool(gh, writer, "media_upload", { message: "x", content_base64: b64(png("x")) });
  assert.deepEqual(noMedia.structuredContent, { error: "forbidden", permission: PERM.media, status: 403 });
  const path = (await rest(gh, ALL, `/api/media/new-path?name=image&type=png&hash=${await sha256Hex(svg)}`)).body.path;
  const markup = await tool(gh, ALL, "media_upload", { message: "x", content_base64: b64(svg) });
  assert.deepEqual(markup.structuredContent, refusalOf(await rest(gh, ALL, "/api/changes", { message: "x", changes: [{ op: "put", path, version: null, base64: b64(svg) }] })));
  const notB64 = await tool(gh, ALL, "media_upload", { message: "x", content_base64: "%%%" });
  assert.deepEqual(notB64.structuredContent, refusalOf(await rest(gh, ALL, "/api/changes", { message: "x", changes: [{ op: "put", path, version: null, base64: "%%%" }] })));
});
