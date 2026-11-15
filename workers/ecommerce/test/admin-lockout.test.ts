// GO-155: the shared password's lockout. Wrong passwords are counted per client
// address in D1 (auth_attempts); at the limit the address is locked for one
// window — 429 even for the right password — and a good sign-in clears it.
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { passwordGate } from "../functions/api/shop/_admin-password";
import { PASSWORD, ask, shopEnv, throughGate } from "./admin-fixtures";

const STATS = "/api/shop/admin/stats";
const from = (ip: string) => ({ "cf-connecting-ip": ip });

async function attempt(env: ReturnType<typeof shopEnv>, password: string, ip = "198.51.100.7"): Promise<Response> {
  return (await throughGate(await ask(STATS, { password, headers: from(ip) }), env)).res;
}

describe("the password lockout", () => {
  it("locks an address at the limit, for the right password too, and not its neighbour", async () => {
    const env = shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD, SHOP_ADMIN_MAX_FAILURES: "3", SHOP_ADMIN_LOCKOUT_MINUTES: "5" });
    assert.equal((await attempt(env, "nope")).status, 401);
    assert.equal((await attempt(env, "nope")).status, 401);
    const third = await attempt(env, "nope");
    assert.equal(third.status, 429);
    assert.equal(third.headers.get("retry-after"), "300");
    assert.equal(((await third.json()) as { error: string }).error, "too_many_attempts");

    const locked = await attempt(env, PASSWORD);
    assert.equal(locked.status, 429);
    assert.ok(Number(locked.headers.get("retry-after")) > 0);
    assert.equal((await attempt(env, PASSWORD, "198.51.100.8")).status, 200);
  });

  it("stores the address hashed, never as typed", async () => {
    const env = shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD });
    await attempt(env, "nope");
    const row = (await env.SHOP_DB.prepare("SELECT ip_hash, failures, locked_until FROM auth_attempts").first()) as Record<string, unknown>;
    assert.match(String(row.ip_hash), /^[0-9a-f]{64}$/);
    assert.equal(row.failures, 1);
    assert.equal(row.locked_until, null);
  });

  it("clears the count on a good sign-in", async () => {
    const env = shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD });
    await attempt(env, "nope");
    assert.equal((await attempt(env, PASSWORD)).status, 200);
    assert.equal(await env.SHOP_DB.prepare("SELECT COUNT(*) AS n FROM auth_attempts").first("n"), 0);
  });

  it("starts a new window once the old one has passed, and lets a lock run out", async () => {
    const env = shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD, SHOP_ADMIN_MAX_FAILURES: "2" });
    await attempt(env, "nope");
    await env.SHOP_DB.prepare("UPDATE auth_attempts SET window_start = '2000-01-01T00:00:00.000Z'").run();
    assert.equal((await attempt(env, "nope")).status, 401, "the old failure no longer counts");
    assert.equal(await env.SHOP_DB.prepare("SELECT failures FROM auth_attempts").first("failures"), 1);
    assert.equal((await attempt(env, "nope")).status, 429);
    await env.SHOP_DB.prepare("UPDATE auth_attempts SET locked_until = '2000-01-01T00:00:00.000Z'").run();
    assert.equal((await attempt(env, PASSWORD)).status, 200);
  });

  it("falls back to 10 in 10 minutes for a setting that is not a positive number", async () => {
    const env = shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD, SHOP_ADMIN_MAX_FAILURES: "zero", SHOP_ADMIN_LOCKOUT_MINUTES: "-1" });
    for (let i = 0; i < 9; i++) assert.equal((await attempt(env, "nope")).status, 401);
    const tenth = await attempt(env, "nope");
    assert.equal(tenth.status, 429);
    assert.equal(tenth.headers.get("retry-after"), "600");
  });

  it("counts an address Cloudflare did not name as one address, and works without a salt", async () => {
    const env = shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD, SHOP_IP_SALT: undefined });
    assert.equal((await throughGate(await ask(STATS, { password: "nope" }), env)).res.status, 401);
    assert.equal((await throughGate(await ask(STATS, { password: "nope" }), env)).res.status, 401);
    assert.equal(await env.SHOP_DB.prepare("SELECT failures FROM auth_attempts").first("failures"), 2);
  });

  it("refuses the right password when a parallel guess locked the address meanwhile", async () => {
    const env = shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD });
    await throughGate(await ask(STATS, { password: "nope" }), env); // the schema exists from here on
    const db = env.SHOP_DB;
    let reads = 0;
    const racing = {
      ...db,
      prepare(sql: string) {
        if (sql.startsWith("SELECT locked_until") && ++reads === 2) {
          db.sqlite.exec("UPDATE auth_attempts SET locked_until = '2999-01-01T00:00:00.000Z'");
        }
        return db.prepare(sql);
      },
    };
    const res = await passwordGate(await ask(STATS, { password: PASSWORD }), { ...env, SHOP_DB: racing as never }, PASSWORD);
    assert.equal(res?.status, 429);
  });
});
