// #435: the edges of POST /api/auth/jwt — the per-IP lockout, the second
// factor never skipped, a blocked account, the worker's own configuration
// first, a body that is not JSON. The main flow is external-signin.test.ts.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ACCESS_COOKIE } from "../functions/api/auth/_cookies.ts";
import { Browser, T0, useClock } from "./fakes.ts";
import { count, providerUser } from "./go151.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { FakeIssuer, ISS, errorCode, jwtEnv, mint, post, sqlite, where } from "./external-fake.ts";

const METHOD = `jwt:${ISS}`;

async function world(over: Record<string, string> = {}) {
  useClock();
  const env = jwtEnv(over);
  const fake = new FakeIssuer().install();
  return { env, fake };
}

test("the lockout: failed tokens count per IP under their own key; a locked IP is refused before any check", async () => {
  const { env, fake } = await world({ AUTH_LOCK_IP_FAILURES: "2" });
  const quiet = console.warn;
  console.warn = () => {};
  try {
    await providerUser(env, "ada@example.com", METHOD, "ext-1001");
    const browser = new Browser("198.51.100.9");
    for (let i = 0; i < 2; i++) assert.equal(errorCode(await post(env, "x.y.z", { browser })), "jwt_failed");
    assert.equal(errorCode(await post(env, await mint(), { browser })), "too_many_attempts");
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM login_attempts WHERE kind = 'jwt_ip'"), 1);
    assert.equal(where(await post(env, await mint(), { browser: new Browser("198.51.100.10") })), "/", "another IP is not affected");
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM login_attempts WHERE kind = 'jwt_ip' AND failures > 0"), 1, "a success gives its reservation back");
  } finally {
    console.warn = quiet;
    fake.restore();
  }
});

test("the second factor is never skipped: a person with one gets the challenge, not a session", async () => {
  const { env, fake } = await world({ AUTH_MFA_KEY: "k".repeat(40) });
  try {
    const id = await providerUser(env, "ada@example.com", METHOD, "ext-1001");
    const at = new Date(T0).toISOString();
    sqlite(env).prepare("INSERT INTO mfa_factors (id, user_id, kind, name, confirmed_at, created_at) VALUES (?, ?, 'totp', 'phone', ?, ?)").run(crypto.randomUUID(), id, at, at);
    const browser = new Browser();
    const res = await post(env, await mint(), { browser, next: "/x" });
    assert.equal(where(res), "/auth/mfa");
    assert.ok(!browser.jar.has(ACCESS_COOKIE));
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM sessions"), 0);
    assert.equal(count(env, `SELECT COUNT(*) AS n FROM mfa_pending WHERE method = '${METHOD}'`), 1);
  } finally {
    fake.restore();
  }
});

test("a blocked account is refused whatever the token says", async () => {
  const { env, fake } = await world();
  try {
    const id = await providerUser(env, "ada@example.com", METHOD, "ext-1001");
    sqlite(env).prepare("UPDATE users SET status = 'blocked' WHERE id = ?").run(id);
    assert.equal(errorCode(await post(env, await mint())), "account_blocked");
  } finally {
    fake.restore();
  }
});

test("the worker's own configuration first (503 without D1); a body that is not JSON still reads the bearer header", async () => {
  const { env, fake } = await world();
  try {
    const noDb = await post({ ...env, AUTH_DB: undefined } as unknown as Env, await mint());
    assert.equal(noDb.status, 503);
    await providerUser(env, "ada@example.com", METHOD, "ext-1001");
    const { onRequestPost } = await import("../functions/api/auth/jwt.ts");
    const request = new Request("https://example.com/api/auth/jwt", {
      method: "POST",
      headers: { origin: "https://example.com", "x-ssg-auth": "1", "content-type": "application/json", authorization: `Bearer ${await mint()}` },
      body: "{not json",
    });
    const res = await (onRequestPost as unknown as (c: { request: Request; env: Env }) => Promise<Response>)({ request, env });
    assert.equal(where(res), "/");
  } finally {
    fake.restore();
  }
});
