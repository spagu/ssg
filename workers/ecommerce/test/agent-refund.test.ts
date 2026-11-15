// A refund recorded by the payment provider's webhook tells the agent
// platform the order came from (GO-167): ACP `order_update`, the UCP order
// event with a refund adjustment — and nothing for a storefront order.

import { env } from "./_env";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import * as ucpCreate from "../functions/api/shop/ucp/checkout-sessions/index.ts";
import * as ucpComplete from "../functions/api/shop/ucp/checkout-sessions/[id]/complete.ts";
import * as acpCreate from "../functions/api/shop/acp/checkout_sessions/index.ts";
import * as acpComplete from "../functions/api/shop/acp/checkout_sessions/[id]/complete.ts";
import { onRequestPost as stripeWebhook } from "../functions/api/shop/webhooks/stripe";
import { announceRefund } from "../functions/api/shop/_agent-refund";
import { hmacHex } from "../functions/api/shop/_gateway";
import { drainOutbox } from "../functions/api/shop/_outbox";
import { ctx, freshShop, seedProduct } from "./_helpers";
import { acpRequest, call, enableAcp, fakeStripe as fakeAcpStripe } from "./acp-helpers";
import { enableUcp, fakeReceiver, fakeStripe, payWith, ucpRequest, WEBHOOK_URL } from "./ucp-helpers";

type Json = Record<string, any>;
const WHSEC = "whsec_refund_test";
const realFetch = globalThis.fetch;
let events = 0;

async function ucpOrder(token: string): Promise<{ orderId: string; pi: string }> {
  globalThis.fetch = fakeStripe().fetch;
  const made = (await (await call(ucpCreate.onRequestPost as never, ucpRequest("POST", "/checkout-sessions", {
    line_items: [{ item: { id: "EBOOK-1" }, quantity: 1 }], buyer: { email: "buyer@example.com" }, context: { address_country: "DE" },
  }))).json()) as Json;
  const done = (await (await call(ucpComplete.onRequestPost as never, ucpRequest("POST", `/checkout-sessions/${made.id}/complete`, payWith(token)), { id: made.id })).json()) as Json;
  return { orderId: done.order.id, pi: `pi_1_${token}` };
}

async function acpOrder(token: string): Promise<{ orderId: string; pi: string }> {
  globalThis.fetch = fakeAcpStripe().fetch;
  const made = (await (await call(acpCreate.onRequestPost as never, acpRequest("POST", "/checkout_sessions", {
    items: [{ id: "EBOOK-1", quantity: 1 }], fulfillment_details: { address: { country: "DE" } },
  }))).json()) as Json;
  const done = (await (await call(acpComplete.onRequestPost as never, acpRequest("POST", `/checkout_sessions/${made.id}/complete`, {
    buyer: { email: "buyer@example.com" }, payment_data: { provider: "stripe", token },
  }), { id: made.id })).json()) as Json;
  return { orderId: done.order.id, pi: `pi_1_${token}` };
}

/** Stripe's charge.refunded, signed, through the shop's webhook route. */
async function refund(pi: string, amount: number, charge = "ch_1"): Promise<Json> {
  const body = JSON.stringify({ id: `evt_${++events}`, type: "charge.refunded", data: { object: { id: charge, payment_intent: pi, amount_refunded: amount, currency: "eur" } } });
  const t = Math.floor(Date.now() / 1000);
  const request = new Request("https://shop.example.com/api/shop/webhooks/stripe", {
    method: "POST",
    headers: { "stripe-signature": `t=${t},v1=${await hmacHex(WHSEC, `${t}.${body}`)}`, "content-type": "application/json" },
    body,
  });
  globalThis.fetch = (async () => { throw new Error("offline"); }) as typeof fetch;
  const c = ctx(request);
  const res = await stripeWebhook(c as never);
  await c.settled();
  return (await res.json()) as Json;
}

const events_ = (kind: string) =>
  env.SHOP_DB.prepare(`SELECT payload_json FROM outbox WHERE kind = ? ORDER BY created_at, rowid`).bind(kind).all<{ payload_json: string }>()
    .then((r) => (r.results ?? []).map((x) => JSON.parse(x.payload_json) as Json));

beforeEach(async () => {
  await freshShop();
  enableUcp();
  enableAcp();
  Object.assign(env, { STRIPE_WEBHOOK_SECRET: WHSEC, SHOP_GATEWAYS: "stripe" });
  await seedProduct({ sku: "EBOOK-1", name: "A Book", priceMinor: 2400 });
});

afterEach(() => {
  globalThis.fetch = realFetch;
  env.SHOP_DB.state.failOn = null;
});

describe("refunds reach the agent platform", () => {
  it("UCP: a partial refund is an adjustment, a full one removes the lines; Webhook-Id is the adjustment", async () => {
    const { orderId, pi } = await ucpOrder("tok_ok_r1");
    assert.equal((await refund(pi, 500, "re_part")).action, "refunded-partial");
    let sent = await events_("ucp");
    assert.equal(sent.length, 2);
    const partial = sent[1]!;
    const adj = partial.order.adjustments;
    assert.equal(adj.length, 1);
    assert.equal(partial.eventId, adj[0].id);
    assert.deepEqual({ ...adj[0], id: "x", occurred_at: "t" }, { id: "x", type: "refund", occurred_at: "t", status: "completed", totals: [{ type: "total", display_text: "Refund", amount: -500 }], description: "Refund" });
    assert.equal(partial.order.line_items[0].status, "fulfilled");

    await refund(pi, 2400, "re_full");
    sent = await events_("ucp");
    const full = sent[2]!.order;
    assert.equal(full.adjustments.length, 2);
    assert.equal(sent[2]!.eventId, full.adjustments[1].id);
    assert.equal(full.line_items[0].status, "removed");
    assert.deepEqual(full.line_items[0].quantity, { original: 1, total: 0, fulfilled: 1 });
    assert.equal(full.id, orderId);

    // Delivered by the outbox's own wiring, signed like every UCP event.
    const receiver = fakeReceiver();
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) =>
      String(input) === WEBHOOK_URL ? receiver.handle(new Request(String(input), init)) : new Response("no", { status: 500 })) as typeof fetch;
    await env.SHOP_DB.prepare(`UPDATE outbox SET next_attempt = '2000-01-01T00:00:00.000Z'`).run();
    await drainOutbox(env, 20);
    assert.equal(receiver.received.length, 3);
    assert.ok(receiver.received.every((r) => r.verified));
  });

  it("ACP: an order_update with the refund as an adjustment and the amount refunded", async () => {
    const { pi } = await acpOrder("spt_ok_r2");
    await refund(pi, 700, "re_acp");
    const sent = await events_("acp");
    assert.deepEqual(sent.map((e) => e.type), ["order_create", "order_update"]);
    const order = sent[1]!.data;
    assert.equal(order.adjustments[0].type, "refund");
    assert.equal(order.adjustments[0].amount, 700);
    assert.equal(order.adjustments[0].currency, "eur");
    assert.deepEqual(order.totals.at(-1), { type: "amount_refunded", display_text: "Refunded", amount: 700 });
    assert.equal(sent[0]!.data.adjustments, undefined);
  });

  it("a storefront order tells nobody, and nor does an order whose session or URL is gone", async () => {
    const { orderId } = await ucpOrder("tok_ok_r3");
    await env.SHOP_DB.prepare(`UPDATE orders SET channel = 'web' WHERE id = ?`).bind(orderId).run();
    assert.equal(await announceRefund(env, orderId, "https://shop.example.com"), null);
    await env.SHOP_DB.prepare(`UPDATE orders SET channel = 'ucp' WHERE id = ?`).bind(orderId).run();
    // No refund recorded yet: nothing to name the event after.
    assert.equal(await announceRefund(env, orderId, "https://shop.example.com"), null);
    await env.SHOP_DB.prepare(`DELETE FROM ucp_sessions`).run();
    assert.equal(await announceRefund(env, orderId, "https://shop.example.com"), null);
    assert.equal(await announceRefund(env, "no-such-order", "https://shop.example.com"), null);

    const acp = await acpOrder("spt_ok_r4");
    await env.SHOP_DB.prepare(`DELETE FROM acp_sessions`).run();
    assert.equal(await announceRefund(env, acp.orderId, "https://shop.example.com"), null);
    const before = (await events_("acp")).length + (await events_("ucp")).length;
    enableAcp({ SHOP_ACP_WEBHOOK_URL: undefined });
    const again = await acpOrder("spt_ok_r5");
    assert.equal(await announceRefund(env, again.orderId, "https://shop.example.com"), null);
    assert.equal((await events_("acp")).length + (await events_("ucp")).length, before);
  });

  it("a failure to queue the event is logged; the refund itself still stands", async () => {
    const { orderId, pi } = await ucpOrder("tok_ok_r6");
    env.SHOP_DB.state.failOn = /^SELECT \* FROM ucp_sessions WHERE order_id/;
    assert.equal((await refund(pi, 2400, "re_fail")).action, "refunded");
    env.SHOP_DB.state.failOn = null;
    const status = await env.SHOP_DB.prepare(`SELECT status FROM orders WHERE id = ?`).bind(orderId).first<Json>();
    assert.equal(status?.status, "refunded");
    const logged = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM audit_log WHERE action = 'order.refund_event_failed'`).first<{ n: number }>();
    assert.equal(logged?.n, 1);
  });
});
