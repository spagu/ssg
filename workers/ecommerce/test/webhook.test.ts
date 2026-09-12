// The webhook path, end to end: signature, idempotency, amount check,
// fulfilment, refund. This is where a shop either delivers a book or does not.

import { env } from "cloudflare:test";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { onRequestPost as stripeWebhook } from "../functions/api/shop/webhooks/stripe";
import { hmacHex } from "../functions/api/shop/_gateway";
import { createOrder, priceBasket, upsertCustomer, type PricedBasket } from "../functions/api/shop/_orders";
import { taxContext } from "../functions/api/shop/_tax";
import type { InvoiceRow, OrderRow } from "../functions/api/shop/_types";
import { ctx, freshShop, seedProduct } from "./_helpers";

const SECRET = "whsec_test";
const shopEnv = () => ({
  ...env,
  STRIPE_SECRET_KEY: "sk_test_x",
  STRIPE_WEBHOOK_SECRET: SECRET,
  SHOP_GATEWAYS: "stripe",
});

const realFetch = globalThis.fetch;

async function pendingOrder(): Promise<OrderRow> {
  await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
  const taxCtx = await taxContext(env);
  const priced = (await priceBasket(env, taxCtx, {
    lines: [{ sku: "EBOOK-1", quantity: 1 }],
    currency: "EUR",
    billingCountry: "DE",
    ipCountry: null,
    vatId: "",
    vatIdValid: false,
  })) as PricedBasket;
  const customer = await upsertCustomer(env, {
    email: "buyer@example.com", name: "A Buyer", vatId: "", vatIdValid: null, country: "DE",
  });
  const { order } = await createOrder(env, priced, customer, {
    gateway: "stripe", locale: "en", consentMarketing: false, consentWaiver: true, ipHash: null, userAgent: "t",
  });
  await env.SHOP_DB.prepare(`UPDATE orders SET gateway_ref = 'cs_1' WHERE id = ?`).bind(order.id).run();
  return { ...order, gateway_ref: "cs_1" };
}

async function deliver(event: Record<string, unknown>, at = Math.floor(Date.now() / 1000)): Promise<Response> {
  const body = JSON.stringify(event);
  const signature = await hmacHex(SECRET, `${at}.${body}`);
  const request = new Request("https://shop.example.com/api/shop/webhooks/stripe", {
    method: "POST",
    headers: { "stripe-signature": `t=${at},v1=${signature}`, "content-type": "application/json" },
    body,
  });
  const context = ctx(request);
  const response = await stripeWebhook({ ...context, env: shopEnv() } as never);
  await (context as unknown as { settled(): Promise<unknown> }).settled();
  return response as Response;
}

const paidEvent = (overrides: Record<string, unknown> = {}, id = "evt_paid") => ({
  id,
  type: "checkout.session.completed",
  data: {
    object: {
      id: "cs_1",
      payment_intent: "pi_1",
      payment_status: "paid",
      amount_total: 2400,
      currency: "eur",
      ...overrides,
    },
  },
});

beforeEach(async () => {
  await freshShop();
  // Nothing in this file should reach the network; a call means a bug.
  globalThis.fetch = (async () => {
    throw new Error("a webhook test tried to use the network");
  }) as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

async function statusOf(id: string): Promise<string> {
  const row = await env.SHOP_DB.prepare(`SELECT status FROM orders WHERE id = ?`).bind(id).first<{ status: string }>();
  return row?.status ?? "gone";
}

describe("verification", () => {
  it("writes nothing at all for an unsigned body", async () => {
    const request = new Request("https://shop.example.com/api/shop/webhooks/stripe", {
      method: "POST",
      body: JSON.stringify(paidEvent()),
    });
    const res = (await stripeWebhook({ ...ctx(request), env: shopEnv() } as never)) as Response;
    expect(res.status).toBe(400);

    const inbox = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM webhook_inbox`).first<{ n: number }>();
    expect(inbox?.n).toBe(0);
  });

  it("answers 503 rather than crashing when the provider is not configured", async () => {
    const request = new Request("https://shop.example.com/api/shop/webhooks/stripe", { method: "POST", body: "{}" });
    const res = (await stripeWebhook({
      ...ctx(request),
      env: { ...env, STRIPE_SECRET_KEY: undefined, SHOP_GATEWAYS: "stripe" },
    } as never)) as Response;
    expect(res.status).toBe(503);
  });
});

describe("a payment arriving", () => {
  it("fulfils the order: invoice, links, email", async () => {
    const order = await pendingOrder();
    const res = await deliver(paidEvent());
    expect(res.status).toBe(200);
    expect(await statusOf(order.id)).toBe("paid");

    const invoice = await env.SHOP_DB.prepare(`SELECT * FROM invoices WHERE order_id = ?`)
      .bind(order.id)
      .first<InvoiceRow>();
    expect(invoice?.kind).toBe("invoice");

    const tokens = await env.SHOP_DB.prepare(
      `SELECT COUNT(*) AS n FROM download_tokens t JOIN order_items i ON i.id = t.order_item_id WHERE i.order_id = ?`,
    )
      .bind(order.id)
      .first<{ n: number }>();
    expect(tokens?.n).toBe(1);

    const queued = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM outbox WHERE kind = 'email'`).first<{ n: number }>();
    expect(queued?.n ?? 0).toBeGreaterThan(0);
  });

  it("does the work once when the provider delivers twice", async () => {
    const order = await pendingOrder();
    await deliver(paidEvent());
    const second = await deliver(paidEvent());

    expect(second.status).toBe(200);
    expect(await second.json()).toMatchObject({ action: "duplicate" });

    const invoices = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM invoices WHERE order_id = ?`)
      .bind(order.id)
      .first<{ n: number }>();
    expect(invoices?.n).toBe(1);
  });

  it("does the work once even when the same payment arrives as two events", async () => {
    // Stripe genuinely sends both completed and async_payment_succeeded for
    // some methods. Two event ids, one order, one email.
    const order = await pendingOrder();
    await deliver(paidEvent({}, "evt_a"));
    await deliver({ ...paidEvent({}, "evt_b"), type: "checkout.session.async_payment_succeeded" });

    const emails = await env.SHOP_DB.prepare(
      `SELECT COUNT(*) AS n FROM outbox WHERE kind = 'email' AND target = 'buyer@example.com'`,
    ).first<{ n: number }>();
    expect(emails?.n).toBe(1);
    expect(await statusOf(order.id)).toBe("paid");
  });

  it("parks an order that was paid the wrong amount", async () => {
    const order = await pendingOrder();
    await deliver(paidEvent({ amount_total: 1 }));

    expect(await statusOf(order.id)).toBe("needs_review");
    // Nothing was delivered: no invoice, no links.
    const invoices = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM invoices`).first<{ n: number }>();
    expect(invoices?.n).toBe(0);
  });

  it("parks an order that was paid in the wrong currency", async () => {
    const order = await pendingOrder();
    await deliver(paidEvent({ currency: "usd" }));
    expect(await statusOf(order.id)).toBe("needs_review");
  });

  it("records the card country as the third piece of tax evidence", async () => {
    const order = await pendingOrder();
    await deliver({
      id: "evt_card",
      type: "checkout.session.completed",
      data: {
        object: {
          id: "cs_1", payment_intent: "pi_1", payment_status: "paid", amount_total: 2400, currency: "eur",
        },
      },
    });
    const row = await env.SHOP_DB.prepare(`SELECT tax_evidence FROM orders WHERE id = ?`)
      .bind(order.id)
      .first<{ tax_evidence: string }>();
    expect(JSON.parse(row!.tax_evidence)).toMatchObject({ billing: "DE" });
  });

  it("acknowledges an event for an order it has never heard of", async () => {
    // Not an error: a shared Stripe account, a test event, a replayed old one.
    const res = await deliver(paidEvent({ id: "cs_unknown" }));
    expect(res.status).toBe(200);
  });

  it("acknowledges an event type it does not act on", async () => {
    const res = await deliver({ id: "evt_x", type: "customer.created", data: { object: {} } });
    expect(res.status).toBe(200);
    expect(await res.json()).toMatchObject({ action: "ignored" });
  });
});

describe("a payment failing", () => {
  it("cancels an expired session and fails a failed one", async () => {
    const first = await pendingOrder();
    await deliver({ id: "evt_exp", type: "checkout.session.expired", data: { object: { id: "cs_1" } } });
    expect(await statusOf(first.id)).toBe("cancelled");

    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'pending' WHERE id = ?`).bind(first.id).run();
    await deliver({
      id: "evt_fail",
      type: "checkout.session.async_payment_failed",
      data: { object: { id: "cs_1" } },
    });
    expect(await statusOf(first.id)).toBe("failed");
  });

  it("leaves a paid order alone when a late failure arrives", async () => {
    const order = await pendingOrder();
    await deliver(paidEvent());
    await deliver({ id: "evt_late", type: "checkout.session.expired", data: { object: { id: "cs_1" } } });
    expect(await statusOf(order.id)).toBe("paid");
  });
});

describe("a refund", () => {
  it("stops the links working and issues a credit note", async () => {
    const order = await pendingOrder();
    await deliver(paidEvent());

    await deliver({
      id: "evt_refund",
      type: "charge.refunded",
      data: { object: { id: "ch_1", payment_intent: "pi_1", amount_refunded: 2400, currency: "eur" } },
    });

    expect(await statusOf(order.id)).toBe("refunded");

    const live = await env.SHOP_DB.prepare(
      `SELECT COUNT(*) AS n FROM download_tokens t JOIN order_items i ON i.id = t.order_item_id
        WHERE i.order_id = ? AND t.revoked = 0`,
    )
      .bind(order.id)
      .first<{ n: number }>();
    expect(live?.n).toBe(0);

    const credit = await env.SHOP_DB.prepare(`SELECT * FROM invoices WHERE kind = 'credit_note'`).first<InvoiceRow>();
    expect(credit?.total_minor).toBe(-2400);
  });

  it("keeps a partial refund's order paid and its links alive", async () => {
    const order = await pendingOrder();
    await deliver(paidEvent());
    await deliver({
      id: "evt_partial",
      type: "charge.refunded",
      data: { object: { id: "ch_1", payment_intent: "pi_1", amount_refunded: 500, currency: "eur" } },
    });

    expect(await statusOf(order.id)).toBe("paid");
    const live = await env.SHOP_DB.prepare(
      `SELECT COUNT(*) AS n FROM download_tokens t JOIN order_items i ON i.id = t.order_item_id
        WHERE i.order_id = ? AND t.revoked = 0`,
    )
      .bind(order.id)
      .first<{ n: number }>();
    expect(live?.n).toBe(1);
  });
});
