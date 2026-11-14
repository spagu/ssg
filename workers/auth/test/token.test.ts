// The permissions inside the token (GO-133): login and bootstrap put the
// effective list in `perms`, refresh re-reads the grants, revoke-sessions
// forces it, and a list that does not fit in a cookie is refused whole.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ACCESS_COOKIE_MAX, COOKIE_LIMIT } from "../functions/api/auth/_context.ts";
import { ACCESS_COOKIE, REFRESH_COOKIE } from "../functions/api/auth/_cookies.ts";
import { addGrant, listGrants, removeGrant } from "../functions/api/auth/_grants.ts";
import { onRequestPost as bootstrap } from "../functions/api/auth/bootstrap.ts";
import { onRequestPost as login } from "../functions/api/auth/login.ts";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { onRequestGet as session } from "../functions/api/auth/session.ts";
import { onRequestPost as revokeSessions } from "../functions/api/auth/admin/users/[id]/revoke-sessions.ts";
import { Browser, PASSWORD, T0, call, makeEnv, useClock } from "./fakes.ts";
import { grantPerms, roleId, seedUser, signedIn } from "./seed.ts";

type Env = ReturnType<typeof makeEnv>;
const permsOf = (b: Browser): string[] => JSON.parse(Buffer.from((b.jar.get(ACCESS_COOKIE) as string).split(".")[1], "base64url").toString()).perms;
const doRefresh = (env: Env, b: Browser) => call(refresh, env, "/api/auth/refresh", { method: "POST", browser: b });
const quiet = async <T>(fn: () => Promise<T>): Promise<{ value: T; logs: string[] }> => {
  const logs: string[] = [];
  const real = console.error;
  console.error = (...a: unknown[]) => logs.push(a.map(String).join(" "));
  try {
    return { value: await fn(), logs };
  } finally {
    console.error = real;
  }
};

test("token: login carries the effective permissions; the bootstrap account is the owner", async () => {
  useClock();
  const env = makeEnv({ AUTH_BOOTSTRAP_TOKEN: "t".repeat(40) });
  const owner = new Browser();
  const res = await call(bootstrap, env, "/api/auth/bootstrap", { json: { token: "t".repeat(40), email: "o@example.com", password: PASSWORD }, browser: owner });
  assert.equal(res.status, 201);
  assert.deepEqual(permsOf(owner), ["*"]);
  assert.equal(env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM active_owners").get()?.n, 1);
  const { browser } = await signedIn(env, "ed@example.com", ["newsletter:send", "newsletter:read"]);
  assert.deepEqual(permsOf(browser), ["newsletter:read", "newsletter:send"]);
});

test("token: refresh re-reads the grants — a granted role appears, a revoked one disappears", async () => {
  const clock = useClock();
  const env = makeEnv();
  const { browser, id } = await signedIn(env, "ed@example.com", ["newsletter:read"]);
  await addGrant(env, roleId(env, "comments-moderator"), { userId: id }, "x", T0);
  assert.deepEqual(permsOf(browser), ["newsletter:read"], "the current token is unchanged until refresh");
  clock.advance(60_000);
  assert.equal((await doRefresh(env, browser)).status, 200);
  assert.deepEqual(permsOf(browser), ["comments:moderate", "comments:read", "newsletter:read"]);
  for (const g of await listGrants(env, { userId: id })) await removeGrant(env, g.id);
  assert.equal((await doRefresh(env, browser)).status, 200);
  assert.deepEqual(permsOf(browser), []);
});

test("revoke-sessions: the account's next refresh fails; logging in again gives the new permissions", async () => {
  useClock();
  const env = makeEnv();
  const admin = await signedIn(env, "root@example.com", "owner");
  const ed = await signedIn(env, "ed@example.com", ["newsletter:send"]);
  const stale = Object.assign(new Browser(), { jar: new Map(ed.browser.jar) });
  const res = await call(revokeSessions, env, `/api/auth/admin/users/${ed.id}/revoke-sessions`, { method: "POST", browser: admin.browser, params: { id: ed.id } });
  assert.deepEqual(await res.json(), { ok: true, revoked: 1 });
  assert.equal((await call(session, env, "/api/auth/session", { browser: stale })).status, 401, "this worker refuses the token at once");
  assert.equal((await doRefresh(env, stale)).status, 401, "and the refresh");
  assert.equal(stale.jar.has(REFRESH_COOKIE), false);
  const notFound = await call(revokeSessions, env, "/api/auth/admin/users/nobody/revoke-sessions", { method: "POST", browser: admin.browser, params: { id: "nobody" } });
  assert.equal(notFound.status, 404);
  assert.equal((await notFound.json()).error, "user_not_found");
});

// big: n distinct permissions of the longest allowed length (64 characters).
const big = (n: number, len = 64) => Array.from({ length: n }, (_, i) => `w${String(i).padStart(3, "0")}:${"a".repeat(len - 5)}`);

test("ceiling: how many permissions fit in the access cookie (README numbers)", async () => {
  useClock();
  const fits = async (perms: string[]) => {
    const env = makeEnv();
    const id = await seedUser(env, "a-typical-address@example.com");
    await grantPerms(env, id, perms);
    const b = new Browser();
    const { value } = await quiet(() => call(login, env, "/api/auth/login", { json: { email: "a-typical-address@example.com", password: PASSWORD }, browser: b }));
    return value.status === 200;
  };
  const most = async (make: (n: number) => string[]) => {
    let lo = 0;
    let hi = 200;
    while (lo < hi) {
      const mid = Math.ceil((lo + hi) / 2);
      if (await fits(make(mid))) lo = mid;
      else hi = mid - 1;
    }
    return lo;
  };
  const longest = await most((n) => big(n));
  const typical = await most((n) => big(n, 20));
  assert.equal(COOKIE_LIMIT - ACCESS_COOKIE_MAX, 296);
  // Pinned so a change of the claims or the limit is noticed (and the README updated).
  assert.equal(longest, 36, "64-character permissions");
  assert.equal(typical, 104, "20-character permissions, like newsletter:settings");
});

test("ceiling: too many permissions — login refused with a named reason and a log line, never a cut list", async () => {
  useClock();
  const env = makeEnv();
  const id = await seedUser(env);
  await grantPerms(env, id, big(60));
  const b = new Browser();
  const { value: res, logs } = await quiet(() => call(login, env, "/api/auth/login", { json: { email: "ada@example.com", password: PASSWORD }, browser: b }));
  assert.equal(res.status, 500);
  const body = await res.json();
  assert.equal(body.error, "permissions_too_large");
  assert.match(body.detail, /60 permissions .* 3800 bytes/);
  assert.equal(res.headers.getSetCookie().length, 0, "no cookie at all");
  assert.equal(logs.length, 1);
  assert.match(logs[0], new RegExp(`auth: no access token for account ${id}`));
  assert.equal(env.AUTH_DB.sqlite.prepare("SELECT revoke_reason FROM sessions").get()?.revoke_reason, "permissions_too_large");
  const { value: form } = await quiet(() => call(login, env, "/api/auth/login", { form: { email: "ada@example.com", password: PASSWORD }, browser: new Browser() }));
  assert.equal(form.status, 303);
  assert.match(form.headers.get("location") as string, /error=permissions_too_large/);
});

test("ceiling: a refresh that no longer fits keeps the session (rotated refresh cookie) and answers 500", async () => {
  const clock = useClock();
  const env = makeEnv();
  const { browser, id } = await signedIn(env, "ed@example.com", ["newsletter:read"]);
  const role = await grantPerms(env, id, big(60));
  clock.advance(60_000);
  const { value: res } = await quiet(() => doRefresh(env, browser));
  assert.equal(res.status, 500);
  assert.equal((await res.json()).error, "permissions_too_large");
  assert.deepEqual(res.headers.getSetCookie().map((c) => c.split("=")[0]), [REFRESH_COOKIE]);
  env.AUTH_DB.sqlite.prepare("DELETE FROM grants WHERE role_id = ?").run(role);
  assert.equal((await doRefresh(env, browser)).status, 200, "the same session works again once the roles are narrowed");
  assert.deepEqual(permsOf(browser), ["newsletter:read"]);
});

test("ceiling: a bootstrap whose token cannot fit (an absurd AUTH_ISSUER) is refused the same way", async () => {
  useClock();
  const env = makeEnv({ AUTH_BOOTSTRAP_TOKEN: "t".repeat(40), AUTH_ISSUER: "https://example.com/" + "x".repeat(3000) });
  const { value: res } = await quiet(() => call(bootstrap, env, "/api/auth/bootstrap", { json: { token: "t".repeat(40), email: "o@example.com", password: PASSWORD } }));
  assert.equal(res.status, 500);
  assert.equal((await res.json()).error, "permissions_too_large");
  assert.equal(res.headers.getSetCookie().length, 0);
});
