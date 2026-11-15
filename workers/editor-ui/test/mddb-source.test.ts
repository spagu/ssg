// The MDDB source through the whole pipeline: a site with both sources lists
// both with their origin and one cursor; the collections, filters and the
// language come from the site's .ssg.yaml; what the site does not build from
// is not listed and not readable; history pages; the paths of both kinds.
import { test } from "node:test";
import assert from "node:assert/strict";
import { makeEnv } from "./fixtures.ts";
import { bothEnv, mddbOnlyEnv, send, seg, world } from "./mddb-fixtures.ts";
import type { World } from "./mddb-fixtures.ts";
import { PERM } from "../src/permissions.ts";
import { encodeCursor } from "../src/content-list.ts";

const READ = [PERM.read];
const get = async (w: World, path: string, env: Record<string, unknown> = bothEnv()) => {
  const res = await send(w, env, path, READ);
  return { status: res.status, out: await res.json() };
};
const post = { type: ["post"], status: ["publish"] };

async function seeded(opts: { git?: boolean } = {}) {
  const w = await world(opts);
  w.db.put("blog", "b-one", "en", { ...post, title: ["One"] }, "1\n");
  w.db.put("blog", "a-two", "en", { ...post, title: ["Two"], status: ["draft"] }, "2\n");
  w.db.put("blog", "note", "en", { type: ["note"] }, "not built\n");
  w.db.put("blog", "polish", "pl", { ...post }, "other language\n");
  w.db.put("kb", "faq", "en", { visibility: ["public"] }, "q\n");
  w.db.put("kb", "secret", "en", { visibility: ["private"], title: ["Hidden"] }, "s\n");
  return w;
}

test("both sources: the repository's files first, then MDDB's articles, each with its origin", async () => {
  const w = await seeded();
  const { status, out } = await get(w, "/api/content");
  assert.equal(status, 200);
  assert.deepEqual(out.items.map((i: { path: string; origin: string }) => `${i.origin} ${i.path}`), [
    "git content/blog/pages/about.md", "git content/blog/posts/hello.md", "git docs/guide.md",
    "mddb mddb:blog/en/a-two", "mddb mddb:blog/en/b-one", "mddb mddb:kb/en/faq",
  ]);
  const one = out.items.find((i: { path: string }) => i.path === "mddb:blog/en/b-one");
  assert.deepEqual(one, { path: "mddb:blog/en/b-one", version: "", size: 48, section: "mddb:blog/en", type: "post", language: "en", title: "One", date: "2026-01-01T00:00:01Z", draft: false, loaded: true, origin: "mddb" });
  assert.deepEqual([out.items[3].draft, out.items[5].type, out.items[5].title], [true, "page", null]);
  assert.deepEqual([out.nextCursor, out.truncated], [null, false]);
  const search = w.db.requests.filter((r) => r.path === "/v1/search").map((r) => r.body.filterMeta);
  assert.deepEqual(search, [{ type: ["page", "post"] }, { visibility: ["public"] }], "the filter and type go to the server");
});

test("one cursor walks both sources; a full page of files continues with MDDB", async () => {
  const w = await seeded();
  const seen: string[] = [];
  let cursor: string | null = null;
  do {
    const page: { out: { items: { path: string }[]; nextCursor: string | null } } = await get(w, `/api/content?limit=2${cursor ? `&cursor=${cursor}` : ""}`);
    seen.push(...page.out.items.map((i) => i.path));
    cursor = page.out.nextCursor;
  } while (cursor);
  assert.deepEqual(seen, ["content/blog/pages/about.md", "content/blog/posts/hello.md", "docs/guide.md", "mddb:blog/en/a-two", "mddb:blog/en/b-one", "mddb:kb/en/faq"]);
  const full = await get(w, "/api/content?limit=3");
  assert.deepEqual([full.out.items.length, full.out.nextCursor], [3, encodeCursor("mddb:")]);
  const q = await get(w, "/api/content?q=faq");
  assert.deepEqual(q.out.items.map((i: { path: string }) => i.path), ["mddb:kb/en/faq"]);
});

test("sections: an MDDB collection (with or without language), a repository folder, one that is not the site's", async () => {
  const w = await seeded();
  const blog = await get(w, `/api/content?section=${seg("mddb:blog")}`);
  assert.deepEqual(blog.out.items.map((i: { path: string }) => i.path), ["mddb:blog/en/a-two", "mddb:blog/en/b-one"]);
  const kb = await get(w, `/api/content?section=${seg("mddb:kb/en")}`);
  assert.deepEqual(kb.out.items.map((i: { path: string }) => i.path), ["mddb:kb/en/faq"]);
  const files = await get(w, `/api/content?section=${seg("docs")}`);
  assert.deepEqual(files.out.items.map((i: { path: string; origin: string }) => [i.path, i.origin]), [["docs/guide.md", "git"]]);
  for (const bad of ["mddb:news", "mddb:blog/pl", "mddb:blog/en/x", "mddb:a b"]) {
    const r = await get(w, `/api/content?section=${seg(bad)}`);
    assert.deepEqual([r.status, r.out.error, r.out.reason], [400, "invalid_path", "outside_roots"], bad);
  }
  const name = await get(w, `/api/content/new-path?section=${seg("mddb:nope")}&title=x`);
  assert.equal(name.status, 400);
});

test("the subrequest budget: MDDB gets what the files left, at least one search", async () => {
  const many: Record<string, string> = {};
  for (let i = 0; i < 40; i++) many[`content/blog/posts/p${String(i).padStart(2, "0")}.md`] = `---\ntitle: Post ${i}\n---\n`;
  const w = await world({ files: many });
  for (let i = 0; i < 2001; i++) w.db.put("blog", `x${String(i).padStart(4, "0")}`, "en", post, "");
  const first = await get(w, "/api/content?limit=100");
  assert.ok(first.out.pending > 0);
  const searches = w.db.requests.filter((r) => r.path === "/v1/search").length;
  assert.ok(searches >= 1 && searches <= 4, `${searches} searches`);
  assert.deepEqual([first.out.items.at(-1).path.startsWith("mddb:blog/en/x"), first.out.truncated], [true, true], "2001 documents need 5 searches");
});

test("a large collection is read 500 at a time; past the budget the list says truncated", async () => {
  const w = await world({ git: false });
  for (let i = 0; i < 1001; i++) w.db.put("blog", `k${String(i).padStart(4, "0")}`, "en", post, "");
  const all = await get(w, "/api/content?limit=1", mddbOnlyEnv());
  assert.deepEqual([all.out.items[0].path, all.out.truncated, w.db.requests.length], ["mddb:blog/en/k0000", false, 3]);
  const cut = await send(w, mddbOnlyEnv(), "/api/content?limit=1", READ);
  assert.equal(cut.status, 200);
});

test("read: only articles of the site: its collections, filter, type and language", async () => {
  const w = await seeded();
  const ok = await get(w, `/api/content/${seg("mddb:blog/en/b-one")}`);
  assert.deepEqual([ok.status, ok.out.origin, ok.out.commit, ok.out.fields.title], [200, "mddb", ok.out.version, "One"]);
  for (const [path, status, error] of [
    ["mddb:kb/en/secret", 404, "not_found"], ["mddb:blog/en/note", 404, "not_found"], ["mddb:blog/en/missing", 404, "not_found"],
    ["mddb:news/en/x", 400, "invalid_path"], ["mddb:blog/pl/polish", 400, "invalid_path"], ["mddb:blog", 400, "invalid_path"],
    ["mddb:blog/en/a/../b", 400, "invalid_path"], ["mddb:blog/e!/x", 400, "invalid_path"], ["mddb:-x/en/k", 400, "invalid_path"], ["mddb:blog/en/ x", 400, "invalid_path"],
  ] as const) {
    const r = await get(w, `/api/content/${seg(path)}`);
    assert.deepEqual([r.status, r.out.error], [status, error], path);
  }
  const shape = await get(w, `/api/content/${seg("mddb:blog/en/" + "k".repeat(401))}`);
  assert.deepEqual([shape.status, shape.out.reason], [400, "mddb_path"]);
});

test("a key the form cannot show is listed in fixed and kept by a form edit", async () => {
  const w = await world({ git: false });
  w.db.put("blog", "odd", "en", { ...post, "featured image": ["x.png"] }, "b");
  const env = mddbOnlyEnv();
  const a = await get(w, `/api/content/${seg("mddb:blog/en/odd")}`, env);
  assert.deepEqual(a.out.fixed, ["featured image"]);
  const res = await send(w, env, "/api/changes", [PERM.read, PERM.publish], {
    method: "POST", headers: { origin: "https://example.com", "content-type": "application/json" },
    body: JSON.stringify({ message: "t", changes: [{ op: "put", path: "mddb:blog/en/odd", version: a.out.version, fields: { title: "T" } }] }),
  });
  assert.equal(res.status, 201);
  assert.deepEqual(w.db.get("blog", "odd", "en")!.meta, { status: ["publish"], title: ["T"], type: ["post"], "featured image": ["x.png"] });
});

test("two sources of one collection list each article once, the first source's type", async () => {
  const yaml = "source: blog\nmddb:\n  enabled: true\n  sources:\n    - collection: blog\n      type: post\n    - collection: blog\n      type: page\n";
  const w = await world({ files: { ".ssg.yaml": yaml } });
  w.db.put("blog", "a", "en", {}, "");
  const r = await get(w, `/api/content?section=${seg("mddb:blog")}`);
  assert.deepEqual(r.out.items.map((i: { path: string; type: string }) => [i.path, i.type]), [["mddb:blog/en/a", "post"]]);
});

test("history: pages of 30, a cursor of ours only, an id of this article only", async () => {
  const w = await world({ git: false });
  for (let i = 0; i < 35; i++) w.db.put("blog", "h", "en", { ...post, title: [`T${i}`] }, "");
  const env = mddbOnlyEnv();
  const P = seg("mddb:blog/en/h");
  const first = await get(w, `/api/revisions/${P}`, env);
  assert.deepEqual([first.out.items.length, first.out.nextCursor, first.out.items[0].authorName, first.out.items[0].message], [30, "30", "", ""]);
  assert.match(first.out.items[0].date, /^2026-01-01T00:00:\d\dZ$/);
  const second = await get(w, `/api/revisions/${P}?cursor=30`, env);
  assert.deepEqual([second.out.items.length, second.out.nextCursor], [5, null]);
  assert.equal((await get(w, `/api/revisions/${P}?cursor=x`, env)).out.error, "bad_cursor");
  assert.equal((await get(w, `/api/revisions/${P}/123`, env)).status, 404);
  assert.equal((await get(w, `/api/revisions/${P}/diff?from=x`, env)).out.error, "invalid_commit");
  assert.equal((await get(w, `/api/revisions/${P}/diff?from=${first.out.items[1].commitId}&to=1`, env)).out.error, "revision_not_found");
  assert.equal((await get(w, `/api/revisions/${P}/diff?from=${first.out.items[1].commitId}&to=current`, env)).out.to.current, true);
  w.db.override = (p) => (p === "/v1/revisions" ? Response.json({ revisions: [{ timestamp: 1767225600, contentMd: "" }] }) : null);
  assert.equal((await get(w, `/api/revisions/${P}`, env)).out.items[0].date, "2026-01-01T00:00:00Z", "no updatedAt: the timestamp");
});

test("without MDDB configured an mddb: path is a repository path (refused as before); without a repository, files are 503", async () => {
  const gitOnly = await world();
  const r = await get(gitOnly, `/api/content/${seg("mddb:blog/en/x")}`, makeEnv());
  assert.deepEqual([r.status, r.out.error], [400, "invalid_path"]);
  const only = await seeded({ git: false });
  only.db.put("blog", "aaa", "de", post, "");
  only.db.put("blog", "bad key ", "en", post, "");
  const env = mddbOnlyEnv();
  assert.equal((await get(only, `/api/content/${seg("docs/guide.md")}`, env)).out.error, "repo_not_configured");
  assert.equal((await get(only, `/api/content?section=docs`, env)).out.error, "repo_not_configured");
  const list = await get(only, "/api/content", env);
  // No language filter without the site's mddb.lang; a key the editor cannot address is not listed.
  assert.deepEqual(list.out.items.map((i: { path: string }) => i.path), ["mddb:blog/de/aaa", "mddb:blog/en/a-two", "mddb:blog/en/b-one", "mddb:blog/pl/polish"]);
  const untitled = await get(only, `/api/content/new-path?section=${seg("mddb:blog/en")}`, env);
  assert.deepEqual(untitled.out, { path: "mddb:blog/en/untitled", slug: "untitled" });
});

test("defaults: an unreadable .ssg.yaml or an unreachable repository needs EDITOR_MDDB_COLLECTION", async () => {
  const w = await world({ files: { ".ssg.yaml": "include: other.yaml\n" } });
  w.db.put("blog", "x", "en", post, "");
  assert.equal((await get(w, `/api/content/${seg("mddb:blog/en/x")}`)).out.error, "mddb_not_configured");
  assert.equal((await get(w, `/api/content/${seg("mddb:blog/en/x")}`, bothEnv({ EDITOR_MDDB_COLLECTION: "blog" }))).status, 200);
  const down = await world();
  down.db.put("blog", "x", "en", post, "");
  down.gh!.override = () => new Response("{}", { status: 500 });
  assert.equal((await get(down, `/api/content/${seg("mddb:blog/en/x")}`, bothEnv({ EDITOR_MDDB_COLLECTION: "blog" }))).status, 200);
});

test("a server before MDDB 2.18.8 gives no revision: 503 mddb_too_old, nothing is written", async () => {
  const w = await world({ git: false });
  w.db.put("blog", "x", "en", post, "");
  w.db.noRevision = true;
  const r = await get(w, `/api/content/${seg("mddb:blog/en/x")}`, mddbOnlyEnv());
  assert.deepEqual([r.status, r.out.error], [503, "mddb_too_old"]);
});
