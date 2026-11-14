// My own sessions (GO-134): list them, end one — mine only — with JSON or a
// plain form; and GET /api/auth/resume, which picks a session up again for
// the pages through the refresh cookie.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestDelete as endJson, onRequestPost as endForm } from "../functions/api/auth/sessions/[id].ts";
import { onRequestGet as list } from "../functions/api/auth/sessions/index.ts";
import { onRequestGet as resume } from "../functions/api/auth/resume.ts";
import { onRequestPost as login } from "../functions/api/auth/login.ts";
import { onRequestGet as session } from "../functions/api/auth/session.ts";
import { ACCESS_COOKIE, REFRESH_COOKIE } from "../functions/api/auth/_cookies.ts";
import { Browser, PASSWORD, call, makeEnv, useClock } from "./fakes.ts";
import { loggedIn } from "./seed.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const again = async (env: Env, email = "ada@example.com") => {
  const b = new Browser("198.51.100.20");
  await call(login, env, "/api/auth/login", { json: { email, password: PASSWORD }, browser: b });
  return b;
};
const items = async (env: Env, b: Browser) => (await (await call(list, env, "/api/auth/sessions", { browser: b })).json()).items;

test("sessions: my live sessions, newest use first, this one marked, nothing secret in them", async () => {
  const clock = useClock();
  const env = makeEnv({ AUTH_HASH_SALT: "salt" });
  const { browser } = await loggedIn(env);
  clock.advance(60_000);
  const second = await again(env);
  const mine = await items(env, second);
  assert.equal(mine.length, 2);
  assert.deepEqual(Object.keys(mine[0]).sort(), ["createdAt", "current", "expiresAt", "id", "lastUsedAt"]);
  assert.deepEqual(mine.map((s: { current: boolean }) => s.current), [true, false]);
  assert.equal((await items(env, browser)).filter((s: { current: boolean }) => s.current).length, 1);
  const body = JSON.stringify(mine);
  for (const v of [...second.jar.values(), ...browser.jar.values()]) assert.ok(!body.includes(v));
  assert.equal((await call(list, env, "/api/auth/sessions", { browser: new Browser() })).status, 401);
});

test("sessions: end one of mine (JSON); anyone else's, an ended or an unknown one is 404; ending this one clears the cookies", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  const other = await again(env);
  const eve = await loggedIn(env, "eve@example.com");
  const [eveSession] = await items(env, eve.browser);
  const otherId = (await items(env, other)).find((s: { current: boolean }) => s.current).id;
  const del = (b: Browser, id: string) => call(endJson, env, `/api/auth/sessions/${id}`, { method: "DELETE", browser: b, params: { id } });
  assert.deepEqual([(await del(browser, eveSession.id)).status], [404], "someone else's");
  assert.equal((await call(session, env, "/api/auth/session", { browser: eve.browser })).status, 200, "untouched");
  assert.deepEqual(await (await del(browser, otherId)).json(), { ok: true, current: false });
  assert.equal((await call(session, env, "/api/auth/session", { browser: other })).status, 401);
  assert.equal((await del(browser, otherId)).status, 404, "already ended");
  const mineId = (await items(env, browser))[0].id;
  const self = await del(browser, mineId);
  assert.deepEqual(await self.json(), { ok: true, current: true });
  assert.equal(browser.jar.size, 0);
  assert.equal((await del(browser, mineId)).status, 401);
  const noHeader = await call(endJson, env, "/api/auth/sessions/x", { method: "DELETE", browser: eve.browser, params: { id: "x" }, csrf: false, headers: { origin: "https://example.com" } });
  assert.equal(noHeader.status, 403, "DELETE takes no plain form");
});

test("sessions: the account page's plain forms — back to the page, or to the login page for this one", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  const other = await again(env);
  const otherId = (await items(env, other)).find((s: { current: boolean }) => s.current).id;
  const post = (b: Browser, id: string, form: Record<string, string> = { next: "/auth/account" }) => call(endForm, env, `/api/auth/sessions/${id}`, { form, browser: b, params: { id } });
  assert.equal((await post(browser, otherId)).headers.get("location"), "/auth/account?ok=session_ended");
  assert.equal((await post(browser, otherId, { next: "//evil.example.com" })).headers.get("location"), "/auth/account?error=session_not_found");
  assert.equal((await post(new Browser(), otherId)).headers.get("location"), "/auth/account?error=not_authenticated");
  const mineId = (await items(env, browser))[0].id;
  const self = await post(browser, mineId);
  assert.equal(self.headers.get("location"), "/auth/login?ok=signed_out");
  assert.equal(browser.jar.size, 0);
  const json = await call(endForm, env, "/api/auth/sessions/x", { json: {}, browser: other, params: { id: "x" } });
  assert.equal(json.headers.get("location"), "/auth/account?error=not_authenticated", "POST with JSON answers like the form");
});

test("sessions: after the access token expired, the refresh cookie still finds the session (forms posted from the account page)", async () => {
  const clock = useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  const other = await again(env);
  const otherId = (await items(env, other)).find((s: { current: boolean }) => s.current).id;
  clock.advance(11 * 60_000);
  browser.jar.delete(ACCESS_COOKIE);
  const res = await call(endForm, env, `/api/auth/sessions/${otherId}`, { form: {}, browser, params: { id: otherId } });
  assert.equal(res.headers.get("location"), "/auth/account?ok=session_ended");
});

test("resume: a live refresh cookie becomes new cookies and a 303 to next; otherwise the login page", async () => {
  const clock = useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  clock.advance(11 * 60_000);
  browser.jar.delete(ACCESS_COOKIE);
  const before = browser.jar.get(REFRESH_COOKIE);
  const ok = await call(resume, env, "/api/auth/resume?next=%2Fauth%2Faccount%3Fok%3Dx", { browser, csrf: false, headers: { "sec-fetch-site": "same-origin" } });
  assert.equal(ok.status, 303);
  assert.equal(ok.headers.get("location"), "/auth/account?ok=x");
  assert.ok(browser.jar.has(ACCESS_COOKIE));
  assert.notEqual(browser.jar.get(REFRESH_COOKIE), before, "rotated");
  const evil = await call(resume, env, "/api/auth/resume?next=//evil.example.com", { browser, csrf: false });
  assert.equal(evil.headers.get("location"), "/auth/", "a hostile next falls back");
  const cross = await call(resume, env, "/api/auth/resume?next=/auth/account", { browser, csrf: false, headers: { "sec-fetch-site": "cross-site" } });
  assert.equal(cross.headers.get("location"), "/auth/login?next=%2Fauth%2Faccount");
  assert.equal(cross.headers.getSetCookie().length, 0, "nothing touched");
  const none = await call(resume, env, "/api/auth/resume", { browser: new Browser(), csrf: false });
  assert.equal(none.headers.get("location"), "/auth/login?next=%2Fauth%2F");
  const stale = new Browser();
  stale.jar.set(REFRESH_COOKIE, before as string);
  const reused = await call(resume, env, "/api/auth/resume?next=/auth/", { browser: stale, csrf: false });
  assert.equal(reused.headers.get("location"), "/auth/login?next=%2Fauth%2F&error=session_ended");
  assert.equal((await call(resume, makeEnv({ AUTH_SIGNING_KEY: undefined }), "/api/auth/resume", { csrf: false })).status, 503);
  const junk = new Browser();
  junk.jar.set(REFRESH_COOKIE, "not-a-token");
  assert.match((await call(resume, env, "/api/auth/resume", { browser: junk, csrf: false })).headers.get("location") as string, /error=session_ended/);
});

test("resume: a blocked account ends the session; permissions that no longer fit say so; the grace path issues an access token", async () => {
  useClock();
  const env = makeEnv();
  const { browser, id } = await loggedIn(env);
  const copy = new Browser();
  copy.jar.set(REFRESH_COOKIE, browser.jar.get(REFRESH_COOKIE) as string);
  await call(resume, env, "/api/auth/resume", { browser, csrf: false });
  const grace = await call(resume, env, "/api/auth/resume", { browser: copy, csrf: false });
  assert.equal(grace.headers.get("location"), "/auth/");
  assert.ok(copy.jar.has(ACCESS_COOKIE), "two tabs at once: an access token, no new refresh token");
  env.AUTH_DB.sqlite.exec(`INSERT INTO roles (id, name, created_at, updated_at) VALUES ('big', 'big', 'x', 'x')`);
  for (let i = 0; i < 60; i++) env.AUTH_DB.sqlite.prepare("INSERT INTO role_permissions (role_id, permission) VALUES ('big', ?)").run(`w${i}:` + "a".repeat(60));
  env.AUTH_DB.sqlite.prepare("INSERT INTO grants (id, role_id, user_id, granted_at) VALUES ('g', 'big', ?, 'x')").run(id);
  const error = console.error;
  console.error = () => {};
  try {
    const big = await call(resume, env, "/api/auth/resume", { browser, csrf: false });
    assert.match(big.headers.get("location") as string, /error=permissions_too_large/);
  } finally {
    console.error = error;
  }
  env.AUTH_DB.sqlite.exec("DELETE FROM grants WHERE id = 'g'");
  env.AUTH_DB.sqlite.prepare("UPDATE users SET status = 'blocked' WHERE id = ?").run(id);
  const blocked = await call(resume, env, "/api/auth/resume", { browser, csrf: false });
  assert.match(blocked.headers.get("location") as string, /error=session_ended/);
  assert.equal(env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM sessions WHERE revoke_reason = 'user_inactive'").get()?.n, 1);
});
