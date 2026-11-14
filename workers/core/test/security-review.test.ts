// Regression tests for the security review (workers/auth/docs/SECURITY-REVIEW.md).
import { test } from "node:test";
import assert from "node:assert/strict";
import * as mod from "../functions/api/core/module/index.ts";
import { PW_ENV, basic, call, freshIP } from "./harness.ts";
import { fakeD1 } from "./d1.ts";

const get = (env: Record<string, unknown>, ip: string, password: string) =>
  call(mod.onRequest, { path: "/api/core/module", headers: { "cf-connecting-ip": ip, authorization: basic(password) }, env });

for (const [name, extra] of [
  ["memory", () => ({})],
  ["CORE_DB", () => ({ CORE_DB: fakeD1() })],
] as const) {
  test(`SR-12: parallel wrong passwords are counted before they are compared (${name} store)`, async () => {
    const env = { ...PW_ENV, ...extra(), CORE_LOGIN_MAX_FAILURES: "3" };
    const ip = freshIP();
    const res = await Promise.all(Array.from({ length: 30 }, (_, i) => get(env, ip, `wrong-${i}`)));
    const n = res.reduce<Record<number, number>>((m, r) => ((m[r.status] = (m[r.status] ?? 0) + 1), m), {});
    assert.ok((n[401] ?? 0) <= 3, `at most 3 compared, got ${JSON.stringify(n)}`);
    assert.equal((n[401] ?? 0) + (n[429] ?? 0), 30);
  });
}

test("SR-12: the right password sent after a burst that reached the limit is refused like a wrong one", async () => {
  const env = { ...PW_ENV, CORE_LOGIN_MAX_FAILURES: "3" };
  const ip = freshIP();
  const res = await Promise.all([...Array.from({ length: 5 }, (_, i) => get(env, ip, `wrong-${i}`)), get(env, ip, PW_ENV.CORE_ADMIN_PASSWORD)]);
  assert.equal(res.at(-1)?.status, 429);
});
