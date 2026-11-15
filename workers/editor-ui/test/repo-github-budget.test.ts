// GO-168 gate in code: the Free plan's 50 subrequests hold for the worst case allowed by PLAN_LIMITS.free,
// and one article with a 1 MB image fits it. Measurement report: see the GO-168 hand-off.
import { test } from "node:test";
import assert from "node:assert/strict";
import { setup, AUTHOR } from "./repo-fixtures";
import type { Change, CommitRequest } from "../portable/repo-provider";

const req = (changes: Change[]): CommitRequest => ({ branch: "main", message: "Publish", author: AUTHOR, changes });

test("subrequest budget (GO-168 gate): 10 files in 10 directories, two lost races = 37 ≤ 50 (Workers Free)", async () => {
  const { gh, provider } = await setup({ maxSubrequests: 50 });
  let races = 2;
  const other = async () => { await gh.store.commitFiles("main", { [`content/other${races}.md`]: "x" }); if (--races > 0) gh.beforePatch = other; };
  gh.beforePatch = other;
  const changes: Change[] = Array.from({ length: 10 }, (_, i) => ({ op: "put", path: `content/d${i}/f.md`, content: { text: `f${i}` }, baseBlobId: null }));
  const r = await provider.commit(req(changes));
  assert.equal(r.attempts, 3);
  assert.equal(provider.used(), 37);
});

test("GO-168 gate: one article + one 1 MB image fits the Free plan limits", async () => {
  const { gh, provider } = await setup(); // default limits = PLAN_LIMITS.free
  const image = Buffer.from(new Uint8Array(1024 * 1024).map((_, i) => i % 251)).toString("base64");
  const r = await provider.commit(req([
    { op: "put", path: "content/posts/new.md", content: { text: "# New\n\n![x](/images/x.png)\n" }, baseBlobId: null },
    { op: "put", path: "static/images/x.png", content: { base64: image }, baseBlobId: null },
  ]));
  assert.equal(gh.store.blobs.get(r.blobIds["static/images/x.png"]!)!.length, 1024 * 1024);
  assert.ok(provider.used() <= 10);
});
