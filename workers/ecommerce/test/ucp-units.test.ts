// The UCP pieces one at a time (GO-167): input readers, the Google Pay
// instrument, pricing corners, the order payload, signing and delivery, and
// the agent rate limit on ACP's routes.

import { env } from "./_env";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import * as acpCreate from "../functions/api/shop/acp/checkout_sessions/index.ts";
import { agentProfile, ucpMissing } from "../functions/api/shop/_ucp-auth";
import { readCountry, readLanguage, readLineItems } from "../functions/api/shop/_ucp-input";
import { deliverUcpEvent, permalink, ucpOrder } from "../functions/api/shop/_ucp-order";
import { readInstrument, stripeTokenOf } from "../functions/api/shop/_ucp-payment";
import { priceUcp } from "../functions/api/shop/_ucp-pricing";
import { publicJwk, signedHeaders } from "../functions/api/shop/_ucp-sign";
import type { UcpSessionRow } from "../functions/api/shop/_ucp-session";
import { putSettings } from "../functions/api/shop/_settings";
import { freshShop, seedProduct } from "./_helpers";
import { acpRequest, call, enableAcp } from "./acp-helpers";
import { enableUcp, gpayToken, HANDLER, SIGNING_KEY, verifyWebhook, WEBHOOK_URL } from "./ucp-helpers";

type Json = Record<string, any>;
const ORIGIN = "https://shop.example.com";
const realFetch = globalThis.fetch;

beforeEach(async () => {
  await freshShop();
  enableUcp();
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("reading the platform's input", () => {
  it("refuses a line that is not an object, and reads countries only as alpha-2", () => {
    assert.equal((readLineItems({ line_items: ["x"] }) as Json).body.path, "$.line_items[0].item.id");
    assert.deepEqual(readCountry({ fulfillment: { methods: [null, { destinations: "x" }, { destinations: [{ address_country: "DEU" }] }] } }), { country: null });
    assert.deepEqual(readCountry({ fulfillment: { methods: [{ selected_destination_id: "b", destinations: [{ id: "a", address_country: "FR" }, { id: "b", address_country: "pl" }] }] } }), { country: "PL", source: "fulfillment" });
    assert.deepEqual(readCountry({ context: "x" }), { country: null });
    assert.equal(readLanguage(new Request(ORIGIN), { context: { language: "not a tag!" } }), null);
    assert.equal(readLanguage(new Request(ORIGIN), {}), null);
  });

  it("checks the UCP-Agent header's shape", () => {
    assert.equal(agentProfile('profile="https://p.example.com/.well-known/ucp"'), "https://p.example.com/.well-known/ucp");
    assert.equal(agentProfile('other="x", profile="https://p.example.com/x"'), "https://p.example.com/x");
    assert.equal(agentProfile(null), null);
    assert.equal(agentProfile('profile=""'), null);
  });

  it("names every missing setting", () => {
    enableUcp({ SHOP_UCP_STRIPE_PUBLISHABLE_KEY: undefined, SHOP_UCP_API_KEY: undefined });
    assert.deepEqual(ucpMissing(env), ["SHOP_UCP_API_KEY", "SHOP_UCP_STRIPE_PUBLISHABLE_KEY"]);
  });
});

describe("the Google Pay instrument", () => {
  const instrument = (extra: Json) => ({ instruments: [{ id: "pm", handler_id: HANDLER, type: "card", credential: { type: "PAYMENT_GATEWAY", token: gpayToken("tok_ok") }, ...extra }] });

  it("takes the only instrument, and reads the billing country only when it is alpha-2", () => {
    const plain = readInstrument(env, instrument({}));
    assert.ok(plain.ok);
    assert.equal(plain.ok && plain.billingCountry, null);
    assert.equal(plain.ok && "credential" in plain.instrument, false);
    const odd = readInstrument(env, instrument({ billing_address: { address_country: "Germany" } }));
    assert.equal(odd.ok && odd.billingCountry, null);
    const lower = readInstrument(env, instrument({ billing_address: { address_country: "de" } }));
    assert.equal(lower.ok && lower.billingCountry, "DE");
    assert.deepEqual(readInstrument(env, null), { ok: false, missing: true });
    assert.deepEqual(readInstrument(env, { instruments: [{ ...instrument({}).instruments[0], credential: "x" }] }), { ok: false, missing: false });
  });

  it("finds a Stripe token only in a JSON object with a tok_ id", () => {
    assert.equal(stripeTokenOf(gpayToken("tok_1Abc")), "tok_1Abc");
    for (const token of ["5", "[]", "null", JSON.stringify({ id: "pm_1" }), JSON.stringify({ id: 7 })]) assert.equal(stripeTokenOf(token), null, token);
  });
});

describe("pricing corners", () => {
  it("points an empty basket at the line items, and gives absolute image URLs", async () => {
    const empty = await priceUcp(env, { items: [], currency: "EUR", country: "DE", origin: ORIGIN });
    assert.equal(empty.state.messages[0]!.path, "$.line_items");
    await seedProduct({ sku: "IMG-1" });
    await env.SHOP_DB.prepare(`UPDATE products SET image_url = '/img/cover.webp' WHERE sku = 'IMG-1'`).run();
    const priced = await priceUcp(env, { items: [{ id: "IMG-1", quantity: 1 }], currency: "EUR", country: "DE", origin: ORIGIN });
    assert.equal((priced.state.lineItems[0]!.item as Json).image_url, "https://shop.example.com/img/cover.webp");
  });
});

describe("the order payload", () => {
  const session = (over: Partial<UcpSessionRow> = {}): UcpSessionRow => ({
    id: "chk_1", status: "completed", version: 1, currency: "EUR", items_json: "[]", buyer_json: null, details_json: null,
    country: "DE", locale: null, priced_json: "{}", order_id: null, order_key: null,
    created_at: "", updated_at: "", expires_at: "", ...over,
  });

  it("is null without a completed session's order, and builds a permalink without a key", async () => {
    assert.equal(await ucpOrder(env, session(), ORIGIN), null);
    assert.equal(await ucpOrder(env, session({ order_id: "missing" }), ORIGIN), null);
    assert.equal(permalink(ORIGIN, "o1", null), "https://shop.example.com/shop/thanks/?order=o1");
  });

  it("follows the shop's price mode and leaves the destination empty without a tax country", async () => {
    await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
    const { createOrder, priceBasket, upsertCustomer } = await import("../functions/api/shop/_orders");
    const { taxContext } = await import("../functions/api/shop/_tax");
    const basket = (await priceBasket(env, await taxContext(env), { lines: [{ sku: "EBOOK-1", quantity: 1 }], currency: "EUR", billingCountry: "DE", ipCountry: null, vatId: "", vatIdValid: false })) as never;
    const customer = await upsertCustomer(env, { email: "b@example.com", name: "", vatId: "", vatIdValid: null, country: "DE" });
    const { order } = await createOrder(env, basket, customer, { gateway: "stripe", locale: "en", consentMarketing: false, consentWaiver: false, ipHash: null, userAgent: "t", channel: "ucp" });
    await env.SHOP_DB.prepare(`UPDATE orders SET tax_country = NULL WHERE id = ?`).bind(order.id).run();
    await putSettings(env, { "pricing.mode": "net" });
    const built = (await ucpOrder(env, session({ order_id: order.id }), ORIGIN)) as Json;
    assert.equal(built.line_items[0].status, "processing");
    assert.deepEqual(built.line_items[0].quantity, { total: 1, fulfilled: 0 });
    assert.deepEqual(built.totals.map((t: Json) => t.type), ["subtotal", "tax", "total"]);
    assert.equal(built.line_items[0].item.price, order.total_minor - order.tax_minor);
    assert.deepEqual(built.fulfillment.expectations[0].destination, {});
  });
});

describe("signing and delivering order events", () => {
  const payload = { eventId: "order_1", occurredAt: "2026-11-14T12:00:00.000Z", profile: `${ORIGIN}/.well-known/ucp`, order: { id: "order_1" } };

  it("publishes no key and signs nothing without a valid signing key", async () => {
    enableUcp({ SHOP_UCP_SIGNING_KEY: undefined });
    assert.equal(publicJwk(env), null);
    await assert.rejects(
      signedHeaders(env, new URL(WEBHOOK_URL), "{}", { "ucp-agent": "x", "idempotency-key": "k", "content-type": "application/json" }),
      /SHOP_UCP_SIGNING_KEY/,
    );
  });

  it("signs the query string when the URL has one", async () => {
    enableUcp({ SHOP_UCP_SIGNING_KEY: SIGNING_KEY });
    const url = new URL(`${WEBHOOK_URL}?key=abc`);
    const base = { "ucp-agent": 'profile="https://shop.example.com/.well-known/ucp"', "idempotency-key": "k", "content-type": "application/json" };
    const headers = await signedHeaders(env, url, "{}", base);
    assert.match(headers["signature-input"]!, /"@path" "@query" "ucp-agent"/);
    assert.equal(await verifyWebhook(new Request(url, { method: "POST", headers: { ...base, ...headers }, body: "{}" }), "{}"), true);
  });

  it("refuses to deliver without a URL, to plain http, and treats a non-2xx as a failure", async () => {
    enableUcp({ SHOP_UCP_WEBHOOK_URL: undefined });
    await assert.rejects(deliverUcpEvent(env, payload), /SHOP_UCP_WEBHOOK_URL/);
    enableUcp({ SHOP_UCP_WEBHOOK_URL: "http://platform.example.com/hook" });
    await assert.rejects(deliverUcpEvent(env, payload), /https/);
    enableUcp({ SHOP_UCP_WEBHOOK_API_KEY: undefined });
    let sent: Headers | null = null;
    globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
      sent = new Headers(init?.headers);
      return new Response("busy", { status: 503 });
    }) as typeof fetch;
    await assert.rejects(deliverUcpEvent(env, payload), /UCP webhook 503/);
    assert.equal(sent!.get("x-goog-api-key"), null);
    assert.equal(sent!.get("webhook-timestamp"), payload.occurredAt);
  });
});

describe("the agent rate limit on ACP's routes", () => {
  it("answers 429 in ACP's error shape with Retry-After", async () => {
    enableAcp();
    const limited = { ...env, RATE_LIMITER: { limit: async () => ({ success: false }) } };
    const res = await (acpCreate.onRequestPost as any)({
      request: acpRequest("POST", "/checkout_sessions", { items: [{ id: "X" }] }), env: limited, params: {}, waitUntil: () => {},
    });
    assert.equal(res.status, 429);
    assert.equal(res.headers.get("retry-after"), "60");
    assert.deepEqual(await res.json(), { type: "invalid_request", code: "rate_limited", message: "Too many requests; retry after the time in Retry-After." });
    // Without a limiter the same call goes through.
    assert.notEqual((await call(acpCreate.onRequestPost as never, acpRequest("POST", "/checkout_sessions", { items: [{ id: "X" }] }))).status, 429);
  });
});
