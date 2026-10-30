// POST /api/push/notify against a fake feed and fake push services. Run from
// the repository root:
//   node --import ./workers/web-push/test/resolve.mjs --test 'workers/web-push/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as notify } from "../functions/api/push/notify.ts";
import { b64urlEncode } from "../functions/api/push/_lib.ts";
import { ctx, fakeDB, fakeKV, SUB } from "./fakes.ts";

const pair = (await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign"])) as CryptoKeyPair;
const VAPID_PUBLIC = b64urlEncode(new Uint8Array((await crypto.subtle.exportKey("raw", pair.publicKey)) as ArrayBuffer));
const VAPID_PRIVATE = (await crypto.subtle.exportKey("jwk", pair.privateKey)).d as string;

const feed = (ids: string[]) =>
  `<feed xmlns="http://www.w3.org/2005/Atom">${ids
    .map((id) => `<entry><title>Post ${id}</title><link href="/p/${id}/"/><id>${id}</id><summary>About ${id}</summary></entry>`)
    .join("")}</feed>`;

function setup(opts: { ids?: string[]; last?: string; status?: Record<string, number>; feedStatus?: number; env?: Record<string, unknown> } = {}) {
  const db = fakeDB();
  for (const name of ["a", "b", "c"]) {
    const endpoint = `https://fcm.googleapis.com/fcm/send/${name}`;
    db.rows.set(endpoint, { endpoint, p256dh: SUB.keys.p256dh, auth: SUB.keys.auth, ua_hash: null });
  }
  const kv = fakeKV(opts.last ? { "last-item": opts.last } : {});
  const pushed: { endpoint: string; ttl: string }[] = [];
  const feedURLs: string[] = [];
  const fetchFn = async (url: string, init: RequestInit = {}) => {
    if (url.startsWith("https://fcm.googleapis.com/")) {
      const status = opts.status?.[url.split("/").pop() as string] ?? 201;
      if (status < 0) throw new Error("network");
      pushed.push({ endpoint: url, ttl: (init.headers as Record<string, string>).ttl });
      return new Response(null, { status });
    }
    feedURLs.push(url);
    return new Response(feed(opts.ids ?? ["3", "2", "1"]), { status: opts.feedStatus ?? 200, headers: { "content-type": "application/atom+xml" } });
  };
  const env = {
    PUSH_DB: db,
    PUSH_KV: kv,
    PUSH_KEY: "secret",
    PUSH_SUBJECT: "mailto:news@example.com",
    PUSH_VAPID_PUBLIC_KEY: VAPID_PUBLIC,
    PUSH_VAPID_PRIVATE_KEY: VAPID_PRIVATE,
    ...opts.env,
  };
  return { db, kv, pushed, feedURLs, fetchFn, env };
}

const call = (env: Record<string, unknown>, auth = "Bearer secret") =>
  notify(ctx(new Request("https://example.com/api/push/notify", { method: "POST", headers: auth ? { authorization: auth } : {} }), env));

test("notify needs PUSH_KEY, the right token, and the rest of the config", async (t) => {
  const s = setup();
  t.mock.method(globalThis, "fetch", s.fetchFn);
  assert.equal((await call({ ...s.env, PUSH_KEY: undefined })).status, 503);
  assert.equal((await call(s.env, "Bearer wrong")).status, 401);
  assert.equal((await call(s.env, "")).status, 401);
  assert.equal((await call(s.env, "Basic secret")).status, 401);
  assert.equal((await call({ ...s.env, PUSH_SUBJECT: "" })).status, 503);
  assert.equal(s.feedURLs.length, 0);
});

test("the first call only remembers the newest item", async (t) => {
  const s = setup();
  t.mock.method(globalThis, "fetch", s.fetchFn);
  const res = await call(s.env);
  assert.deepEqual(await res.json(), { sent: 0, removed: 0, items: [], primed: "3" });
  assert.equal(s.kv.data.get("last-item"), "3");
  assert.equal(s.pushed.length, 0);
  assert.deepEqual(s.feedURLs, ["https://example.com/feed.xml"]);
});

test("new items go to every subscriber, oldest first, capped", async (t) => {
  const s = setup({ ids: ["6", "5", "4", "3", "2"], last: "2", env: { PUSH_MAX_ITEMS: "2", PUSH_TTL: "600", PUSH_FEED_URL: "/blog/feed.xml" } });
  t.mock.method(globalThis, "fetch", s.fetchFn);
  const body = (await (await call(s.env)).json()) as Record<string, unknown>;
  assert.deepEqual(body, {
    sent: 6,
    failed: 0,
    removed: 0,
    items: [
      { title: "Post 6", url: "https://example.com/p/6/" },
      { title: "Post 5", url: "https://example.com/p/5/" },
    ],
  });
  assert.equal(s.kv.data.get("last-item"), "6");
  assert.equal(s.pushed.length, 6);
  assert.equal(s.pushed[0].ttl, "600");
  assert.deepEqual(s.feedURLs, ["https://example.com/blog/feed.xml"]);
});

test("gone subscriptions are removed; other failures are counted", async (t) => {
  const s = setup({ ids: ["4", "3"], last: "3", status: { a: 410, b: 500, c: -1 } });
  t.mock.method(globalThis, "fetch", s.fetchFn);
  const body = (await (await call(s.env)).json()) as Record<string, number>;
  assert.equal(body.sent, 0);
  assert.equal(body.failed, 2);
  assert.equal(body.removed, 1);
  assert.ok(!s.db.rows.has("https://fcm.googleapis.com/fcm/send/a"));
  assert.equal(s.db.rows.size, 2);
});

test("a 404 stops later items reaching the same subscription", async (t) => {
  const s = setup({ ids: ["5", "4", "3"], last: "3", status: { a: 404 } });
  t.mock.method(globalThis, "fetch", s.fetchFn);
  const body = (await (await call(s.env)).json()) as Record<string, number>;
  assert.equal(body.removed, 1);
  assert.equal(body.sent, 4);
  assert.equal(s.pushed.filter((p) => p.endpoint.endsWith("/a")).length, 1);
});

test("nothing new, a lost marker, an empty feed, a failing feed", async (t) => {
  const same = setup({ last: "3" });
  t.mock.method(globalThis, "fetch", same.fetchFn);
  assert.deepEqual(await (await call(same.env)).json(), { sent: 0, removed: 0, items: [] });

  const lost = setup({ last: "deleted" });
  t.mock.method(globalThis, "fetch", lost.fetchFn);
  assert.deepEqual(await (await call(lost.env)).json(), { sent: 0, removed: 0, items: [] });
  assert.equal(lost.kv.data.get("last-item"), "3");
  assert.equal(lost.pushed.length, 0);

  const empty = setup({ ids: [] });
  t.mock.method(globalThis, "fetch", empty.fetchFn);
  assert.deepEqual(await (await call(empty.env)).json(), { sent: 0, removed: 0, items: [] });

  const down = setup({ feedStatus: 500 });
  t.mock.method(globalThis, "fetch", down.fetchFn);
  const res = await call(down.env);
  assert.equal(res.status, 502);
  assert.deepEqual(await res.json(), { error: "feed_unavailable", status: 500 });
});
