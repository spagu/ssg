// GO-140: the second step of signing in — no session before it, the pending
// handle (single use, expiry, bound to its account), the code, backup code and
// passkey answers, the lockout, the plain-form path, login providers.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as options } from "../functions/api/auth/mfa/passkey/options.ts";
import { onRequestPost as passkeyRegister } from "../functions/api/auth/mfa/passkey/register.ts";
import { createRole } from "../functions/api/auth/_roles.ts";
import type { Role } from "../functions/api/auth/_roles.ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { Browser, PASSWORD, T0, call, makeEnv, useClock } from "./fakes.ts";
import { SoftAuthenticator, code, enrolTotp, login, mfaEnv, signIn, verify } from "./mfa.ts";
import { seedUser, roleId } from "./seed.ts";
import { events, fakeProviders, providerEnv, providerUser, roundTrip } from "./go151.ts";
import { MFA_KEY } from "./mfa.ts";

const claimsOf = (b: Browser) => JSON.parse(Buffer.from((b.jar.get("__Host-ssg-auth") as string).split(".")[1], "base64url").toString());

// enrolled: ada (owner) with an authenticator app; a fresh browser for the next sign-in.
async function enrolled(env = mfaEnv()) {
  const clock = useClock();
  const id = await seedUser(env, "ada@example.com");
  await addGrant(env, roleId(env as never, "owner"), { userId: id }, "test", T0);
  const first = await signIn(env, "ada@example.com");
  const totp = await enrolTotp(env, first.browser, clock.now);
  clock.advance(60_000);
  return { clock, env, id, ...totp };
}

test("with a second factor, a correct password gives no session: 401 second_factor_required and the pending cookie only", async () => {
  const { env } = await enrolled();
  const { res, body, browser } = await signIn(env, "ada@example.com");
  assert.equal(res.status, 401);
  assert.equal(body.error, "second_factor_required");
  assert.deepEqual(body.factors, ["totp", "backup"]);
  assert.equal(body.challengeUrl, "/auth/mfa");
  assert.deepEqual([...browser.jar.keys()], ["__Host-ssg-auth-mfa"]);
  assert.match(res.headers.getSetCookie()[0], /^__Host-ssg-auth-mfa=[\w-]{43}; Max-Age=300; Path=\/; Secure; HttpOnly; SameSite=Lax$/);
  const form = await call(login, env, "/api/auth/login", { form: { email: "ada@example.com", password: PASSWORD, next: "/auth/account" }, browser: new Browser() });
  assert.equal(form.headers.get("location"), "/auth/mfa");
  const wrong = await call(login, env, "/api/auth/login", { json: { email: "ada@example.com", password: "wrong password!!" } });
  assert.equal(wrong.status, 401);
  assert.equal((await wrong.json()).error, "invalid_credentials", "the second step says nothing before the first factor passed");
});

test("the code finishes the sign-in: session cookies, the factor on the session, `mfa_at`, owner's * in the token, events", async () => {
  const { env, clock, secret, id } = await enrolled();
  const { browser } = await signIn(env, "ada@example.com");
  const res = await call(verify, env, "/api/auth/mfa/verify", { json: { code: await code(secret, clock.now) }, browser });
  assert.equal(res.status, 200, await res.clone().text());
  const body = await res.json();
  assert.equal(body.user.email, "ada@example.com");
  assert.equal(body.next, "/");
  assert.ok(!browser.jar.has("__Host-ssg-auth-mfa"), "the pending cookie is cleared");
  const claims = claimsOf(browser);
  assert.deepEqual(claims.perms, ["*"]);
  assert.equal(claims.mfa_at, Math.floor(clock.now / 1000));
  const row = env.AUTH_DB.sqlite.prepare("SELECT factor FROM session_factors WHERE session_id = ?").get(claims.sid) as { factor: string };
  assert.equal(row.factor, "totp");
  const signin = events(env, "signin").at(-1);
  assert.deepEqual(signin?.detail, { sessionId: claims.sid, secondFactor: "totp" });
  assert.deepEqual(events(env, "mfa_passed").at(-1)?.detail, { purpose: "signin", factor: "totp" });
  assert.equal(events(env, "mfa_passed").at(-1)?.user_id, id);
});

test("the pending handle: single use, five minutes, and bound to its own account", async () => {
  const { env, clock, secret } = await enrolled();
  const a = await signIn(env, "ada@example.com");
  const stolen = new Browser();
  stolen.jar.set("__Host-ssg-auth-mfa", a.browser.jar.get("__Host-ssg-auth-mfa") as string);
  assert.equal((await call(verify, env, "/api/auth/mfa/verify", { json: { code: await code(secret, clock.now) }, browser: a.browser })).status, 200);
  clock.advance(30_000);
  const again = await call(verify, env, "/api/auth/mfa/verify", { json: { code: await code(secret, clock.now) }, browser: stolen });
  assert.deepEqual([again.status, (await again.json()).error], [401, "mfa_pending_invalid"], "used after success");
  // Expiry.
  const b = await signIn(env, "ada@example.com");
  clock.advance(301_000);
  const late = await call(verify, env, "/api/auth/mfa/verify", { json: { code: await code(secret, clock.now) }, browser: b.browser });
  assert.equal(late.status, 401);
  // Bound to the account: bob's handle with ada's code is bob's wrong code.
  await seedUser(env, "bob@example.com");
  const bobBrowser = (await signIn(env, "bob@example.com")).browser;
  const bobTotp = await enrolTotp(env, bobBrowser, clock.now);
  clock.advance(60_000);
  const bob = await signIn(env, "bob@example.com");
  const swap = await call(verify, env, "/api/auth/mfa/verify", { json: { code: await code(secret, clock.now) }, browser: bob.browser });
  assert.deepEqual([swap.status, (await swap.json()).error], [401, "invalid_code"]);
  const own = await call(verify, env, "/api/auth/mfa/verify", { json: { code: await code(bobTotp.secret, clock.now) }, browser: bob.browser });
  assert.equal(own.status, 200);
  assert.equal((await own.json()).user.email, "bob@example.com");
  const none = await call(verify, env, "/api/auth/mfa/verify", { json: { code: "123456" }, browser: new Browser() });
  assert.equal(none.status, 401);
});

test("a backup code signs in once; the code it replaced does not come back", async () => {
  const { env, codes } = await enrolled();
  const first = await signIn(env, "ada@example.com");
  const ok = await call(verify, env, "/api/auth/mfa/verify", { json: { backupCode: (codes as string[])[0] }, browser: first.browser });
  assert.equal(ok.status, 200);
  assert.equal(claimsOf(first.browser).perms[0], "*");
  assert.deepEqual(events(env, "mfa_backup_code_used").at(-1)?.detail, { left: 9 });
  const second = await signIn(env, "ada@example.com");
  const again = await call(verify, env, "/api/auth/mfa/verify", { json: { backupCode: (codes as string[])[0] }, browser: second.browser });
  assert.deepEqual([again.status, (await again.json()).error], [401, "invalid_code"]);
  assert.deepEqual(events(env, "mfa_failed").at(-1)?.detail, { purpose: "signin", factor: "backup" });
});

test("lockout: five wrong codes lock the account's second step, even for the right code; the IP lock counts across accounts", async () => {
  const { env, clock, secret } = await enrolled();
  const b = await signIn(env, "ada@example.com");
  for (let i = 0; i < 5; i++) assert.equal((await call(verify, env, "/api/auth/mfa/verify", { json: { code: "000000" }, browser: b.browser })).status, 401);
  const locked = await call(verify, env, "/api/auth/mfa/verify", { json: { code: await code(secret, clock.now) }, browser: b.browser });
  assert.equal(locked.status, 429);
  assert.ok(Number(locked.headers.get("retry-after")) > 0);
  const form = await call(verify, env, "/api/auth/mfa/verify", { form: { code: "000000" }, browser: b.browser });
  assert.equal(form.headers.get("location"), "/auth/mfa?error=too_many_attempts");
  clock.advance(31_000);
  assert.equal((await call(verify, env, "/api/auth/mfa/verify", { json: { code: await code(secret, clock.now) }, browser: b.browser })).status, 200, "after the lock");
  const ipEnv = mfaEnv({ AUTH_LOCK_IP_FAILURES: "2", AUTH_LOCK_ACCOUNT_FAILURES: "50" });
  const s = await enrolled(ipEnv);
  const x = await signIn(ipEnv, "ada@example.com");
  for (let i = 0; i < 2; i++) await call(verify, ipEnv, "/api/auth/mfa/verify", { json: { code: "000000" }, browser: x.browser });
  assert.equal((await call(verify, ipEnv, "/api/auth/mfa/verify", { json: { code: await code(s.secret, s.clock.now) }, browser: x.browser })).status, 429);
});

test("plain forms: a wrong code back to the page (backup: its fold), an expired step to the login page, a pass to `next`", async () => {
  const { env, clock, secret, codes } = await enrolled();
  const b = new Browser();
  await call(login, env, "/api/auth/login", { form: { email: "ada@example.com", password: PASSWORD, next: "/auth/account" }, browser: b });
  const bad = await call(verify, env, "/api/auth/mfa/verify", { form: { code: "000000" }, browser: b });
  assert.equal(bad.headers.get("location"), "/auth/mfa?error=invalid_code");
  const badBackup = await call(verify, env, "/api/auth/mfa/verify", { form: { backupCode: "AAAA-AAAA-AAAA-AAAA" }, browser: b });
  assert.equal(badBackup.headers.get("location"), "/auth/mfa?error=invalid_code&use=backup");
  const ok = await call(verify, env, "/api/auth/mfa/verify", { form: { code: await code(secret, clock.now) }, browser: b });
  assert.equal(ok.headers.get("location"), "/auth/account");
  assert.ok(b.jar.has("__Host-ssg-auth"));
  const gone = await call(verify, env, "/api/auth/mfa/verify", { form: { backupCode: (codes as string[])[0] }, browser: b });
  assert.equal(gone.headers.get("location"), "/auth/login?error=mfa_expired");
});

test("a lost race for the handle (used between the check and the use) is the expired answer", async () => {
  const { env, clock, secret } = await enrolled();
  const b = await signIn(env, "ada@example.com");
  const db = env.AUTH_DB as unknown as { prepare(sql: string): unknown };
  const real = db.prepare.bind(db);
  db.prepare = (sql: string) => (sql.startsWith("UPDATE mfa_pending") ? real("UPDATE mfa_pending SET used_at = ? WHERE token_hash = ? AND 0 AND expires_at > ?") : real(sql));
  const res = await call(verify, env, "/api/auth/mfa/verify", { json: { code: await code(secret, clock.now) }, browser: b.browser });
  db.prepare = real;
  assert.deepEqual([res.status, (await res.json()).error], [401, "mfa_pending_invalid"]);
});

test("permissions too large after the second step: no session, the reason, the pending cookie cleared", async () => {
  const { env, clock, secret, id } = await enrolled();
  const perms = Array.from({ length: 60 }, (_, i) => `w${i}:` + "a".repeat(60));
  const big = (await createRole(env, { name: "big", description: "", permissions: perms }, T0)) as Role;
  await addGrant(env, big.id, { userId: id }, "t", T0);
  const quiet = console.error;
  console.error = () => {};
  try {
    const a = await signIn(env, "ada@example.com");
    const res = await call(verify, env, "/api/auth/mfa/verify", { json: { code: await code(secret, clock.now) }, browser: a.browser });
    assert.deepEqual([res.status, (await res.json()).error], [500, "permissions_too_large"]);
    clock.advance(30_000);
    const f = new Browser();
    await call(login, env, "/api/auth/login", { form: { email: "ada@example.com", password: PASSWORD }, browser: f });
    const form = await call(verify, env, "/api/auth/mfa/verify", { form: { code: await code(secret, clock.now) }, browser: f });
    assert.equal(form.headers.get("location"), "/auth/login?error=permissions_too_large");
    assert.ok(!f.jar.has("__Host-ssg-auth-mfa"));
  } finally {
    console.error = quiet;
  }
});

test("app codes with an unusable AUTH_MFA_KEY: 503 mfa_unavailable, not counted; the second factor off: the route is absent", async () => {
  const { env } = await enrolled();
  const b = await signIn(env, "ada@example.com");
  const broken = { ...env, AUTH_MFA_KEY: "short" };
  const res = await call(verify, broken, "/api/auth/mfa/verify", { json: { code: "123456" }, browser: b.browser });
  assert.deepEqual([res.status, (await res.json()).error], [503, "mfa_unavailable"]);
  const off = await call(verify, makeEnv(), "/api/auth/mfa/verify", { json: { code: "123456" } });
  assert.equal(off.status, 404);
  // With the key removed the second factor is off entirely: a password signs in.
  const plain = await signIn({ ...env, AUTH_MFA_KEY: undefined }, "ada@example.com");
  assert.equal(plain.res.status, 200);
});

test("a passkey answers the second step through the options route and verify (JSON and the page's form)", async () => {
  const clock = useClock();
  const env = mfaEnv();
  await seedUser(env, "ada@example.com");
  const first = await signIn(env, "ada@example.com");
  const key = await new SoftAuthenticator().init();
  const reg = await call(options, env, "/api/auth/mfa/passkey/options", { json: { purpose: "register" }, browser: first.browser });
  const created = await call(passkeyRegister, env, "/api/auth/mfa/passkey/register", { json: { credential: await key.create((await reg.json()).publicKey.challenge), name: "Key" }, browser: first.browser });
  assert.equal(created.status, 201);
  const pending = await signIn(env, "ada@example.com");
  assert.deepEqual(pending.body.factors, ["passkey", "backup"]);
  const opts = await call(options, env, "/api/auth/mfa/passkey/options", { json: { purpose: "signin" }, browser: pending.browser });
  const { publicKey } = await opts.json();
  assert.deepEqual(publicKey.allowCredentials.map((c: { id: string }) => c.id), [key.id]);
  const res = await call(verify, env, "/api/auth/mfa/verify", { json: { passkey: await key.get(publicKey.challenge) }, browser: pending.browser });
  assert.equal(res.status, 200);
  assert.equal((env.AUTH_DB.sqlite.prepare("SELECT factor FROM session_factors WHERE session_id = ?").get(claimsOf(pending.browser).sid) as { factor: string }).factor, "passkey");
  const viaForm = await signIn(env, "ada@example.com");
  const c = (await (await call(options, env, "/api/auth/mfa/passkey/options", { json: { purpose: "signin" }, browser: viaForm.browser })).json()).publicKey.challenge;
  const form = await call(verify, env, "/api/auth/mfa/verify", { form: { passkey: JSON.stringify(await key.get(c)) }, browser: viaForm.browser });
  assert.equal(form.headers.get("location"), "/");
  assert.ok(clock.now >= T0);
});

test("passkey options: purposes, the pending handle or the session, and refusals", async () => {
  useClock();
  const env = mfaEnv();
  await seedUser(env, "ada@example.com");
  const { browser } = await signIn(env, "ada@example.com");
  const ask = (json: unknown, b?: Browser, e = env) => call(options, e, "/api/auth/mfa/passkey/options", { json, browser: b });
  assert.deepEqual([(await ask({ purpose: "nope" }, browser)).status], [400]);
  assert.equal((await ask({ purpose: "signin" })).status, 401);
  assert.equal((await ask({ purpose: "step-up" })).status, 401);
  assert.equal((await ask({ purpose: "step-up" }, browser)).status, 404, "no passkey yet");
  assert.equal((await ask({ purpose: "register" })).status, 401);
  assert.equal((await ask({ purpose: "signin" }, browser, makeEnv())).status, 404, "the second factor off");
  assert.equal((await call(options, env, "/api/auth/mfa/passkey/options", { json: { purpose: "signin" }, csrf: false })).status, 403);
  assert.equal((await call(options, env, "/api/auth/mfa/passkey/options", { form: { purpose: "register" }, browser })).status, 403, "JSON only");
});

test("login providers: an account with a factor goes to the challenge page; the provider is never taken for a second factor", async () => {
  const clock = useClock();
  const env = providerEnv({ AUTH_MFA_KEY: MFA_KEY } as never);
  await providerUser(env, "pat@example.com", "google", "g-1");
  const fake = fakeProviders();
  try {
    const first = await roundTrip(env, fake, new Browser(), "google", { sub: "g-1", email: "pat@example.com", verified: true }, { next: "/auth/account" });
    assert.equal(first.location, "/auth/account", "no factor yet: straight in");
    const signed = new Browser();
    await roundTrip(env, fake, signed, "google", { sub: "g-1", email: "pat@example.com", verified: true });
    const totp = await enrolTotp(env, signed, clock.now);
    clock.advance(60_000);
    const b = new Browser();
    const again = await roundTrip(env, fake, b, "google", { sub: "g-1", email: "pat@example.com", verified: true }, { next: "/auth/account" });
    assert.equal(again.location, "/auth/mfa");
    assert.ok(b.jar.has("__Host-ssg-auth-mfa") && !b.jar.has("__Host-ssg-auth"));
    const done = await call(verify, env, "/api/auth/mfa/verify", { form: { code: await code(totp.secret, clock.now) }, browser: b });
    assert.equal(done.headers.get("location"), "/auth/account");
    const method = env.AUTH_DB.sqlite.prepare("SELECT method FROM session_methods WHERE session_id = ?").get(claimsOf(b).sid) as { method: string };
    assert.equal(method.method, "google", "the session keeps its first factor");
  } finally {
    fake.restore();
  }
});
