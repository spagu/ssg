// The handler around the gate: the debounce answer, a configuration problem
// in the dispatch (503, not 502) and the configured ref.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost } from "../functions/api/republish/index.ts";
import { BASE, answer, github } from "./helpers.ts";

const KEY = { authorization: "Bearer k3y-for-tests" };

test("a missing target is a 503 configuration answer", async () => {
  const [status, , body] = await answer(onRequestPost, { ...BASE, REPUBLISH_REPO: undefined }, { headers: KEY });
  assert.deepEqual([status, JSON.parse(body as string)], [503, { error: "dispatch failed", provider: "github", upstream_status: 503, detail: "REPUBLISH_REPO (owner/repo) is required" }]);
});

test("the configured ref is reported; a second call inside the window is throttled", async (t) => {
  github(t);
  const kv = new Map<string, string>();
  const env = { ...BASE, REPUBLISH_REF: "release", REPUBLISH_MIN_INTERVAL_SEC: "60", REPUBLISH_KV: { get: async (k: string) => kv.get(k) ?? null, put: async (k: string, v: string) => void kv.set(k, v) } };
  const first = await answer(onRequestPost, env, { headers: KEY });
  assert.deepEqual([first[0], JSON.parse(first[2] as string)], [202, { ok: true, provider: "github", ref: "release" }]);
  const second = await answer(onRequestPost, env, { headers: KEY });
  assert.equal(second[0], 429);
});
