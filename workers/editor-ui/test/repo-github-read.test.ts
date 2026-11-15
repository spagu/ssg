// repo-github.ts read side: branch, head, tree, read, readAt, history — and symlinks never followed.
import { test } from "node:test";
import assert from "node:assert/strict";
import { setup, rejects, ENV } from "./repo-fixtures";
import { nextPage } from "../portable/repo-github";

const text = (b: Uint8Array) => new TextDecoder().decode(b);

test("branch: configured, or the repository's default looked up once", async () => {
  const { provider } = await setup();
  assert.equal(await provider.branch(), "main");
  const d = await setup({}, { ...ENV, EDITOR_GIT_BRANCH: "" });
  d.gh.defaultBranch = "main";
  assert.equal(await d.provider.branch(), "main");
  assert.equal(await d.provider.branch(), "main");
  assert.equal(d.gh.requests.filter((r) => r.url === d.gh.base).length, 1);
  const bad = await setup({}, { ...ENV, EDITOR_GIT_BRANCH: "" });
  bad.gh.defaultBranch = "../x";
  assert.equal((await rejects(bad.provider.branch())).code, "provider_error");
});

test("head and read: bytes from the blob, version id = blob id", async () => {
  const { gh, provider, blob } = await setup();
  const head = await provider.head("main");
  assert.equal(head, gh.store.refs.get("main"));
  const f = await provider.read("main", "content/posts/hello.md");
  assert.equal(text(f.bytes), "# Hello\n");
  assert.equal(f.blobId, blob("content/posts/hello.md"));
  assert.equal(f.path, "content/posts/hello.md");
  const at = await provider.readAt(head, "content/posts/hello.md");
  assert.equal(at.blobId, f.blobId);
  assert.equal((await rejects(provider.head("nope"))).code, "not_found");
  assert.equal((await rejects(provider.head("../x"))).code, "invalid_change");
  assert.equal((await rejects(provider.readAt("main", "content/posts/hello.md"))).code, "invalid_change");
  assert.equal((await rejects(provider.read("main", "content/posts/missing.md"))).code, "not_found");
  assert.equal((await rejects(provider.read("main", "content/nodir/missing.md"))).code, "not_found");
  assert.equal((await rejects(provider.read("main", "content/posts"))).code, "invalid_path"); // a directory
  assert.equal((await rejects(provider.read("main", "content/posts/hello.md/x"))).code, "not_found");
  assert.equal((await rejects(provider.read("main", "content//posts/hello.md"))).code, "invalid_path");
});

test("symlinks, submodules and directories are never read through", async () => {
  const { gh, provider } = await setup();
  await gh.store.commitFiles("main", { "content/posts/evil.md": { symlink: "../../.ssg.yaml" }, "content/posts/sub": { submodule: "c".repeat(40) } });
  for (const p of ["content/posts/evil.md", "content/posts/sub"]) {
    const e = await rejects(provider.read("main", p));
    assert.equal(e.code, "invalid_path", p);
  }
  assert.ok(!gh.requests.some((r) => r.url.includes("/git/blobs/")), "no blob of a symlink fetched");
});

test("read refuses a file above maxFileBytes before downloading it", async () => {
  const { gh, provider } = await setup({ limits: { maxFilesPerChange: 5, maxFileBytes: 4, maxChangeBytes: 10 } });
  assert.equal((await rejects(provider.read("main", "content/posts/hello.md"))).code, "too_large");
  assert.ok(!gh.requests.some((r) => r.url.includes("/git/blobs/")));
});

test("a directory listing at GitHub's 1000-entry cap is re-read through the Git trees endpoint", async () => {
  const capped = await setup(); // the fake cuts listings at 1000 entries, as GitHub does
  const files: Record<string, string> = {};
  for (let i = 0; i < 1000; i++) files[`content/many/f${String(i).padStart(4, "0")}.md`] = `n${i}`;
  files["content/many/zz-last.md"] = "last";
  await capped.gh.store.commitFiles("main", files);
  const f = await capped.provider.read("main", "content/many/zz-last.md");
  assert.equal(text(f.bytes), "last");
  assert.ok(capped.gh.requests.some((r) => /\/git\/trees\/[0-9a-f]{40}$/.test(r.url)), "fell back to git/trees");
  // root with ≥1000 entries: root tree of the commit
  const root: Record<string, string> = {};
  for (let i = 0; i < 1000; i++) root[`r${i}.md`] = `r${i}`;
  root["zz.md"] = "zz";
  await capped.gh.store.commitFiles("main", root);
  assert.equal(text((await capped.provider.read("main", "zz.md")).bytes), "zz");
  assert.equal((await rejects(capped.provider.read("main", "content/many/nope/x.md"))).code, "not_found");
});

test("tree: files and directories under a prefix, symlinks and submodules left out", async () => {
  const { gh, provider, blob } = await setup();
  await gh.store.commitFiles("main", { "content/posts/link.md": { symlink: "hello.md" }, "content/mod": { submodule: "c".repeat(40) } });
  const t = await provider.tree("main", "content");
  const paths = t.entries.map((e) => `${e.type}:${e.path}`).sort();
  assert.deepEqual(paths, ["dir:content/pages", "dir:content/posts", "file:content/pages/about.md", "file:content/posts/hello.md", "file:content/posts/second.md"]);
  assert.equal(t.truncated, false);
  assert.equal(t.entries.find((e) => e.path === "content/posts/hello.md")?.blobId, blob("content/posts/hello.md"));
  assert.equal(t.entries.find((e) => e.path === "content/posts/hello.md")?.size, 8);
  const all = await provider.tree(gh.store.refs.get("main")!, "");
  assert.ok(all.entries.some((e) => e.path === ".ssg.yaml"));
  assert.equal((await rejects(provider.tree("main", "nothing"))).code, "not_found");
  assert.equal((await rejects(provider.tree("main", "../x"))).code, "invalid_path");
});

test("history: newest first, paginated by an opaque page cursor", async () => {
  const { gh, provider } = await setup({ historyPageSize: 2 });
  for (let i = 1; i <= 4; i++) await gh.store.commitFiles("main", { "content/posts/hello.md": `v${i}`, "content/posts/other.md": `o${i}` }, `edit ${i}`);
  const p1 = await provider.history("content/posts/hello.md", null);
  assert.deepEqual(p1.items.map((i) => i.message), ["edit 4", "edit 3"]);
  assert.equal(p1.nextCursor, "2");
  assert.equal(p1.items[0].authorEmail, "seed@example.com");
  const p3 = await provider.history("content/posts/hello.md", "3", "main");
  assert.deepEqual(p3.items.map((i) => i.message), ["seed"]);
  assert.equal(p3.nextCursor, null);
  assert.equal((await rejects(provider.history("content/posts/hello.md", "0"))).code, "invalid_change");
  assert.equal((await rejects(provider.history("content/posts/hello.md", "2&per_page=100"))).code, "invalid_change");
  assert.equal((await rejects(provider.history("/etc/passwd", null))).code, "invalid_path");
});

test("history tolerates odd entries and refuses an unparsable body", async () => {
  const { gh, provider } = await setup();
  gh.override = (m, p) => (p === "/commits" ? new Response(JSON.stringify([{ sha: "x" }, { sha: "a".repeat(40), commit: { message: "m" } }]), { status: 200 }) : undefined);
  const h = await provider.history("content/posts/hello.md", null);
  assert.deepEqual(h.items, [{ commitId: "a".repeat(40), message: "m", authorName: "", authorEmail: "", date: "" }]);
  gh.override = (m, p) => (p === "/commits" ? new Response("{", { status: 200 }) : undefined);
  assert.equal((await rejects(provider.history("content/posts/hello.md", null))).code, "provider_error");
  gh.override = (m, p) => (p === "/commits" ? new Response("{}", { status: 200 }) : undefined);
  assert.deepEqual((await provider.history("content/posts/hello.md", null)).items, []);
});

test("nextPage keeps only the page number of rel=next", () => {
  assert.equal(nextPage(null), null);
  assert.equal(nextPage('<https://api.github.com/x?page=1>; rel="prev"'), null);
  assert.equal(nextPage('<https://api.github.com/x?per_page=2&page=7>; rel="next"'), "7");
  assert.equal(nextPage('<https://api.github.com/x?after=abc>; rel="next"'), null);
  assert.equal(nextPage('<https://evil.example.com/?page=2&x=1>; rel="next"'), "2");
});

test("malformed provider answers become provider_error", async () => {
  const { gh, provider } = await setup();
  gh.override = (m, p) => (p.startsWith("/git/ref/") ? new Response(JSON.stringify({ object: {} }), { status: 200 }) : undefined);
  assert.equal((await rejects(provider.head("main"))).code, "provider_error");
  gh.override = (m, p) => (p.startsWith("/git/commits/") ? new Response(JSON.stringify({ tree: {} }), { status: 200 }) : undefined);
  assert.equal((await rejects(provider.tree("main", ""))).code, "provider_error");
  gh.override = (m, p) => (p.startsWith("/git/trees/") ? new Response(JSON.stringify({ tree: "x" }), { status: 200 }) : undefined);
  assert.deepEqual(await provider.tree("main", ""), { entries: [], truncated: false });
  gh.override = (m, p) => (p.startsWith("/git/trees/") ? new Response(JSON.stringify({ tree: [{ path: 1 }, { path: "a", mode: "100644", sha: "b".repeat(40) }], truncated: true }), { status: 200 }) : undefined);
  assert.deepEqual(await provider.tree("main", ""), { entries: [{ path: "a", type: "file", blobId: "b".repeat(40) }], truncated: true });
  gh.override = (m, p) => (p === "/contents/content/posts" ? new Response(JSON.stringify([{ name: 1 }, { name: "hello.md", sha: "c".repeat(40), type: "file" }]), { status: 200 }) : undefined);
  gh.override = ((orig) => (m: string, p: string) => (p.startsWith("/git/blobs/") ? new Response("raw", { status: 200 }) : orig(m, p)))(gh.override);
  const f = await provider.read("main", "content/posts/hello.md");
  assert.equal(text(f.bytes), "raw");
});

test("a capped listing whose directory vanished from its parent's listing reads as absent", async () => {
  const { gh, provider } = await setup();
  const full = Array.from({ length: 1000 }, (_, i) => ({ name: `f${i}.md`, sha: "a".repeat(40), type: "file" }));
  gh.override = (_m, p) => (p === "/contents/content/posts" ? new Response(JSON.stringify(full), { status: 200 })
    : p === "/contents/content" ? new Response("[]", { status: 200 }) : undefined);
  assert.equal((await rejects(provider.read("main", "content/posts/hello.md"))).code, "not_found");
});
