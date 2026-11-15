// POST /api/changes through the whole pipeline against the fake GitHub: one
// commit per publication, authored by the caller; permissions per kind of
// change; the path policy; conflicts that lose neither version.
import { test } from "node:test";
import assert from "node:assert/strict";
import { BASE, ORIGIN, makeEnv, request, run, site } from "./fixtures.ts";
import type { FakeGitHub } from "./fake-github.ts";
import { PERM } from "../src/permissions.ts";

const WRITER = [PERM.read, PERM.publish];
const POST = "---\ntitle: Hello # greeting\ntags:\n  - a\n---\n# Hello\n";

type Init = { perms?: string[] | null; kind?: "user" | "service"; headers?: Record<string, string>; raw?: string };
async function publish(gh: FakeGitHub, body: unknown, init: Init = {}): Promise<Response> {
  const { perms = WRITER, kind = "user", headers = {}, raw } = init;
  const r = await request(BASE + "/api/changes", perms, {
    method: "POST", kind, body: raw ?? JSON.stringify(body),
    headers: { ...(kind === "user" ? { origin: ORIGIN } : {}), "content-type": "application/json", ...headers },
  });
  return run(r, makeEnv(), gh);
}
const v = (gh: FakeGitHub, path: string) => gh.store.blobIdAt("main", path);
const textAt = (gh: FakeGitHub, path: string) => gh.store.text(v(gh, path)!);
const head = (gh: FakeGitHub) => gh.store.commits.get(gh.store.refs.get("main")!)!;
const P = "content/blog/posts/hello.md";

test("a whole-text edit: one commit, the caller as author, the commit's URL and the new versions", async () => {
  const gh = await site({ "docs/b.md": "b" });
  const before = gh.store.refs.get("main");
  const res = await publish(gh, { message: "Fix the intro\n\nLonger text.", changes: [{ op: "put", path: P, version: v(gh, P), text: "# Hi\n" }, { op: "put", path: "docs/b.md", version: v(gh, "docs/b.md"), text: "B" }] });
  assert.equal(res.status, 201);
  const out = await res.json();
  assert.equal(out.commitId, gh.store.refs.get("main"));
  assert.equal(out.url, `https://github.com/acme/site/commit/${out.commitId}`);
  assert.equal(out.branch, "main");
  assert.deepEqual(out.files, [{ path: P, version: v(gh, P) }, { path: "docs/b.md", version: v(gh, "docs/b.md") }]);
  assert.deepEqual(head(gh).parents, [before], "exactly one new commit");
  assert.equal(head(gh).message, "Fix the intro\n\nLonger text.");
  assert.deepEqual({ name: head(gh).author.name, email: head(gh).author.email }, { name: "ada@example.com", email: "ada@example.com" });
  assert.equal(textAt(gh, P), "# Hi\n");
  assert.equal(gh.patches.at(-1)!.force, false);
});

test("a form edit changes only the lines of the changed keys; a new file from the form", async () => {
  const gh = await site({ [P]: POST });
  const res = await publish(gh, { message: "Retitle", changes: [
    { op: "put", path: P, version: v(gh, P), fields: { title: "Bye", tags: ["a"] } },
    { op: "put", path: "content/blog/posts/new.md", version: null, fields: { title: "New", status: "draft" }, body: "Text\n" },
  ] });
  assert.equal(res.status, 201);
  assert.equal(textAt(gh, P), POST.replace("title: Hello #", "title: Bye #"));
  assert.equal(textAt(gh, "content/blog/posts/new.md"), "---\ntitle: New\nstatus: draft\n---\nText\n");
  const raw = await publish(gh, { message: "Raw", changes: [{ op: "put", path: P, version: v(gh, P), frontmatterRaw: "title: Raw\n", body: "B\n" }] });
  assert.equal(raw.status, 201);
  assert.equal(textAt(gh, P), "---\ntitle: Raw\n---\nB\n");
});

test("form edits the editor cannot make are refused by key; a file that is not UTF-8 needs its whole text", async () => {
  const gh = await site({ [P]: "---\nauthor:\n  name: Ada\n---\n" });
  const res = await publish(gh, { message: "x", changes: [{ op: "put", path: P, version: v(gh, P), fields: { author: "Bob" } }] });
  assert.equal(res.status, 400);
  assert.deepEqual(await res.json(), { error: "fields_refused", detail: "these frontmatter keys cannot be changed from the form", path: P, refused: [{ key: "author", reason: "not_simple" }] });
  const head0 = gh.store.refs.get("main")!;
  const flat = new Map(gh.store.flatAt(head0));
  flat.set(P, { mode: "100644", sha: await gh.store.putBlob(new Uint8Array([0xff])) });
  gh.store.refs.set("main", await gh.store.putCommit(await gh.store.putTree(flat), [head0], "binary"));
  const bin = await publish(gh, { message: "x", changes: [{ op: "put", path: P, version: v(gh, P), body: "x" }] });
  assert.deepEqual([bin.status, (await bin.json()).error], [422, "not_text"]);
  const many = Array.from({ length: 6 }, (_, i) => ({ op: "put", path: `docs/f${i}.md`, version: null, body: "x" }));
  const big = await publish(gh, { message: "x", changes: many });
  assert.deepEqual([big.status, (await big.json()).error], [413, "too_large"]);
});

test("move with and without new content; delete needs editor-ui:delete as well", async () => {
  const gh = await site({ [P]: POST, "docs/old.md": "old", "docs/gone.md": "g" });
  const moved = await publish(gh, { message: "Rename", changes: [
    { op: "move", from: P, to: "content/blog/posts/renamed.md", version: v(gh, P), fields: { title: "Renamed" } },
    { op: "move", from: "docs/old.md", to: "docs/new.md", version: v(gh, "docs/old.md") },
  ] });
  assert.equal(moved.status, 201);
  assert.equal(v(gh, P), null);
  assert.equal(textAt(gh, "content/blog/posts/renamed.md"), POST.replace("title: Hello #", "title: Renamed #"));
  assert.equal(textAt(gh, "docs/new.md"), "old");
  const del = { message: "Remove", changes: [{ op: "delete", path: "docs/gone.md", version: v(gh, "docs/gone.md") }] };
  const refused = await publish(gh, del);
  assert.deepEqual([refused.status, await refused.json()], [403, { error: "forbidden", permission: PERM.delete }]);
  const ok = await publish(gh, del, { perms: [...WRITER, PERM.delete] });
  assert.equal(ok.status, 201);
  assert.deepEqual((await ok.json()).files, [{ path: "docs/gone.md", version: null }]);
  assert.equal(v(gh, "docs/gone.md"), null);
  const deleter = await publish(gh, del, { perms: [PERM.read, PERM.delete] });
  assert.deepEqual([deleter.status, (await deleter.json()).permission], [403, PERM.publish], "a publication needs editor-ui:publish");
});

test("editor-ui:read without editor-ui:publish is refused before anything is read", async () => {
  const gh = await site();
  const before = gh.requests.length;
  const res = await publish(gh, { message: "x", changes: [{ op: "put", path: P, version: v(gh, P), text: "x" }] }, { perms: [PERM.read] });
  assert.deepEqual([res.status, await res.json()], [403, { error: "forbidden", permission: PERM.publish }]);
  assert.equal(gh.requests.length, before);
});

test("the path policy: only Markdown inside the content folders", async () => {
  const gh = await site();
  const all = [...WRITER, PERM.delete];
  for (const [path, reason] of [[".ssg.yaml", "dot_segment"], [".github/workflows/x.yml", "denied"], ["workers/editor-ui/x.md", "denied"], ["wrangler.toml", "denied"], ["static/x.md", "outside_roots"], ["content/blog/media/a.png", "media"]]) {
    // An image of the media folder may be deleted (GO-171, media-changes.test.ts); put as text and move stay refused.
    const del = reason === "media" ? [] : [{ op: "delete", path, version: "a".repeat(40) }];
    for (const change of [{ op: "put", path, version: null, text: "x" }, ...del, { op: "move", from: P, to: path, version: v(gh, P) }]) {
      const res = await publish(gh, { message: "x", changes: [change] }, { perms: all });
      assert.equal(res.status, 400, `${change.op} ${path}`);
      const body = await res.json();
      assert.deepEqual([body.error, body.path, body.reason], ["invalid_path", path, reason], `${change.op} ${path}`);
    }
  }
});

test("conflict: the file moved on — both texts in the answer, nothing written", async () => {
  const gh = await site({ [P]: POST });
  const mine = v(gh, P);
  await gh.store.commitFiles("main", { [P]: "---\ntitle: Theirs\n---\ntheirs\n" }, "someone else");
  const tip = gh.store.refs.get("main");
  const res = await publish(gh, { message: "x", changes: [{ op: "put", path: P, version: mine, text: "yours\n" }] });
  assert.equal(res.status, 409);
  assert.deepEqual(await res.json(), { error: "conflict", detail: "these files changed on the branch since they were opened", files: [{ path: P, yours: "yours\n", theirs: "---\ntitle: Theirs\n---\ntheirs\n", theirVersion: v(gh, P) }] });
  assert.equal(gh.store.refs.get("main"), tip);
  const form = await publish(gh, { message: "x", changes: [{ op: "put", path: P, version: mine, fields: { title: "T" }, body: "mine\n" }] });
  assert.deepEqual((await form.json()).files, [{ path: P, yours: "---\ntitle: T\n---\nmine\n", theirs: "---\ntitle: Theirs\n---\ntheirs\n", theirVersion: v(gh, P) }], "your fields and body over their file");
  const gone = await publish(gh, { message: "x", changes: [{ op: "put", path: "docs/none.md", version: "b".repeat(40), body: "x" }] });
  assert.deepEqual((await gone.json()).files, [{ path: "docs/none.md", yours: "---\n---\nx", theirs: null, theirVersion: null }]);
  await gh.store.commitFiles("main", { [P]: "plain\n" }, "someone else again");
  const bare = await publish(gh, { message: "x", changes: [{ op: "put", path: P, version: mine, fields: { title: "T" }, body: "mine\n" }] });
  assert.equal((await bare.json()).files[0].yours, "---\ntitle: T\n---\nmine\n", "their file has no frontmatter: yours on a fresh block, nothing typed is lost");
  const refusedMine = await publish(gh, { message: "x", changes: [{ op: "put", path: P, version: mine, fields: { "bad key": 1 } }] });
  assert.equal((await refusedMine.json()).files[0].yours, null, "an edit that cannot be applied anywhere");
});

test("conflict: a new file that exists; a change to another file is no conflict; the race window", async () => {
  const gh = await site();
  const exists = await publish(gh, { message: "x", changes: [{ op: "put", path: P, version: null, text: "new\n" }] });
  assert.deepEqual((await exists.json()).files, [{ path: P, yours: "new\n", theirs: "# Hello\n", theirVersion: v(gh, P) }]);
  const mine = v(gh, P);
  await gh.store.commitFiles("main", { "docs/guide.md": "other\n" }, "someone else");
  assert.equal((await publish(gh, { message: "x", changes: [{ op: "put", path: P, version: mine, text: "ok\n" }] })).status, 201);
  const now = v(gh, P);
  gh.beforePatch = async () => { await gh.store.commitFiles("main", { [P]: "raced\n" }, "race"); };
  const raced = await publish(gh, { message: "x", changes: [{ op: "put", path: P, version: now, text: "late\n" }] });
  assert.deepEqual((await raced.json()).files, [{ path: P, yours: "late\n", theirs: "raced\n", theirVersion: v(gh, P) }]);
  const del = await publish(gh, { message: "x", changes: [{ op: "delete", path: P, version: now }] }, { perms: [...WRITER, PERM.delete] });
  assert.deepEqual((await del.json()).files, [{ path: P, yours: null, theirs: "raced\n", theirVersion: v(gh, P) }]);
});

test("branch_busy when the branch keeps moving; a protected branch is named", async () => {
  const gh = await site();
  let n = 0;
  const busy = async () => { await gh.store.commitFiles("main", { [`docs/x${n++}.md`]: "x" }, "busy"); gh.beforePatch = busy; };
  gh.beforePatch = busy;
  const res = await publish(gh, { message: "x", changes: [{ op: "put", path: P, version: v(gh, P), text: "y" }] });
  assert.deepEqual([res.status, (await res.json()).error], [409, "branch_busy"]);
  gh.beforePatch = null;
  gh.protectedBranches.add("main");
  const prot = await publish(gh, { message: "x", changes: [{ op: "put", path: P, version: v(gh, P), text: "y" }] });
  assert.deepEqual([prot.status, (await prot.json()).error], [409, "branch_protected"]);
});

test("a service token: no CSRF check, the service account as author", async () => {
  const gh = await site();
  const res = await publish(gh, { message: "Bot", changes: [{ op: "put", path: P, version: v(gh, P), text: "bot\n" }] }, { kind: "service" });
  assert.equal(res.status, 201);
  assert.deepEqual({ name: head(gh).author.name, email: head(gh).author.email }, { name: "service account u-ada", email: "u-ada@service-account.invalid" });
});

test("the request: CSRF, media type, JSON, size, message, shape of each change", async () => {
  const gh = await site();
  const csrf = await run(await request(BASE + "/api/changes", WRITER, { method: "POST", body: "{}", headers: { "content-type": "application/json" } }), makeEnv(), gh);
  assert.deepEqual([csrf.status, (await csrf.json()).error], [403, "cross_origin"]);
  const err = async (body: unknown, init: Init = {}) => {
    const r = await publish(gh, body, init);
    return [r.status, (await r.json()).error];
  };
  assert.deepEqual(await err({}, { headers: { "content-type": "text/plain" } }), [415, "unsupported_media_type"]);
  assert.deepEqual(await err(null, { raw: "{nope" }), [400, "bad_json"]);
  assert.deepEqual(await err(null, { raw: "{}", headers: { "content-length": String(9 * 1024 * 1024) } }), [413, "too_large"]);
  assert.deepEqual(await err(null, { raw: JSON.stringify({ message: "x".repeat(9 * 1024 * 1024) }) }), [413, "too_large"]);
  const ok = { op: "put", path: P, version: v(gh, P), text: "x" };
  assert.deepEqual(await err([]), [400, "invalid_change"]);
  assert.deepEqual(await err({ message: 1, changes: [ok] }), [400, "invalid_message"]);
  assert.deepEqual(await err({ message: "\uD800", changes: [ok] }), [400, "invalid_message"]);
  assert.deepEqual(await err({ message: "  ", changes: [ok] }), [400, "invalid_message"]);
  assert.deepEqual(await err({ message: "x".repeat(201), changes: [ok] }), [400, "invalid_message"]);
  assert.equal((await publish(gh, { message: "x".repeat(200) + "\n" + "y".repeat(500), changes: [ok] })).status, 201);
  const bad = async (change: unknown) => {
    const r = await publish(gh, { message: "x", changes: [change] }, { perms: [...WRITER, PERM.delete] });
    const body = await r.json();
    return [r.status, body.error, body.index];
  };
  assert.deepEqual(await err({ message: "x", changes: [] }), [400, "invalid_change"]);
  for (const change of [
    "put", { op: "copy" }, { op: "put", path: P, version: 1, text: "x" }, { op: "delete", path: P, version: null },
    { op: "put", path: P, version: null, text: "x", body: "y" }, { op: "put", path: P, version: null, text: 1 },
    { op: "put", path: P, version: null, fields: [] }, { op: "put", path: P, version: null }, { op: "put", path: P, version: null, body: "\uDC00" },
    { op: "move", from: P, to: "docs/z.md", version: "" },
  ]) assert.deepEqual(await bad(change), [400, "invalid_change", 0], JSON.stringify(change));
  const twice = await publish(gh, { message: "x", changes: [ok, ok] });
  assert.equal(twice.status, 400, "a path twice: the provider's own check");
});
