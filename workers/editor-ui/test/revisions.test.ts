// Revisions through the whole pipeline against the fake GitHub: the file's
// history, a revision's text (only ids from that history — a foreign id is
// 404 before any object is read), the diff, and restoring = publishing the
// old text as a new commit on top of the branch.
import { test } from "node:test";
import assert from "node:assert/strict";
import { BASE, ORIGIN, makeEnv, request, run, site } from "./fixtures.ts";
import type { FakeGitHub } from "./fake-github.ts";
import { PERM } from "../src/permissions.ts";
import { restoreMessage } from "../src/revision-read.ts";

const P = "content/blog/posts/hello.md";
const SEG = encodeURIComponent(P);
const get = async (gh: FakeGitHub, path: string) => run(await request(BASE + path, [PERM.read]), makeEnv(), gh);
const body = async (gh: FakeGitHub, path: string) => {
  const res = await get(gh, path);
  return { status: res.status, out: await res.json() };
};
const headOf = (gh: FakeGitHub) => gh.store.refs.get("main")!;

// history: the seed, two edits of the article, one commit to another file.
async function history() {
  const gh = await site();
  const seed = headOf(gh);
  const v2 = await gh.store.commitFiles("main", { [P]: "# Hello\nworld\n" }, "Second\n\nwith a body");
  const other = await gh.store.commitFiles("main", { "docs/guide.md": "# Guide 2\n" }, "Other file");
  const v3 = await gh.store.commitFiles("main", { [P]: "# Hello\r\nWorld\r\n" }, "Third");
  await gh.store.commitFiles("main", { "docs/guide.md": "# Guide 3\n" }, "Other again");
  return { gh, seed, v2, v3, other };
}

test("the list: commits that changed the file, newest first, the first line of each message", async () => {
  const { gh, seed, v2, v3 } = await history();
  const { status, out } = await body(gh, `/api/revisions/${SEG}`);
  assert.equal(status, 200);
  assert.equal(out.path, P);
  assert.deepEqual(out.items.map((i: { commitId: string }) => i.commitId), [v3, v2, seed]);
  assert.deepEqual(out.items[1], { commitId: v2, authorName: "Seed", authorEmail: "seed@example.com", date: gh.store.commits.get(v2)!.author.date, message: "Second" });
  assert.equal(out.nextCursor, null);
  assert.deepEqual(Object.keys(out).sort(), ["items", "nextCursor", "path"]);
  const bad = await body(gh, `/api/revisions/${SEG}?cursor=x`);
  assert.deepEqual([bad.status, bad.out.error], [400, "bad_cursor"]);
  const outside = await body(gh, `/api/revisions/${encodeURIComponent(".ssg.yaml")}`);
  assert.deepEqual([outside.status, outside.out.error], [400, "invalid_path"]);
});

test("pages of 30; a revision past the scanned pages needs its cursor", async () => {
  const gh = await site();
  const seed = headOf(gh);
  for (let i = 0; i < 100; i++) await gh.store.commitFiles("main", { [P]: `# Hello ${i}\n` }, `Edit ${i}`);
  const first = await body(gh, `/api/revisions/${SEG}`);
  assert.deepEqual([first.out.items.length, first.out.nextCursor], [30, "2"]);
  const last = await body(gh, `/api/revisions/${SEG}?cursor=4`);
  assert.deepEqual([last.out.items.length, last.out.nextCursor, last.out.items.at(-1).commitId], [11, null, seed]);
  const far = await body(gh, `/api/revisions/${SEG}/${seed}`);
  assert.deepEqual([far.status, far.out.error, far.out.commit], [404, "revision_not_found", seed]);
  assert.equal((await body(gh, `/api/revisions/${SEG}/${seed}?cursor=4`)).status, 200);
  assert.equal((await body(gh, `/api/revisions/${SEG}/${seed}?cursor=2`)).status, 200, "pages 2, 3 and 4 are scanned");
});

test("a commit id outside the file's history is 404 and never reaches the repository host", async () => {
  const { gh, other } = await history();
  const foreign = await gh.store.commitFiles("feature", { [P]: "# Not on main\n" }, "elsewhere");
  const unknown = "e".repeat(40);
  await body(gh, `/api/revisions/${SEG}`); // the site model is read (and cached) by the first request
  for (const id of [foreign, other, unknown]) {
    gh.requests.length = 0;
    const { status, out } = await body(gh, `/api/revisions/${SEG}/${id}`);
    assert.deepEqual([status, out.error, out.commit], [404, "revision_not_found", id]);
    assert.ok(gh.requests.every((r) => !r.url.includes(id)), "no request names the id");
    assert.ok(gh.requests.every((r) => !r.url.includes("/git/blobs/") && !r.url.includes("/contents/")), "no listing, no blob read");
    const diff = await body(gh, `/api/revisions/${SEG}/diff?from=${id}`);
    assert.deepEqual([diff.status, diff.out.error], [404, "revision_not_found"]);
  }
  const malformed = await body(gh, `/api/revisions/${SEG}/ABC`);
  assert.deepEqual([malformed.status, malformed.out.error, malformed.out.param], [400, "invalid_commit", "commit"]);
});

test("a revision's text, its version and the current one; the latest revision is identical", async () => {
  const { gh, v2, v3 } = await history();
  const { status, out } = await body(gh, `/api/revisions/${SEG}/${v2}`);
  assert.equal(status, 200);
  assert.equal(out.text, "# Hello\nworld\n");
  assert.equal(out.message, "Second\n\nwith a body");
  assert.equal(out.version, gh.store.flatAt(v2).get(P)!.sha);
  assert.deepEqual(out.current, { commit: headOf(gh), version: gh.store.blobIdAt("main", P) });
  assert.equal(out.identical, false);
  assert.deepEqual(out.restore.change, { op: "put", path: P, version: gh.store.blobIdAt("main", P), text: "# Hello\nworld\n" });
  assert.equal(out.restore.message, `Restore hello.md to ${v2.slice(0, 7)} of 2026-01-01`);
  assert.equal((await body(gh, `/api/revisions/${SEG}/${v3}`)).out.identical, true);
  assert.equal(restoreMessage("a.md", "abcdef0123", ""), "Restore a.md to abcdef0");
});

test("restoring: the revision's change published as is — a new commit whose parent is the branch head", async () => {
  const { gh, v2 } = await history();
  const listed = await body(gh, `/api/revisions/${SEG}`);
  const rev = (await body(gh, `/api/revisions/${SEG}/${listed.out.items[1].commitId}`)).out;
  assert.equal(rev.commitId, v2);
  const before = headOf(gh);
  const r = await request(BASE + "/api/changes", [PERM.read, PERM.publish], {
    method: "POST", headers: { origin: ORIGIN, "content-type": "application/json" },
    body: JSON.stringify({ message: rev.restore.message, changes: [rev.restore.change] }),
  });
  const res = await run(r, makeEnv(), gh);
  assert.equal(res.status, 201);
  const commit = gh.store.commits.get(headOf(gh))!;
  assert.deepEqual(commit.parents, [before], "one new commit on the head; history kept");
  assert.equal(commit.message, rev.restore.message);
  assert.equal(gh.store.text(gh.store.blobIdAt("main", P)!), "# Hello\nworld\n");
  assert.ok(gh.patches.every((p) => p.force === false));
  const after = await body(gh, `/api/revisions/${SEG}`);
  assert.deepEqual(after.out.items.slice(0, 2).map((i: { commitId: string }) => i.commitId), [headOf(gh), listed.out.items[0].commitId]);
  // The same change again: the file has moved on, so it is a conflict, not an overwrite.
  const again = await run(await request(BASE + "/api/changes", [PERM.read, PERM.publish], {
    method: "POST", headers: { origin: ORIGIN, "content-type": "application/json" },
    body: JSON.stringify({ message: "again", changes: [{ ...rev.restore.change, text: "x\n" }] }),
  }), makeEnv(), gh);
  assert.equal(again.status, 409);
});

test("removed, re-created, not UTF-8, too large: each revision answered plainly", async () => {
  const gh = await site();
  const removed = await gh.store.commitFiles("main", { [P]: null }, "Remove");
  await gh.store.commitFiles("main", { [P]: "# Back\n" }, "Back");
  const gone = await body(gh, `/api/revisions/${SEG}/${removed}`);
  assert.deepEqual([gone.status, gone.out.error], [404, "not_in_revision"]);
  const d = await body(gh, `/api/revisions/${SEG}/diff?from=${removed}`);
  assert.deepEqual([d.status, d.out.from.exists, d.out.to.exists, d.out.added, d.out.identical], [200, false, true, 1, false]);
  const none = (await body(gh, `/api/revisions/${SEG}/diff?from=${removed}&to=${removed}`)).out;
  assert.deepEqual([none.from.exists, none.to.exists, none.identical], [false, false, true], "absent on both sides");
  const head0 = headOf(gh);
  const flat = new Map(gh.store.flatAt(head0));
  flat.set(P, { mode: "100644", sha: await gh.store.putBlob(new Uint8Array([0xff])) });
  const binary = await gh.store.putCommit(await gh.store.putTree(flat), [head0], "binary");
  gh.store.refs.set("main", binary);
  const bin = await body(gh, `/api/revisions/${SEG}/${binary}`);
  assert.deepEqual([bin.status, bin.out.error], [422, "not_text"]);
  const big = await gh.store.commitFiles("main", { [P]: "x".repeat(2 * 1024 * 1024 + 1) }, "Big");
  const small = await gh.store.commitFiles("main", { [P]: "small\n" }, "Small");
  assert.deepEqual([(await body(gh, `/api/revisions/${SEG}/${big}`)).status], [413]);
  const bigDiff = (await body(gh, `/api/revisions/${SEG}/diff?from=${big}&to=${small}`)).out;
  assert.deepEqual([bigDiff.tooLarge, bigDiff.hunks, bigDiff.identical, bigDiff.from.version], ["file", [], false, null]);
  await gh.store.commitFiles("main", { [P]: "y".repeat(2 * 1024 * 1024 + 1) }, "Big again");
  assert.equal((await body(gh, `/api/revisions/${SEG}/${small}`)).status, 413, "the current version is too large to base a restore on");
});

test("the diff: two revisions, a revision and the current version, limits, downloads", async () => {
  const { gh, seed, v2, v3 } = await history();
  const cur = (await body(gh, `/api/revisions/${SEG}/diff?from=${v2}`)).out;
  assert.deepEqual([cur.to.current, cur.to.commitId, cur.to.eol, cur.from.eol, cur.added, cur.removed], [true, headOf(gh), "crlf", "lf", 1, 1]);
  assert.deepEqual(cur.hunks[0].lines.map((l: { kind: string; text: string }) => l.kind[0] + l.text), ["s# Hello", "rworld", "aWorld"]);
  assert.deepEqual(cur.download, { from: `api/revisions/${SEG}/${v2}`, to: `api/content/${SEG}` });
  const two = (await body(gh, `/api/revisions/${SEG}/diff?from=${seed}&to=${v2}&cursor=1`)).out;
  assert.deepEqual([two.to.current, two.to.commitId, two.added, two.removed, two.identical], [false, v2, 1, 0, false]);
  assert.deepEqual(two.download, { from: `api/revisions/${SEG}/${seed}?cursor=1`, to: `api/revisions/${SEG}/${v2}?cursor=1` });
  const same = (await body(gh, `/api/revisions/${SEG}/diff?from=${v3}&to=current`)).out;
  assert.deepEqual([same.identical, same.hunks], [true, []]);
  const other = (await body(gh, `/api/revisions/${SEG}/diff?from=${v3}&to=`)).out;
  assert.equal(other.identical, true);
  const missing = await body(gh, `/api/revisions/${SEG}/diff?from=${seed}&to=${"f".repeat(40)}`);
  assert.deepEqual([missing.status, missing.out.commit], [404, "f".repeat(40)]);
  const noFrom = await body(gh, `/api/revisions/${SEG}/diff`);
  assert.deepEqual([noFrom.status, noFrom.out.error, noFrom.out.param], [400, "invalid_commit", "from"]);
  const huge = await gh.store.commitFiles("main", { [P]: "z\n".repeat(200_000) }, "Huge");
  const tl = (await body(gh, `/api/revisions/${SEG}/diff?from=${seed}&to=${huge}`)).out;
  assert.deepEqual([tl.tooLarge, tl.hunks, tl.identical], ["size", [], false]);
});

test("the repository host failing is not a missing revision", async () => {
  const { gh, v2 } = await history();
  gh.override = (_m, path) => (path === "/commits" ? new Response("{}", { status: 500 }) : undefined);
  assert.equal((await body(gh, `/api/revisions/${SEG}`)).status, 502);
  gh.override = (_m, path) => (path.startsWith("/git/blobs/") ? new Response("{}", { status: 500 }) : undefined);
  assert.equal((await body(gh, `/api/revisions/${SEG}/${v2}`)).status, 502);
});
