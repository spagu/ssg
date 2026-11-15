// The MDDB client on its own, with scripted answers: the credential header
// ssg uses, redirects never followed, every failure mapped to a code and a
// sentence of our own (never MDDB's body, never the key), the call budget.
import { test } from "node:test";
import assert from "node:assert/strict";
import { MAX_CALLS, MAX_RESPONSE, mddbClient } from "../src/mddb-client.ts";
import type { MddbConfig } from "../src/mddb-config.ts";
import { OpError } from "../portable/ops.ts";

const REF = { collection: "blog", key: "k", lang: "en" };
const config = (apiKey: string | null = "plain-key"): MddbConfig => ({ url: "https://mddb.example.com/base", apiKey, sources: [], lang: null, newLang: "en", allowHttp: false });
type Seen = { url: string; init: RequestInit };
function client(answer: (seen: Seen) => Response | Promise<Response>, key: string | null = "plain-key") {
  const seen: Seen[] = [];
  const c = mddbClient(config(key), { fetch: async (url, init = {}) => { seen.push({ url, init }); return answer({ url, init }); } });
  return { c, seen };
}
const code = async (p: Promise<unknown>) => {
  try {
    await p;
    return "resolved";
  } catch (err) {
    assert.ok(err instanceof OpError);
    return `${err.status} ${err.code}`;
  }
};

test("the key: X-API-Key for a key, Authorization: Bearer for a JWT, none without; redirect manual; the base path kept", async () => {
  const { c, seen } = client(() => Response.json({ key: "k", lang: "en", meta: { a: ["x", 1, true, null], b: "no" }, contentMd: "c", revision: "r" }));
  const doc = await c.get(REF);
  const proto = await client(() => new Response('{"meta": {"__proto__": ["p"]}}')).c.get(REF);
  assert.deepEqual([Object.keys(proto!.meta), Object.getPrototypeOf(proto!.meta)], [["__proto__"], Object.prototype]);
  assert.deepEqual(doc, { key: "k", lang: "en", meta: { a: ["x", "1", "true"] }, contentMd: "c", addedAt: 0, updatedAt: 0, revision: "r" });
  const h = new Headers(seen[0].init.headers);
  assert.deepEqual([seen[0].url, h.get("x-api-key"), h.get("authorization"), seen[0].init.redirect], ["https://mddb.example.com/base/v1/get", "plain-key", null, "manual"]);
  const jwt = client(() => Response.json({}), "aaa.bbb.ccc");
  await jwt.c.get(REF);
  assert.deepEqual([new Headers(jwt.seen[0].init.headers).get("authorization"), new Headers(jwt.seen[0].init.headers).get("x-api-key")], ["Bearer aaa.bbb.ccc", null]);
  const none = client(() => Response.json({}), null);
  await none.c.get(REF);
  assert.deepEqual([new Headers(none.seen[0].init.headers).get("authorization"), new Headers(none.seen[0].init.headers).get("x-api-key")], [null, null]);
});

test("failures: redirect, refused key, rate limit, server error, no answer, too large, not JSON, wrong shape", async () => {
  const at = (r: () => Response | Promise<Response>) => code(client(r).c.get(REF));
  assert.equal(await at(() => new Response(null, { status: 302, headers: { location: "https://elsewhere.example.com/" } })), "502 mddb_error");
  assert.equal(await at(() => new Response("bad key mddb_live_x", { status: 401 })), "502 mddb_rejected");
  assert.equal(await at(() => new Response(null, { status: 403 })), "502 mddb_rejected");
  assert.equal(await at(() => new Response(null, { status: 429 })), "429 rate_limited");
  assert.equal(await at(() => new Response(null, { status: 503 })), "502 mddb_unavailable");
  assert.equal(await at(() => Promise.reject(new Error("socket"))), "502 mddb_unavailable");
  assert.equal(await at(() => new Response("{}", { headers: { "content-length": String(MAX_RESPONSE + 1) } })), "502 mddb_error");
  assert.equal(await at(() => ({ status: 200, headers: new Headers(), text: async () => "x".repeat(MAX_RESPONSE + 1) }) as unknown as Response), "502 mddb_error");
  assert.equal(await at(() => new Response("<html>")), "502 mddb_error");
  assert.equal(await at(() => Response.json([1])), "502 mddb_error");
  assert.equal(await at(() => new Response("not json", { status: 400 })), "422 mddb_refused");
  assert.equal(await at(() => Response.json({ error: "schema: title required" }, { status: 400 })), "422 mddb_refused");
  try {
    await client(() => Response.json({ error: "schema: title required" }, { status: 400 })).c.add(REF, {}, "");
  } catch (err) {
    assert.equal((err as OpError).detail, "MDDB refused the request: schema: title required");
  }
});

test("writes and lists: 404s, conflicts with and without a body, the refusals of each call", async () => {
  assert.equal(await client(() => Response.json({ error: "document not found" }, { status: 404 })).c.get(REF), null);
  assert.equal(await code(client(() => Response.json({}, { status: 400 })).c.add(REF, {}, "")), "422 mddb_refused");
  assert.deepEqual(await client(() => Response.json({}, { status: 404 })).c.update(REF, {}, "", "r"), { ok: false, missing: true });
  assert.deepEqual(await client(() => Response.json({ currentRevision: "now" }, { status: 409 })).c.update(REF, {}, "", "r"), { ok: false, conflict: "now" });
  assert.deepEqual(await client(() => new Response("x", { status: 409 })).c.update(REF, {}, "", "r"), { ok: false, conflict: null });
  assert.equal(await code(client(() => Response.json({}, { status: 400 })).c.update(REF, {}, "", "r")), "422 mddb_refused");
  const up = client(() => Response.json({ key: "k", revision: "r2" }));
  assert.equal(((await up.c.update(REF, { a: ["b"] }, "c", "r1")) as { doc: { revision: string } }).doc.revision, "r2");
  assert.deepEqual(JSON.parse(String(up.seen[0].init.body)), { collection: "blog", key: "k", lang: "en", meta: { a: ["b"] }, contentMd: "c", expectedRevision: "r1" });
  assert.equal(up.seen[0].init.method, "PATCH");
  assert.equal(await code(client(() => Response.json({}, { status: 404 })).c.remove(REF)), "resolved");
  assert.equal(await code(client(() => Response.json({}, { status: 400 })).c.remove(REF)), "422 mddb_refused");
  assert.deepEqual(await client(() => Response.json({}, { status: 404 })).c.search("blog", null, {}, 0, 10), []);
  assert.equal(await code(client(() => Response.json({})).c.search("blog", null, {}, 0, 10)), "502 mddb_error");
  const s = client(() => Response.json([{ key: "a" }]));
  await s.c.search("blog", "en", { type: ["post"] }, 5, 10);
  assert.deepEqual(JSON.parse(String(s.seen[0].init.body)), { collection: "blog", lang: "en", filterMeta: { type: ["post"] }, sort: "key", asc: true, limit: 10, offset: 5 });
  assert.deepEqual(await client(() => Response.json({}, { status: 404 })).c.revisions(REF), []);
  assert.deepEqual(await client(() => Response.json({ revisions: "no" })).c.revisions(REF), []);
  assert.equal(await code(client(() => Response.json({}, { status: 400 })).c.revisions(REF)), "502 mddb_error");
  assert.deepEqual(await client(() => Response.json({ revisions: [{ timestamp: 5, meta: { a: ["b"] } }] })).c.revisions(REF), [{ timestamp: 5, updatedAt: 0, contentMd: "", meta: { a: ["b"] } }]);
});

test("the budget: MAX_CALLS requests per editor request, then 503 mddb_budget", async () => {
  const { c } = client(() => Response.json({}));
  for (let i = 0; i < MAX_CALLS; i++) await c.get(REF);
  assert.equal(c.used(), MAX_CALLS);
  assert.equal(await code(c.get(REF)), "503 mddb_budget");
});

test("without an injected fetch the global one is used (a refused host answers mddb_unavailable)", async () => {
  const saved = globalThis.fetch;
  globalThis.fetch = (async () => { throw new Error("offline"); }) as typeof fetch;
  try {
    assert.equal(await code(mddbClient(config(), {}).get(REF)), "502 mddb_unavailable");
  } finally {
    globalThis.fetch = saved;
  }
});
