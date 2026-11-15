// The fake MDDB answers as docs/API.md says the server does — checked here
// for the parts the editor's suites do not reach, so the fake cannot drift
// into something easier than MDDB.
import { test } from "node:test";
import assert from "node:assert/strict";
import { FakeMddb, MDDB_HOST, MDDB_KEY, revisionOf } from "./fake-mddb.ts";

const at = (path: string) => `https://${MDDB_HOST}${path}`;
const call = async (db: FakeMddb, method: string, path: string, body?: unknown) => {
  const res = await db.fetch(at(path), { method, headers: { "x-api-key": MDDB_KEY }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
  return { status: res.status, out: await res.json().catch(() => null) };
};

test("add with expectedRevision: a conflict for a missing or moved document, a write when it holds", async () => {
  const db = new FakeMddb();
  const id = { collection: "c", key: "K", lang: "EN" };
  const missing = await call(db, "POST", "/v1/add", { ...id, expectedRevision: "x", meta: {}, contentMd: "" });
  assert.deepEqual([missing.status, missing.out], [409, { error: "revision conflict", expectedRevision: "x", currentRevision: "" }]);
  const first = await call(db, "POST", "/v1/add", { ...id, meta: { a: ["b"] } });
  assert.deepEqual([first.status, first.out.key, first.out.contentMd, first.out.id], [200, "K", "", "c|k|en"]);
  const moved = await call(db, "POST", "/v1/add", { ...id, expectedRevision: "x", meta: {} });
  assert.equal(moved.out.currentRevision, first.out.revision);
  const held = await call(db, "POST", "/v1/add", { ...id, key: "k", expectedRevision: first.out.revision, meta: {}, contentMd: "2" });
  assert.equal(held.status, 200);
  assert.equal(revisionOf(db.get("c", "k", "en")!), held.out.revision);
});

test("update keeps what it is not given; missing fields 400; unknown routes 404; another host; no body", async () => {
  const db = new FakeMddb();
  db.put("c", "k", "en", { a: ["b"] }, "text");
  const u = await call(db, "PATCH", "/v1/update", { collection: "c", key: "k", lang: "en" });
  assert.deepEqual([u.out.meta, u.out.contentMd], [{ a: ["b"] }, "text"]);
  assert.equal((await call(db, "POST", "/v1/get", { collection: "c" })).status, 400);
  assert.equal((await call(db, "POST", "/v1/nope", { collection: "c", key: "k", lang: "en" })).status, 404);
  assert.equal((await call(db, "GET", "/v1/get")).status, 400);
  db.key = null;
  assert.equal((await db.fetch(at("/v1/get"))).status, 400, "no method, no body");
  assert.equal((await db.fetch("https://other.example.com/v1/get", {})).status, 599);
  const revs = await call(db, "POST", "/v1/revisions", { collection: "c", key: "none", lang: "en" });
  assert.deepEqual(revs.out, { collection: "c", key: "none", lang: "en", revisions: [], total: 0 });
});

test("search: no filter, default page, sorted by key then as stored, a bare array without revisions", async () => {
  const db = new FakeMddb();
  db.put("c", "b", "en", {}, "");
  db.put("c", "a", "pl", {}, "");
  db.put("c", "a", "en", {}, "");
  db.put("other", "z", "en", {}, "");
  const res = await db.fetch(at("/v1/search"), { method: "POST", headers: { authorization: `Bearer ${MDDB_KEY}` }, body: JSON.stringify({ collection: "c" }) });
  const out = await res.json();
  assert.deepEqual(out.map((d: { key: string; lang: string }) => `${d.key}/${d.lang}`), ["a/pl", "a/en", "b/en"]);
  assert.equal(res.headers.get("x-total-count"), "3");
  assert.ok(out.every((d: object) => !("revision" in d)));
  db.key = null;
  const none = await db.fetch(at("/v1/search"), { method: "POST", body: JSON.stringify({ collection: "c", filterMeta: { x: ["1"] } }) });
  assert.deepEqual([none.status, await none.json()], [200, []], "no key configured: open; a filter key a document lacks");
});
