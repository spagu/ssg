// The four routes, against in-memory bindings. Run from the repository root:
//   node --import ./workers/web-push/test/resolve.mjs --test 'workers/web-push/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as key } from "../functions/api/push/key.ts";
import { onRequestPost as subscribe } from "../functions/api/push/subscribe.ts";
import { onRequestPost as unsubscribe } from "../functions/api/push/unsubscribe.ts";
import { ctx, fakeDB, SUB } from "./fakes.ts";

const ORIGIN = "https://example.com";
const post = (path: string, body: unknown, headers: Record<string, string> = {}) =>
  new Request(ORIGIN + path, { method: "POST", body: typeof body === "string" ? body : JSON.stringify(body), headers });

test("key returns the public key, cacheable", async () => {
  const res = await key(ctx(new Request(ORIGIN + "/api/push/key"), { PUSH_VAPID_PUBLIC_KEY: "BPUB" }));
  assert.equal(res.status, 200);
  assert.deepEqual(await res.json(), { publicKey: "BPUB" });
  assert.equal(res.headers.get("cache-control"), "public, max-age=300");
  const missing = await key(ctx(new Request(ORIGIN + "/api/push/key"), {}));
  assert.equal(missing.status, 503);
});

test("subscribe stores a valid subscription, once per endpoint", async () => {
  const db = fakeDB();
  const env = { PUSH_DB: db, PUSH_HASH_SALT: "salt" };
  const res = await subscribe(ctx(post("/api/push/subscribe", SUB, { origin: ORIGIN, "user-agent": "UA" }), env));
  assert.equal(res.status, 201);
  await subscribe(ctx(post("/api/push/subscribe", SUB), env));
  assert.equal(db.rows.size, 1);
  assert.match(String(db.rows.get(SUB.endpoint)?.ua_hash ?? ""), /^$|^[0-9a-f]{64}$/);
});

test("subscribe hashes the User-Agent only with a salt", async () => {
  const db = fakeDB();
  await subscribe(ctx(post("/api/push/subscribe", SUB, { "user-agent": "UA" }), { PUSH_DB: db, PUSH_HASH_SALT: "s" }));
  assert.match(db.rows.get(SUB.endpoint)!.ua_hash!, /^[0-9a-f]{64}$/);
  const plain = fakeDB();
  await subscribe(ctx(post("/api/push/subscribe", SUB, { "user-agent": "UA" }), { PUSH_DB: plain }));
  assert.equal(plain.rows.get(SUB.endpoint)!.ua_hash, null);
});

test("subscribe refuses other origins, missing config and bad subscriptions", async () => {
  const env = { PUSH_DB: fakeDB() };
  assert.equal((await subscribe(ctx(post("/api/push/subscribe", SUB, { origin: "https://evil.example.com" }), env))).status, 403);
  assert.equal((await subscribe(ctx(post("/api/push/subscribe", SUB), {}))).status, 503);
  const bad = [
    "not json",
    "[1]",
    "x".repeat(5000),
    {},
    { ...SUB, endpoint: 5 },
    { ...SUB, endpoint: "https://fcm.googleapis.com/" + "a".repeat(2100) },
    { endpoint: SUB.endpoint },
    { ...SUB, keys: { p256dh: 1, auth: SUB.keys.auth } },
    { ...SUB, keys: { p256dh: "A".repeat(200), auth: SUB.keys.auth } },
    { ...SUB, endpoint: "not a url" },
    { ...SUB, endpoint: "http://fcm.googleapis.com/x" },
    { ...SUB, endpoint: "https://user:pw@fcm.googleapis.com/x" },
    { ...SUB, endpoint: "https://internal.example.com/x" },
    { ...SUB, endpoint: "https://evilfcm.googleapis.com.example.com/x" },
    { ...SUB, keys: { p256dh: "AAAA", auth: SUB.keys.auth } },
    { ...SUB, keys: { p256dh: "!!", auth: SUB.keys.auth } },
    { ...SUB, keys: { p256dh: SUB.keys.p256dh.replace(/^BC/, "Ac"), auth: SUB.keys.auth } },
    { ...SUB, keys: { p256dh: SUB.keys.p256dh, auth: "AAAA" } },
  ];
  for (const body of bad) {
    const res = await subscribe(ctx(post("/api/push/subscribe", body), env));
    assert.equal(res.status, 422, JSON.stringify(body).slice(0, 80));
  }
});

test("subscribe accepts every default push service and an override list", async () => {
  for (const host of ["fcm.googleapis.com", "updates.push.services.mozilla.com", "web.push.apple.com", "wns2-par02p.notify.windows.com"]) {
    const res = await subscribe(ctx(post("/api/push/subscribe", { ...SUB, endpoint: `https://${host}/x` }), { PUSH_DB: fakeDB() }));
    assert.equal(res.status, 201, host);
  }
  const env = { PUSH_DB: fakeDB(), PUSH_ENDPOINT_HOSTS: " push.example.com , " };
  assert.equal((await subscribe(ctx(post("/api/push/subscribe", { ...SUB, endpoint: "https://push.example.com/x" }), env))).status, 201);
  assert.equal((await subscribe(ctx(post("/api/push/subscribe", SUB), env))).status, 422);
});

test("unsubscribe answers the same for known and unknown endpoints", async () => {
  const db = fakeDB();
  const env = { PUSH_DB: db };
  await subscribe(ctx(post("/api/push/subscribe", SUB), env));
  for (const endpoint of [SUB.endpoint, "https://fcm.googleapis.com/unknown"]) {
    const res = await unsubscribe(ctx(post("/api/push/unsubscribe", { endpoint }), env));
    assert.equal(res.status, 200);
    assert.deepEqual(await res.json(), { ok: true });
  }
  assert.equal(db.rows.size, 0);
  assert.equal((await unsubscribe(ctx(post("/api/push/unsubscribe", {}), env))).status, 400);
  assert.equal((await unsubscribe(ctx(post("/api/push/unsubscribe", { endpoint: SUB.endpoint }, { origin: "https://evil.example.com" }), env))).status, 403);
  assert.equal((await unsubscribe(ctx(post("/api/push/unsubscribe", { endpoint: SUB.endpoint }), {}))).status, 503);
});
