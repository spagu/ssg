// The shared-password lockout: checked before the password, per address,
// for one window; the rate-limit binding as a ceiling; the memory bound.
import { mock, test } from "node:test";
import assert from "node:assert/strict";
import * as mod from "../functions/api/core/module/index.ts";
import { MAX_ENTRIES, memoryLockout } from "../functions/api/core/_lockout.ts";
import { useLockoutStore } from "../functions/api/core/_password.ts";
import { PASSWORD, PW_ENV, basic, call, freshIP } from "./harness.ts";

const get = (env: Record<string, unknown>, ip: string, password: string) =>
  call(mod.onRequest, { path: "/api/core/module", headers: { "cf-connecting-ip": ip, authorization: basic(password) }, env });

test("after max failures the address is locked — even the right password gets 429 — until the window ends", async () => {
  mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-13T10:00:00Z") });
  try {
    const env = { ...PW_ENV, CORE_LOGIN_MAX_FAILURES: "3", CORE_LOGIN_WINDOW_MINUTES: "5" };
    const ip = freshIP();
    for (let i = 0; i < 3; i++) assert.equal((await get(env, ip, "wrong")).status, 401);
    const locked = await get(env, ip, PASSWORD);
    assert.equal(locked.status, 429);
    assert.equal(locked.json().error, "too_many_failed_logins");
    assert.equal(locked.headers["retry-after"], "300");
    assert.equal((await get(env, freshIP(), PASSWORD)).status, 200, "another address is not affected");
    mock.timers.tick(120_000);
    assert.equal((await get(env, ip, PASSWORD)).headers["retry-after"], "180");
    mock.timers.tick(180_000);
    assert.equal((await get(env, ip, PASSWORD)).status, 200, "the lock ends with the window");
  } finally {
    mock.timers.reset();
  }
});

test("a success clears the count; failures in an old window start a new one", async () => {
  mock.timers.enable({ apis: ["Date"], now: 0 });
  try {
    const env = { ...PW_ENV, CORE_LOGIN_MAX_FAILURES: "2" };
    const ip = freshIP();
    assert.equal((await get(env, ip, "wrong")).status, 401);
    assert.equal((await get(env, ip, PASSWORD)).status, 200);
    assert.equal((await get(env, ip, "wrong")).status, 401);
    mock.timers.tick(10 * 60_000); // default window: 10 minutes
    assert.equal((await get(env, ip, "wrong")).status, 401, "the earlier failure fell out of the window");
    assert.equal((await get(env, ip, PASSWORD)).status, 200, "one failure in this window does not lock");
    assert.equal((await get(env, ip, "wrong")).status, 401);
    assert.equal((await get(env, ip, "wrong")).status, 401);
    assert.equal((await get(env, ip, PASSWORD)).status, 429, "the second failure in one window locks");
  } finally {
    mock.timers.reset();
  }
});

test("the rate-limit binding is asked for every password attempt and refuses before the password is compared", async () => {
  const keys: string[] = [];
  let allow = true;
  const env = { ...PW_ENV, CORE_LOGIN_LIMITER: { limit: async ({ key }: { key: string }) => (keys.push(key), { success: allow }) } };
  const ip = freshIP();
  assert.equal((await get(env, ip, PASSWORD)).status, 200);
  allow = false;
  const r = await get(env, ip, PASSWORD);
  assert.deepEqual([r.status, r.headers["retry-after"]], [429, "60"]);
  assert.deepEqual(keys, [`core-login:${ip}`, `core-login:${ip}`]);
  assert.equal((await call(mod.onRequest, { path: "/api/core/module", headers: { "cf-connecting-ip": ip }, env })).status, 401, "a request without a credential is not counted");
  assert.equal(keys.length, 2);
});

test("the memory store stays bounded, dropping the least recently failed address", async () => {
  const store = memoryLockout();
  for (let i = 0; i <= MAX_ENTRIES; i++) await store.fail(`k${i}`, 0, 1, 1000);
  assert.equal(store.size(), MAX_ENTRIES);
  assert.equal(await store.lockedFor("k0", 0), 0, "the oldest entry went first");
  assert.equal(await store.lockedFor(`k${MAX_ENTRIES}`, 0), 1);
  await store.clear("k1");
  assert.equal(store.size(), MAX_ENTRIES - 1);
});

test("useLockoutStore swaps the storage (the seam for a durable store)", async () => {
  const seen: string[] = [];
  const fake = { lockedFor: async () => 7, fail: async () => void seen.push("fail"), clear: async () => void seen.push("clear") };
  const previous = useLockoutStore(fake);
  try {
    const r = await get(PW_ENV, freshIP(), PASSWORD);
    assert.deepEqual([r.status, r.headers["retry-after"]], [429, "7"]);
  } finally {
    assert.equal(useLockoutStore(previous), fake);
  }
});
