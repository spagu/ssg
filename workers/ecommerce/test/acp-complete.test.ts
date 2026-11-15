// Completing an ACP session (GO-163): the Shared Payment Token charge, the
// same fulfilment as a storefront order, the order's channel, the event for
// the platform, and everything that must happen only once.

import { env } from "./_env";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import * as createRoute from "../functions/api/shop/acp/checkout_sessions/index.ts";
import * as sessionRoute from "../functions/api/shop/acp/checkout_sessions/[id].ts";
import * as completeRoute from "../functions/api/shop/acp/checkout_sessions/[id]/complete.ts";
import * as cancelRoute from "../functions/api/shop/acp/checkout_sessions/[id]/cancel.ts";
import * as storefront from "../functions/api/shop/checkout";
import * as adminOrder from "../functions/api/shop/admin/orders/[id].ts";
import { drainOutbox } from "../functions/api/shop/_outbox";
import { freshShop, seedProduct, post, ctx } from "./_helpers";
import { acpRequest, call, enableAcp, fakeReceiver, fakeStripe, lossyD1, RECEIVER, SECRETS, type FakeStripe } from "./acp-helpers";
import { completeSession } from "../functions/api/shop/_acp-complete";

type Json = Record<string, any>;
const DE = { address: { name: "A Buyer", line_one: "1 Example Way", city: "Berlin", state: "BE", country: "DE", postal_code: "10115" } };
const BUYER = { email: "buyer@example.com", first_name: "Ann", last_name: "Buyer" };
const realFetch = globalThis.fetch;
let stripe: FakeStripe;
const seen: string[] = [];

const pay = (token: string) => ({ handler_id: "card_tokenized", instrument: { type: "card", credential: { type: "spt", token } } });

async function session(body: Json = {}): Promise<Json> {
  const res = await call(createRoute.onRequestPost as never, acpRequest("POST", "/checkout_sessions", { items: [{ id: "EBOOK-1", quantity: 2 }], fulfillment_details: DE, ...body }));
  const json = (await res.json()) as Json;
  seen.push(JSON.stringify(json));
  return json;
}

async function complete(id: string, body: unknown, headers: Record<string, string> = {}): Promise<{ res: Response; json: Json }> {
  const res = await call(completeRoute.onRequestPost as never, acpRequest("POST", `/checkout_sessions/${id}/complete`, body, headers), { id });
  const text = await res.text();
  seen.push(text, JSON.stringify([...res.headers]));
  return { res, json: JSON.parse(text) as Json };
}

async function count(sql: string, ...args: unknown[]): Promise<number> {
  const row = await env.SHOP_DB.prepare(sql).bind(...args).first<{ n: number }>();
  return row?.n ?? 0;
}

beforeEach(async () => {
  await freshShop();
  enableAcp();
  await seedProduct({ sku: "EBOOK-1", name: "A Book", priceMinor: 2400 });
  stripe = fakeStripe();
  globalThis.fetch = stripe.fetch;
});

afterEach(() => {
  globalThis.fetch = realFetch;
  env.SHOP_DB.state.failOn = null;
});

describe("completing a session", () => {
  it("charges the token once and fulfils the order exactly like a storefront order", async () => {
    const made = await session();
    const { res, json } = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_1") });
    assert.equal(res.status, 200);
    assert.equal(json.status, "completed");
    assert.equal(json.buyer.email, "buyer@example.com");

    // The Stripe call, as Stripe's SPT page describes it.
    assert.equal(stripe.created.length, 1);
    const { form, headers } = stripe.created[0]!;
    assert.equal(form.get("amount"), "4800");
    assert.equal(form.get("currency"), "eur");
    assert.equal(form.get("confirm"), "true");
    assert.equal(form.get("payment_method_data[shared_payment_granted_token]"), "spt_ok_1");
    assert.equal(form.get("metadata[acp_checkout_session_id]"), made.id);
    assert.equal(headers.get("stripe-version"), "2026-09-30.preview");
    assert.match(headers.get("idempotency-key") ?? "", new RegExp(`^acp:${made.id}:[0-9a-f]{32}$`));

    // The order: channel acp, paid, an invoice, a download token, an email.
    const order = json.order;
    assert.equal(order.type, "order");
    assert.equal(order.checkout_session_id, made.id);
    assert.equal(order.status, "confirmed");
    assert.match(order.permalink_url, /^https:\/\/shop\.example\.com\/shop\/thanks\/\?order=.+&k=.+/);
    assert.match(order.confirmation.invoice_number, /^FV\/\d{4}\/000001$/);
    assert.equal(order.line_items[0].quantity.fulfilled, 2);
    assert.deepEqual(order.totals.map((t: Json) => t.amount), [4486, 314, 4800]);
    const row = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(order.id).first<Json>();
    assert.equal(row?.channel, "acp");
    assert.equal(row?.status, "paid");
    assert.equal(row?.gateway, "stripe");
    assert.equal(row?.gateway_ref, "pi_1_spt_ok_1");
    assert.equal(row?.consent_waiver, 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM payments WHERE order_id = ? AND kind = 'charge' AND amount_minor = 4800`, order.id), 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM download_tokens t JOIN order_items i ON i.id = t.order_item_id WHERE i.order_id = ?`, order.id), 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM outbox WHERE kind = 'email' AND target = 'buyer@example.com'`), 1);
    const raw = await env.SHOP_DB.prepare(`SELECT raw_json FROM payments WHERE order_id = ?`).bind(order.id).first<{ raw_json: string }>();
    assert.ok(!raw?.raw_json.includes("pi_secret_must_not_be_stored"));

    // The admin API shows the channel.
    const admin = await adminOrder.onRequestGet(ctx(new Request(`https://shop.example.com/api/shop/admin/orders/${order.id}`), { id: order.id }) as never);
    assert.equal(((await admin.json()) as Json).order.channel, "acp");

    // A GET afterwards tells the same story.
    const again = (await (await call(sessionRoute.onRequestGet as never, acpRequest("GET", `/checkout_sessions/${made.id}`), { id: made.id })).json()) as Json;
    assert.deepEqual(again.order, order);
  });

  it("prices and taxes the basket exactly as the storefront checkout does", async () => {
    const made = await session();
    const { json } = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_2") });
    const acp = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(json.order.id).first<Json>();

    globalThis.fetch = (async () => Response.json({ id: "cs_test_1", url: "https://checkout.stripe.com/x" })) as typeof fetch;
    const res = await storefront.onRequestPost(ctx(post("/api/shop/checkout", {
      items: [{ sku: "EBOOK-1", quantity: 2 }], email: "web@example.com", country: "DE", gateway: "stripe", consentWaiver: true,
    })) as never);
    assert.equal(res.status, 200);
    const web = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(((await res.json()) as Json).orderId).first<Json>();
    assert.equal(web?.channel, "web");
    for (const field of ["currency", "subtotal_minor", "tax_minor", "total_minor", "tax_country", "reverse_charge"]) {
      assert.equal(acp?.[field], web?.[field], field);
    }
    const items = async (id: string) => (await env.SHOP_DB.prepare(`SELECT sku, quantity, unit_minor, tax_rate_bp, tax_minor, total_minor FROM order_items WHERE order_id = ?`).bind(id).all()).results;
    assert.deepEqual(await items(acp!.id), await items(web!.id));
  });

  it("queues a signed order_create event and delivers it through the outbox", async () => {
    const made = await session();
    const { json } = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_3") });
    const row = await env.SHOP_DB.prepare(`SELECT * FROM outbox WHERE kind = 'acp'`).first<Json>();
    assert.equal(row?.target, RECEIVER);
    const payload = JSON.parse(row!.payload_json);
    assert.equal(payload.type, "order_create");
    assert.deepEqual(payload.data, json.order);
    for (const secret of SECRETS) assert.ok(!row!.payload_json.includes(secret));

    const receiver = fakeReceiver();
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input) === RECEIVER) return receiver.handle(new Request(String(input), init));
      return new Response("ok");
    }) as typeof fetch;
    await env.SHOP_DB.prepare(`UPDATE outbox SET next_attempt = '2000-01-01T00:00:00.000Z', done_at = NULL WHERE kind = 'acp'`).run();
    await drainOutbox(env, 10);
    assert.equal(receiver.received.length, 1);
    assert.equal(receiver.received[0]!.verified, true);
    assert.equal(receiver.received[0]!.body.type, "order_create");
    const done = await env.SHOP_DB.prepare(`SELECT done_at FROM outbox WHERE kind = 'acp'`).first<Json>();
    assert.ok(done?.done_at);
  });

  it("queues no event when no receiver is configured", async () => {
    enableAcp({ SHOP_ACP_WEBHOOK_URL: undefined });
    const made = await session();
    const { json } = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_4") });
    assert.equal(json.status, "completed");
    assert.equal(await count(`SELECT COUNT(*) AS n FROM outbox WHERE kind = 'acp'`), 0);
  });

  it("accepts the legacy {token, provider} payment_data and the buyer stored on the session", async () => {
    const made = await session({ buyer: { email: "stored@example.com", full_name: "Stored Buyer" } });
    const { json } = await complete(made.id, { payment_data: { token: "spt_ok_legacy", provider: "stripe" } });
    assert.equal(json.status, "completed");
    const customer = await env.SHOP_DB.prepare(`SELECT name FROM customers WHERE email = 'stored@example.com'`).first<Json>();
    assert.equal(customer?.name, "Stored Buyer");
  });

  it("refuses payment data it cannot charge", async () => {
    const made = await session({ buyer: BUYER });
    const bad: unknown[] = [
      undefined, null, "spt_x", {},
      { provider: "paypal", token: "spt_x" },
      { provider: "stripe", token: "has spaces" },
      { handler_id: "other", instrument: { type: "card", credential: { type: "spt", token: "spt_x" } } },
      { instrument: { type: "card" } },
      { instrument: { type: "card", credential: { type: "vault_token", token: "vt_x" } } },
    ];
    for (const payment_data of bad) {
      const { res, json } = await complete(made.id, { payment_data });
      assert.equal(res.status, 400, JSON.stringify(payment_data));
      assert.deepEqual([json.code, json.param], ["invalid", "$.payment_data"]);
    }
    const badBuyer = await complete(made.id, { payment_data: pay("spt_ok"), buyer: { email: "x" } });
    assert.equal(badBuyer.json.param, "$.buyer.email");
    assert.equal(stripe.calls, 0);
  });

  it("needs a buyer email, a ready session and an existing id", async () => {
    const noBuyer = await session();
    const a = await complete(noBuyer.id, { payment_data: pay("spt_ok") });
    assert.deepEqual([a.res.status, a.json.code, a.json.param], [400, "missing", "$.buyer.email"]);
    const notReady = await session({ fulfillment_details: undefined });
    const b = await complete(notReady.id, { buyer: BUYER, payment_data: pay("spt_ok") });
    assert.deepEqual([b.res.status, b.json.code], [400, "not_ready_for_payment"]);
    const c = await complete("cs_missing", { buyer: BUYER, payment_data: pay("spt_ok") });
    assert.equal(c.res.status, 404);
    assert.equal(stripe.calls, 0);
  });
});

describe("only once", () => {
  it("replays the same Idempotency-Key without a second charge", async () => {
    const made = await session();
    const key = { "idempotency-key": "complete-once" };
    const first = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_idem") }, key);
    const second = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_idem") }, key);
    assert.equal(second.res.status, 200);
    assert.equal(second.res.headers.get("idempotent-replayed"), "true");
    assert.deepEqual(second.json, first.json);
    assert.equal(stripe.calls, 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM invoices`), 1);
  });

  it("lets exactly one of two concurrent completes win", async () => {
    const made = await session();
    const [a, b] = await Promise.all([
      complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_a") }),
      complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_b") }),
    ]);
    const statuses = [a.res.status, b.res.status].sort();
    assert.deepEqual(statuses, [200, 409]);
    const loser = a.res.status === 409 ? a : b;
    assert.equal(loser.json.code, "invalid_session_state");
    assert.equal(stripe.created.length, 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM orders WHERE channel = 'acp'`), 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM invoices`), 1);
  });

  it("answers 409 when the claim itself loses the race", async () => {
    const made = await session({ buyer: BUYER });
    const lossy = { ...env, SHOP_DB: lossyD1(env.SHOP_DB, /^UPDATE acp_sessions SET status = \?, buyer_json/) };
    const out = await completeSession({ request: new Request("https://shop.example.com/"), env: lossy as never, body: { payment_data: pay("spt_ok_l") }, id: made.id, origin: "https://shop.example.com", waitUntil: () => {} });
    assert.equal(out.status, 409);
    assert.equal(stripe.calls, 0);
  });

  it("keeps the background outbox drain from failing the completion", async () => {
    const made = await session();
    env.SHOP_DB.state.failOn = /FROM outbox WHERE done_at IS NULL/;
    const { json } = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_drain") });
    assert.equal(json.status, "completed");
  });

  it("passes an order status it has no word for through unchanged", async () => {
    const made = await session();
    const { json } = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_odd") });
    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'on_hold' WHERE id = ?`).bind(json.order.id).run();
    const again = (await (await call(sessionRoute.onRequestGet as never, acpRequest("GET", `/checkout_sessions/${made.id}`), { id: made.id })).json()) as Json;
    assert.equal(again.order.status, "on_hold");
  });

  it("refuses to complete a completed session again, and to cancel it", async () => {
    const made = await session();
    await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_c") });
    const again = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_d") });
    assert.equal(again.res.status, 409);
    assert.equal(stripe.created.length, 1);
    const cancel = await call(cancelRoute.onRequestPost as never, acpRequest("POST", `/checkout_sessions/${made.id}/cancel`, {}), { id: made.id });
    assert.equal(cancel.status, 405);
  });
});

describe("when the payment does not go through", () => {
  it("puts the session back after a decline, keeps the order, and completes with another token", async () => {
    const made = await session();
    const declined = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_declined_1") });
    assert.equal(declined.res.status, 200);
    assert.equal(declined.json.status, "ready_for_payment");
    assert.equal(declined.json.order, undefined);
    const error = declined.json.messages.find((m: Json) => m.type === "error");
    assert.deepEqual([error.code, error.param, error.resolution], ["payment_declined", "$.payment_data", "requires_buyer_input"]);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM orders WHERE status = 'pending' AND channel = 'acp'`), 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM invoices`), 0);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM audit_log WHERE action = 'acp.payment_declined'`), 1);

    const paid = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_after") });
    assert.equal(paid.json.status, "completed");
    assert.equal(await count(`SELECT COUNT(*) AS n FROM orders WHERE channel = 'acp'`), 1);
    assert.match(paid.json.order.confirmation.invoice_number, /000001$/);
  });

  it("treats 3-D Secure and a bodiless refusal as declines", async () => {
    const made = await session();
    for (const token of ["spt_3ds_1", "spt_bare_1"]) {
      const { json } = await complete(made.id, { buyer: BUYER, payment_data: pay(token) });
      assert.equal(json.status, "ready_for_payment");
    }
    const reasons = (await env.SHOP_DB.prepare(`SELECT detail_json FROM audit_log WHERE action = 'acp.payment_declined'`).all<{ detail_json: string }>()).results.map((r) => JSON.parse(r.detail_json).reason);
    assert.deepEqual(reasons.sort(), ["http_400", "status_requires_action"]);
  });

  it("answers 502 when Stripe fails, stores nothing for the key, and the retry charges once", async () => {
    const made = await session();
    const key = { "idempotency-key": "stripe-outage" };
    const failed = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_500") }, key);
    assert.equal(failed.res.status, 502);
    assert.deepEqual([failed.json.type, failed.json.code], ["processing_error", "payment_unavailable"]);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM acp_idempotency WHERE key = 'stripe-outage'`), 0);
    const state = (await (await call(sessionRoute.onRequestGet as never, acpRequest("GET", `/checkout_sessions/${made.id}`), { id: made.id })).json()) as Json;
    assert.equal(state.status, "ready_for_payment");
    const ok = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_retry") }, { "idempotency-key": "stripe-outage-2" });
    assert.equal(ok.json.status, "completed");
  });

  it("asks for a review instead of completing at a price that changed", async () => {
    const made = await session();
    await env.SHOP_DB.prepare(`UPDATE prices SET amount_minor = 3000`).run();
    const { res, json } = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_p") });
    assert.equal(res.status, 200);
    assert.equal(json.status, "ready_for_payment");
    assert.equal(json.messages.find((m: Json) => m.code === "price_change").param, "$.totals");
    assert.equal(json.totals[2].amount, 6000);
    assert.equal(stripe.calls, 0);
    const then = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_p2") });
    assert.equal(then.json.status, "completed");
    assert.equal(stripe.created[0]!.form.get("amount"), "6000");
  });

  it("goes back to not ready when the product disappeared", async () => {
    const made = await session();
    await env.SHOP_DB.prepare(`UPDATE products SET status = 'archived'`).run();
    const { json } = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_gone") });
    assert.equal(json.status, "not_ready_for_payment");
    assert.equal(json.messages[0].code, "out_of_stock");
  });

  it("replaces an order a price change made stale, and cancels it with the session", async () => {
    const made = await session();
    await complete(made.id, { buyer: BUYER, payment_data: pay("spt_declined_2") });
    await env.SHOP_DB.prepare(`UPDATE prices SET amount_minor = 2000`).run();
    await call(sessionRoute.onRequestPost as never, acpRequest("POST", `/checkout_sessions/${made.id}`, { items: [{ id: "EBOOK-1", quantity: 2 }] }), { id: made.id });
    await complete(made.id, { buyer: BUYER, payment_data: pay("spt_declined_3") });
    assert.equal(await count(`SELECT COUNT(*) AS n FROM orders WHERE status = 'cancelled'`), 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM orders WHERE status = 'pending'`), 1);
    const res = await call(cancelRoute.onRequestPost as never, acpRequest("POST", `/checkout_sessions/${made.id}/cancel`, {}), { id: made.id });
    assert.equal(res.status, 200);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM orders WHERE status = 'pending'`), 0);
  });

  it("parks the order for a human when Stripe charged another amount", async () => {
    const made = await session();
    const { json } = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_short_1") });
    assert.equal(json.status, "completed");
    assert.equal(json.order.status, "manual_review");
    assert.equal(json.order.line_items[0].status, "pending");
    assert.equal(await count(`SELECT COUNT(*) AS n FROM invoices`), 0);
  });
});

describe("recovering a completion that died half-way", () => {
  it("finishes a charged order without charging again", async () => {
    const made = await session();
    // A first attempt that charged, then died before fulfilling.
    env.SHOP_DB.state.failOn = /^UPDATE orders SET status = 'paid'/;
    const died = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_r1") });
    assert.equal(died.res.status, 502);
    env.SHOP_DB.state.failOn = null;
    assert.equal(await count(`SELECT COUNT(*) AS n FROM orders WHERE gateway_ref IS NOT NULL`), 1);

    const resumed = await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_r2") });
    assert.equal(resumed.json.status, "completed");
    assert.equal(stripe.created.length, 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM invoices`), 1);
  });

  it("waits for a live completion and takes over a stale one", async () => {
    const made = await session({ buyer: BUYER });
    await env.SHOP_DB.prepare(`UPDATE acp_sessions SET status = 'complete_in_progress' WHERE id = ?`).bind(made.id).run();
    const busy = await complete(made.id, { payment_data: pay("spt_ok_s") });
    assert.equal(busy.res.status, 409);
    assert.equal(busy.res.headers.get("retry-after"), "2");
    await env.SHOP_DB.prepare(`UPDATE acp_sessions SET updated_at = '2000-01-01T00:00:00.000Z' WHERE id = ?`).bind(made.id).run();
    const taken = await complete(made.id, { payment_data: pay("spt_ok_s") });
    assert.equal(taken.json.status, "completed");
  });
});

describe("secrets", () => {
  it("never appear in a response, a header, the audit log or the outbox", async () => {
    const made = await session();
    await complete(made.id, { buyer: BUYER, payment_data: pay("spt_declined_x") });
    await complete(made.id, { buyer: BUYER, payment_data: pay("spt_500") });
    await complete(made.id, { buyer: BUYER, payment_data: pay("spt_ok_x") });
    const tables = ["audit_log", "outbox", "payments", "acp_sessions", "acp_idempotency"];
    const dump: string[] = [...seen];
    for (const t of tables) dump.push(JSON.stringify((await env.SHOP_DB.prepare(`SELECT * FROM ${t}`).all()).results));
    for (const secret of SECRETS) assert.ok(!dump.join("\n").includes(secret), "a secret leaked");
  });
});
