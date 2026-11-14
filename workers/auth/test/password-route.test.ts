// POST /api/auth/password: change your own password.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as changePassword } from "../functions/api/auth/password.ts";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { onRequestPost as login } from "../functions/api/auth/login.ts";
import { parseHash } from "../functions/api/auth/_password.ts";
import { Browser, PASSWORD, call, counting, makeEnv, useClock } from "./fakes.ts";
import { loggedIn } from "./seed.ts";

const NEW = "a brand new passphrase";
type Env = ReturnType<typeof makeEnv>;
const change = (env: Env, b: Browser, current: string, password: string) =>
  call(changePassword, env, "/api/auth/password", { json: { current, password }, browser: b });

test("password: changed, other sessions revoked, this one kept, old password dead", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  const other = new Browser("198.51.100.30");
  await call(login, env, "/api/auth/login", { json: { email: "ada@example.com", password: PASSWORD }, browser: other });
  env.AUTH_DB.sqlite.exec("UPDATE users SET must_change_password = 1");
  const res = await change(env, browser, PASSWORD, NEW);
  assert.deepEqual(await res.json(), { ok: true, revoked: 1 });
  assert.equal((await call(refresh, env, "/api/auth/refresh", { method: "POST", browser: other })).status, 401);
  assert.equal((await call(refresh, env, "/api/auth/refresh", { method: "POST", browser })).status, 200);
  assert.equal((env.AUTH_DB.sqlite.prepare("SELECT must_change_password FROM users").get() as { must_change_password: number }).must_change_password, 0);
  const attempt = (password: string) => call(login, env, "/api/auth/login", { json: { email: "ada@example.com", password }, browser: new Browser() });
  assert.equal((await attempt(PASSWORD)).status, 401);
  assert.equal((await attempt(NEW)).status, 200);
});

test("password: refused without a session, with a wrong current one (counted), or breaking a rule", async () => {
  useClock();
  const env = makeEnv({ AUTH_LOCK_ACCOUNT_FAILURES: "2" });
  const { browser } = await loggedIn(env);
  assert.deepEqual(await (await change(env, new Browser(), PASSWORD, NEW)).json(), { error: "not_authenticated" });
  assert.deepEqual(await (await change(env, browser, PASSWORD, "too short")).json(), { error: "password_too_short" });
  assert.deepEqual(await (await change(env, browser, PASSWORD, " ADA@example.com ")).json(), { error: "password_equals_email" });
  const wrong = await change(env, browser, "not the password", NEW);
  assert.equal(wrong.status, 401);
  assert.deepEqual(await wrong.json(), { error: "invalid_credentials" });
  assert.equal((await change(env, browser, "not the password", NEW)).status, 401);
  const locked = await change(env, browser, PASSWORD, NEW);
  assert.equal(locked.status, 429);
  assert.ok(Number(locked.headers.get("retry-after")) > 0);
});

test("password: as a plain form — 303 with ok or error", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  const form = (fields: Record<string, string>) => call(changePassword, env, "/api/auth/password", { form: fields, browser });
  const bad = await form({ current: PASSWORD, password: "short" });
  assert.equal(bad.status, 303);
  assert.equal(bad.headers.get("location"), "/auth/account?error=password_too_short");
  const ok = await form({ current: PASSWORD, password: NEW, next: "/me" });
  assert.equal(ok.headers.get("location"), "/me?ok=password_changed");
});

test("password: a cost the runtime refuses is a 503 with a hint, and nothing changes", async () => {
  useClock();
  const env = makeEnv({ AUTH_PBKDF2_ITERATIONS: "20000" });
  const c = counting(15_000);
  const warn = console.warn;
  console.warn = () => {};
  try {
    const { browser } = await loggedIn(env);
    const before = env.AUTH_DB.sqlite.prepare("SELECT password_hash FROM users").get() as { password_hash: string };
    const res = await change(env, browser, PASSWORD, NEW);
    assert.equal(res.status, 503);
    assert.match((await res.json()).hint, /AUTH_PBKDF2_ITERATIONS/);
    const after = env.AUTH_DB.sqlite.prepare("SELECT password_hash FROM users").get() as { password_hash: string };
    assert.equal(after.password_hash, before.password_hash);
    assert.equal(parseHash(after.password_hash)?.iterations, 10_000);
  } finally {
    c.restore();
    console.warn = warn;
  }
});
