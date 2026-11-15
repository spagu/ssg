// The payment providers' less travelled paths: answers with fields missing,
// configuration half done, events that arrive out of order or twice. Every one
// of these is something a provider, a seller or a network has done to a shop.

import { env } from "./_env";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { enabledGateways, isTestMode, maxFileBytes, paypalBase } from "../functions/api/shop/_env";
import { decimalStringToMinor } from "../functions/api/shop/_money";
import {
  hmacHex,
  parseSignatureHeader,
  redact,
  registerGateway,
  type CheckoutInput,
} from "../functions/api/shop/_gateway";
import { gateway } from "../functions/api/shop/_gateways";
import { mapStripeEvent, StripeGateway } from "../functions/api/shop/_stripe";
import { PaypalGateway, resetPaypalToken } from "../functions/api/shop/_paypal";
import { handleWebhook } from "../functions/api/shop/_webhook";
import { onRequestPost as stripeWebhook } from "../functions/api/shop/webhooks/stripe";
import { onRequestPost as paypalWebhook } from "../functions/api/shop/webhooks/paypal";
import { onRequestGet as readGateways, onRequestPost as writeOrder } from "../functions/api/shop/admin/gateways";
import { createOrder, priceBasket, upsertCustomer, type PricedBasket } from "../functions/api/shop/_orders";
import { taxContext } from "../functions/api/shop/_tax";
import type { CustomerRow, OrderItemRow, OrderRow, PaymentRow } from "../functions/api/shop/_types";
import { bodyOf, ctx, freshShop, ORIGIN, seedProduct, owner } from "./_helpers";
import { sparseD1 } from "./fakes";

const SECRET = "whsec_corners";
const realFetch = globalThis.fetch;

const stripeEnv = () => ({
  ...env,
  STRIPE_SECRET_KEY: "sk_test_x",
  STRIPE_WEBHOOK_SECRET: SECRET,
  SHOP_GATEWAYS: "stripe",
});

const payEnv = () => ({
  ...env,
  SHOP_GATEWAYS: "paypal",
  PAYPAL_CLIENT_ID: "client",
  PAYPAL_CLIENT_SECRET: "secret",
  PAYPAL_WEBHOOK_ID: "wh-1",
  SHOP_PAYPAL_ENV: "sandbox",
});

const order: OrderRow = {
  id: "order-1", number: "O-2026-000001", key_hash: "h", status: "pending", customer_id: "c1",
  currency: "EUR", subtotal_minor: 2243, tax_minor: 157, total_minor: 2400, tax_country: "DE",
  tax_evidence: "{}", reverse_charge: 0, gateway: "stripe", gateway_ref: null, consent_marketing: 0,
  consent_waiver: 1, ip_hash: null, user_agent: "t", locale: "en", notes: null,
  created_at: "2026-01-01T00:00:00.000Z", paid_at: null, updated_at: "2026-01-01T00:00:00.000Z",
};
const items: OrderItemRow[] = [{
  id: "i1", order_id: "order-1", product_id: "p1", sku: "EBOOK-1", name: "A Book",
  quantity: 2, unit_minor: 1000, tax_rate_bp: 2000, tax_minor: 400, total_minor: 2400,
}];
const customer: CustomerRow = {
  id: "c1", email: "buyer@example.com", email_lc: "buyer@example.com", name: "A Buyer",
  vat_id: null, vat_id_valid: null, country: "DE", created_at: "2026-01-01T00:00:00.000Z",
};
const input = (over: Partial<CheckoutInput> = {}): CheckoutInput => ({
  order, items, customer,
  successUrl: `${ORIGIN}/shop/thanks/`,
  cancelUrl: `${ORIGIN}/shop/cancel/`,
  locale: "en", pricingMode: "gross", providerTax: false,
  ...over,
});
const payment = { id: "pay-1", gateway_ref: "ref-1", currency: "EUR" } as PaymentRow;

let calls: Array<{ url: string; init: RequestInit }> = [];

/** Answers every request with whatever `answer` returns for it. */
function network(answer: (url: string, init: RequestInit) => Response): void {
  globalThis.fetch = (async (url: unknown, init: RequestInit = {}) => {
    calls.push({ url: String(url), init });
    return answer(String(url), init);
  }) as unknown as typeof fetch;
}

/** PayPal with a working token endpoint and `api` for everything else. */
function paypal(api: (url: string) => Response): void {
  network((url) =>
    url.includes("/v1/oauth2/token")
      ? new Response(JSON.stringify({ access_token: "tok", expires_in: 3600 }))
      : api(url),
  );
}

const jsonResponse = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

beforeEach(() => {
  calls = [];
  resetPaypalToken();
});

afterEach(() => {
  globalThis.fetch = realFetch;
  env.SHOP_DB.state.failOn = null;
});

describe("which providers a deployment has", () => {
  it("offers Stripe when nothing is listed, and only what has its secrets", () => {
    assert.deepEqual(enabledGateways({ ...env, SHOP_GATEWAYS: undefined, STRIPE_SECRET_KEY: "sk_test_x" }), ["stripe"]);
    assert.deepEqual(enabledGateways({ ...env, SHOP_GATEWAYS: undefined }), []);
    // PayPal needs both halves of its credentials; a name nobody implements is dropped.
    assert.deepEqual(enabledGateways({ ...env, SHOP_GATEWAYS: "paypal, square", PAYPAL_CLIENT_ID: "id" }), []);
    assert.deepEqual(
      enabledGateways({ ...env, SHOP_GATEWAYS: " PayPal ,stripe", PAYPAL_CLIENT_ID: "id", PAYPAL_CLIENT_SECRET: "s" }),
      ["paypal"],
    );
  });

  it("knows test keys from live ones", () => {
    assert.equal(isTestMode({ ...env, STRIPE_SECRET_KEY: "sk_test_x" }), true);
    assert.equal(isTestMode({ ...env, STRIPE_SECRET_KEY: "sk_live_x" }), false);
    assert.equal(isTestMode({ ...env, SHOP_PAYPAL_ENV: undefined, PAYPAL_CLIENT_ID: "id" }), true);
    assert.equal(isTestMode({ ...env, SHOP_PAYPAL_ENV: "live", PAYPAL_CLIENT_ID: "id" }), false);
    assert.equal(isTestMode({ ...env, SHOP_PAYPAL_ENV: undefined }), false);
  });

  it("talks to PayPal's sandbox unless told it is live", () => {
    assert.equal(paypalBase({ ...env, SHOP_PAYPAL_ENV: undefined }), "https://api-m.sandbox.paypal.com");
    assert.equal(paypalBase({ ...env, SHOP_PAYPAL_ENV: "live" }), "https://api-m.paypal.com");
  });

  it("caps uploads at 100 MB unless a sensible number says otherwise", () => {
    assert.equal(maxFileBytes({ ...env, SHOP_MAX_FILE_MB: undefined }), 100 * 1024 * 1024);
    assert.equal(maxFileBytes({ ...env, SHOP_MAX_FILE_MB: "5" }), 5 * 1024 * 1024);
    assert.equal(maxFileBytes({ ...env, SHOP_MAX_FILE_MB: "lots" }), 100 * 1024 * 1024);
    assert.equal(maxFileBytes({ ...env, SHOP_MAX_FILE_MB: "-1" }), 100 * 1024 * 1024);
  });
});

describe("amounts and signatures", () => {
  it("reads amounts in currencies with no minor unit and with too many decimals", () => {
    assert.equal(decimalStringToMinor("1500", "JPY"), 1500);
    assert.equal(decimalStringToMinor("12.345", "EUR"), 1234);
    assert.equal(decimalStringToMinor("-3.5", "EUR"), -350);
    assert.equal(decimalStringToMinor("7", "EUR"), 700);
  });

  it("reads a signature entry that has no value as an empty one", () => {
    assert.deepEqual(parseSignatureHeader("t=1, v1"), { t: "1", v1: [""] });
    assert.deepEqual(parseSignatureHeader("v1=abc"), { t: "", v1: ["abc"] });
  });

  it("redacts secrets inside lists and stops at a sane depth", () => {
    const deep: Record<string, unknown> = { secret: "x" };
    let node = deep;
    for (let i = 0; i < 12; i++) node = (node.child = { password: "p" }) as Record<string, unknown>;
    const out = redact({ list: [{ access_token: "t", keep: 1 }], deep }) as Record<string, unknown>;
    assert.deepEqual(out.list, [{ access_token: "[redacted]", keep: 1 }]);
    assert.equal((out.deep as Record<string, unknown>).secret, "[redacted]");
  });

  it("offers no provider whose factory declines to build one", () => {
    try {
      registerGateway("stripe", () => null as never);
      assert.equal(gateway(stripeEnv(), "stripe"), null);
    } finally {
      registerGateway("stripe", () => new StripeGateway());
    }
    assert.ok(gateway(stripeEnv(), "stripe") instanceof StripeGateway);
  });
});

describe("Stripe, answering oddly", () => {
  it("sends net unit prices in net pricing, and lets Stripe pick a language it does not know", async () => {
    network(() => jsonResponse({ id: "cs_1", url: "https://checkout.stripe.example/x" }));
    const stripe = new StripeGateway();
    await stripe.createCheckout(input({ pricingMode: "net", locale: "xx-XX" }), stripeEnv());
    const form = new URLSearchParams(String(calls[0]!.init.body));
    assert.equal(form.get("line_items[0][price_data][unit_amount]"), "1000");
    assert.equal(form.get("locale"), "auto");

    await stripe.createCheckout(input({ locale: "" }), stripeEnv());
    const second = new URLSearchParams(String(calls[1]!.init.body));
    assert.equal(second.get("locale"), "auto");
    assert.equal(second.get("line_items[0][price_data][unit_amount]"), "1200");
  });

  it("sends no key at all rather than the word undefined when none is set", async () => {
    network(() => jsonResponse({ id: "re_1" }));
    await new StripeGateway().refund(payment, 100, { ...env, STRIPE_SECRET_KEY: undefined });
    assert.equal((calls[0]!.init.headers as Record<string, string>).authorization, "Bearer ");
  });

  it("says what failed when Stripe gives no reason", async () => {
    network(() => jsonResponse({}, 400));
    const stripe = new StripeGateway();
    await assert.rejects(stripe.createCheckout(input(), stripeEnv()), /stripe rejected the session/);
    await assert.rejects(stripe.refund(payment, 100, stripeEnv()), /stripe refused the refund/);
  });

  it("records a refund Stripe accepted without naming it", async () => {
    network(() => jsonResponse({}));
    assert.deepEqual(await new StripeGateway().refund(payment, 100, stripeEnv()), { refundRef: "" });
  });

  it("refuses webhooks it cannot authenticate, whatever is wrong with them", async () => {
    const stripe = new StripeGateway();
    const at = Math.floor(Date.now() / 1000);
    const signed = async (body: string, header?: string) =>
      stripe.verifyWebhook(
        new Request(`${ORIGIN}/w`, {
          method: "POST",
          headers: { "stripe-signature": header ?? `t=${at},v1=${await hmacHex(SECRET, `${at}.${body}`)}` },
          body,
        }),
        body,
        stripeEnv(),
      );
    assert.equal(await stripe.verifyWebhook(new Request(`${ORIGIN}/w`), "{}", stripeEnv()), null);
    assert.equal(
      await stripe.verifyWebhook(new Request(`${ORIGIN}/w`, { headers: { "stripe-signature": "t=1,v1=a" } }), "{}", {
        ...stripeEnv(),
        STRIPE_WEBHOOK_SECRET: undefined,
      }),
      null,
    );
    assert.equal(await signed("{}", "v1=abc"), null);
    assert.equal(await signed("{}", `t=${at}`), null);
    assert.equal(await signed("{}", `t=${at},v1=00`), null);
    // Signed correctly, but not something the shop can read.
    assert.equal(await signed("not json"), null);
    assert.equal(await signed(JSON.stringify({ id: "evt_1" })), null);
    // An event with no object is still an event; it maps to nothing in particular.
    const bare = await signed(JSON.stringify({ id: "evt_2", type: "checkout.session.expired" }));
    assert.deepEqual(bare?.event, { kind: "failed", providerRef: "", reason: "expired", raw: {} });
  });

  it("reads events whose objects leave fields out", () => {
    assert.deepEqual(mapStripeEvent("checkout.session.completed", { payment_status: "unpaid" }), {
      kind: "ignored",
      eventType: "checkout.session.completed",
    });
    const paid = mapStripeEvent("checkout.session.async_payment_succeeded", { id: "cs_9" });
    assert.equal(paid.kind, "paid");
    assert.deepEqual(
      { ...paid, raw: undefined },
      { kind: "paid", providerRef: "cs_9", paymentRef: "cs_9", amountMinor: 0, currency: "", cardCountry: null, raw: undefined },
    );
    assert.deepEqual(
      { ...mapStripeEvent("checkout.session.async_payment_succeeded", {}), raw: undefined },
      { kind: "paid", providerRef: "", paymentRef: "", amountMinor: 0, currency: "", cardCountry: null, raw: undefined },
    );
    assert.deepEqual(mapStripeEvent("checkout.session.async_payment_failed", {}), {
      kind: "failed", providerRef: "", reason: "async_payment_failed", raw: {},
    });
    assert.deepEqual(mapStripeEvent("charge.refunded", {}), {
      kind: "refunded", providerRef: "", paymentRef: "", amountMinor: 0, currency: "", raw: {},
    });
    assert.deepEqual(mapStripeEvent("customer.created", {}), { kind: "ignored", eventType: "customer.created" });
  });
});

describe("PayPal, answering oddly", () => {
  const pp = new PaypalGateway();

  it("asks for a token with empty credentials rather than the word undefined", async () => {
    paypal(() => jsonResponse({ id: "R1" }));
    await pp.refund(payment, 100, { ...env, PAYPAL_CLIENT_ID: undefined, PAYPAL_CLIENT_SECRET: undefined });
    assert.equal((calls[0]!.init.headers as Record<string, string>).authorization, `Basic ${btoa(":")}`);
  });

  it("refuses to go on without a token", async () => {
    network(() => jsonResponse({ error: "invalid_client" }, 401));
    await assert.rejects(pp.refund(payment, 100, payEnv()), /paypal auth failed: 401/);
    network(() => jsonResponse({}));
    await assert.rejects(pp.refund(payment, 100, payEnv()), /no access token/);
  });

  it("keeps a token with no stated lifetime for nine hours, then asks again", async () => {
    network((url) => (url.includes("oauth2") ? jsonResponse({ access_token: "long" }) : jsonResponse({ id: "R" })));
    await pp.refund(payment, 100, payEnv());
    await pp.refund(payment, 100, payEnv());
    assert.equal(calls.filter((c) => c.url.includes("oauth2")).length, 1);
  });

  it("copes with an empty answer and with one that is not JSON", async () => {
    paypal(() => new Response(""));
    assert.deepEqual(await pp.refund(payment, 100, payEnv()), { refundRef: "" });
    paypal(() => new Response("<html>gateway timeout</html>", { status: 504 }));
    await assert.rejects(pp.refund(payment, 100, payEnv()), /gateway timeout/);
  });

  it("refuses an order PayPal created without a way to approve it", async () => {
    paypal(() => jsonResponse({ id: "PP-1" }));
    await assert.rejects(pp.createCheckout(input(), payEnv()), /no approval link/);
    paypal(() => jsonResponse({ id: "PP-1", links: [{ rel: "approve", href: "https://paypal.example/approve" }] }));
    assert.deepEqual(await pp.createCheckout(input(), payEnv()), {
      redirectUrl: "https://paypal.example/approve",
      providerRef: "PP-1",
    });
  });

  /** A PayPal webhook with every header PayPal sends, verified by PayPal. */
  async function verify(event: Record<string, unknown>, api: (url: string) => Response) {
    paypal((url) => (url.includes("verify-webhook-signature") ? jsonResponse({ verification_status: "SUCCESS" }) : api(url)));
    const body = JSON.stringify(event);
    const request = new Request(`${ORIGIN}/api/shop/webhooks/paypal`, {
      method: "POST",
      headers: {
        "paypal-auth-algo": "SHA256withRSA",
        "paypal-cert-url": "https://api.paypal.example/cert",
        "paypal-transmission-id": "tid",
        "paypal-transmission-sig": "sig",
        "paypal-transmission-time": "2026-09-01T00:00:00Z",
      },
      body,
    });
    return (await pp.verifyWebhook(request, body, payEnv()))?.event;
  }

  it("reads events that carry no resource, no amount and no ids", async () => {
    const none = () => jsonResponse({});
    assert.deepEqual(await verify({ id: "e1", event_type: "PAYMENT.CAPTURE.COMPLETED" }, none), {
      kind: "paid", providerRef: "", paymentRef: "", amountMinor: 0, currency: "", cardCountry: null, raw: {},
    });
    assert.deepEqual(
      await verify({ id: "e2", event_type: "PAYMENT.CAPTURE.COMPLETED", resource: { amount: { value: "5.00" } } }, none),
      { kind: "paid", providerRef: "", paymentRef: "", amountMinor: 500, currency: "", cardCountry: null, raw: { amount: { value: "5.00" } } },
    );
    assert.deepEqual(await verify({ id: "e3", event_type: "PAYMENT.CAPTURE.REFUNDED" }, none), {
      kind: "refunded", providerRef: "", paymentRef: "", amountMinor: 0, currency: "", raw: {},
    });
    assert.deepEqual(await verify({ id: "e4", event_type: "PAYMENT.CAPTURE.DENIED" }, none), {
      kind: "failed", providerRef: "", reason: "PAYMENT.CAPTURE.DENIED", raw: {},
    });
  });

  it("finds the order a capture belongs to only from a link that names one", async () => {
    const event = await verify(
      {
        id: "e5",
        event_type: "PAYMENT.CAPTURE.COMPLETED",
        resource: { id: "CAP", links: [{ rel: "up", href: "https://api.paypal.example/v2/payments/somewhere-else" }] },
      },
      () => jsonResponse({}),
    );
    assert.equal(event?.kind === "paid" && event.providerRef, "");
  });

  it("takes a capture PayPal reports with nothing but its status", async () => {
    const event = await verify(
      { id: "e6", event_type: "CHECKOUT.ORDER.APPROVED", resource: { id: "PP-9" } },
      () => jsonResponse({ status: "COMPLETED" }),
    );
    assert.deepEqual(event, {
      kind: "paid", providerRef: "PP-9", paymentRef: "PP-9", amountMinor: 0, currency: "", cardCountry: null, raw: { id: "PP-9" },
    });
    const empty = await verify(
      { id: "e7", event_type: "CHECKOUT.ORDER.APPROVED", resource: { id: "PP-8" } },
      () => jsonResponse({ status: "COMPLETED", purchase_units: [{ payments: { captures: [{ id: "C1", amount: {} }] } }], payer: {} }),
    );
    assert.equal(empty?.kind === "paid" && empty.paymentRef, "C1");
    const noCaptures = await verify(
      { id: "e8", event_type: "CHECKOUT.ORDER.APPROVED", resource: { id: "PP-7" } },
      () => jsonResponse({ status: "COMPLETED", purchase_units: [{}] }),
    );
    assert.equal(noCaptures?.kind === "paid" && noCaptures.paymentRef, "PP-7");
  });

  it("approves an order whose id it was not told as a failed capture", async () => {
    const event = await verify({ id: "e9", event_type: "CHECKOUT.ORDER.APPROVED" }, () => jsonResponse({}, 422));
    assert.deepEqual(event, { kind: "failed", providerRef: "", reason: "capture_failed", raw: {} });
  });
});

// ── The shared webhook handler ──────────────────────────────────────────────

async function pendingOrder(ref = "cs_1", sku = "EBOOK-1"): Promise<OrderRow> {
  await seedProduct({ sku, priceMinor: 2400 });
  const priced = (await priceBasket(env, await taxContext(env), {
    lines: [{ sku, quantity: 1 }],
    currency: "EUR",
    billingCountry: "DE",
    ipCountry: null,
    vatId: "",
    vatIdValid: false,
  })) as PricedBasket;
  const buyer = await upsertCustomer(env, {
    email: "buyer@example.com", name: "A Buyer", vatId: "", vatIdValid: null, country: "DE",
  });
  const { order: created } = await createOrder(env, priced, buyer, {
    gateway: "stripe", locale: "en", consentMarketing: false, consentWaiver: true, ipHash: null, userAgent: "t",
  });
  await env.SHOP_DB.prepare(`UPDATE orders SET gateway_ref = ? WHERE id = ?`).bind(ref, created.id).run();
  return { ...created, gateway_ref: ref };
}

async function deliver(event: Record<string, unknown>, over: Record<string, unknown> = {}): Promise<{ status: number; action?: string }> {
  const at = Math.floor(Date.now() / 1000);
  const body = JSON.stringify(event);
  const request = new Request(`${ORIGIN}/api/shop/webhooks/stripe`, {
    method: "POST",
    headers: { "stripe-signature": `t=${at},v1=${await hmacHex(SECRET, `${at}.${body}`)}` },
    body,
  });
  const context = ctx(request);
  const res = (await stripeWebhook({ ...context, env: { ...stripeEnv(), ...over } } as never)) as Response;
  await context.settled();
  const text = await res.text();
  return { status: res.status, action: res.status === 200 ? (JSON.parse(text) as { action: string }).action : undefined };
}

const paid = (id: string, object: Record<string, unknown>) => ({
  id,
  type: "checkout.session.completed",
  data: { object: { id: "cs_1", payment_intent: "pi_1", payment_status: "paid", amount_total: 2400, currency: "eur", ...object } },
});
const refunded = (id: string, amount: number) => ({
  id,
  type: "charge.refunded",
  data: { object: { id: `ch_${id}`, payment_intent: "pi_1", amount_refunded: amount, currency: "eur" } },
});

describe("the webhook handler", () => {
  beforeEach(async () => {
    await freshShop();
    globalThis.fetch = (async () => new Response("{}")) as typeof fetch;
  });

  it("says what is missing when the database or the provider is", async () => {
    const res = (await handleWebhook({ ...ctx(new Request(`${ORIGIN}/w`, { method: "POST" })), env: { ...env, SHOP_DB: undefined } } as never, "stripe"));
    assert.equal(res.status, 503);
    const none = (await paypalWebhook({ ...ctx(new Request(`${ORIGIN}/w`, { method: "POST", body: "{}" })), env: stripeEnv() } as never)) as Response;
    assert.equal(none.status, 503);
    assert.equal(await none.text(), "gateway not configured");
  });

  it("treats a second delivery of one event as a duplicate", async () => {
    await pendingOrder();
    assert.equal((await deliver(paid("evt_a", {}))).action, "fulfilled");
    assert.equal((await deliver(paid("evt_a", {}))).action, "duplicate");
  });

  it("acknowledges, and records, an event its own handling failed on", async () => {
    await pendingOrder();
    env.SHOP_DB.state.failOn = /INSERT OR IGNORE INTO payments/;
    assert.deepEqual(await deliver(paid("evt_b", {})), { status: 200, action: "error" });
    env.SHOP_DB.state.failOn = null;
    const row = await env.SHOP_DB.prepare(`SELECT result FROM webhook_inbox WHERE event_id = 'evt_b'`).first<{ result: string }>();
    assert.match(row!.result, /^error:D1_ERROR/);
  });

  it("answers for an order nobody can find", async () => {
    assert.equal((await deliver(paid("evt_c", { id: "cs_unknown", payment_intent: "pi_unknown" }))).action, "unknown-order");
  });

  it("checks the amount when the provider names no currency, and records it in the order's", async () => {
    await pendingOrder();
    assert.equal((await deliver(paid("evt_d", { currency: undefined, amount_total: 100 }))).action, "needs-review-amount");
    const p = await env.SHOP_DB.prepare(`SELECT currency FROM payments`).first<{ currency: string }>();
    assert.equal(p?.currency, "EUR");
  });

  it("leaves an order that is no longer pending alone when its session expires", async () => {
    const o = await pendingOrder();
    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'fulfilled' WHERE id = ?`).bind(o.id).run();
    const expired = { id: "evt_e", type: "checkout.session.expired", data: { object: { id: "cs_1" } } };
    assert.equal((await deliver(expired)).action, "no-change");
  });

  it("reads a claim or a status change that reports nothing changed as nothing changed", async () => {
    // D1's types make `meta` optional; without it the handler must take the
    // safe reading — a duplicate, no change — rather than act twice.
    await pendingOrder();
    const silentInbox = { SHOP_DB: sparseD1(env.SHOP_DB, /INTO webhook_inbox/, { first: null }) };
    assert.equal((await deliver(paid("evt_s1", {}), silentInbox)).action, "duplicate");
    const silentOrders = { SHOP_DB: sparseD1(env.SHOP_DB, /UPDATE orders SET status = \?/, { first: null }) };
    const expired = { id: "evt_s2", type: "checkout.session.expired", data: { object: { id: "cs_1" } } };
    assert.equal((await deliver(expired, silentOrders)).action, "no-change");
  });

  it("refunds an order that never got an invoice without inventing one", async () => {
    await pendingOrder();
    await env.SHOP_DB.prepare(
      `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, raw_json, created_at)
       SELECT 'p0', id, 'stripe', 'pi_1', 'charge', 'succeeded', 2400, 'EUR', '{}', '2026-01-01' FROM orders`,
    ).run();
    assert.equal((await deliver(refunded("evt_f", 2400))).action, "refunded");
    const notes = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM invoices`).first<{ n: number }>();
    assert.equal(notes?.n, 0);
  });

  it("issues one credit note per refund, even for a buyer whose record is gone", async () => {
    const o = await pendingOrder();
    assert.equal((await deliver(paid("evt_g", {}))).action, "fulfilled");
    await env.SHOP_DB.prepare(`UPDATE orders SET customer_id = NULL WHERE id = ?`).bind(o.id).run();
    assert.equal((await deliver(refunded("evt_h", 1000))).action, "refunded-partial");
    // Another delivery of the same refund under a new event id finds its note.
    assert.equal((await deliver({ ...refunded("evt_i", 1000), data: refunded("evt_h", 1000).data })).action, "refunded-partial");
    const notes = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM invoices WHERE kind != 'invoice'`).first<{ n: number }>();
    assert.equal(notes?.n, 1);
  });

  it("takes the card's country as evidence even when the order kept none, or kept nonsense", async () => {
    const o = await pendingOrder();
    // Stripe events carry no card country; feed one through the gateway's own seam.
    const withCard = {
      async verifyWebhook() {
        return {
          eventId: "evt_j",
          eventType: "x",
          event: { kind: "paid", providerRef: "cs_1", paymentRef: "pi_j", amountMinor: 2400, currency: "EUR", cardCountry: "DE", raw: {} },
        };
      },
    };
    try {
      registerGateway("stripe", () => withCard as never);
      await env.SHOP_DB.prepare(`UPDATE orders SET tax_evidence = NULL WHERE id = ?`).bind(o.id).run();
      const context = ctx(new Request(`${ORIGIN}/w`, { method: "POST", body: "{}" }));
      const res = await handleWebhook({ ...context, env: stripeEnv() } as never, "stripe");
      await context.settled();
      assert.equal(((await res.json()) as { action: string }).action, "fulfilled");
      const row = await env.SHOP_DB.prepare(`SELECT tax_evidence FROM orders WHERE id = ?`).bind(o.id).first<{ tax_evidence: string }>();
      assert.deepEqual(JSON.parse(row!.tax_evidence), { card: "DE" });

      // Evidence the shop cannot parse is replaced, not trusted.
      const o2 = await pendingOrder("cs_2", "EBOOK-2");
      await env.SHOP_DB.prepare(`UPDATE orders SET tax_evidence = '{broken', gateway = NULL WHERE id = ?`).bind(o2.id).run();
      await env.SHOP_DB.prepare(
        `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, raw_json, created_at)
         VALUES ('p2', ?, 'stripe', 'cs_2', 'charge', 'pending', 0, 'EUR', '{}', '2026-01-01')`,
      ).bind(o2.id).run();
      withCard.verifyWebhook = async () => ({
        eventId: "evt_k",
        eventType: "x",
        event: { kind: "paid", providerRef: "cs_2", paymentRef: "pi_k", amountMinor: 2400, currency: "EUR", cardCountry: "PL", raw: {} },
      });
      const c2 = ctx(new Request(`${ORIGIN}/w`, { method: "POST", body: "{}" }));
      await handleWebhook({ ...c2, env: stripeEnv() } as never, "stripe");
      await c2.settled();
      const row2 = await env.SHOP_DB.prepare(`SELECT tax_evidence FROM orders WHERE id = ?`).bind(o2.id).first<{ tax_evidence: string }>();
      assert.deepEqual(JSON.parse(row2!.tax_evidence), { card: "PL" });
      // The order had lost its provider; the payment is still recorded, under none.
      const pay = await env.SHOP_DB.prepare(`SELECT gateway FROM payments WHERE gateway_ref = 'pi_k'`).first<{ gateway: string }>();
      assert.equal(pay?.gateway, "");
    } finally {
      registerGateway("stripe", () => new StripeGateway());
    }
  });
});

// ── The payments screen ─────────────────────────────────────────────────────

describe("the payments screen", () => {
  const admin = { admin: owner };
  const read = async (over: Record<string, unknown>) =>
    bodyOf<{ gateways: Array<{ name: string; mode: string; missing: string[] }>; order: string[] }>(
      (await readGateways({ ...ctx(new Request(`${ORIGIN}/api/shop/admin/gateways`), {}, admin), env: { ...env, ...over } } as never)) as Response,
    );

  beforeEach(async () => {
    await freshShop();
  });

  it("tells live keys from test keys, and says when it cannot tell", async () => {
    const live = await read({ STRIPE_SECRET_KEY: "sk_live_x", PAYPAL_CLIENT_ID: "id", SHOP_PAYPAL_ENV: "live" });
    assert.deepEqual(live.gateways.map((g) => g.mode), ["live", "live"]);
    const odd = await read({ STRIPE_SECRET_KEY: "rk_x", PAYPAL_CLIENT_ID: undefined });
    assert.deepEqual(odd.gateways.map((g) => g.mode), ["unknown", "unknown"]);
  });

  it("shows no events and no payments when the counts come back empty", async () => {
    const body = (await read({ SHOP_DB: sparseD1(env.SHOP_DB, /SELECT COUNT\(\*\)/) })) as unknown as {
      gateways: Array<{ eventsSeen: number; paymentsTaken: number; lastEventAt: string | null }>;
    };
    assert.deepEqual(body.gateways.map((g) => [g.eventsSeen, g.paymentsTaken, g.lastEventAt]), [[0, 0, null], [0, 0, null]]);
  });

  it("keeps the order an owner chose, and reports the providers still missing keys", async () => {
    const both = { STRIPE_SECRET_KEY: "sk_test_x", STRIPE_WEBHOOK_SECRET: "w", PAYPAL_CLIENT_ID: "id", PAYPAL_CLIENT_SECRET: "s", PAYPAL_WEBHOOK_ID: "h" };
    const res = (await writeOrder({
      ...ctx(new Request(`${ORIGIN}/api/shop/admin/gateways`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ order: ["paypal", "stripe"] }),
      }), {}, admin),
      env: { ...env, ...both },
    } as never)) as Response;
    assert.equal(res.status, 200);
    const after = await read({ STRIPE_SECRET_KEY: undefined, STRIPE_WEBHOOK_SECRET: undefined });
    assert.deepEqual(after.order, ["paypal", "stripe"]);
    assert.deepEqual(after.gateways[0]!.missing, ["STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET"]);
  });

  it("names a provider that is half configured when refusing to offer it", async () => {
    const res = (await writeOrder({
      ...ctx(new Request(`${ORIGIN}/api/shop/admin/gateways`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ order: ["paypal"] }),
      }), {}, admin),
      env: { ...env, PAYPAL_CLIENT_ID: "id", PAYPAL_CLIENT_SECRET: "s", PAYPAL_WEBHOOK_ID: undefined },
    } as never)) as Response;
    assert.equal(res.status, 422);
    assert.match(JSON.stringify(await res.json()), /PayPal needs PAYPAL_WEBHOOK_ID/);
  });
});
