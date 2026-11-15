// Settings, and the environment reading that decides what this deployment can
// do at all.

import { env } from "./_env";
import { modeOf } from "../functions/api/shop/_auth";
import { beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  allSettings,
  getSetting,
  getSettingBool,
  getSettingString,
  invalidateSettings,
  putSettings,
  sellerSnapshot,
  SETTING_DEFAULTS,
  shopIsConfigured,
} from "../functions/api/shop/_settings";
import {
  enabledGateways,
  isTestMode,
  maxFileBytes,
  paypalBase,
} from "../functions/api/shop/_env";
import { freshShop } from "./_helpers";

beforeEach(async () => {
  await freshShop();
});

describe("settings", () => {
  it("answers with defaults for anything nobody set", async () => {
    await putSettings(env, {});
    invalidateSettings();
    await env.SHOP_DB.prepare(`DELETE FROM settings`).run();
    invalidateSettings();
    const all = await allSettings(env);
    assert.equal(all["invoice.series"], SETTING_DEFAULTS["invoice.series"]);
    assert.equal(all["legal.digital_waiver"], true);
  });

  it("refuses a key nobody declared, and writes nothing when it does", async () => {
    const rejected = await putSettings(env, { "shop.name": "Changed", "shop.nmae": "typo" });
    assert.deepEqual(rejected, ["shop.nmae"]);
    // A typo that was accepted would look saved in the panel and be read
    // nowhere, so the whole write is refused rather than half-applied.
    assert.equal(await getSettingString(env, "shop.name"), "Example Press");
  });

  it("keeps a value that is not JSON rather than losing it", async () => {
    await env.SHOP_DB.prepare(
      `INSERT INTO settings (key, value, updated_at) VALUES ('shop.name', 'written by hand', ?)
         ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
    )
      .bind(new Date().toISOString())
      .run();
    invalidateSettings();
    assert.equal(await getSettingString(env, "shop.name"), "written by hand");
  });

  it("coerces for the caller that wants a string or a flag", async () => {
    await putSettings(env, { "invoice.separate_email": true });
    assert.equal(await getSettingBool(env, "invoice.separate_email"), true);
    assert.equal(await getSettingString(env, "invoice.separate_email"), "true");
    assert.equal(await getSettingString(env, "shop.url"), "https://shop.example.com");
    assert.equal(await getSetting(env, "nothing.here"), undefined);
  });

  it("caches, and lets go when told to", async () => {
    const first = await allSettings(env);
    await env.SHOP_DB.prepare(`UPDATE settings SET value = '"Later"' WHERE key = 'shop.name'`).run();
    assert.equal((await allSettings(env))["shop.name"], first["shop.name"]);
    invalidateSettings();
    assert.equal((await allSettings(env))["shop.name"], "Later");
  });
});

describe("the seller block an invoice freezes", () => {
  it("is taken from settings, upper-cased where it should be", async () => {
    assert.deepEqual(await sellerSnapshot(env), {
      name: "Example Press",
      address: "1 Example Street",
      country: "PL",
      vatId: "PL1234567890",
      email: "help@example.com",
      registry: "",
    });
  });
});

describe("whether the shop can take money", () => {
  it("is satisfied by a name, a currency and a country", async () => {
    assert.deepEqual(await shopIsConfigured(env), { ok: true, missing: [] });
  });

  it("names what is missing", async () => {
    await env.SHOP_DB.prepare(`DELETE FROM settings WHERE key IN ('seller.name', 'seller.country')`).run();
    invalidateSettings();
    const verdict = await shopIsConfigured(env);
    assert.equal(verdict.ok, false);
    assert.deepEqual(verdict.missing, ["seller.name", "seller.country"]);
  });

  it("does not insist on a seller country when there is no tax table", async () => {
    await freshShop({ "tax.mode": "none", "seller.country": "" });
    assert.equal((await shopIsConfigured(env)).ok, true);
  });
});

describe("reading the environment", () => {
  // Was authMode (access | jwt | none); the shop's own JWT login is gone
  // (GO-155), and the three shared modes are read by _auth.ts's modeOf.
  it("prefers Cloudflare Access when more than one mode is configured", () => {
    assert.equal(modeOf({ ...env, SHOP_ACCESS_TEAM: "t", SHOP_ACCESS_AUD: "a" }), "access");
    assert.equal(modeOf({ ...env, SHOP_ACCESS_TEAM: undefined, SHOP_ACCESS_AUD: undefined }), "password");
    assert.equal(modeOf({ ...env, SHOP_ACCESS_TEAM: undefined, SHOP_ACCESS_AUD: undefined, SHOP_ADMIN_PASSWORD: undefined }), "none");
  });

  it("drops a gateway that is named but not configured", () => {
    // A button that always fails is worse than one button.
    assert.deepEqual(enabledGateways({ ...env, STRIPE_SECRET_KEY: "sk_test_x", PAYPAL_CLIENT_ID: undefined }), [
      "stripe",
    ]);
    assert.deepEqual(enabledGateways({
        ...env,
        SHOP_GATEWAYS: "paypal,stripe",
        STRIPE_SECRET_KEY: "sk_test_x",
        PAYPAL_CLIENT_ID: "id",
        PAYPAL_CLIENT_SECRET: "secret",
      }), ["paypal", "stripe"]);
    assert.deepEqual(enabledGateways({ ...env, SHOP_GATEWAYS: "bitcoin", STRIPE_SECRET_KEY: "sk_x" }), []);
    assert.deepEqual(enabledGateways({ ...env, SHOP_GATEWAYS: undefined, STRIPE_SECRET_KEY: undefined }), []);
  });

  it("knows when the keys in use are not real ones", () => {
    assert.equal(isTestMode({ ...env, STRIPE_SECRET_KEY: "sk_test_x" }), true);
    assert.equal(isTestMode({ ...env, STRIPE_SECRET_KEY: "sk_live_x", PAYPAL_CLIENT_ID: undefined }), false);
    assert.equal(isTestMode({ ...env, STRIPE_SECRET_KEY: undefined, PAYPAL_CLIENT_ID: "id", SHOP_PAYPAL_ENV: "sandbox" }), true);
  });

  it("never lets the PayPal host be a free-form URL", () => {
    assert.equal(paypalBase({ ...env, SHOP_PAYPAL_ENV: "live" }), "https://api-m.paypal.com");
    assert.equal(paypalBase({ ...env, SHOP_PAYPAL_ENV: "anything else" }), "https://api-m.sandbox.paypal.com");
  });

  it("has a sane upload cap whatever the variable says", () => {
    assert.equal(maxFileBytes({ ...env, SHOP_MAX_FILE_MB: "5" }), 5 * 1024 * 1024);
    assert.equal(maxFileBytes({ ...env, SHOP_MAX_FILE_MB: "nonsense" }), 100 * 1024 * 1024);
    assert.equal(maxFileBytes({ ...env, SHOP_MAX_FILE_MB: "-1" }), 100 * 1024 * 1024);
  });
});
