// Pins /api/republish's answers with the shared REPUBLISH_KEY exactly as they
// were before the `auth` mode (GO-141): status, headers and body bytes, for
// every way the key can be presented. These must keep passing unedited. Run
// from the repository root:
//   node --import ./workers/republish-trigger/test/resolve.mjs --test 'workers/republish-trigger/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet, onRequestPost } from "../functions/api/republish/index.ts";
import { BASE, answer, github } from "./helpers.ts";

const H = [["cache-control", "no-store"], ["content-type", "application/json"]];
const OK: unknown[] = [202, H, '{"ok":true,"provider":"github","ref":"main"}'];

test("compat: missing REPUBLISH_KEY, TOKEN or PROVIDER → 503", async () => {
  for (const missing of ["REPUBLISH_KEY", "REPUBLISH_TOKEN", "REPUBLISH_PROVIDER"]) {
    const env = { ...BASE, [missing]: undefined };
    assert.deepEqual(await answer(onRequestPost, env, { headers: { authorization: "Bearer k3y-for-tests" } }), [503, H, '{"error":"republish not configured"}'], missing);
  }
});

test("compat: no key, a wrong key, another scheme → 401", async (t) => {
  const calls = github(t);
  for (const headers of [{}, { authorization: "Bearer nope" }, { authorization: "Basic k3y-for-tests" }, { "x-republish-key": "nope" }] as Record<string, string>[]) {
    assert.deepEqual(await answer(onRequestPost, BASE, { headers }), [401, H, '{"error":"unauthorized"}'], JSON.stringify(headers));
  }
  assert.deepEqual(await answer(onRequestPost, BASE, { url: "https://example.com/api/republish?key=nope" }), [401, H, '{"error":"unauthorized"}']);
  assert.equal(calls.length, 0);
});

test("compat: the right key as Bearer, X-Republish-Key or ?key= → 202 and one dispatch each", async (t) => {
  const calls = github(t);
  assert.deepEqual(await answer(onRequestPost, BASE, { headers: { authorization: "Bearer k3y-for-tests" } }), OK);
  assert.deepEqual(await answer(onRequestPost, BASE, { headers: { "x-republish-key": "k3y-for-tests" } }), OK);
  assert.deepEqual(await answer(onRequestPost, BASE, { url: "https://example.com/api/republish?key=k3y-for-tests" }), OK);
  assert.deepEqual(calls, Array(3).fill("https://api.github.com/repos/owner/site/actions/workflows/deploy.yml/dispatches"));
});

test("compat: GET is 405 unless REPUBLISH_ALLOW_GET, then it is the same gate", async (t) => {
  github(t);
  const url = "https://example.com/api/republish?key=k3y-for-tests";
  assert.deepEqual(await answer(onRequestGet, BASE, { url, method: "GET" }), [405, H, '{"error":"use POST (set REPUBLISH_ALLOW_GET=true to allow GET triggering)"}']);
  const open = { ...BASE, REPUBLISH_ALLOW_GET: "true" };
  assert.deepEqual(await answer(onRequestGet, open, { url, method: "GET" }), OK);
  assert.deepEqual(await answer(onRequestGet, open, { url: "https://example.com/api/republish", method: "GET" }), [401, H, '{"error":"unauthorized"}']);
});

test("compat: the provider refusing → 502 with its status", async (t) => {
  github(t, 422);
  assert.deepEqual(await answer(onRequestPost, BASE, { headers: { authorization: "Bearer k3y-for-tests" } }), [
    502,
    H,
    '{"error":"dispatch failed","provider":"github","upstream_status":422,"detail":"nope"}',
  ]);
});
