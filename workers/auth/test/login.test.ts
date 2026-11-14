// POST /api/auth/login: success, no account enumeration, lockouts, Turnstile,
// the plain-form path, and the hash upgrade.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as login } from "../functions/api/auth/login.ts";
import { ACCESS_COOKIE, REFRESH_COOKIE } from "../functions/api/auth/_cookies.ts";
import { parseHash } from "../functions/api/auth/_password.ts";
import { Browser, ORIGIN, PASSWORD, call, counting, makeEnv, useClock } from "./fakes.ts";
import { seedUser } from "./seed.ts";

const EMAIL = "ada@example.com";
const attempt = (env: ReturnType<typeof makeEnv>, email: string, password: string, browser = new Browser(), extra: Record<string, unknown> = {}) =>
  call(login, env, "/api/auth/login", { json: { email, password, ...extra }, browser });

test("login: 200 with the user and both cookies; the account row records it", async () => {
  const clock = useClock();
  const env = makeEnv();
  const id = await seedUser(env);
  const browser = new Browser();
  const res = await attempt(env, " Ada@Example.com ", PASSWORD, browser);
  assert.equal(res.status, 200);
  const body = await res.json();
  assert.deepEqual(body.user, { id, email: EMAIL, name: "Ada", mustChangePassword: false });
  assert.equal(body.expiresAt, clock.now / 1000 + 600);
  assert.ok(browser.jar.has(ACCESS_COOKIE) && browser.jar.has(REFRESH_COOKIE));
  const row = env.AUTH_DB.sqlite.prepare("SELECT last_login_at FROM users WHERE id = ?").get(id) as { last_login_at: string };
  assert.equal(row.last_login_at, new Date(clock.now).toISOString());
});

test("login: unknown address, wrong password, blocked and password-less accounts look identical and cost one derivation", async () => {
  useClock();
  const env = makeEnv();
  await seedUser(env);
  await seedUser(env, "blocked@example.com");
  env.AUTH_DB.sqlite.exec("UPDATE users SET status = 'blocked' WHERE email = 'blocked@example.com'");
  await seedUser(env, "nopass@example.com");
  env.AUTH_DB.sqlite.exec("UPDATE users SET password_hash = NULL WHERE email = 'nopass@example.com'");
  const cases: [string, string][] = [
    ["nobody@example.com", PASSWORD],
    [EMAIL, "wrong password here"],
    ["blocked@example.com", PASSWORD],
    ["nopass@example.com", PASSWORD],
    [EMAIL, "x".repeat(1000)],
  ];
  const seen = new Set<string>();
  for (const [email, password] of cases) {
    const c = counting();
    try {
      const res = await attempt(env, email, password, new Browser(`198.51.100.${seen.size + 1}`));
      seen.add(`${res.status} ${await res.text()} ${res.headers.getSetCookie().length}`);
      assert.equal(c.calls.length, 1, `${email}: exactly one key derivation`);
    } finally {
      c.restore();
    }
  }
  assert.deepEqual([...seen], ['401 {"error":"invalid_credentials"} 0']);
});

test("login: missing fields or a broken body are 400 before any work", async () => {
  useClock();
  const env = makeEnv();
  assert.equal((await attempt(env, "", PASSWORD)).status, 400);
  assert.equal((await attempt(env, EMAIL, "")).status, 400);
  const res = await login({ request: new Request(ORIGIN + "/api/auth/login", { method: "POST", headers: { "x-ssg-auth": "1" }, body: "{" }), env } as never);
  assert.deepEqual(await res.json(), { error: "invalid_request" });
});

test("login: the account lock — 429 before hashing, even with the right password, growing delay", async () => {
  const clock = useClock();
  const env = makeEnv();
  await seedUser(env);
  for (let i = 0; i < 5; i++) assert.equal((await attempt(env, EMAIL, "wrong password " + i, new Browser(`198.51.100.${i}`))).status, 401);
  const c = counting();
  const locked = await attempt(env, EMAIL, PASSWORD, new Browser("198.51.100.99"));
  c.restore();
  assert.equal(locked.status, 429);
  assert.equal(locked.headers.get("retry-after"), "30");
  assert.deepEqual(await locked.json(), { error: "too_many_attempts" });
  assert.equal(c.calls.length, 0, "a locked request costs no hashing");
  clock.advance(31_000);
  assert.equal((await attempt(env, EMAIL, "wrong again!!")).status, 401, "the sixth failure…");
  assert.equal((await attempt(env, EMAIL, PASSWORD)).headers.get("retry-after"), "60", "…locks twice as long");
  clock.advance(61_000);
  assert.equal((await attempt(env, EMAIL, PASSWORD)).status, 200);
  assert.equal((await attempt(env, EMAIL, "wrong again!!")).status, 401, "success cleared the account counter");
  assert.equal((await attempt(env, EMAIL, PASSWORD)).status, 200);
});

test("login: an unknown address locks exactly like a real one", async () => {
  useClock();
  const env = makeEnv();
  await seedUser(env);
  const statuses = async (email: string) => {
    const out: number[] = [];
    for (let i = 0; i < 6; i++) out.push((await attempt(env, email, "wrong password " + i, new Browser(`192.0.2.${i}`))).status);
    return out;
  };
  assert.deepEqual(await statuses(EMAIL), await statuses("nobody@example.com"));
});

test("login: the IP lock covers guesses across addresses; another IP is unaffected; counters reset after a day", async () => {
  const clock = useClock();
  const env = makeEnv({ AUTH_LOCK_IP_FAILURES: "3", AUTH_LOCK_MAX_SECONDS: "45" });
  await seedUser(env);
  const guesser = new Browser("203.0.113.66");
  for (let i = 0; i < 3; i++) assert.equal((await attempt(env, `user${i}@example.com`, "wrong password", guesser)).status, 401);
  assert.equal((await attempt(env, EMAIL, PASSWORD, guesser)).status, 429);
  assert.equal((await attempt(env, EMAIL, PASSWORD, new Browser("203.0.113.67"))).status, 200);
  clock.advance(46_000);
  assert.equal((await attempt(env, "user9@example.com", "wrong password", guesser)).status, 401);
  assert.equal((await attempt(env, EMAIL, PASSWORD, guesser)).headers.get("retry-after"), "45", "capped at AUTH_LOCK_MAX_SECONDS");
  clock.advance(25 * 3600_000);
  assert.equal((await attempt(env, "user9@example.com", "wrong password", guesser)).status, 401, "a day later the count restarts…");
  assert.equal((await attempt(env, EMAIL, PASSWORD, guesser)).status, 200, "…so one failure locks nothing");
});

test("login: Turnstile is required after repeated failures when a secret is set", async () => {
  useClock();
  const env = makeEnv({ AUTH_TURNSTILE_SECRET: "sec", AUTH_TURNSTILE_AFTER: "2" });
  await seedUser(env);
  const realFetch = globalThis.fetch;
  globalThis.fetch = (async (_u: string, init: RequestInit) => Response.json({ success: (init.body as FormData).get("response") === "ok" })) as typeof fetch;
  try {
    const b = new Browser();
    assert.equal((await attempt(env, EMAIL, "wrong password", b)).status, 401);
    assert.equal((await attempt(env, EMAIL, "wrong password", b)).status, 401);
    const c = counting();
    const need = await attempt(env, EMAIL, PASSWORD, b);
    c.restore();
    assert.equal(need.status, 403);
    assert.deepEqual(await need.json(), { error: "turnstile_required" });
    assert.equal(c.calls.length, 0);
    assert.equal((await attempt(env, EMAIL, PASSWORD, b, { "cf-turnstile-response": "nope" })).status, 403);
    assert.equal((await attempt(env, EMAIL, PASSWORD, b, { "cf-turnstile-response": "ok" })).status, 200);
  } finally {
    globalThis.fetch = realFetch;
  }
});

test("login: a plain HTML form — 303 to a safe next, back to the login page on failure", async () => {
  useClock();
  const env = makeEnv();
  await seedUser(env);
  const form = (fields: Record<string, string>, browser = new Browser()) => call(login, env, "/api/auth/login", { form: fields, browser });
  const b = new Browser();
  const ok = await form({ email: EMAIL, password: PASSWORD, next: "/newsletter-admin/?tab=1" }, b);
  assert.equal(ok.status, 303);
  assert.equal(ok.headers.get("location"), "/newsletter-admin/?tab=1");
  assert.ok(b.jar.has(ACCESS_COOKIE));
  assert.equal((await form({ email: EMAIL, password: PASSWORD, next: "//evil.example.com" })).headers.get("location"), "/");
  const bad = await form({ email: EMAIL, password: "wrong password", next: "/admin" });
  assert.equal(bad.status, 303);
  assert.equal(bad.headers.get("location"), "/auth/login?error=invalid_credentials&next=%2Fadmin");
  assert.equal(bad.headers.getSetCookie().length, 0);
  assert.equal((await form({ email: EMAIL })).headers.get("location"), "/auth/login?error=invalid_request");
});

test("login: a weaker stored hash is upgraded on success; a refused cost keeps the old one", async () => {
  useClock();
  const env = makeEnv({ AUTH_PBKDF2_ITERATIONS: "20000" });
  const id = await seedUser(env, EMAIL, PASSWORD, 10_000);
  const hash = () => (env.AUTH_DB.sqlite.prepare("SELECT password_hash FROM users WHERE id = ?").get(id) as { password_hash: string }).password_hash;
  const refused = counting(15_000);
  const warn = console.warn;
  const warned: string[] = [];
  console.warn = (m: string) => warned.push(m);
  try {
    assert.equal((await attempt(env, EMAIL, PASSWORD)).status, 200);
  } finally {
    refused.restore();
    console.warn = warn;
  }
  assert.equal(parseHash(hash())?.iterations, 10_000);
  assert.equal(warned.length, 1);
  assert.doesNotMatch(warned[0], /ada|example/);
  assert.equal((await attempt(env, EMAIL, PASSWORD)).status, 200);
  assert.equal(parseHash(hash())?.iterations, 20_000, "re-hashed with the configured cost");
  assert.equal((await attempt(env, EMAIL, PASSWORD)).status, 200, "and the new hash verifies");
});

test("login: IP and User-Agent hashes are stored only with AUTH_HASH_SALT", async () => {
  useClock();
  const headers = { "user-agent": "TestBrowser/1.0" };
  for (const salt of [undefined, "pepper"]) {
    const env = makeEnv({ AUTH_HASH_SALT: salt });
    await seedUser(env);
    await call(login, env, "/api/auth/login", { json: { email: EMAIL, password: PASSWORD }, browser: new Browser(), headers });
    const row = env.AUTH_DB.sqlite.prepare("SELECT ip_hash, ua_hash FROM sessions").get() as { ip_hash: string | null; ua_hash: string | null };
    if (salt) assert.match(`${row.ip_hash} ${row.ua_hash}`, /^[0-9a-f]{64} [0-9a-f]{64}$/);
    else assert.deepEqual({ ...row }, { ip_hash: null, ua_hash: null });
  }
});
