// POST /api/push/notify with service tokens from `auth` (GO-141): accepted
// with web-push:notify, refused without it, never from a cookie; beside the
// shared key when both are set; half-configured → 503.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as notify } from "../functions/api/push/notify.ts";
import { ctx, fakeDB, fakeKV } from "./fakes.ts";
import { authVars, mint } from "./auth-tokens.ts";

const FEED = `<feed xmlns="http://www.w3.org/2005/Atom"><entry><title>One</title><link href="/p/1/"/><id>1</id></entry></feed>`;
const PRIMED = '{"sent":0,"removed":0,"items":[],"primed":"1"}';

function env(over: Record<string, unknown> = {}) {
  return {
    PUSH_DB: fakeDB(),
    PUSH_KV: fakeKV(),
    PUSH_SUBJECT: "mailto:news@example.com",
    PUSH_VAPID_PUBLIC_KEY: "pub",
    PUSH_VAPID_PRIVATE_KEY: "priv",
    ...authVars("PUSH"),
    ...over,
  };
}

async function call(e: Record<string, unknown>, headers: Record<string, string> = {}, accept = "") {
  const h = accept ? { ...headers, accept } : headers;
  const res = await notify(ctx(new Request("https://example.com/api/push/notify", { method: "POST", headers: h }), e));
  return { status: res.status, body: await res.text(), res };
}

const feed = (t: { mock: { method: (o: object, n: string, f: unknown) => unknown } }) =>
  t.mock.method(globalThis, "fetch", async () => new Response(FEED, { headers: { "content-type": "application/atom+xml" } }));

test("auth only: a service token with web-push:notify is accepted", async (t) => {
  feed(t);
  assert.deepEqual([(await call(env(), { authorization: `Bearer ${await mint(["web-push:notify"])}` })).body], [PRIMED]);
  assert.equal((await call(env(), { authorization: `Bearer ${await mint(["web-push:*"])}` })).status, 200, "a wildcard covers it");
});

test("auth only: without the permission → 403 naming it", async () => {
  const r = await call(env(), { authorization: `Bearer ${await mint(["newsletter:read"])}` });
  assert.deepEqual([r.status, JSON.parse(r.body)], [403, { error: "forbidden", permission: "web-push:notify" }]);
});

test("auth only: no header, Basic, junk, expired, a user token → 401 JSON, never a redirect", async () => {
  const user = await mint(["web-push:notify"], { kind: undefined });
  const expired = await mint(["web-push:notify"], { exp: Math.floor(Date.now() / 1000) - 60 });
  const cases: [Record<string, string>, string][] = [
    [{}, "no_session"],
    [{ authorization: "Basic eDp5" }, "no_session"],
    [{ authorization: "Bearer junk" }, "invalid_session"],
    [{ authorization: `Bearer ${expired}` }, "expired"],
    [{ authorization: `Bearer ${user}` }, "invalid_session"],
  ];
  for (const [headers, reason] of cases) {
    const r = await call(env(), headers, "text/html");
    assert.deepEqual([r.status, JSON.parse(r.body)], [401, { error: "unauthorized", reason }], JSON.stringify(headers));
  }
});

test("a person's session cookie is not read, even holding the permission", async () => {
  const cookie = `__Host-ssg-auth=${await mint(["web-push:notify"], { kind: undefined })}`;
  const r = await call(env(), { cookie });
  assert.deepEqual([r.status, JSON.parse(r.body).reason], [401, "no_session"]);
  const svc = `__Host-ssg-auth=${await mint(["web-push:notify"])}`;
  assert.equal((await call(env(), { cookie: svc })).status, 401, "nor a service token put in the cookie");
});

test("both set: the key and a service token both work; a wrong key gets auth's answer", async (t) => {
  feed(t);
  const both = env({ PUSH_KEY: "s3cret-key" });
  assert.equal((await call(both, { authorization: "Bearer s3cret-key" })).body, PRIMED);
  assert.equal((await call(env({ PUSH_KEY: "s3cret-key" }), { authorization: `Bearer ${await mint(["web-push:notify"])}` })).body, PRIMED);
  const wrong = await call(both, { authorization: "Bearer s3cret-keyX" });
  assert.deepEqual([wrong.status, JSON.parse(wrong.body)], [401, { error: "unauthorized", reason: "invalid_session" }]);
});

test("half-configured auth → 503 naming what is missing, even with the right key", async () => {
  const half = env({ PUSH_KEY: "s3cret-key", PUSH_AUTH_PUBLIC_KEY: undefined });
  const r = await call(half, { authorization: "Bearer s3cret-key" });
  assert.equal(r.status, 503);
  assert.deepEqual(JSON.parse(r.body), { error: "not_configured", detail: "set exactly one of PUSH_AUTH_PUBLIC_KEY and PUSH_AUTH_JWKS_URL" });
});

test("auth set, the rest of web-push not: the caller is checked first, then 503", async () => {
  const r = await call(env({ PUSH_SUBJECT: undefined }), { authorization: `Bearer ${await mint(["web-push:notify"])}` });
  assert.deepEqual([r.status, r.body], [503, '{"error":"not_configured"}']);
});
