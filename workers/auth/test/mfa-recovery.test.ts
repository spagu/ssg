// GO-140: losing a second factor — a reset by another owner, and the
// break-glass secret for a sole owner. Each resets exactly the account it
// names (or, for break-glass, the account that just passed its password) and
// never another one.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as reset } from "../functions/api/auth/mfa/reset.ts";
import { onRequestPost as breakGlass } from "../functions/api/auth/mfa/break-glass.ts";
import { onRequestPost as refresh } from "../functions/api/auth/refresh.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { Browser, T0, call, makeEnv, useClock } from "./fakes.ts";
import { enrolTotp, mfaEnv, signIn } from "./mfa.ts";
import { grantPerms, roleId, seedUser } from "./seed.ts";
import { events } from "./go151.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const TOKEN = "break-glass-token-break-glass-token-1234";
const count = (env: Env, sql: string, ...a: unknown[]) => ((env.AUTH_DB as unknown as { sqlite: { prepare(s: string): { get(...x: unknown[]): unknown } } }).sqlite.prepare(sql).get(...a) as { n: number }).n;

async function person(env: Env, clock: ReturnType<typeof useClock>, email: string, isOwner: boolean) {
  const id = await seedUser(env, email);
  if (isOwner) await addGrant(env, roleId(env as never, "owner"), { userId: id }, "t", T0);
  else await grantPerms(env, id, ["newsletter:read"]);
  const { browser } = await signIn(env, email);
  const { secret } = await enrolTotp(env, browser, clock.now);
  return { id, browser, secret };
}

test("reset by another owner: that account's factors, codes and sessions go; the owner's own and a third account's stay", async () => {
  const clock = useClock();
  const env = mfaEnv();
  const ada = await person(env, clock, "ada@example.com", true);
  const bob = await person(env, clock, "bob@example.com", true);
  const eva = await person(env, clock, "eva@example.com", false);
  const res = await call(reset, env, "/api/auth/mfa/reset", { json: { userId: bob.id }, browser: ada.browser });
  assert.equal(res.status, 200, await res.clone().text());
  assert.deepEqual(await res.json(), { ok: true, removed: 1, revoked: 1 });
  assert.equal(count(env, "SELECT COUNT(*) AS n FROM mfa_factors WHERE user_id = ?", bob.id), 0);
  assert.equal(count(env, "SELECT COUNT(*) AS n FROM mfa_backup_codes WHERE user_id = ?", bob.id), 0);
  assert.equal(count(env, "SELECT COUNT(*) AS n FROM sessions WHERE user_id = ? AND revoked_at IS NULL", bob.id), 0);
  for (const other of [ada.id, eva.id]) assert.equal(count(env, "SELECT COUNT(*) AS n FROM mfa_factors WHERE user_id = ?", other), 1);
  assert.equal((await call(refresh, env, "/api/auth/refresh", { method: "POST", browser: bob.browser })).status, 401, "bob's session ended");
  assert.deepEqual(events(env, "mfa_reset").at(-1)?.detail, { via: "owner", by: ada.id, removed: 1 });
  // Bob signs in with his password alone again — and is led to enrol, without *.
  const again = await signIn(env, "bob@example.com");
  assert.deepEqual([again.res.status, again.body.enrol], [200, true]);
});

test("reset: only an owner, with a recent second factor, never oneself, an existing account", async () => {
  const clock = useClock();
  const env = mfaEnv();
  const ada = await person(env, clock, "ada@example.com", true);
  const eva = await person(env, clock, "eva@example.com", false);
  const r = (json: unknown, b?: Browser) => call(reset, env, "/api/auth/mfa/reset", { json, browser: b });
  assert.deepEqual(await (await r({ userId: ada.id }, eva.browser)).json(), { error: "forbidden", permission: "*" });
  assert.equal((await r({ userId: eva.id }, ada.browser)).status, 200);
  assert.equal((await r({ userId: ada.id }, ada.browser)).status, 400);
  assert.equal((await (await r({ userId: ada.id }, ada.browser)).json()).error, "own_account");
  assert.equal((await r({ userId: "nobody" }, ada.browser)).status, 404);
  assert.equal((await r({}, ada.browser)).status, 400);
  assert.equal((await r(undefined, undefined)).status, 401);
  assert.equal((await call(reset, env, "/api/auth/mfa/reset", { method: "POST", browser: ada.browser, headers: { "content-type": "application/json" } })).status, 400, "no body");
  clock.advance(11 * 60_000);
  await call(refresh, env, "/api/auth/refresh", { method: "POST", browser: ada.browser });
  assert.deepEqual(await (await r({ userId: eva.id }, ada.browser)).json(), { error: "step_up_required", stepUpUrl: "/auth/mfa/step-up", minutes: 10 });
  const off = await call(reset, makeEnv(), "/api/auth/mfa/reset", { json: { userId: eva.id }, browser: ada.browser });
  assert.ok(off.status === 401 || off.status === 404);
});

test("break-glass: once, for the owner who just passed the password, and for nobody else", async () => {
  const clock = useClock();
  const env = mfaEnv({ AUTH_MFA_RESET_TOKEN: TOKEN });
  const ada = await person(env, clock, "ada@example.com", true);
  const bob = await person(env, clock, "bob@example.com", true);
  const eva = await person(env, clock, "eva@example.com", false);
  clock.advance(60_000);
  const glass = (json: unknown, b?: Browser, e: Env = env) => call(breakGlass, e, "/api/auth/mfa/break-glass", { json, browser: b });
  // A non-owner's pending step: refused, and the token is not used up.
  const evaPending = await signIn(env, "eva@example.com");
  assert.deepEqual([(await glass({ token: TOKEN }, evaPending.browser)).status], [403]);
  // No pending step: nothing to reset.
  assert.equal((await glass({ token: TOKEN })).status, 401);
  // Wrong tokens count towards a lock.
  const adaPending = await signIn(env, "ada@example.com");
  for (let i = 0; i < 5; i++) assert.equal((await glass({ token: "wrong" }, adaPending.browser)).status, 401);
  const locked = await glass({ token: TOKEN }, adaPending.browser);
  assert.equal(locked.status, 429);
  clock.advance(31_000);
  const ok = await glass({ token: TOKEN }, adaPending.browser);
  assert.equal(ok.status, 200, await ok.clone().text());
  const body = await ok.json();
  assert.deepEqual([body.user.email, body.enrol], ["ada@example.com", true]);
  assert.equal(count(env, "SELECT COUNT(*) AS n FROM mfa_factors WHERE user_id = ?", ada.id), 0);
  for (const other of [bob.id, eva.id]) assert.equal(count(env, "SELECT COUNT(*) AS n FROM mfa_factors WHERE user_id = ?", other), 1, "another account untouched");
  assert.deepEqual(events(env, "mfa_reset").at(-1)?.detail, { via: "break_glass", removed: 1 });
  // The same value never works again — not for bob either.
  const bobPending = await signIn(env, "bob@example.com");
  assert.equal((await glass({ token: TOKEN }, bobPending.browser)).status, 404);
  assert.equal(count(env, "SELECT COUNT(*) AS n FROM mfa_factors WHERE user_id = ?", bob.id), 1);
  // Unset, or the second factor off: as if the route did not exist.
  assert.equal((await glass({ token: TOKEN }, bobPending.browser, mfaEnv())).status, 404);
});

test("break-glass from the challenge page's form: to enrolment, or back to the page with the reason", async () => {
  const clock = useClock();
  const env = mfaEnv({ AUTH_MFA_RESET_TOKEN: TOKEN });
  await person(env, clock, "ada@example.com", true);
  clock.advance(60_000);
  const b = new Browser();
  const { onRequestPost: login } = await import("../functions/api/auth/login.ts");
  await call(login, env, "/api/auth/login", { form: { email: "ada@example.com", password: "correct horse battery", next: "/auth/account" }, browser: b });
  const wrong = await call(breakGlass, env, "/api/auth/mfa/break-glass", { form: { token: "nope" }, browser: b });
  assert.equal(wrong.headers.get("location"), "/auth/mfa?error=invalid_token&use=recover");
  const ok = await call(breakGlass, env, "/api/auth/mfa/break-glass", { form: { token: TOKEN }, browser: b });
  assert.equal(ok.headers.get("location"), "/auth/mfa/enrol?required=1&next=%2Fauth%2Faccount");
  assert.ok(b.jar.has("__Host-ssg-auth") && !b.jar.has("__Host-ssg-auth-mfa"));
  const expired = await call(breakGlass, env, "/api/auth/mfa/break-glass", { form: { token: TOKEN }, browser: new Browser() });
  assert.equal(expired.headers.get("location"), "/auth/login?error=mfa_expired");
});
