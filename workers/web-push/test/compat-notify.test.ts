// Pins POST /api/push/notify's answers with the shared PUSH_KEY exactly as
// they were before the `auth` mode (GO-141): status, headers and body bytes.
// These must keep passing unedited.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as notify } from "../functions/api/push/notify.ts";
import { ctx, fakeDB, fakeKV } from "./fakes.ts";

const FEED = `<feed xmlns="http://www.w3.org/2005/Atom"><entry><title>One</title><link href="/p/1/"/><id>1</id></entry></feed>`;

function env(over: Record<string, unknown> = {}) {
  return {
    PUSH_DB: fakeDB(),
    PUSH_KV: fakeKV(),
    PUSH_KEY: "s3cret-key",
    PUSH_SUBJECT: "mailto:news@example.com",
    PUSH_VAPID_PUBLIC_KEY: "pub",
    PUSH_VAPID_PRIVATE_KEY: "priv",
    ...over,
  };
}

async function answer(e: Record<string, unknown>, headers: Record<string, string>) {
  const res = await notify(ctx(new Request("https://example.com/api/push/notify", { method: "POST", headers }), e));
  return [res.status, [...res.headers].sort(), await res.text()];
}

const JSON_HEADERS = [["cache-control", "no-store"], ["content-type", "application/json"]];

test("compat: no PUSH_KEY → 503 not_configured", async () => {
  assert.deepEqual(await answer(env({ PUSH_KEY: undefined }), { authorization: "Bearer s3cret-key" }), [503, JSON_HEADERS, '{"error":"not_configured"}']);
  assert.deepEqual(await answer(env({ PUSH_KEY: "" }), {}), [503, JSON_HEADERS, '{"error":"not_configured"}']);
});

test("compat: no key, a wrong key, another scheme → 401 unauthorized", async () => {
  for (const headers of [{}, { authorization: "Bearer wrong" }, { authorization: "Bearer s3cret-keyX" }, { authorization: "Basic s3cret-key" }, { authorization: "bearer s3cret-key" }] as Record<string, string>[]) {
    assert.deepEqual(await answer(env(), headers), [401, JSON_HEADERS, '{"error":"unauthorized"}'], JSON.stringify(headers));
  }
});

test("compat: the right key, the rest unconfigured → 503 not_configured", async () => {
  assert.deepEqual(await answer(env({ PUSH_SUBJECT: undefined }), { authorization: "Bearer s3cret-key" }), [503, JSON_HEADERS, '{"error":"not_configured"}']);
});

test("compat: the right key → the feed is read (first call primes)", async (t) => {
  t.mock.method(globalThis, "fetch", async () => new Response(FEED, { headers: { "content-type": "application/atom+xml" } }));
  assert.deepEqual(await answer(env(), { authorization: "Bearer s3cret-key" }), [200, JSON_HEADERS, '{"sent":0,"removed":0,"items":[],"primed":"1"}']);
});
