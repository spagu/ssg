// Ending sessions: logout, logout-all, and the stale-data purge.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { onRequestPost as logout } from "../functions/api/auth/logout.ts";
import { onRequestPost as logoutAll } from "../functions/api/auth/logout-all.ts";
import { ACCESS_COOKIE, REFRESH_COOKIE } from "../functions/api/auth/_cookies.ts";
import { purgeStale } from "../functions/api/auth/_sessions.ts";
import { Browser, T0, call, makeEnv, useClock } from "./fakes.ts";
import { loggedIn } from "./seed.ts";

type Env = ReturnType<typeof makeEnv>;
const doRefresh = (env: Env, b: Browser) => call(refresh, env, "/api/auth/refresh", { method: "POST", browser: b });
const clone = (b: Browser) => Object.assign(new Browser(b.ip), { jar: new Map(b.jar) });
const sessionRow = (env: Env) => env.AUTH_DB.sqlite.prepare("SELECT revoked_at, revoke_reason FROM sessions").get() as Record<string, string | null>;

test("logout: ends this session and clears the cookies; works through the refresh cookie and as a form", async () => {
  const clock = useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  const res = await call(logout, env, "/api/auth/logout", { method: "POST", browser });
  assert.deepEqual(await res.json(), { ok: true });
  assert.equal(browser.jar.size, 0);
  assert.equal(sessionRow(env).revoke_reason, "logout");
  assert.equal((await call(logout, env, "/api/auth/logout", { method: "POST", browser: new Browser() })).status, 200, "no session is fine");

  const env2 = makeEnv();
  const s2 = await loggedIn(env2);
  const prev = clone(s2.browser);
  await doRefresh(env2, s2.browser);
  clock.advance(20 * 60_000);
  prev.jar.delete(ACCESS_COOKIE);
  const form = await call(logout, env2, "/api/auth/logout", { form: { next: "/bye" }, browser: prev });
  assert.equal(form.status, 303);
  assert.equal(form.headers.get("location"), "/bye");
  // Security review SR-01: a rotated token used past the two-tab grace window
  // is a copy someone kept — the session ends as theft (as /refresh does),
  // where it used to be found and ended as a logout. Inside the window it is
  // still a logout (security-review.test.ts).
  assert.equal(sessionRow(env2).revoke_reason, "refresh_reuse", "a rotated refresh token past the grace window");
  const unknown = Object.assign(new Browser(), { jar: new Map([[REFRESH_COOKIE, "A".repeat(43)]]) });
  assert.equal((await call(logout, env2, "/api/auth/logout", { method: "POST", browser: unknown })).status, 200, "an unknown refresh token");
  // The current token of the session just revoked: found, no longer live.
  assert.equal((await call(logout, env2, "/api/auth/logout", { method: "POST", browser: s2.browser })).status, 200, "a revoked session's token");
  const plain = await call(logout, env2, "/api/auth/logout", { form: {}, browser: new Browser() });
  assert.equal(plain.headers.get("location"), "/auth/login");
});

test("logout-all: every session of the account ends; 401 without a session", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  const phone = new Browser("198.51.100.20");
  await call((await import("../functions/api/auth/login.ts")).onRequestPost, env, "/api/auth/login", {
    json: { email: "ada@example.com", password: "correct horse battery" },
    browser: phone,
  });
  const res = await call(logoutAll, env, "/api/auth/logout-all", { method: "POST", browser });
  assert.deepEqual(await res.json(), { ok: true, revoked: 2 });
  assert.equal(browser.jar.size, 0);
  assert.equal((await doRefresh(env, phone)).status, 401);
  assert.equal((await call(logoutAll, env, "/api/auth/logout-all", { method: "POST", browser: new Browser() })).status, 401);
  const again = await loggedIn(makeEnv());
  const form = await call(logoutAll, makeEnv(), "/api/auth/logout-all", { form: {}, browser: again.browser });
  assert.equal(form.status, 401, "a session of another database is no session here");
});

test("logout-all as a form: 303 to next", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  const res = await call(logoutAll, env, "/api/auth/logout-all", { form: { next: "/x" }, browser });
  assert.equal(res.status, 303);
  assert.equal(res.headers.get("location"), "/x");
  const env2 = makeEnv();
  const s2 = await loggedIn(env2);
  assert.equal((await call(logoutAll, env2, "/api/auth/logout-all", { form: {}, browser: s2.browser })).headers.get("location"), "/auth/login");
});

test("purgeStale drops ended history, month-old sessions and stale counters only", async () => {
  useClock();
  const env = makeEnv();
  await loggedIn(env);
  const db = env.AUTH_DB.sqlite;
  db.exec(`INSERT INTO sessions (id, user_id, refresh_hash, created_at, last_used_at, expires_at, revoked_at) VALUES
    ('old', 'u', 'h1', '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', '2026-01-02T00:00:00.000Z', NULL),
    ('ended', 'u', 'h2', '2026-11-01T00:00:00.000Z', '2026-11-01T00:00:00.000Z', '2026-12-01T00:00:00.000Z', '2026-11-12T00:00:00.000Z')`);
  db.exec(`INSERT INTO refresh_history VALUES ('x1', 'ended', 'h2', '2026-11-12T00:00:00.000Z')`);
  db.exec(`INSERT INTO login_attempts VALUES ('k1', 'ip', 3, '2026-11-01T00:00:00.000Z', NULL),
    ('k2', 'ip', 9, '2026-11-01T00:00:00.000Z', '2026-12-01T00:00:00.000Z'), ('k3', 'ip', 1, '2026-11-13T09:00:00.000Z', NULL)`);
  await purgeStale(env, T0);
  const ids = (sql: string) => db.prepare(sql).all().map((r) => Object.values(r)[0]);
  assert.deepEqual(ids("SELECT id FROM sessions WHERE id IN ('old', 'ended') ORDER BY id"), ["ended"]);
  assert.deepEqual(ids("SELECT hash FROM refresh_history"), []);
  assert.deepEqual(ids("SELECT key FROM login_attempts ORDER BY key"), ["k2", "k3"]);
});
