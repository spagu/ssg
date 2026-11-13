// HTTP Basic and the failed-login lockout. Access has its own file.
import { test } from "node:test";
import assert from "node:assert/strict";
import { REALM, requireAdmin, timingSafeEqual } from "../src/auth.ts";
import { ensureSchema } from "../src/schema.ts";
import { PASSWORD, makeEnv, req } from "./fakes.ts";

async function basicEnv(overrides: Record<string, unknown> = {}) {
  const env = makeEnv(overrides);
  await ensureSchema(env as never);
  return env;
}

test("neither Access nor a password configured: 503, never open", async () => {
  const env = await basicEnv({ NEWSLETTER_ADMIN_PASSWORD: undefined });
  const res = (await requireAdmin(req("/x", { auth: "anything" }), env as never)) as Response;
  assert.equal(res.status, 503);
  assert.match((await res.json()).hint, /NEWSLETTER_ADMIN_PASSWORD/);
});

test("Access half-configured falls back to Basic, not to open", async () => {
  const env = await basicEnv({ NEWSLETTER_ACCESS_TEAM: "myteam" });
  assert.equal(((await requireAdmin(req("/x"), env as never)) as Response).status, 401);
});

test("Basic: right password passes, wrong one is 401 with a challenge", async () => {
  const env = await basicEnv();
  assert.equal(await requireAdmin(req("/x", { auth: PASSWORD }), env as never), null);
  const res = (await requireAdmin(req("/x", { auth: "wrong" }), env as never)) as Response;
  assert.equal(res.status, 401);
  assert.equal(res.headers.get("www-authenticate"), REALM);
});

test("Basic: no header is a challenge and is not counted", async () => {
  const env = await basicEnv();
  const res = (await requireAdmin(req("/x"), env as never)) as Response;
  assert.equal(res.status, 401);
  assert.equal(res.headers.get("www-authenticate"), REALM);
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM auth_attempts").get()!.n, 0);
});

test("Basic: malformed and non-Basic headers are failures", async () => {
  const env = await basicEnv();
  for (const h of ["Basic !!!not-base64", "Bearer abc"]) {
    const r = req("/x", { headers: { authorization: h } });
    assert.equal(((await requireAdmin(r, env as never)) as Response).status, 401, h);
  }
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT failures FROM auth_attempts").get()!.failures, 2);
});

test("Basic: the 10th failure locks the IP — even the right password then gets 429", async () => {
  const env = await basicEnv();
  for (let i = 0; i < 9; i++) await requireAdmin(req("/x", { auth: "wrong" }), env as never);
  // A different address is unaffected.
  assert.equal(await requireAdmin(req("/x", { auth: PASSWORD, ip: "198.51.100.9" }), env as never), null);
  assert.equal(((await requireAdmin(req("/x", { auth: "wrong" }), env as never)) as Response).status, 401);
  const locked = (await requireAdmin(req("/x", { auth: PASSWORD }), env as never)) as Response;
  assert.equal(locked.status, 429);
  const wait = Number(locked.headers.get("retry-after"));
  assert.ok(wait > 590 && wait <= 600, String(wait));
});

test("Basic: a success clears the count; limits are configurable", async () => {
  const env = await basicEnv({ NEWSLETTER_AUTH_MAX_FAILURES: "2", NEWSLETTER_AUTH_WINDOW_MINUTES: "1", NEWSLETTER_HASH_SALT: "s" });
  await requireAdmin(req("/x", { auth: "wrong" }), env as never);
  assert.equal(await requireAdmin(req("/x", { auth: PASSWORD }), env as never), null);
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM auth_attempts").get()!.n, 0);
  await requireAdmin(req("/x", { auth: "wrong" }), env as never);
  await requireAdmin(req("/x", { auth: "wrong" }), env as never);
  const res = (await requireAdmin(req("/x", { auth: PASSWORD }), env as never)) as Response;
  assert.equal(res.status, 429);
  assert.ok(Number(res.headers.get("retry-after")) <= 60);
});

test("Basic: an old window restarts the count; an expired lock is lifted", async () => {
  const env = await basicEnv({ NEWSLETTER_AUTH_MAX_FAILURES: "2" });
  const db = env.NEWSLETTER_DB.sqlite;
  await requireAdmin(req("/x", { auth: "wrong" }), env as never);
  db.prepare("UPDATE auth_attempts SET window_start = '2000-01-01T00:00:00.000Z'").run();
  await requireAdmin(req("/x", { auth: "wrong" }), env as never);
  assert.equal(db.prepare("SELECT failures FROM auth_attempts").get()!.failures, 1, "window restarted");
  await requireAdmin(req("/x", { auth: "wrong" }), env as never);
  assert.ok(db.prepare("SELECT locked_until FROM auth_attempts").get()!.locked_until);
  db.prepare("UPDATE auth_attempts SET locked_until = '2000-01-01T00:00:00.000Z'").run();
  assert.equal(await requireAdmin(req("/x", { auth: PASSWORD }), env as never), null);
});

test("timingSafeEqual", () => {
  assert.equal(timingSafeEqual("abc", "abc"), true);
  assert.equal(timingSafeEqual("abd", "abc"), false);
  assert.equal(timingSafeEqual("ab", "abc"), false);
  assert.equal(timingSafeEqual("", "abc"), false);
});
