// /api/republish with service tokens from `auth` (GO-141): accepted with
// republish-trigger:run, refused without it, never from a cookie; beside the
// shared key when both are set; half-configured → 503.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet, onRequestPost } from "../functions/api/republish/index.ts";
import { BASE, answer, github } from "./helpers.ts";
import { authVars, mint } from "./auth-tokens.ts";

const H = [["cache-control", "no-store"], ["content-type", "application/json"]];
const OK: unknown[] = [202, H, '{"ok":true,"provider":"github","ref":"main"}'];
const AUTH_ONLY = { ...BASE, REPUBLISH_KEY: undefined, ...authVars("REPUBLISH") };
const BOTH = { ...BASE, ...authVars("REPUBLISH") };
const bearer = async (perms: string[], over = {}) => ({ authorization: `Bearer ${await mint(perms, over)}` });
const reason = (a: unknown[]) => [a[0], JSON.parse(a[2] as string)];

test("auth only: a service token with republish-trigger:run dispatches", async (t) => {
  const calls = github(t);
  assert.deepEqual(await answer(onRequestPost, AUTH_ONLY, { headers: await bearer(["republish-trigger:run"]) }), OK);
  assert.deepEqual(await answer(onRequestGet, { ...AUTH_ONLY, REPUBLISH_ALLOW_GET: "true" }, { method: "GET", headers: await bearer(["*"]) }), OK);
  assert.equal(calls.length, 2);
});

test("auth only: without the permission → 403 naming it, nothing dispatched", async (t) => {
  const calls = github(t);
  assert.deepEqual(reason(await answer(onRequestPost, AUTH_ONLY, { headers: await bearer(["web-push:notify"]) })), [403, { error: "forbidden", permission: "republish-trigger:run" }]);
  assert.equal(calls.length, 0);
});

test("auth only: no credential, a key nobody configured, a user token, a cookie → 401 JSON", async (t) => {
  const calls = github(t);
  const user = await mint(["republish-trigger:run"], { kind: undefined });
  const cases: [Record<string, string>, string, string?][] = [
    [{}, "no_session"],
    [{ "x-republish-key": "k3y-for-tests" }, "no_session"],
    [{}, "no_session", "https://example.com/api/republish?key=k3y-for-tests"],
    [{ authorization: `Bearer ${user}` }, "invalid_session"],
    [{ cookie: `__Host-ssg-auth=${user}`, accept: "text/html" }, "no_session"],
  ];
  for (const [headers, why, url] of cases) {
    assert.deepEqual(reason(await answer(onRequestPost, AUTH_ONLY, { headers, url })), [401, { error: "unauthorized", reason: why }], JSON.stringify(headers));
  }
  assert.equal(calls.length, 0);
});

test("both set: the key in each of its forms and a service token all work", async (t) => {
  github(t);
  assert.deepEqual(await answer(onRequestPost, BOTH, { headers: { authorization: "Bearer k3y-for-tests" } }), OK);
  assert.deepEqual(await answer(onRequestPost, BOTH, { headers: { "x-republish-key": "k3y-for-tests" } }), OK);
  assert.deepEqual(await answer(onRequestPost, BOTH, { url: "https://example.com/api/republish?key=k3y-for-tests" }), OK);
  assert.deepEqual(await answer(onRequestPost, BOTH, { headers: await bearer(["republish-trigger:run"]) }), OK);
  assert.deepEqual(reason(await answer(onRequestPost, BOTH, { headers: { authorization: "Bearer nope" } })), [401, { error: "unauthorized", reason: "invalid_session" }]);
});

test("half-configured auth → 503 naming what is missing, even with the right key", async () => {
  const half = { ...BOTH, REPUBLISH_AUTH_ISSUER: "" };
  assert.deepEqual(reason(await answer(onRequestPost, half, { headers: { authorization: "Bearer k3y-for-tests" } })), [
    503,
    { error: "not_configured", detail: "REPUBLISH_AUTH_ISSUER is not set" },
  ]);
});

test("auth set, provider not → 503 as before", async () => {
  for (const missing of ["REPUBLISH_TOKEN", "REPUBLISH_PROVIDER"]) {
    assert.deepEqual(await answer(onRequestPost, { ...AUTH_ONLY, [missing]: undefined }, { headers: await bearer(["republish-trigger:run"]) }), [503, H, '{"error":"republish not configured"}']);
  }
});
