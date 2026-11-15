// GO-168 acceptance: one publication = one commit, force never true, races resolved without overwriting.
import { test } from "node:test";
import assert from "node:assert/strict";
import { setup, rejects, AUTHOR, config } from "./repo-fixtures";
import { githubProvider } from "../portable/repo-github";
import type { Change, CommitRequest } from "../portable/repo-provider";

const b64 = (s: string) => btoa(s);
const req = (changes: Change[], message = "Publish"): CommitRequest => ({ branch: "main", message, author: AUTHOR, changes });

test("3 files (text + 2 images) = exactly one new commit on the branch, no intermediate commit, force:false", async () => {
  const { gh, provider, blob } = await setup();
  const before = gh.store.refs.get("main")!;
  const commitsBefore = gh.store.commits.size;
  const r = await provider.commit(req([
    { op: "put", path: "content/posts/hello.md", content: { text: "# Hello again\n" }, baseBlobId: blob("content/posts/hello.md") },
    { op: "put", path: "static/images/a.png", content: { base64: b64("PNG-A") }, baseBlobId: null },
    { op: "put", path: "static/images/b.png", content: { base64: b64("PNG-B") }, baseBlobId: null },
  ], "New post\r\nwith images"));
  assert.equal(r.attempts, 1);
  assert.equal(gh.store.commits.size, commitsBefore + 1, "one commit object created");
  const c = gh.store.commits.get(r.commitId)!;
  assert.deepEqual(c.parents, [before]);
  assert.equal(c.message, "New post\nwith images");
  assert.deepEqual({ name: c.author.name, email: c.author.email }, AUTHOR);
  assert.equal(gh.store.refs.get("main"), r.commitId);
  assert.equal(gh.store.text(blob("static/images/a.png")!), "PNG-A");
  assert.equal(r.blobIds["static/images/b.png"], blob("static/images/b.png"));
  assert.equal(r.blobIds["content/posts/hello.md"], blob("content/posts/hello.md"));
  assert.equal(gh.store.blobIdAt("main", ".ssg.yaml") !== null, true, "untouched files kept (base_tree)");
  assert.equal(gh.patches.length, 1);
  assert.ok(gh.patches.every((p) => p.force === false));
  const writes = gh.requests.filter((q) => q.method !== "GET").map((q) => `${q.method} ${q.url.slice(gh.base.length)}`);
  assert.deepEqual(writes, ["POST /git/blobs", "POST /git/blobs", "POST /git/blobs", "POST /git/trees", "POST /git/commits", "PATCH /git/refs/heads/main"]);
});

test("delete and move (with and without new content) in one commit", async () => {
  const { gh, provider, blob } = await setup();
  const hello = blob("content/posts/hello.md")!, second = blob("content/posts/second.md")!, about = blob("content/pages/about.md")!;
  const r = await provider.commit(req([
    { op: "delete", path: "content/posts/second.md", baseBlobId: second },
    { op: "move", from: "content/posts/hello.md", to: "content/posts/hello-world.md", baseBlobId: hello },
    { op: "move", from: "content/pages/about.md", to: "content/pages/about-us.md", baseBlobId: about, content: { text: "# About us\n" } },
  ]));
  assert.equal(blob("content/posts/second.md"), null);
  assert.equal(blob("content/posts/hello.md"), null);
  assert.equal(blob("content/posts/hello-world.md"), hello);
  assert.equal(gh.store.text(blob("content/pages/about-us.md")!), "# About us\n");
  assert.deepEqual(r.blobIds, { "content/posts/second.md": null, "content/posts/hello.md": null, "content/posts/hello-world.md": hello, "content/pages/about.md": null, "content/pages/about-us.md": blob("content/pages/about-us.md") });
});

test("race on the same file: the second publication gets conflict with the current version id", async () => {
  const { gh, provider, blob } = await setup();
  const base = blob("content/posts/hello.md")!;
  // The other publication lands between our commit and our PATCH.
  gh.beforePatch = async () => { await gh.store.commitFiles("main", { "content/posts/hello.md": "# Theirs\n" }, "theirs"); };
  const e = await rejects(provider.commit(req([{ op: "put", path: "content/posts/hello.md", content: { text: "# Mine\n" }, baseBlobId: base }])));
  assert.equal(e.code, "conflict");
  assert.equal(e.status, 409);
  assert.deepEqual(e.conflicts, [{ path: "content/posts/hello.md", currentBlobId: blob("content/posts/hello.md"), headCommit: gh.store.refs.get("main") }]);
  assert.equal(gh.store.text(blob("content/posts/hello.md")!), "# Theirs\n", "their change is not overwritten");
  assert.ok(gh.patches.every((p) => p.force === false));
  // A later publication from the stale version is caught before anything is uploaded.
  const posts = gh.requests.length;
  const again = await rejects(provider.commit(req([{ op: "put", path: "content/posts/hello.md", content: { text: "# Mine\n" }, baseBlobId: base }])));
  assert.equal(again.conflicts?.[0].currentBlobId, blob("content/posts/hello.md"));
  assert.ok(!gh.requests.slice(posts).some((q) => q.method !== "GET"), "no write after a conflict found up front");
});

test("race on different files: both publications land, the second retried on the new tip", async () => {
  const { gh, provider, blob } = await setup();
  gh.beforePatch = async () => { await gh.store.commitFiles("main", { "content/posts/second.md": "# Theirs\n" }, "theirs"); };
  const r = await provider.commit(req([{ op: "put", path: "content/posts/hello.md", content: { text: "# Mine\n" }, baseBlobId: blob("content/posts/hello.md") }]));
  assert.equal(r.attempts, 2);
  assert.equal(gh.store.text(blob("content/posts/second.md")!), "# Theirs\n");
  assert.equal(gh.store.text(blob("content/posts/hello.md")!), "# Mine\n");
  assert.equal(gh.store.commits.get(r.commitId)!.message, "Publish");
  assert.equal(gh.requests.filter((q) => q.url.endsWith("/git/blobs")).length, 1, "blobs uploaded once");
  assert.ok(gh.requests.some((q) => q.url.includes("/compare/")), "retry checked with one comparison");
  assert.ok(gh.patches.every((p) => p.force === false));
});

test("two real concurrent publications of different files both succeed", async () => {
  const { gh, blob } = await setup();
  const a = githubProvider(config(), { fetch: gh.fetch }), b = githubProvider(config(), { fetch: gh.fetch });
  const [ra, rb] = await Promise.all([
    a.commit(req([{ op: "put", path: "content/posts/hello.md", content: { text: "A" }, baseBlobId: blob("content/posts/hello.md") }])),
    b.commit(req([{ op: "put", path: "content/posts/second.md", content: { text: "B" }, baseBlobId: blob("content/posts/second.md") }])),
  ]);
  assert.equal(ra.attempts + rb.attempts, 3);
  assert.equal(gh.store.text(blob("content/posts/hello.md")!), "A");
  assert.equal(gh.store.text(blob("content/posts/second.md")!), "B");
});

test("a file removed or created by someone else meanwhile is a conflict (new-file race)", async () => {
  const { gh, provider, blob } = await setup();
  gh.beforePatch = async () => { await gh.store.commitFiles("main", { "content/posts/new.md": "theirs", "content/posts/second.md": null }); };
  const e = await rejects(provider.commit(req([
    { op: "put", path: "content/posts/new.md", content: { text: "mine" }, baseBlobId: null },
    { op: "delete", path: "content/posts/second.md", baseBlobId: blob("content/posts/second.md")! },
  ])));
  assert.deepEqual(e.conflicts?.map((c) => [c.path, c.currentBlobId === null]), [["content/posts/new.md", false], ["content/posts/second.md", true]]);
});

test("history rewritten or a comparison cut at 300 files → everything re-verified at the new tip", async () => {
  for (const variant of ["diverged", "cap"] as const) {
    const { gh, provider, blob } = await setup();
    if (variant === "cap") {
      const files = Array.from({ length: 300 }, (_, i) => ({ filename: `content/x${i}.md`, status: "added", sha: "f".repeat(40) }));
      gh.override = (m, p) => (p.startsWith("/compare/") ? new Response(JSON.stringify({ status: "ahead", files }), { status: 200 }) : undefined);
    }
    gh.beforePatch = async () => {
      const tip = gh.store.commits.get(gh.store.refs.get("main")!)!;
      if (variant === "diverged") gh.store.refs.set("main", await gh.store.putCommit(tip.tree, tip.parents, "rewritten"));
      else await gh.store.commitFiles("main", { "content/posts/second.md": "x" });
    };
    const r = await provider.commit(req([{ op: "put", path: "content/posts/hello.md", content: { text: "M" }, baseBlobId: blob("content/posts/hello.md") }]));
    assert.equal(r.attempts, 2, variant);
    assert.equal(gh.requests.filter((q) => q.url.includes("/contents/content/posts?")).length, 2, variant);
  }
});

test("a branch that keeps moving: bounded attempts, then a retryable conflict without versions", async () => {
  const { gh, provider, blob } = await setup({ maxAttempts: 2 });
  let n = 0;
  const other = async () => { await gh.store.commitFiles("main", { [`content/posts/o${n++}.md`]: "x" }); gh.beforePatch = other; };
  gh.beforePatch = other;
  const e = await rejects(provider.commit(req([{ op: "put", path: "content/posts/hello.md", content: { text: "M" }, baseBlobId: blob("content/posts/hello.md") }])));
  assert.equal(e.code, "conflict");
  assert.deepEqual(e.conflicts, []);
  assert.equal(gh.patches.length, 2);
});

test("protected branch → branch_protected, nothing changed", async () => {
  const { gh, provider, blob } = await setup();
  gh.protectedBranches.add("main");
  const before = gh.store.refs.get("main");
  const e = await rejects(provider.commit(req([{ op: "put", path: "content/posts/hello.md", content: { text: "M" }, baseBlobId: blob("content/posts/hello.md") }])));
  assert.equal(e.code, "branch_protected");
  assert.equal(gh.store.refs.get("main"), before);
});

test("symlink or directory at a written path is refused, never written through", async () => {
  const { gh, provider } = await setup();
  await gh.store.commitFiles("main", { "content/posts/link.md": { symlink: "../../.github/workflows/x.yml" } });
  for (const path of ["content/posts/link.md", "content/posts"]) {
    const e = await rejects(provider.commit(req([{ op: "put", path, content: { text: "x" }, baseBlobId: null }])));
    assert.equal(e.code, "invalid_path", path);
  }
  assert.equal(gh.patches.length, 0);
});

test("input validation happens before any request", async () => {
  const { gh, provider, blob } = await setup({ limits: { maxFilesPerChange: 2, maxFileBytes: 4, maxChangeBytes: 6 } });
  const h = blob("content/posts/hello.md")!;
  const put = (path: string, text: string, base: string | null = null): Change => ({ op: "put", path, content: { text }, baseBlobId: base });
  const cases: [unknown, string][] = [
    [req([]), "invalid_change"],
    [{ ...req([put("content/a.md", "x")]), changes: "x" }, "invalid_change"],
    [req([put("content/a.md", "x"), put("content/b.md", "x"), put("content/c.md", "x")]), "too_large"],
    [req([put("content/a.md", "12345")]), "too_large"],
    [req([put("content/a.md", "1234"), put("content/b.md", "1234")]), "too_large"],
    [req([{ op: "move", from: "content/a.md", to: "content/b.md", baseBlobId: h, content: { base64: "MTIzNDU2" } }]), "too_large"],
    [req([put("content/a.md", "x"), { op: "delete", path: "content/a.md", baseBlobId: h }]), "invalid_change"],
    [req([{ op: "move", from: "content/a.md", to: "content/a.md", baseBlobId: h }]), "invalid_change"],
    [req([put("/content/a.md", "x")]), "invalid_path"],
    [req([put("content/../a.md", "x")]), "invalid_path"],
    [req([put("content/a.md", "x", "nothex")]), "invalid_change"],
    [req([{ op: "delete", path: "content/a.md", baseBlobId: null } as never]), "invalid_change"],
    [req([{ op: "rename", path: "content/a.md" } as never]), "invalid_change"],
    [req([null as never]), "invalid_change"],
    [req([{ op: "put", path: "content/a.md", content: { html: "x" }, baseBlobId: null } as never]), "invalid_change"],
    [req([{ op: "put", path: "content/a.md", content: null, baseBlobId: null } as never]), "invalid_change"],
    [{ ...req([put("content/a.md", "x")]), branch: "a..b" }, "invalid_change"],
    [{ ...req([put("content/a.md", "x")]), message: " " }, "invalid_message"],
    [{ ...req([put("content/a.md", "x")]), author: { name: "x", email: "x\n@example.com" } }, "invalid_author"],
    [undefined, "invalid_message"],
  ];
  for (const [r, code] of cases) assert.equal((await rejects(provider.commit(r as CommitRequest))).code, code, JSON.stringify(r)?.slice(0, 160));
  assert.equal(gh.requests.length, 0);
});

test("a new file in a new directory", async () => {
  const { gh, provider, blob } = await setup();
  await provider.commit(req([{ op: "put", path: "content/2026/11/new.md", content: { text: "n" }, baseBlobId: null }]));
  assert.equal(gh.store.text(blob("content/2026/11/new.md")!), "n");
});

test("malformed answers while committing → provider_error, branch untouched", async () => {
  for (const target of ["/git/blobs", "/git/trees", "/git/commits"]) {
    const { gh, provider, blob } = await setup();
    const before = gh.store.refs.get("main");
    gh.override = (m, p) => (m === "POST" && p === target ? new Response(JSON.stringify({ sha: "nope" }), { status: 201 }) : undefined);
    const e = await rejects(provider.commit(req([{ op: "put", path: "content/posts/hello.md", content: { text: "M" }, baseBlobId: blob("content/posts/hello.md") }])));
    assert.equal(e.code, "provider_error", target);
    assert.equal(gh.store.refs.get("main"), before);
  }
  const { gh, provider, blob } = await setup();
  gh.override = (m, p) => (p.startsWith("/compare/") ? new Response(JSON.stringify({ status: "ahead", files: [{ filename: "content/posts/x.md", status: "renamed", previous_filename: "content/posts/hello.md", sha: "e".repeat(40) }, {}] }), { status: 200 }) : undefined);
  gh.beforePatch = async () => { await gh.store.commitFiles("main", { "content/posts/x.md": "x" }); };
  const e = await rejects(provider.commit(req([{ op: "put", path: "content/posts/hello.md", content: { text: "M" }, baseBlobId: blob("content/posts/hello.md") }])));
  assert.deepEqual(e.conflicts?.map((c) => [c.path, c.currentBlobId]), [["content/posts/hello.md", null]]);
  gh.override = (m, p) => (p.startsWith("/compare/") ? new Response(JSON.stringify({ status: "ahead" }), { status: 200 }) : undefined);
  gh.beforePatch = async () => { await gh.store.commitFiles("main", { "content/posts/y.md": "y" }); };
  const r = await provider.commit(req([{ op: "put", path: "content/posts/hello.md", content: { text: "M" }, baseBlobId: blob("content/posts/hello.md") }]));
  assert.equal(r.attempts, 2);
});
