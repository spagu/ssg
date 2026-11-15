// The module switches. What is tested is that each one is read where the work
// happens — a switch that only changes the panel is worse than no switch.

import { env } from "./_env";
import { beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { moduleOn, moduleStates, putSettings } from "../functions/api/shop/_settings";
import { onRequestGet as readSettings, onRequestPut as writeSettings } from "../functions/api/shop/admin/settings";
import { onRequestPost as publicResend } from "../functions/api/shop/resend";
import { onRequestGet as catalogue } from "../functions/api/shop/products/index";
import { onRequestPost as checkout } from "../functions/api/shop/checkout";
import { assertPartial, bodyOf, ctx, freshShop, get, ORIGIN, post, seedProduct, owner } from "./_helpers";

const shopEnv = () => ({ ...env, STRIPE_SECRET_KEY: "sk_test_x", SHOP_GATEWAYS: "stripe" });
const asAdmin = (request: Request, overrides: Record<string, unknown> = {}) => ({
  ...ctx(request, {}, { admin: owner }),
  env: { ...shopEnv(), ...overrides },
});
const put = (body: unknown, overrides: Record<string, unknown> = {}) =>
  asAdmin(
    new Request(`${ORIGIN}/api/shop/admin/settings`, {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body),
    }),
    overrides,
  );

beforeEach(async () => {
  await freshShop();
});

describe("what a module is", () => {
  it("is off when nobody declared it, so a typo turns nothing on", async () => {
    assert.equal(await moduleOn(env, "invoices"), true);
    assert.equal(await moduleOn(env, "invoicing"), false);
    assert.equal(await moduleOn(env, ""), false);
  });

  it("separates switched off from unable to run", async () => {
    const states = await moduleStates(env);
    const byName = Object.fromEntries(states.map((s) => [s.name, s]));

    // Nothing is bound in a bare test environment, so the ones that need a
    // secret say what they need rather than pretending to work.
    assert.match(byName.emails?.blockedBy, /SHOP_MAIL_URL/);
    assert.match(byName.tracking?.blockedBy, /GA4 or Meta/);
    assert.match(byName.turnstile?.blockedBy, /TURNSTILE_SECRET/);
    // These two need nothing at all.
    assert.equal(byName.invoices?.blockedBy, null);
    assert.equal(byName.resend?.blockedBy, null);
  });

  it("stops saying a module is blocked once it is configured", async () => {
    const configured = {
      ...env,
      SHOP_MAIL_URL: "https://mail.example/send",
      SHOP_MAIL_FROM: "shop@example.com",
      SHOP_MAIL_ADMIN: "owner@example.com",
      TURNSTILE_SECRET: "secret",
      GA4_MEASUREMENT_ID: "G-1",
      GA4_API_SECRET: "s",
    };
    await putSettings(env, { "webhooks.endpoints": [{ url: "https://x.example/h", events: ["order.paid"] }] });

    for (const state of await moduleStates(configured)) {
      assert.equal(state.blockedBy, null, state.name);
    }
  });

  it("is off by default where the default should be off", async () => {
    // Tracking sends a purchase to Google or Meta. A shop does not start doing
    // that because someone set a measurement id.
    assert.equal(await moduleOn(env, "tracking"), false);
  });
});

describe("the admin API", () => {
  it("reports every module with its state", async () => {
    const res = (await readSettings(asAdmin(get("/api/shop/admin/settings")) as never)) as Response;
    const body = await bodyOf<{ modules: Array<{ name: string; on: boolean }> }>(res);
    assert.deepEqual(body.modules.map((m) => m.name), [
      "invoices", "emails", "admin_notices", "webhooks", "tracking", "turnstile", "resend",
    ]);
  });

  it("refuses to switch on what cannot run, and says what is missing", async () => {
    const res = (await writeSettings(put({ "modules.tracking": true }) as never)) as Response;
    assert.equal(res.status, 422);
    assertPartial(await bodyOf<{ error: string; message: string }>(res), {
      error: "module_unavailable",
    });
    assert.equal(await moduleOn(env, "tracking"), false);
  });

  it("switches one on once what it needs is there", async () => {
    const res = (await writeSettings(
      put({ "modules.tracking": true }, { GA4_MEASUREMENT_ID: "G-1", GA4_API_SECRET: "s" }) as never,
    )) as Response;
    assert.equal(res.status, 200);
    assert.equal(await moduleOn(env, "tracking"), true);
  });

  it("always allows switching one off", async () => {
    // Off never needs anything: it is the state that does nothing.
    const res = (await writeSettings(put({ "modules.emails": false }) as never)) as Response;
    assert.equal(res.status, 200);
    assert.equal(await moduleOn(env, "emails"), false);
  });
});

describe("a module that is off is off", () => {
  it("makes the public resend endpoint stop existing", async () => {
    await putSettings(env, { "modules.resend": false });
    const res = (await publicResend({
      ...ctx(post("/api/shop/resend", { email: "buyer@example.com" })),
      env: shopEnv(),
    } as never)) as Response;
    // 404 rather than a polite refusal: a shop that does not offer the form
    // should not advertise that it has one.
    assert.equal(res.status, 404);
  });

  it("tells the storefront, so a theme stops linking to a form that 404s", async () => {
    const on = (await catalogue(ctx(get("/api/shop/products")) as never)) as Response;
    assertPartial(await bodyOf<{ shop: { resend: boolean } }>(on), { shop: { resend: true } });

    await putSettings(env, { "modules.resend": false });
    const off = (await catalogue(ctx(get("/api/shop/products")) as never)) as Response;
    assertPartial(await bodyOf<{ shop: { resend: boolean } }>(off), { shop: { resend: false } });
  });

  it("skips the anti-spam check even with a secret configured", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    await putSettings(env, { "modules.turnstile": false });

    // Turnstile would refuse this, and the checkout is never asked.
    const realFetch = globalThis.fetch;
    globalThis.fetch = (async (url: unknown) => {
      if (String(url).includes("challenges.cloudflare.com")) {
        throw new Error("the anti-spam check was called while its module was off");
      }
      return new Response(JSON.stringify({ id: "cs_1", url: "https://x" }));
    }) as unknown as typeof fetch;

    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: [{ sku: "EBOOK-1", quantity: 1 }], gateway: "stripe",
        email: "buyer@example.com", country: "DE", consentWaiver: "1",
      })),
      env: { ...shopEnv(), TURNSTILE_SECRET: "secret" },
    } as never)) as Response;

    globalThis.fetch = realFetch;
    assert.equal(res.status, 200);
  });
});
