// GET /api/content, /api/content/{path}, /api/content/new-path through the
// whole pipeline, against the fake GitHub: what is listed and how, the lazy
// titles within the subrequest budget, filters and pages, the path policy.
import { test } from "node:test";
import assert from "node:assert/strict";
import { BASE, makeEnv, request, run, site } from "./fixtures.ts";
import { PERM } from "../src/permissions.ts";
import { MAX_LISTINGS, clearListingCache } from "../src/content-list.ts";
import { clearMetaCache } from "../src/content-meta.ts";

const READ = [PERM.read];
const get = async (path: string, gh: Awaited<ReturnType<typeof site>>, env = makeEnv()) => run(await request(BASE + path, READ), env, gh);
const enc = encodeURIComponent;
async function fresh(files: Record<string, string | null> = {}) {
  clearListingCache();
  clearMetaCache();
  return site(files);
}

const POST = "---\ntitle: First post\ndate: 2026-07-14\nstatus: publish\nlang: pl\n---\nBody\n";

test("the list: every .md file of the content folders, sorted, with section, type, language, title, date, draft", async () => {
  const gh = await fresh({
    "content/blog/posts/first.md": POST,
    "content/blog/posts/draft.md": "---\ntitle: Not yet\n---\n",
    "content/blog/posts/image.png": "png",
    "content/blog/posts/.hidden.md": "x",
    "content/blog/notes/loose.md": "+++\ntitle = \"t\"\n+++\n# Loose\n",
    "content/blog/posts/open.md": "---\ntitle: open\n",
    "elsewhere/x.md": "# not content\n",
  });
  const res = await get("/api/content", gh);
  assert.equal(res.status, 200);
  const page = await res.json();
  assert.deepEqual(page.items.map((i: { path: string }) => i.path), [
    "content/blog/notes/loose.md", "content/blog/pages/about.md", "content/blog/posts/draft.md", "content/blog/posts/first.md",
    "content/blog/posts/hello.md", "content/blog/posts/open.md", "docs/guide.md",
  ]);
  const by = Object.fromEntries(page.items.map((i: { path: string }) => [i.path, i]));
  assert.deepEqual(by["content/blog/posts/first.md"], {
    path: "content/blog/posts/first.md", version: by["content/blog/posts/first.md"].version, size: POST.length,
    section: "content/blog/posts", type: "post", language: "pl", title: "First post", date: "2026-07-14", draft: false, loaded: true,
  });
  assert.match(by["content/blog/posts/first.md"].version, /^[0-9a-f]{40}$/);
  assert.deepEqual([by["content/blog/posts/hello.md"].title, by["content/blog/posts/hello.md"].draft, by["content/blog/posts/hello.md"].language], ["Hello", false, "en"]);
  assert.deepEqual([by["content/blog/posts/draft.md"].draft, by["content/blog/posts/draft.md"].date], [true, null]);
  assert.deepEqual([by["content/blog/notes/loose.md"].section, by["content/blog/notes/loose.md"].type, by["content/blog/notes/loose.md"].title], ["content/blog", null, "Loose"]);
  assert.deepEqual([by["content/blog/posts/open.md"].draft, by["content/blog/posts/open.md"].title], [null, null]);
  assert.deepEqual([by["docs/guide.md"].section, by["docs/guide.md"].type], ["docs", "page"]);
  assert.deepEqual([page.nextCursor, page.pending, page.truncated], [null, 0, false]);
});

test("titles are read lazily within the subrequest budget, then come from the cache by version", async () => {
  const many: Record<string, string> = {};
  for (let i = 0; i < 40; i++) many[`content/blog/posts/p${String(i).padStart(2, "0")}.md`] = `---\ntitle: Post ${i}\n---\n`;
  const gh = await fresh(many);
  const first = await (await get("/api/content?limit=100", gh)).json();
  assert.ok(first.pending > 0 && first.pending < first.items.length, `pending ${first.pending}`);
  const unread = first.items.find((i: { loaded: boolean }) => !i.loaded);
  assert.deepEqual([unread.title, unread.date, unread.draft, unread.language], [null, null, null, "en"]);
  let pending = first.pending;
  for (let round = 0; round < 5 && pending > 0; round++) pending = (await (await get("/api/content?limit=100", gh)).json()).pending;
  assert.equal(pending, 0);
  const before = gh.requests.length;
  const again = await (await get("/api/content?limit=100", gh)).json();
  assert.equal(gh.requests.length - before, 1, "only the head: listings cached per head, titles per version");
  assert.equal(again.items.find((i: { path: string }) => i.path === "content/blog/posts/p07.md").title, "Post 7");
});

test("filters and pages: q, section, cursor, limit; refusals for a bad cursor or a folder outside", async () => {
  const gh = await fresh({ "content/blog/posts/Alpha.md": "a", "content/blog/posts/beta.md": "b" });
  const q = await (await get("/api/content?q=ALPH", gh)).json();
  assert.deepEqual(q.items.map((i: { path: string }) => i.path), ["content/blog/posts/Alpha.md"]);
  const docs = await (await get(`/api/content?section=${enc("docs/")}`, gh)).json();
  assert.deepEqual(docs.items.map((i: { path: string }) => i.path), ["docs/guide.md"]);
  const seen: string[] = [];
  let cursor: string | null = null;
  do {
    const p: { items: { path: string }[]; nextCursor: string | null } = await (await get(`/api/content?limit=2${cursor ? `&cursor=${cursor}` : ""}`, gh)).json();
    assert.ok(p.items.length <= 2);
    seen.push(...p.items.map((i) => i.path));
    cursor = p.nextCursor;
  } while (cursor);
  assert.equal(seen.length, 5);
  assert.equal(new Set(seen).size, 5);
  assert.equal((await (await get("/api/content?limit=0", gh)).json()).items.length, 1, "clamped to 1");
  assert.equal((await (await get("/api/content?limit=x", gh)).json()).items.length, 5, "not a number: the default");
  const bad = await get("/api/content?cursor=%25%25", gh);
  assert.deepEqual([bad.status, (await bad.json()).error], [400, "bad_cursor"]);
  for (const section of [".github", "workers", "elsewhere", "content"]) {
    const r = await get(`/api/content?section=${enc(section)}`, gh);
    assert.equal(r.status, 400, section);
    assert.equal((await r.json()).error, "invalid_path");
  }
});

test("a content folder missing on the branch lists nothing; a truncated listing says so; host errors pass", async () => {
  const gh = await fresh();
  const env = makeEnv({ EDITOR_CONTENT_DIRS: "docs, nowhere" });
  const page = await (await get("/api/content", gh, env)).json();
  assert.deepEqual(page.items.map((i: { path: string }) => i.path), ["docs/guide.md"]);
  clearListingCache();
  gh.override = (method, path) => {
    if (method !== "GET" || !path.startsWith("/git/trees/")) return undefined;
    return Response.json({ sha: "x", tree: [{ path: "guide.md", mode: "100644", type: "blob", sha: "a".repeat(40) }], truncated: true });
  };
  for (let i = 0; i <= MAX_LISTINGS; i++) assert.equal((await get(`/api/content?section=docs%2Fx${i}`, gh)).status, 200);
  const cut = await (await get("/api/content?section=docs", gh)).json();
  assert.deepEqual([cut.truncated, cut.items[0].size], [true, null]);
  clearListingCache();
  gh.override = (method, path) => (method === "GET" && path.startsWith("/git/trees/") ? Response.json({ message: "boom" }, { status: 500 }) : undefined);
  assert.equal((await get("/api/content", gh)).status, 502);
});

test("read: the article split for the form, its version and the commit it was read at", async () => {
  const gh = await fresh({ "content/blog/posts/first.md": POST, "content/blog/posts/bom.md": "﻿---\ntitle: B\n---\nx" });
  const res = await get("/api/content/" + enc("content/blog/posts/first.md"), gh);
  assert.equal(res.status, 200);
  const a = await res.json();
  assert.deepEqual({ ...a, version: "", commit: "" }, {
    path: "content/blog/posts/first.md", version: "", commit: "", format: "yaml", bom: false,
    frontmatterRaw: "title: First post\ndate: 2026-07-14\nstatus: publish\nlang: pl\n",
    fields: { title: "First post", date: "2026-07-14", status: "publish", lang: "pl" }, fixed: [], body: "Body\n",
  });
  assert.equal(a.commit, gh.store.refs.get("main"));
  const bom = await (await get("/api/content/" + enc("content/blog/posts/bom.md"), gh)).json();
  assert.deepEqual([bom.bom, bom.fields, bom.body], [true, { title: "B" }, "x"]);
});

test("read: refusals — outside the content folders, not Markdown, an image, missing, not UTF-8", async () => {
  const gh = await fresh();
  const reasons: Record<string, string> = {
    ".ssg.yaml": "dot_segment", ".github/workflows/x.yml": "denied", "workers/auth/x.md": "denied", "elsewhere/x.md": "outside_roots",
    "content/blog/posts/a.txt": "extension", "content/blog/media/a.png": "media", "content/../x.md": "traversal",
  };
  for (const [path, reason] of Object.entries(reasons)) {
    const r = await get("/api/content/" + enc(path), gh);
    assert.equal(r.status, 400, path);
    const body = await r.json();
    assert.deepEqual({ ...body, detail: typeof body.detail }, { error: "invalid_path", detail: "string", path, reason }, path);
  }
  assert.equal((await get("/api/content/" + enc("content/blog/posts/none.md"), gh)).status, 404);
  const blob = await gh.store.putBlob(new Uint8Array([0x2d, 0x2d, 0x2d, 0x0a, 0xff, 0x0a]));
  const head = gh.store.refs.get("main")!;
  const flat = new Map(gh.store.flatAt(head));
  flat.set("content/blog/posts/latin1.md", { mode: "100644", sha: blob });
  gh.store.refs.set("main", await gh.store.putCommit(await gh.store.putTree(flat), [head], "binary"));
  const bin = await get("/api/content/" + enc("content/blog/posts/latin1.md"), gh);
  assert.deepEqual([bin.status, (await bin.json()).error], [422, "not_text"]);
});

test("new-path: the title's slug in the section, the first free name, refusals", async () => {
  const gh = await fresh({ "content/blog/posts/hello-world.md": "x" });
  const np = async (q: string) => get("/api/content/new-path?" + q, gh);
  assert.deepEqual(await (await np("section=content%2Fblog%2Fposts&title=Hello%2C%20World!")).json(), { path: "content/blog/posts/hello-world-2.md", slug: "hello-world" });
  assert.deepEqual(await (await np("section=content%2Fblog%2Fposts&title=Za%C5%BC%C3%B3%C5%82%C4%87")).json(), { path: "content/blog/posts/za.md", slug: "za" });
  assert.deepEqual(await (await np("section=content%2Fblog%2Fnew")).json(), { path: "content/blog/new/untitled.md", slug: "untitled" });
  const none = await np("title=x");
  assert.deepEqual([none.status, (await none.json()).error], [400, "invalid_input"]);
  assert.equal((await np("section=.github&title=x")).status, 400);
});
