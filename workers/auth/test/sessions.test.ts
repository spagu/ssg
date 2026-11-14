// Sessions: who-am-I, refresh rotation with theft detection and the two-tab
// grace, expiry. Logout and logout-all are in logout.test.ts.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as session } from "../functions/api/auth/session.ts";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { ACCESS_COOKIE, REFRESH_COOKIE } from "../functions/api/auth/_cookies.ts";
import { signToken } from "../functions/api/auth/_jwt.ts";
import { loadKeys } from "../functions/api/auth/_keys.ts";
import type { KeySet } from "../functions/api/auth/_keys.ts";
import { Browser, T0, call, makeEnv, useClock } from "./fakes.ts";
import { loggedIn } from "./seed.ts";

type Env = ReturnType<typeof makeEnv>;
const me = (env: Env, b: Browser) => call(session, env, "/api/auth/session", { browser: b });
const doRefresh = (env: Env, b: Browser) => call(refresh, env, "/api/auth/refresh", { method: "POST", browser: b });
const clone = (b: Browser) => Object.assign(new Browser(b.ip), { jar: new Map(b.jar) });
const sessionRow = (env: Env) => env.AUTH_DB.sqlite.prepare("SELECT revoked_at, revoke_reason FROM sessions").get() as Record<string, string | null>;

test("session: who am I, and nothing secret in the answer", async () => {
  useClock();
  const env = makeEnv();
  const { browser, id } = await loggedIn(env);
  const res = await me(env, browser);
  assert.equal(res.status, 200);
  const text = await res.text();
  assert.deepEqual(JSON.parse(text), {
    user: { id, email: "ada@example.com", name: "Ada", mustChangePassword: false },
    expiresAt: T0 / 1000 + 600,
    sessionExpiresAt: new Date(T0 + 14 * 86400_000).toISOString(),
  });
  assert.doesNotMatch(text, /pbkdf2|refresh|eyJ/);
  assert.equal((await me(env, new Browser())).status, 401);
});

test("session: an expired access token, a revoked session, a blocked account or a foreign sub → 401", async () => {
  const clock = useClock();
  const env = makeEnv();
  const { browser, id } = await loggedIn(env);
  clock.advance(11 * 60_000);
  assert.equal((await me(env, browser)).status, 401, "expired");
  assert.equal((await doRefresh(env, browser)).status, 200);
  assert.equal((await me(env, browser)).status, 200, "refreshed");
  env.AUTH_DB.sqlite.exec("UPDATE users SET status = 'blocked'");
  assert.equal((await me(env, browser)).status, 401, "blocked");
  env.AUTH_DB.sqlite.exec("UPDATE users SET status = 'active'");
  // A validly signed token whose sub is not the session's owner.
  const keys = (await loadKeys(env)) as KeySet;
  const sid = (env.AUTH_DB.sqlite.prepare("SELECT id FROM sessions").get() as { id: string }).id;
  const claims = { iss: "https://example.com/api/auth", aud: "example.com", sub: "other", sid, email: "x@example.com", iat: clock.now / 1000, exp: clock.now / 1000 + 60, ver: 1, perms: [] };
  const forged = Object.assign(new Browser(), { jar: new Map([[ACCESS_COOKIE, await signToken(keys.signing, claims)]]) });
  assert.equal((await me(env, forged)).status, 401);
  assert.equal((await me(env, Object.assign(new Browser(), { jar: new Map([[ACCESS_COOKIE, await signToken(keys.signing, { ...claims, sub: id })]]) }))).status, 200);
  env.AUTH_DB.sqlite.exec("UPDATE sessions SET revoked_at = '2026-01-01T00:00:00.000Z'");
  assert.equal((await me(env, browser)).status, 401, "revoked: refused at once, token still unexpired");
});

test("refresh: rotates on every use; replaying an old token after the grace revokes the session", async () => {
  const clock = useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  const first = browser.jar.get(REFRESH_COOKIE);
  const stolen = clone(browser);
  const res = await doRefresh(env, browser);
  assert.equal(res.status, 200);
  assert.equal(res.headers.getSetCookie().length, 2);
  assert.notEqual(browser.jar.get(REFRESH_COOKIE), first);
  clock.advance(31_000);
  const replay = await doRefresh(env, stolen);
  assert.equal(replay.status, 401);
  assert.deepEqual(await replay.json(), { error: "session_ended" });
  assert.equal(stolen.jar.size, 0, "cookies cleared");
  assert.equal(sessionRow(env).revoke_reason, "refresh_reuse");
  assert.equal((await doRefresh(env, browser)).status, 401, "the legitimate holder is logged out too");
  assert.equal((await me(env, browser)).status, 401);
});

test("refresh: two tabs at once — the immediately previous token gets an access token within the grace", async () => {
  const clock = useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  const tabB = clone(browser);
  assert.equal((await doRefresh(env, browser)).status, 200);
  clock.advance(5_000);
  const late = await doRefresh(env, tabB);
  assert.equal(late.status, 200);
  assert.deepEqual(late.headers.getSetCookie().map((c) => c.split("=")[0]), [ACCESS_COOKIE], "no new refresh token");
  assert.equal(sessionRow(env).revoked_at, null);
  // Truly concurrent: both read the session before either writes.
  const tab1 = clone(browser);
  const tab2 = clone(browser);
  const [a, b] = await Promise.all([doRefresh(env, tab1), doRefresh(env, tab2)]);
  assert.deepEqual([a.status, b.status], [200, 200]);
  const refreshCookies = [a, b].map((r) => r.headers.getSetCookie().filter((c) => c.startsWith(REFRESH_COOKIE)).length);
  assert.deepEqual(refreshCookies.sort(), [0, 1], "exactly one rotation");
  assert.equal(sessionRow(env).revoked_at, null);
});

test("refresh: the grace covers only the direct predecessor", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  const old = clone(browser);
  await doRefresh(env, browser);
  await doRefresh(env, browser);
  assert.equal((await doRefresh(env, old)).status, 401, "two generations old, even within seconds");
  assert.equal(sessionRow(env).revoke_reason, "refresh_reuse");
});

test("refresh: missing, malformed or unknown tokens are refused without revoking anything", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  assert.equal((await doRefresh(env, new Browser())).status, 401);
  const bad = (v: string) => Object.assign(new Browser(), { jar: new Map([[REFRESH_COOKIE, v]]) });
  assert.equal((await doRefresh(env, bad("short"))).status, 401);
  assert.equal((await doRefresh(env, bad("A".repeat(43)))).status, 401);
  assert.equal(sessionRow(env).revoked_at, null);
  assert.equal((await doRefresh(env, browser)).status, 200);
});

test("refresh: expiry, an ended session, a deleted session, a blocked account", async () => {
  const clock = useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  const old = clone(browser);
  await doRefresh(env, browser);
  env.AUTH_DB.sqlite.exec("UPDATE users SET status = 'blocked'");
  assert.equal((await doRefresh(env, clone(browser))).status, 401);
  assert.equal(sessionRow(env).revoke_reason, "user_inactive");
  assert.equal((await doRefresh(env, browser)).status, 401, "current token of a revoked session");
  assert.equal((await doRefresh(env, old)).status, 401, "history token of a revoked session");

  env.AUTH_DB.sqlite.exec("UPDATE users SET status = 'active'");
  const second = await loggedIn(makeEnv());
  const env2 = makeEnv();
  const s2 = await loggedIn(env2);
  const prev = clone(s2.browser);
  await doRefresh(env2, s2.browser);
  env2.AUTH_DB.sqlite.exec("DELETE FROM sessions");
  assert.equal((await doRefresh(env2, prev)).status, 401, "history row without its session");
  clock.advance(15 * 86400_000);
  assert.equal((await doRefresh(env, second.browser)).status, 401, "a token from another database");
  const env3 = makeEnv();
  const s3 = await loggedIn(env3);
  clock.advance(15 * 86400_000);
  assert.equal((await doRefresh(env3, s3.browser)).status, 401, "past AUTH_SESSION_DAYS");
});
