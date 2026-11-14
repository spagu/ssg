// POST /api/auth/bootstrap: the first account, exactly once.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestPost as bootstrap } from "../functions/api/auth/bootstrap.ts";
import { onRequestGet as session } from "../functions/api/auth/session.ts";
import { Browser, call, counting, makeEnv, useClock } from "./fakes.ts";
import { seedUser } from "./seed.ts";

const TOKEN = "a-long-random-bootstrap-token-0123456789";
const FIRST = { token: TOKEN, email: "Owner@Example.com", password: "the very first passphrase", name: "  Owner  " };
type Env = ReturnType<typeof makeEnv>;
const run = (env: Env, json: unknown, browser = new Browser()) => call(bootstrap, env, "/api/auth/bootstrap", { json, browser });
const users = (env: Env) => (env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM users").get() as { n: number }).n;

test("bootstrap: creates and logs in the first account, then answers 404 forever", async () => {
  useClock();
  const env = makeEnv({ AUTH_BOOTSTRAP_TOKEN: TOKEN });
  const browser = new Browser();
  const res = await run(env, FIRST, browser);
  assert.equal(res.status, 201);
  const body = await res.json();
  assert.equal(body.user.email, "owner@example.com");
  assert.equal(body.user.name, "Owner");
  assert.equal((await call(session, env, "/api/auth/session", { browser })).status, 200);
  const again = await run(env, { ...FIRST, email: "second@example.com" });
  assert.equal(again.status, 404);
  assert.deepEqual(await again.json(), { error: "not_found" });
  assert.equal(users(env), 1);
});

test("bootstrap: the first account's creation and sign-in are in the audit log, and its last sign-in is set", async () => {
  useClock();
  const env = makeEnv({ AUTH_BOOTSTRAP_TOKEN: TOKEN });
  assert.equal((await run(env, FIRST)).status, 201);
  const kinds = env.AUTH_DB.sqlite.prepare("SELECT kind, method, detail FROM auth_events WHERE kind IN ('account_created', 'signin') ORDER BY kind").all() as { kind: string; method: string; detail: string }[];
  assert.deepEqual(kinds.map((k) => [k.kind, k.method, JSON.parse(k.detail).via]), [["account_created", "password", "bootstrap"], ["signin", "password", "bootstrap"]]);
  const row = env.AUTH_DB.sqlite.prepare("SELECT last_login_at FROM users").get() as { last_login_at: string | null };
  assert.ok(row.last_login_at, "last sign-in recorded");
});

test("bootstrap: 404 without the secret or once any account exists — even with the right token", async () => {
  useClock();
  assert.equal((await run(makeEnv(), FIRST)).status, 404);
  const env = makeEnv({ AUTH_BOOTSTRAP_TOKEN: TOKEN });
  await seedUser(env);
  assert.equal((await run(env, FIRST)).status, 404);
});

test("bootstrap: two concurrent calls create exactly one account", async () => {
  useClock();
  const env = makeEnv({ AUTH_BOOTSTRAP_TOKEN: TOKEN });
  const [a, b] = await Promise.all([run(env, FIRST), run(env, { ...FIRST, email: "rival@example.com" })]);
  assert.deepEqual([a.status, b.status].sort(), [201, 404]);
  assert.equal(users(env), 1);
});

test("bootstrap: wrong tokens are 401 and lock the IP after five", async () => {
  const clock = useClock();
  const env = makeEnv({ AUTH_BOOTSTRAP_TOKEN: TOKEN });
  const b = new Browser("203.0.113.5");
  for (let i = 0; i < 5; i++) assert.equal((await run(env, { ...FIRST, token: "guess" + i }, b)).status, 401);
  assert.equal((await run(env, { ...FIRST, token: TOKEN.slice(0, -1) }, new Browser("203.0.113.6"))).status, 401);
  const locked = await run(env, FIRST, b);
  assert.equal(locked.status, 429);
  assert.equal(locked.headers.get("retry-after"), "30");
  assert.equal((await run(env, {}, new Browser("203.0.113.7"))).status, 401, "no token at all");
  clock.advance(31_000);
  assert.equal((await run(env, FIRST, b)).status, 201);
});

test("bootstrap: the address and the password rules are enforced", async () => {
  useClock();
  const env = makeEnv({ AUTH_BOOTSTRAP_TOKEN: TOKEN });
  assert.deepEqual(await (await run(env, { ...FIRST, email: "nope" })).json(), { error: "invalid_email" });
  assert.deepEqual(await (await run(env, { ...FIRST, password: "short" })).json(), { error: "password_too_short" });
  assert.deepEqual(await (await run(env, { ...FIRST, password: "owner@example.com" })).json(), { error: "password_equals_email" });
  assert.equal(users(env), 0);
  const noName = await run(env, { token: TOKEN, email: "o@example.com", password: FIRST.password });
  assert.equal((await noName.json()).user.name, "");
});

test("bootstrap: a cost the runtime refuses is a 503, and no account is created", async () => {
  useClock();
  const env = makeEnv({ AUTH_BOOTSTRAP_TOKEN: TOKEN, AUTH_PBKDF2_ITERATIONS: "200000" });
  const c = counting(100_000);
  try {
    const res = await run(env, FIRST);
    assert.equal(res.status, 503);
    assert.equal((await res.json()).error, "hashing_unavailable");
  } finally {
    c.restore();
  }
  assert.equal(users(env), 0);
});
