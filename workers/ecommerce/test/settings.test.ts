// Settings, and the environment reading that decides what this deployment can
// do at all.

import { env } from "cloudflare:test";
import { beforeEach, describe, expect, it } from "vitest";
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
  authMode,
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
    expect(all["invoice.series"]).toBe(SETTING_DEFAULTS["invoice.series"]);
    expect(all["legal.digital_waiver"]).toBe(true);
  });

  it("refuses a key nobody declared, and writes nothing when it does", async () => {
    const rejected = await putSettings(env, { "shop.name": "Changed", "shop.nmae": "typo" });
    expect(rejected).toEqual(["shop.nmae"]);
    // A typo that was accepted would look saved in the panel and be read
    // nowhere, so the whole write is refused rather than half-applied.
    expect(await getSettingString(env, "shop.name")).toBe("Example Press");
  });

  it("keeps a value that is not JSON rather than losing it", async () => {
    await env.SHOP_DB.prepare(
      `INSERT INTO settings (key, value, updated_at) VALUES ('shop.name', 'written by hand', ?)
         ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
    )
      .bind(new Date().toISOString())
      .run();
    invalidateSettings();
    expect(await getSettingString(env, "shop.name")).toBe("written by hand");
  });

  it("coerces for the caller that wants a string or a flag", async () => {
    await putSettings(env, { "invoice.separate_email": true });
    expect(await getSettingBool(env, "invoice.separate_email")).toBe(true);
    expect(await getSettingString(env, "invoice.separate_email")).toBe("true");
    expect(await getSettingString(env, "shop.url")).toBe("https://shop.example.com");
    expect(await getSetting(env, "nothing.here")).toBeUndefined();
  });

  it("caches, and lets go when told to", async () => {
    const first = await allSettings(env);
    await env.SHOP_DB.prepare(`UPDATE settings SET value = '"Later"' WHERE key = 'shop.name'`).run();
    expect((await allSettings(env))["shop.name"]).toBe(first["shop.name"]);
    invalidateSettings();
    expect((await allSettings(env))["shop.name"]).toBe("Later");
  });
});

describe("the seller block an invoice freezes", () => {
  it("is taken from settings, upper-cased where it should be", async () => {
    expect(await sellerSnapshot(env)).toEqual({
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
    expect(await shopIsConfigured(env)).toEqual({ ok: true, missing: [] });
  });

  it("names what is missing", async () => {
    await env.SHOP_DB.prepare(`DELETE FROM settings WHERE key IN ('seller.name', 'seller.country')`).run();
    invalidateSettings();
    const verdict = await shopIsConfigured(env);
    expect(verdict.ok).toBe(false);
    expect(verdict.missing).toEqual(["seller.name", "seller.country"]);
  });

  it("does not insist on a seller country when there is no tax table", async () => {
    await freshShop({ "tax.mode": "none", "seller.country": "" });
    expect((await shopIsConfigured(env)).ok).toBe(true);
  });
});

describe("reading the environment", () => {
  it("prefers Cloudflare Access when both are configured", () => {
    expect(authMode({ ...env, SHOP_ACCESS_TEAM: "t", SHOP_ACCESS_AUD: "a" })).toBe("access");
    expect(authMode({ ...env, SHOP_ACCESS_TEAM: undefined, SHOP_ACCESS_AUD: undefined })).toBe("jwt");
    expect(
      authMode({ ...env, SHOP_ACCESS_TEAM: undefined, SHOP_ACCESS_AUD: undefined, SHOP_JWT_SECRET: "short" }),
    ).toBe("none");
  });

  it("drops a gateway that is named but not configured", () => {
    // A button that always fails is worse than one button.
    expect(enabledGateways({ ...env, STRIPE_SECRET_KEY: "sk_test_x", PAYPAL_CLIENT_ID: undefined })).toEqual([
      "stripe",
    ]);
    expect(
      enabledGateways({
        ...env,
        SHOP_GATEWAYS: "paypal,stripe",
        STRIPE_SECRET_KEY: "sk_test_x",
        PAYPAL_CLIENT_ID: "id",
        PAYPAL_CLIENT_SECRET: "secret",
      }),
    ).toEqual(["paypal", "stripe"]);
    expect(enabledGateways({ ...env, SHOP_GATEWAYS: "bitcoin", STRIPE_SECRET_KEY: "sk_x" })).toEqual([]);
    expect(enabledGateways({ ...env, SHOP_GATEWAYS: undefined, STRIPE_SECRET_KEY: undefined })).toEqual([]);
  });

  it("knows when the keys in use are not real ones", () => {
    expect(isTestMode({ ...env, STRIPE_SECRET_KEY: "sk_test_x" })).toBe(true);
    expect(isTestMode({ ...env, STRIPE_SECRET_KEY: "sk_live_x", PAYPAL_CLIENT_ID: undefined })).toBe(false);
    expect(
      isTestMode({ ...env, STRIPE_SECRET_KEY: undefined, PAYPAL_CLIENT_ID: "id", SHOP_PAYPAL_ENV: "sandbox" }),
    ).toBe(true);
  });

  it("never lets the PayPal host be a free-form URL", () => {
    expect(paypalBase({ ...env, SHOP_PAYPAL_ENV: "live" })).toBe("https://api-m.paypal.com");
    expect(paypalBase({ ...env, SHOP_PAYPAL_ENV: "anything else" })).toBe("https://api-m.sandbox.paypal.com");
  });

  it("has a sane upload cap whatever the variable says", () => {
    expect(maxFileBytes({ ...env, SHOP_MAX_FILE_MB: "5" })).toBe(5 * 1024 * 1024);
    expect(maxFileBytes({ ...env, SHOP_MAX_FILE_MB: "nonsense" })).toBe(100 * 1024 * 1024);
    expect(maxFileBytes({ ...env, SHOP_MAX_FILE_MB: "-1" })).toBe(100 * 1024 * 1024);
  });
});
