// GO-155: GET /api/shop/admin/me per mode — who is signed in, which shop
// permissions they hold, where the `auth` login and logout are — and the
// health warnings it carries for an account that may read the shop.
import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as me } from "../functions/api/shop/admin/me";
import { putSettings } from "../functions/api/shop/_settings";
import { ACCESS_VARS, AUTH_VARS, ORIGIN, PASSWORD, ask, serveAccessKeys, shopEnv, throughGate } from "./admin-fixtures";
import { fakeKV, fakeR2 } from "./fakes";

const ME = "/api/shop/admin/me";
const ALL = ["ecommerce:read", "ecommerce:orders", "ecommerce:refund", "ecommerce:products", "ecommerce:settings", "ecommerce:export"];
const LOGIN = `${ORIGIN}/auth/login?next=%2Fecommerce-admin%2F`;
let restore: () => void = () => {};
afterEach(() => restore());

type Me = Record<string, unknown> & { warnings: string[] };

async function whoami(env: ReturnType<typeof shopEnv>, a: Parameters<typeof ask>[1]): Promise<{ status: number; body: Me }> {
  const { res } = await throughGate(await ask(ME, a), env, (ctx) => me(ctx as never) as Promise<Response>);
  return { status: res.status, body: (await res.json()) as Me };
}

describe("who /me says is signed in", () => {
  it("the password: nobody by name, every permission, no auth URLs", async () => {
    const { status, body } = await whoami(shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD }), { password: PASSWORD });
    assert.equal(status, 200);
    assert.deepEqual(
      { mode: body.mode, user: body.user, permissions: body.permissions, loginUrl: body.loginUrl, logoutUrl: body.logoutUrl, refreshUrl: body.refreshUrl },
      { mode: "password", user: null, permissions: ALL, loginUrl: null, logoutUrl: null, refreshUrl: null },
    );
  });

  it("Access: the address, every permission", async () => {
    restore = serveAccessKeys();
    const { body } = await whoami(shopEnv(ACCESS_VARS), { access: true });
    assert.equal(body.mode, "access");
    assert.deepEqual(body.user, { id: "owner@example.com", email: "owner@example.com" });
    assert.deepEqual(body.permissions, ALL);
  });

  it("auth: the account, its shop permissions only, wildcards expanded, and auth's URLs", async () => {
    const { body } = await whoami(shopEnv(AUTH_VARS), { perms: ["ecommerce:read", "ecommerce:refund", "comments:moderate"] });
    assert.equal(body.mode, "auth");
    assert.deepEqual(body.user, { id: "u-ada", email: "ada@example.com" });
    assert.deepEqual(body.permissions, ["ecommerce:read", "ecommerce:refund"]);
    assert.equal(body.loginUrl, LOGIN);
    assert.equal(body.logoutUrl, "/api/auth/logout");
    assert.equal(body.refreshUrl, "/api/auth/refresh");
    assert.deepEqual((await whoami(shopEnv(AUTH_VARS), { perms: ["ecommerce:*"] })).body.permissions, ALL);
  });

  it("auth: an account without any shop permission still learns who it is, without the warnings", async () => {
    const { status, body } = await whoami(shopEnv(AUTH_VARS), { perms: ["comments:read"] });
    assert.equal(status, 200);
    assert.deepEqual(body.permissions, []);
    assert.deepEqual(body.warnings, []);
  });

  it("auth without a session: 401 with where to sign in and how to refresh", async () => {
    const { status, body } = await whoami(shopEnv(AUTH_VARS), {});
    assert.equal(status, 401);
    assert.equal(body.mode, "auth");
    assert.equal(body.loginUrl, LOGIN);
    assert.equal(body.refreshUrl, "/api/auth/refresh");
    assert.equal(body.reason, "no_session");
  });

  it("other refusals pass unchanged: the password's 401, a half-configured auth's 503", async () => {
    const pw = await whoami(shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD }), {});
    assert.equal(pw.status, 401);
    assert.equal(pw.body.mode, undefined);
    const half = await whoami(shopEnv({ SHOP_AUTH_ISSUER: "https://shop.example.com/api/auth" }), {});
    assert.equal(half.status, 503);
    assert.equal(half.body.loginUrl, undefined);
  });
});

describe("the health check /me carries", () => {
  it("names everything a bare shop still needs", async () => {
    const env = shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD, SHOP_IP_SALT: undefined, SHOP_TEST: undefined });
    await throughGate(await ask(ME, { password: PASSWORD }), env); // creates the schema
    await env.SHOP_DB.prepare("UPDATE vat_rates SET valid_from = '2020-01-01T00:00:00.000Z'").run();
    await env.SHOP_DB.prepare(
      `INSERT INTO outbox (id, kind, target, payload_json, attempts, next_attempt, created_at) VALUES ('o1', 'email', 'x', '{}', 5, '2026-01-01', '2026-01-01')`,
    ).run();
    const { body } = await whoami(env, { password: PASSWORD });
    const text = body.warnings.join("\n");
    for (const expected of [/Settings still missing/, /No payment provider/, /No file storage/, /Email is not configured/, /No KV namespace/, /request budget/, /SHOP_IP_SALT/, /VAT rates/, /1 queued message/]) {
      assert.match(text, expected);
    }
    assert.equal(body.testMode, false);
    assert.deepEqual(body.gateways, []);
  });

  it("says nothing about a shop that has everything", async () => {
    const env = shopEnv({
      SHOP_ADMIN_PASSWORD: PASSWORD,
      SHOP_FILES: fakeR2(),
      SHOP_KV: fakeKV(),
      SHOP_MAIL_URL: "https://mail.example.com/send",
      SHOP_MAIL_FROM: "shop@example.com",
      STRIPE_SECRET_KEY: "sk_test_x",
      SHOP_GATEWAYS: "stripe",
    });
    await throughGate(await ask(ME, { password: PASSWORD }), env);
    assert.deepEqual(
      await putSettings(env, {
        "seller.name": "Example Press", "seller.address": "1 Example Street", "seller.country": "PL",
        "seller.vat_id": "PL1234567890", "seller.email": "help@example.com", "shop.name": "Example Press",
        "shop.url": ORIGIN, "currency.base": "EUR",
      }),
      [],
    );
    const { body } = await whoami(env, { password: PASSWORD });
    assert.deepEqual(body.warnings, []);
    assert.equal(body.testMode, true);
    assert.deepEqual(body.gateways, ["stripe"]);
    assert.equal(body.shopName, "Example Press");
    assert.equal(body.baseCurrency, "EUR");
  });
});
