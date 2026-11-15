// Completing a UCP session (GO-167): the Google Pay token charged through
// Stripe, the same fulfilment as a storefront order, channel 'ucp', the signed
// order event, and everything that must happen only once.

import { env } from "./_env";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import * as createRoute from "../functions/api/shop/ucp/checkout-sessions/index.ts";
import * as sessionRoute from "../functions/api/shop/ucp/checkout-sessions/[id].ts";
import * as completeRoute from "../functions/api/shop/ucp/checkout-sessions/[id]/complete.ts";
import * as acpCreate from "../functions/api/shop/acp/checkout_sessions/index.ts";
import * as storefront from "../functions/api/shop/checkout";
import * as adminOrder from "../functions/api/shop/admin/orders/[id].ts";
import { drainOutbox } from "../functions/api/shop/_outbox";
import { completeSession } from "../functions/api/shop/_ucp-complete";
import { freshShop, seedProduct, post, ctx } from "./_helpers";
import { acpRequest, call, enableAcp, lossyD1 } from "./acp-helpers";
import { enableUcp, fakeReceiver, fakeStripe, payWith, SECRETS, ucpRequest, WEBHOOK_API_KEY, WEBHOOK_URL, type FakeStripe } from "./ucp-helpers";

type Json = Record<string, any>;
const BUYER = { email: "buyer@example.com", first_name: "Ann", last_name: "Buyer" };
const DEST = { methods: [{ type: "shipping", destinations: [{ address_country: "DE", postal_code: "10115" }] }] };
const LINES = [{ item: { id: "EBOOK-1" }, quantity: 2 }];
const realFetch = globalThis.fetch;
let stripe: FakeStripe;
const seen: string[] = [];

async function session(body: Json = {}): Promise<Json> {
  const res = await call(createRoute.onRequestPost as never, ucpRequest("POST", "/checkout-sessions", { line_items: LINES, buyer: BUYER, fulfillment: DEST, ...body }));
  const json = (await res.json()) as Json;
  seen.push(JSON.stringify(json));
  return json;
}

async function complete(id: string, body: unknown, headers: Record<string, string> = {}): Promise<{ res: Response; json: Json }> {
  const res = await call(completeRoute.onRequestPost as never, ucpRequest("POST", `/checkout-sessions/${id}/complete`, body, headers), { id });
  const text = await res.text();
  seen.push(text, JSON.stringify([...res.headers]));
  return { res, json: JSON.parse(text) as Json };
}

async function count(sql: string, ...args: unknown[]): Promise<number> {
  return (await env.SHOP_DB.prepare(sql).bind(...args).first<{ n: number }>())?.n ?? 0;
}

const op = (id: string, body: Json, db = env.SHOP_DB) => ({
  request: new Request("https://shop.example.com/x"), env: { ...env, SHOP_DB: db } as never, body, id, origin: "https://shop.example.com", waitUntil: () => {},
});

beforeEach(async () => {
  await freshShop();
  enableUcp();
  await seedProduct({ sku: "EBOOK-1", name: "A Book", priceMinor: 2400 });
  stripe = fakeStripe();
  globalThis.fetch = stripe.fetch;
});

afterEach(() => {
  globalThis.fetch = realFetch;
  env.SHOP_DB.state.failOn = null;
});

describe("completing a session", () => {
  it("charges the Google Pay token once and fulfils the order exactly like a storefront order", async () => {
    const made = await session();
    assert.equal(made.status, "ready_for_complete");
    const { res, json } = await complete(made.id, payWith("tok_ok_1"));
    assert.equal(res.status, 200);
    assert.equal(json.status, "completed");

    // The Stripe call: a PaymentIntent confirmed with the card token.
    assert.equal(stripe.created.length, 1);
    const { form, headers } = stripe.created[0]!;
    assert.equal(form.get("amount"), "4800");
    assert.equal(form.get("currency"), "eur");
    assert.equal(form.get("confirm"), "true");
    assert.equal(form.get("payment_method_types[0]"), "card");
    assert.equal(form.get("payment_method_data[type]"), "card");
    assert.equal(form.get("payment_method_data[card][token]"), "tok_ok_1");
    assert.equal(form.get("metadata[ucp_checkout_id]"), made.id);
    assert.equal(form.get("metadata[channel]"), "ucp");
    assert.match(headers.get("idempotency-key") ?? "", new RegExp(`^ucp:${made.id}:[0-9a-f]{32}$`));

    // The confirmation and the echoed instrument — without its credential.
    assert.match(json.order.permalink_url, /^https:\/\/shop\.example\.com\/shop\/thanks\/\?order=.+&k=.+/);
    assert.match(json.order.label, /\d/);
    assert.equal(json.payment.instruments[0].display.last_digits, "4242");
    assert.equal(json.payment.instruments[0].credential, undefined);

    const row = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(json.order.id).first<Json>();
    assert.equal(row?.channel, "ucp");
    assert.equal(row?.status, "paid");
    assert.equal(row?.gateway, "stripe");
    assert.equal(row?.gateway_ref, "pi_1_tok_ok_1");
    assert.equal(row?.consent_waiver, 1);
    assert.equal(row?.locale, "en");
    assert.equal(await count(`SELECT COUNT(*) AS n FROM payments WHERE order_id = ? AND kind = 'charge' AND amount_minor = 4800`, row!.id), 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM invoices WHERE order_id = ? AND kind = 'invoice' AND number LIKE 'FV/%/000001'`, row!.id), 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM download_tokens t JOIN order_items i ON i.id = t.order_item_id WHERE i.order_id = ?`, row!.id), 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM outbox WHERE kind = 'email' AND target = 'buyer@example.com'`), 1);
    const raw = await env.SHOP_DB.prepare(`SELECT raw_json FROM payments WHERE order_id = ?`).bind(row!.id).first<{ raw_json: string }>();
    assert.ok(!raw?.raw_json.includes("pi_secret_never_stored"));

    const admin = await adminOrder.onRequestGet(ctx(new Request(`https://shop.example.com/api/shop/admin/orders/${row!.id}`), { id: row!.id }) as never);
    assert.equal(((await admin.json()) as Json).order.channel, "ucp");

    const again = (await (await call(sessionRoute.onRequestGet as never, ucpRequest("GET", `/checkout-sessions/${made.id}`), { id: made.id })).json()) as Json;
    assert.deepEqual(again.order, json.order);
  });

  it("prices the same basket identically to the storefront and to ACP", async () => {
    const made = await session();
    const { json } = await complete(made.id, payWith("tok_ok_2"));
    const ucp = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(json.order.id).first<Json>();

    globalThis.fetch = (async () => Response.json({ id: "cs_test_1", url: "https://checkout.stripe.com/x" })) as typeof fetch;
    const res = await storefront.onRequestPost(ctx(post("/api/shop/checkout", {
      items: [{ sku: "EBOOK-1", quantity: 2 }], email: "web@example.com", country: "DE", gateway: "stripe", consentWaiver: true,
    })) as never);
    const web = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(((await res.json()) as Json).orderId).first<Json>();
    for (const field of ["currency", "subtotal_minor", "tax_minor", "total_minor", "tax_country", "reverse_charge"]) {
      assert.equal(ucp?.[field], web?.[field], field);
    }
    const items = async (id: string) => (await env.SHOP_DB.prepare(`SELECT sku, quantity, unit_minor, tax_rate_bp, tax_minor, total_minor FROM order_items WHERE order_id = ?`).bind(id).all()).results;
    assert.deepEqual(await items(ucp!.id), await items(web!.id));

    enableAcp();
    const acp = (await (await call(acpCreate.onRequestPost as never, acpRequest("POST", "/checkout_sessions", {
      items: [{ id: "EBOOK-1", quantity: 2 }], fulfillment_details: { address: { country: "DE" } },
    }))).json()) as Json;
    assert.equal(acp.totals.at(-1).amount, made.totals.at(-1).amount);
    assert.equal(acp.totals.at(-1).amount, ucp?.total_minor);
  });

  it("queues a signed order event and delivers it through the outbox, verifiable with the profile's key", async () => {
    const made = await session();
    const { json } = await complete(made.id, payWith("tok_ok_3"));
    const row = await env.SHOP_DB.prepare(`SELECT * FROM outbox WHERE kind = 'ucp'`).first<Json>();
    assert.equal(row?.target, WEBHOOK_URL);
    const payload = JSON.parse(row!.payload_json);
    assert.equal(payload.eventId, json.order.id);
    assert.equal(payload.profile, "https://shop.example.com/.well-known/ucp");
    for (const secret of SECRETS) assert.ok(!row!.payload_json.includes(secret));
    const order = payload.order;
    assert.deepEqual(order.ucp, { version: "2026-04-08", capabilities: { "dev.ucp.shopping.order": [{ version: "2026-04-08" }] } });
    assert.equal(order.checkout_id, made.id);
    assert.equal(order.currency, "EUR");
    assert.deepEqual(order.line_items[0].quantity, { total: 2, fulfilled: 2 });
    assert.equal(order.line_items[0].status, "fulfilled");
    assert.equal(order.line_items[0].id, "li_1");
    assert.equal(order.fulfillment.expectations[0].method_type, "digital");
    assert.deepEqual(order.fulfillment.expectations[0].destination, { address_country: "DE" });
    assert.equal(order.adjustments, undefined);

    const receiver = fakeReceiver();
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input) === WEBHOOK_URL) return receiver.handle(new Request(String(input), init));
      throw new Error(`unexpected fetch ${String(input)}`);
    }) as typeof fetch;
    // The completion already tried once in the background (no network here).
    await env.SHOP_DB.prepare(`UPDATE outbox SET next_attempt = '2000-01-01T00:00:00.000Z'`).run();
    await drainOutbox(env, 10, async (r, e) => {
      if (r.kind !== "ucp") return;
      const { deliverUcpEvent } = await import("../functions/api/shop/_ucp-order");
      await deliverUcpEvent(e, JSON.parse(r.payload_json));
    });
    assert.equal(receiver.received.length, 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM outbox WHERE kind = 'ucp' AND done_at IS NOT NULL`), 1);
    const [got] = receiver.received;
    assert.equal(got!.verified, true);
    assert.deepEqual(got!.body, order);
    assert.equal(got!.headers.get("webhook-id"), json.order.id);
    assert.equal(got!.headers.get("idempotency-key"), json.order.id);
    assert.equal(got!.headers.get("webhook-timestamp"), payload.occurredAt);
    assert.equal(got!.headers.get("x-goog-api-key"), WEBHOOK_API_KEY);
    assert.equal(got!.headers.get("ucp-agent"), 'profile="https://shop.example.com/.well-known/ucp"');
  });

  it("does not charge twice: a replayed key, and a new key with the same token", async () => {
    const made = await session();
    const key = crypto.randomUUID();
    const first = await complete(made.id, payWith("tok_ok_4"), { "idempotency-key": key });
    const replay = await complete(made.id, payWith("tok_ok_4"), { "idempotency-key": key });
    assert.equal(replay.res.headers.get("idempotent-replayed"), "true");
    assert.deepEqual(replay.json, first.json);
    const fresh = await complete(made.id, payWith("tok_ok_4"));
    assert.equal(fresh.json.status, "completed");
    assert.equal(stripe.calls, 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM payments WHERE kind = 'charge'`), 1);
  });

  it("lets exactly one of two concurrent completes take the money", async () => {
    const made = await session();
    const [a, b] = await Promise.all([complete(made.id, payWith("tok_ok_5")), complete(made.id, payWith("tok_ok_6"))]);
    const statuses = [a.res.status, b.res.status].sort();
    assert.ok(statuses.includes(200));
    assert.equal(stripe.created.length, 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM orders WHERE channel = 'ucp' AND status = 'paid'`), 1);
    const loser = a.res.status === 409 ? a : b.res.status === 409 ? b : null;
    if (loser) assert.equal(loser.json.code, "invalid_state");
  });

  it("answers a decline with 200, payment_declined, and the session ready for another card", async () => {
    const made = await session();
    const declined = await complete(made.id, payWith("tok_declined_1"));
    assert.equal(declined.res.status, 200);
    assert.equal(declined.json.status, "ready_for_complete");
    const msg = declined.json.messages.find((m: Json) => m.code === "payment_declined");
    assert.deepEqual(msg, { type: "error", code: "payment_declined", content: msg.content, severity: "recoverable", path: "$.payment.instruments[0]" });
    const orderId = (await env.SHOP_DB.prepare(`SELECT order_id FROM ucp_sessions WHERE id = ?`).bind(made.id).first<Json>())?.order_id;
    const ok = await complete(made.id, payWith("tok_ok_7"));
    assert.equal(ok.json.status, "completed");
    assert.equal(ok.json.order.id, orderId, "the pending order is reused for the next card");
    assert.equal(await count(`SELECT COUNT(*) AS n FROM audit_log WHERE action = 'ucp.payment_declined'`), 1);
  });

  it("treats a card that needs 3-D Secure as declined, and Stripe failing as payment_failed — both 200", async () => {
    const made = await session();
    const tds = await complete(made.id, payWith("tok_3ds_1"));
    assert.ok(tds.json.messages.some((m: Json) => m.code === "payment_declined"));
    const down = await complete(made.id, payWith("tok_500_1"));
    assert.equal(down.res.status, 200);
    assert.equal(down.json.status, "ready_for_complete");
    assert.ok(down.json.messages.some((m: Json) => m.code === "payment_failed"));
    assert.equal(await count(`SELECT COUNT(*) AS n FROM audit_log WHERE action = 'ucp.complete_error'`), 1);
  });

  it("parks an order Stripe charged a different amount for, and does not deliver it", async () => {
    const made = await session();
    const { json } = await complete(made.id, payWith("tok_short_1"));
    assert.equal(json.status, "completed");
    const row = await env.SHOP_DB.prepare(`SELECT status FROM orders WHERE channel = 'ucp'`).first<Json>();
    assert.equal(row?.status, "needs_review");
    assert.equal(await count(`SELECT COUNT(*) AS n FROM download_tokens`), 0);
  });

  it("refuses a complete without payment (400), and an instrument it cannot charge (200, payment_ineligible)", async () => {
    const made = await session();
    const none = await complete(made.id, {});
    assert.equal(none.res.status, 400);
    assert.equal(none.json.path, "$.payment.instruments");
    const ineligible: Json[] = [
      payWith("tok_ok_x", { handler_id: "someone-else" }),
      payWith("tok_ok_x", { credential: { type: "PAYMENT_GATEWAY", token: '{"signature":"…","protocolVersion":"ECv2"}' } }),
      payWith("tok_ok_x", { credential: { type: "DIRECT", token: "x" } }),
      payWith("tok_ok_x", { credential: { type: "PAYMENT_GATEWAY", token: "not json" } }),
      { payment: { instruments: [{ handler_id: "gpay_test_handler" }, { handler_id: "gpay_test_handler" }] } },
    ];
    for (const body of ineligible) {
      const { res, json } = await complete(made.id, body);
      assert.equal(res.status, 200);
      assert.ok(json.messages.some((m: Json) => m.code === "payment_ineligible"));
    }
    assert.equal(stripe.calls, 0);
    assert.equal((await complete("chk_missing", payWith("tok_ok_x"))).json.messages[0].code, "not_found");
  });

  it("charges nothing while the session is incomplete, and fills the country from the billing address", async () => {
    const noBuyer = await session({ buyer: undefined });
    const shown = await complete(noBuyer.id, payWith("tok_ok_8"));
    assert.equal(shown.json.status, "incomplete");
    assert.ok(shown.json.messages.some((m: Json) => m.code === "missing_buyer_info"));
    assert.equal(stripe.calls, 0);

    const noCountry = await session({ fulfillment: undefined });
    assert.equal(noCountry.status, "incomplete");
    const filled = await complete(noCountry.id, payWith("tok_ok_9"));
    assert.equal(filled.json.status, "completed");
    const order = await env.SHOP_DB.prepare(`SELECT tax_country, total_minor FROM orders WHERE id = ?`).bind(filled.json.order.id).first<Json>();
    assert.equal(order?.tax_country, "DE");
    const details = JSON.parse((await env.SHOP_DB.prepare(`SELECT details_json FROM ucp_sessions WHERE id = ?`).bind(noCountry.id).first<Json>())!.details_json);
    assert.equal(details.countrySource, "billing");
  });

  it("puts the session back with totals_changed when the price moved, and charges nothing", async () => {
    const made = await session();
    await env.SHOP_DB.prepare(`UPDATE prices SET amount_minor = 2500`).run();
    const { json } = await complete(made.id, payWith("tok_ok_10"));
    assert.equal(json.status, "ready_for_complete");
    assert.ok(json.messages.some((m: Json) => m.code === "totals_changed"));
    assert.equal(stripe.calls, 0);
    await env.SHOP_DB.prepare(`UPDATE products SET status = 'draft'`).run();
    const gone = await complete(made.id, payWith("tok_ok_11"));
    assert.equal(gone.json.status, "incomplete");
    assert.ok(gone.json.messages.some((m: Json) => m.code === "item_unavailable"));
  });

  it("answers a completed session again without charging, and refuses a canceled one", async () => {
    const made = await session();
    const done = await complete(made.id, payWith("tok_ok_12"));
    const again = await complete(made.id, payWith("tok_ok_13"));
    assert.deepEqual(again.json.order, done.json.order);
    assert.equal(stripe.calls, 1);
    const other = await session();
    await env.SHOP_DB.prepare(`UPDATE ucp_sessions SET status = 'canceled' WHERE id = ?`).bind(other.id).run();
    assert.equal((await complete(other.id, payWith("tok_ok_14"))).res.status, 409);
  });

  it("finishes a completion that died after charging, without a second charge", async () => {
    const made = await session();
    env.SHOP_DB.state.failOn = /^UPDATE orders SET status = 'paid'/;
    const died = await complete(made.id, payWith("tok_ok_15"));
    assert.ok(died.json.messages.some((m: Json) => m.code === "payment_failed"));
    env.SHOP_DB.state.failOn = null;
    // The charge is recorded; the next completion only delivers.
    const resumed = await complete(made.id, payWith("tok_ok_16"));
    assert.equal(resumed.json.status, "completed");
    assert.equal(stripe.created.length, 1);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM orders WHERE status = 'paid'`), 1);
  });

  it("resumes a completion left in progress past its stale time, and not before", async () => {
    const made = await session();
    await env.SHOP_DB.prepare(`UPDATE ucp_sessions SET status = 'complete_in_progress' WHERE id = ?`).bind(made.id).run();
    const busy = await complete(made.id, payWith("tok_ok_17"));
    assert.equal(busy.res.status, 409);
    assert.equal(busy.res.headers.get("retry-after"), "2");
    await env.SHOP_DB.prepare(`UPDATE ucp_sessions SET updated_at = '2000-01-01T00:00:00.000Z' WHERE id = ?`).bind(made.id).run();
    const resumed = await complete(made.id, payWith("tok_ok_17"));
    assert.equal(resumed.json.status, "completed");
  });

  it("loses the claim to a concurrent writer and says so", async () => {
    const made = await session();
    const result = await completeSession(op(made.id, payWith("tok_ok_18"), lossyD1(env.SHOP_DB, /^UPDATE ucp_sessions SET status = \?, country/) as never));
    assert.equal(result.status, 409);
    assert.equal(stripe.calls, 0);
  });

  it("sends no event without a webhook URL, and still completes", async () => {
    enableUcp({ SHOP_UCP_WEBHOOK_URL: undefined });
    const made = await session();
    assert.equal((await complete(made.id, payWith("tok_ok_19"))).json.status, "completed");
    assert.equal(await count(`SELECT COUNT(*) AS n FROM outbox WHERE kind = 'ucp'`), 0);
  });

  it("never puts a secret in any answer", () => {
    for (const secret of SECRETS) assert.ok(!seen.join("\n").includes(secret), "a secret leaked");
  });
});
