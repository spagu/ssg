// GO-139 with GO-140: Access is a first factor like any other. An account
// with a second factor gets the challenge, not a session, after Access; and
// linking Access is a change of sign-in methods, so it needs a recent second
// factor (step-up) from an account that has one.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ACCESS_COOKIE } from "../functions/api/auth/_cookies.ts";
import { onRequestGet as linkGet } from "../functions/auth/access/link.ts";
import { onRequestDelete as methodDelete } from "../functions/api/auth/methods/[id].ts";
import { Browser, T0, call, useClock } from "./fakes.ts";
import { loggedIn } from "./seed.ts";
import { count, providerUser } from "./go151.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { FakeAccess, PERSON, accessEnv, enter, mint, where } from "./access-fake.ts";

const MFA = { AUTH_MFA_KEY: "k".repeat(40) } as Partial<Env>;
const addFactor = (env: Env, userId: string) =>
  (env.AUTH_DB as unknown as { sqlite: { prepare(s: string): { run(...a: unknown[]): unknown } } }).sqlite
    .prepare("INSERT INTO mfa_factors (id, user_id, kind, name, confirmed_at, created_at) VALUES (?, ?, 'totp', 'phone', ?, ?)")
    .run(crypto.randomUUID(), userId, new Date(T0).toISOString(), new Date(T0).toISOString());

test("an account with a second factor: Access leads to the challenge, never straight to a session", async () => {
  useClock();
  const env = accessEnv(MFA);
  const fake = new FakeAccess().install();
  try {
    const id = await providerUser(env, "ada@example.com", "access", PERSON.sub);
    addFactor(env, id);
    const browser = new Browser();
    const res = await enter(env, await mint(), { browser, next: "/x" });
    assert.equal(where(res), "/auth/mfa");
    assert.ok(!browser.jar.has(ACCESS_COOKIE), "no session cookie before the second factor");
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM sessions"), 0);
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM mfa_pending WHERE method = 'access'"), 1);
  } finally {
    fake.restore();
  }
});

test("linking Access from an account with a second factor needs a recent second factor (step-up)", async () => {
  useClock();
  const env = accessEnv(MFA);
  const fake = new FakeAccess().install();
  try {
    const { browser, id } = await loggedIn(env);
    addFactor(env, id); // the session predates the factor: no second factor passed in it
    const res = await call(linkGet, env, "/auth/access/link", { browser, headers: { "cf-access-jwt-assertion": await mint() }, csrf: false });
    assert.equal(where(res), "/auth/mfa/step-up?next=%2Fauth%2Faccount");
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM identities"), 0);
    // The same rule as removing any method (_provider-account.ts changeGate).
    const del = await call(methodDelete, env, "/api/auth/methods/password", { method: "DELETE", browser, params: { id: "password" } });
    assert.equal(del.status, 403);
    assert.equal((await del.json()).error, "step_up_required");
    const viaForm = await call(methodDelete, env, "/api/auth/methods/password", { form: {}, method: "DELETE", browser, params: { id: "password" } });
    assert.equal(where(viaForm), "/auth/mfa/step-up?next=%2Fauth%2Faccount");
  } finally {
    fake.restore();
  }
});
