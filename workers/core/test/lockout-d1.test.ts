// The shared-password lockout in CORE_DB (GO-149): the same rule as in
// memory, but held across isolates — a fresh in-memory store (a new isolate)
// does not reset it. Without CORE_DB the in-memory store is used, as before.
import { mock, test } from "node:test";
import assert from "node:assert/strict";
import * as mod from "../functions/api/core/module/index.ts";
import { memoryLockout } from "../functions/api/core/_lockout.ts";
import { d1Lockout, durableLockout } from "../functions/api/core/_lockout-d1.ts";
import { useLockoutStore } from "../functions/api/core/_password.ts";
import { PASSWORD, PW_ENV, basic, call, freshIP } from "./harness.ts";
import { fakeD1 } from "./d1.ts";

const get = (env: Record<string, unknown>, ip: string, password: string) =>
  call(mod.onRequest, { path: "/api/core/module", headers: { "cf-connecting-ip": ip, authorization: basic(password) }, env });

test("with CORE_DB the lock holds across isolates and ends with the window", async () => {
  mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-13T10:00:00Z") });
  try {
    const db = fakeD1();
    const env = { ...PW_ENV, CORE_DB: db, CORE_LOGIN_MAX_FAILURES: "3", CORE_LOGIN_WINDOW_MINUTES: "5" };
    const ip = freshIP();
    for (let i = 0; i < 2; i++) assert.equal((await get(env, ip, "wrong")).status, 401);
    useLockoutStore(memoryLockout()); // another isolate: its memory knows nothing
    assert.equal((await get(env, ip, "wrong")).status, 401);
    const locked = await get(env, ip, PASSWORD);
    assert.equal(locked.status, 429);
    assert.equal(locked.headers["retry-after"], "300");
    assert.equal((await get({ ...PW_ENV, CORE_LOGIN_MAX_FAILURES: "3" }, ip, PASSWORD)).status, 200, "without CORE_DB the memory store is used");
    assert.equal(db.rows("SELECT failures FROM core_lockout")[0].failures, 3);
    mock.timers.tick(300_000);
    assert.equal((await get(env, ip, PASSWORD)).status, 200, "the lock ends with the window");
    assert.equal(db.rows("SELECT COUNT(*) AS n FROM core_lockout")[0].n, 0, "a success clears the row");
  } finally {
    mock.timers.reset();
  }
});

test("d1Lockout: a new window restarts the count; ended rows are purged by the next failure", async () => {
  const db = fakeD1();
  const store = d1Lockout(db as never);
  await store.fail("a", 0, 2, 1000);
  assert.equal(await store.lockedFor("a", 1), 0);
  await store.fail("a", 2000, 2, 1000); // the old window is over: a count of 1 again
  assert.equal(await store.lockedFor("a", 2001), 0);
  await store.fail("a", 2500, 2, 1000);
  assert.equal(await store.lockedFor("a", 2600), 1, "locked until 3500: 0.9s, rounded up");
  await store.fail("b", 1, 1, 1000);
  assert.equal(await store.lockedFor("b", 2), 1, "max 1 locks on the first failure");
  await store.fail("c", 10_000, 5, 1000); // a and b have both ended by now
  assert.deepEqual(db.rows("SELECT key FROM core_lockout ORDER BY key"), [{ key: "c" }]);
  await store.clear("c");
  assert.equal(await store.lockedFor("c", 10_001), 0);
  assert.equal(durableLockout(db as never), durableLockout(db as never), "one store per database");
});

test("a database failure refuses the sign-in (500), it never lets the guess through", async () => {
  const db = fakeD1();
  db.state.failOn = /core_lockout/;
  const env = { ...PW_ENV, CORE_DB: db };
  const r = await get(env, freshIP(), PASSWORD);
  assert.equal(r.status, 500);
  assert.deepEqual(r.json(), { error: "internal_error" });
});
