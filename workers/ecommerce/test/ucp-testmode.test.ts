// The conformance-suite test mode (GO-187): off by default, refused next to a
// live configuration, and what it switches on — http profiles and webhook
// URLs, the suite's test payment handler and /testing/simulate-shipping — plus
// Get Order, which the order capability defines.

import { env } from "./_env";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import * as createRoute from "../functions/api/shop/ucp/checkout-sessions/index.ts";
import * as completeRoute from "../functions/api/shop/ucp/checkout-sessions/[id]/complete.ts";
import * as orderRoute from "../functions/api/shop/ucp/orders/[id].ts";
import * as simulateRoute from "../functions/testing/simulate-shipping/[id].ts";
import * as profileRoute from "../functions/.well-known/ucp.ts";
import { agentProfile, ucpMissing } from "../functions/api/shop/_ucp-auth";
import { deliverUcpEvent } from "../functions/api/shop/_ucp-order";
import { readInstrument } from "../functions/api/shop/_ucp-payment";
import { chargeTest, liveConfig, testPayment, TEST_HANDLER_ID } from "../functions/api/shop/_ucp-testing";
import type { OrderRow } from "../functions/api/shop/_types";
import { freshShop, seedProduct } from "./_helpers";
import { call } from "./acp-helpers";
import { enableUcp, fakeReceiver, payWith, ucpRequest } from "./ucp-helpers";

type Json = Record<string, any>;
const SIM = "sim-secret-2b7c";
const LOCAL_AGENT = 'profile="http://localhost:8285/profiles/agent.json"';
const LOCAL_HOOK = "http://localhost:8284/webhooks/partners/test_partner/events/order";
const BUYER = { email: "buyer@example.com" };
const LINES = [{ item: { id: "EBOOK-1" }, quantity: 1 }];
const realFetch = globalThis.fetch;

const testPay = (token: string, credential: Json = { type: "token", token }) => ({
  payment: { instruments: [{ id: "instr_1", handler_id: TEST_HANDLER_ID, type: "card", credential, billing_address: { address_country: "US" } }] },
});

async function json(res: Response): Promise<Json> {
  const text = await res.text();
  try {
    return JSON.parse(text) as Json;
  } catch {
    return { text };
  }
}

async function create(headers: Record<string, string> = {}): Promise<{ res: Response; body: Json }> {
  const res = await call(createRoute.onRequestPost as never, ucpRequest("POST", "/checkout-sessions", { line_items: LINES, buyer: BUYER }, headers));
  return { res, body: await json(res) };
}

async function complete(id: string, body: Json): Promise<Json> {
  return json(await call(completeRoute.onRequestPost as never, ucpRequest("POST", `/checkout-sessions/${id}/complete`, body), { id }));
}

/** A completed UCP order, paid with the test handler. */
async function testOrder(): Promise<{ checkoutId: string; orderId: string }> {
  const made = (await create({ "ucp-agent": LOCAL_AGENT })).body;
  const done = await complete(made.id, testPay("success_token"));
  assert.equal(done.status, "completed");
  return { checkoutId: made.id, orderId: done.order.id };
}

const getOrder = async (id: string) => json(await call(orderRoute.onRequestGet as never, ucpRequest("GET", `/orders/${id}`), { id }));
const simulate = (id: string, headers: Record<string, string> = {}) =>
  call(simulateRoute.onRequestPost as never, new Request(`https://shop.example.com/testing/simulate-shipping/${id}`, { method: "POST", headers }), { id });

beforeEach(async () => {
  await freshShop();
  enableUcp();
  await seedProduct({ sku: "EBOOK-1", name: "A Book", priceMinor: 2400 });
  globalThis.fetch = (async () => {
    throw new Error("no network in tests");
  }) as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("the switch", () => {
  it("is off by default: http profiles, the test handler and the hook are refused", async () => {
    assert.equal(agentProfile(LOCAL_AGENT, env), null);
    assert.equal((await create({ "ucp-agent": LOCAL_AGENT })).res.status, 400);
    assert.equal(testPayment(env, testPay("success_token").payment.instruments[0]!), null);
    enableUcp({ SHOP_UCP_SIMULATION_SECRET: SIM });
    assert.equal((await simulate("x", { "simulation-secret": SIM })).status, 404);
  });

  it("takes only the exact value \"true\"", async () => {
    enableUcp({ SHOP_UCP_TEST_MODE: "1" });
    assert.equal(agentProfile(LOCAL_AGENT, env), null);
    enableUcp({ SHOP_UCP_TEST_MODE: "true" });
    assert.equal(agentProfile(LOCAL_AGENT, env), "http://localhost:8285/profiles/agent.json");
    assert.equal(agentProfile('profile="ftp://example.com/x"', env), null);
  });

  it("is refused — every route and the profile 503 — next to anything that takes real money", async () => {
    for (const live of [
      { STRIPE_SECRET_KEY: "sk_live_x" },
      { STRIPE_SECRET_KEY: "rk_live_x" },
      { SHOP_UCP_STRIPE_PUBLISHABLE_KEY: "pk_live_x" },
      { SHOP_UCP_GPAY_ENVIRONMENT: "PRODUCTION" },
    ]) {
      enableUcp({ ...live, SHOP_UCP_TEST_MODE: "true", SHOP_UCP_SIMULATION_SECRET: SIM });
      assert.ok(liveConfig(env));
      assert.ok(ucpMissing(env).includes("SHOP_UCP_TEST_MODE"), JSON.stringify(live));
      assert.equal((await create()).res.status, 503);
      const profile = await call(profileRoute.onRequestGet as never, new Request("https://shop.example.com/.well-known/ucp"));
      assert.equal(profile.status, 503);
      const hook = await simulate("x", { "simulation-secret": SIM });
      assert.equal(hook.status, 503);
      assert.equal((await json(hook)).code, "not_configured");
    }
    // The same live configuration without the test mode sells as before.
    enableUcp({ STRIPE_SECRET_KEY: "sk_live_x" });
    assert.deepEqual(ucpMissing(env), []);
  });
});

describe("the test payment handler", () => {
  beforeEach(() => enableUcp({ SHOP_UCP_TEST_MODE: "true" }));

  it("reads only the suite's handler, token credential and two tokens", () => {
    const one = (i: Json) => testPayment(env, i);
    assert.equal(one({ handler_id: TEST_HANDLER_ID, credential: { type: "token", token: "success_token" } }), "success");
    assert.equal(one({ handler_id: TEST_HANDLER_ID, credential: { type: "token", token: "fail_token" } }), "fail");
    assert.equal(one({ handler_id: TEST_HANDLER_ID, credential: { type: "token", token: "fraud_token" } }), null);
    assert.equal(one({ handler_id: TEST_HANDLER_ID, credential: { type: "token", token: "constructor" } }), null);
    assert.equal(one({ handler_id: TEST_HANDLER_ID, credential: { type: "card", number: "4242424242424242" } }), null);
    assert.equal(one({ handler_id: TEST_HANDLER_ID, credential: { type: "token", token: 1 } }), null);
    assert.equal(one({ handler_id: TEST_HANDLER_ID, credential: "success_token" }), null);
    assert.equal(one({ handler_id: "gpay", credential: { type: "token", token: "success_token" } }), null);
    const read = readInstrument(env, testPay("success_token").payment);
    assert.ok(read.ok && read.test === "success" && read.stripeToken === "" && read.billingCountry === "US");
    assert.ok(read.ok && !("credential" in read.instrument), "the credential is never echoed");
  });

  it("completes and fulfils with success_token, charging nothing", async () => {
    let fetched = 0;
    globalThis.fetch = (async (input: RequestInfo | URL) => {
      if (String(input).includes("stripe.com")) fetched++;
      throw new Error("no network");
    }) as typeof fetch;
    const { orderId } = await testOrder();
    const payment = await env.SHOP_DB.prepare(`SELECT gateway_ref, amount_minor FROM payments WHERE order_id = ?`).bind(orderId).first<Json>();
    assert.match(payment!.gateway_ref, /^test_[0-9a-f]{32}$/);
    assert.equal(payment!.amount_minor, 2400);
    assert.equal((await env.SHOP_DB.prepare(`SELECT status FROM orders WHERE id = ?`).bind(orderId).first<Json>())!.status, "paid");
    assert.equal(fetched, 0, "no payment provider was called");
  });

  it("answers fail_token with payment_failed and leaves the session payable", async () => {
    const made = (await create()).body;
    const done = await complete(made.id, testPay("fail_token"));
    assert.equal(done.status, "ready_for_complete");
    assert.ok(done.messages.some((m: Json) => m.code === "payment_failed"));
    assert.equal(done.order, undefined);
  });

  it("charges as asked: paid in full, or a throw for the failing token", () => {
    const order = { total_minor: 999, currency: "eur" } as OrderRow;
    assert.deepEqual({ ...chargeTest(order, "success"), ref: "x" }, { kind: "paid", ref: "x", amountMinor: 999, currency: "EUR", raw: { test: true } });
    assert.throws(() => chargeTest(order, "fail"), /fail_token/);
  });

  it("is not accepted outside test mode, where it is an ineligible instrument", async () => {
    enableUcp();
    const made = (await create()).body;
    const done = await complete(made.id, testPay("success_token"));
    assert.ok(done.messages.some((m: Json) => m.code === "payment_ineligible"));
  });

  it("leaves the Google Pay handler working as before", async () => {
    await freshShop({ "legal.terms_url": "" });
    await seedProduct({ sku: "EBOOK-1", name: "A Book", priceMinor: 2400 });
    const made = (await create()).body;
    assert.deepEqual(made.payment, { instruments: [] });
    assert.deepEqual(made.links.map((l: Json) => l.type), ["privacy_policy"], "an empty setting is a link left out");
    const read = readInstrument(env, payWith("tok_ok_9").payment);
    assert.ok(read.ok && read.test === null && read.stripeToken === "tok_ok_9");
  });
});

describe("order events over http", () => {
  it("are delivered to an http receiver only in test mode", async () => {
    const receiver = fakeReceiver();
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => receiver.handle(new Request(String(input), init))) as typeof fetch;
    const payload = { eventId: "ord_1", occurredAt: "2026-11-15T00:00:00.000Z", profile: "https://shop.example.com/.well-known/ucp", order: { id: "ord_1" } };
    enableUcp({ SHOP_UCP_WEBHOOK_URL: LOCAL_HOOK });
    await assert.rejects(deliverUcpEvent(env, payload), /must be https/);
    enableUcp({ SHOP_UCP_WEBHOOK_URL: LOCAL_HOOK, SHOP_UCP_TEST_MODE: "true" });
    await deliverUcpEvent(env, payload);
    assert.equal(receiver.received.length, 1);
    assert.ok(receiver.received[0]!.verified);
  });
});

describe("Get Order", () => {
  beforeEach(() => enableUcp({ SHOP_UCP_TEST_MODE: "true" }));

  it("answers the full order of a UCP checkout", async () => {
    const { checkoutId, orderId } = await testOrder();
    const order = await getOrder(orderId);
    assert.equal(order.id, orderId);
    assert.equal(order.checkout_id, checkoutId);
    assert.equal(order.fulfillment.expectations[0].method_type, "digital");
    assert.equal(order.ucp.version, "2026-04-08");
  });

  it("is not_found for an unknown id, a storefront order and an unfinished session's order", async () => {
    const notFound = (body: Json) => {
      assert.equal(body.ucp.status, "error");
      assert.deepEqual(body.messages, [{ type: "error", code: "not_found", content: "Order not found.", severity: "unrecoverable" }]);
    };
    notFound(await getOrder("nope"));
    const { orderId } = await testOrder();
    await env.SHOP_DB.prepare(`UPDATE ucp_sessions SET status = 'complete_in_progress' WHERE order_id = ?`).bind(orderId).run();
    notFound(await getOrder(orderId));
  });

  it("needs the API key, as every UCP route", async () => {
    const res = await call(orderRoute.onRequestGet as never, ucpRequest("GET", "/orders/x", undefined, { "x-api-key": "wrong" }), { id: "x" });
    assert.equal(res.status, 401);
  });
});

describe("/testing/simulate-shipping", () => {
  beforeEach(() => enableUcp({ SHOP_UCP_TEST_MODE: "true", SHOP_UCP_SIMULATION_SECRET: SIM, SHOP_UCP_WEBHOOK_URL: LOCAL_HOOK }));

  it("does not exist without a secret, with UCP off or without a database", async () => {
    enableUcp({ SHOP_UCP_TEST_MODE: "true" });
    assert.equal((await simulate("x", { "simulation-secret": SIM })).status, 404);
    enableUcp({ SHOP_UCP_TEST_MODE: "true", SHOP_UCP_SIMULATION_SECRET: SIM, SHOP_UCP_ENABLED: "false" });
    assert.equal((await simulate("x", { "simulation-secret": SIM })).status, 404);
    enableUcp({ SHOP_UCP_TEST_MODE: "true", SHOP_UCP_SIMULATION_SECRET: SIM });
    const res = await simulateRoute.onRequestPost({
      request: new Request("https://shop.example.com/testing/simulate-shipping/x", { method: "POST", headers: { "simulation-secret": SIM } }),
      env: { ...env, SHOP_DB: undefined },
      params: { id: "x" },
      waitUntil: () => {},
    } as never);
    assert.equal(res.status, 404);
  });

  it("is 403 without the secret or with a wrong one, and 404 for an order it does not know", async () => {
    assert.equal((await simulate("x")).status, 403);
    assert.equal((await simulate("x", { "simulation-secret": "wrong" })).status, 403);
    const res = await simulate("x", { "simulation-secret": SIM });
    assert.equal(res.status, 404);
    assert.equal((await json(res)).code, "not_found");
  });

  it("sends the order again with a simulated shipped event, storing nothing", async () => {
    const { checkoutId, orderId } = await testOrder();
    const res = await simulate(orderId, { "simulation-secret": SIM });
    assert.equal(res.status, 200);
    assert.deepEqual(await json(res), { status: "shipped" });
    const { results } = await env.SHOP_DB.prepare(`SELECT payload_json FROM outbox WHERE kind = 'ucp' ORDER BY rowid`).all<Json>();
    assert.equal(results.length, 2);
    const created = JSON.parse(results[0]!.payload_json);
    const shipped = JSON.parse(results[1]!.payload_json);
    assert.notEqual(shipped.eventId, created.eventId);
    assert.equal(created.order.fulfillment.events, undefined);
    assert.equal(shipped.order.checkout_id, checkoutId);
    const [event] = shipped.order.fulfillment.events;
    assert.equal(event.type, "shipped");
    assert.deepEqual(event.line_items, [{ id: "li_1", quantity: 1 }]);
    assert.equal(shipped.order.fulfillment.expectations[0].method_type, "digital");
    assert.equal((await getOrder(orderId)).fulfillment.events, undefined, "nothing stored");
  });

  it("lists no line of a refunded order, whose lines are removed", async () => {
    const { orderId } = await testOrder();
    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'refunded' WHERE id = ?`).bind(orderId).run();
    assert.equal((await simulate(orderId, { "simulation-secret": SIM })).status, 200);
    const row = await env.SHOP_DB.prepare(`SELECT payload_json FROM outbox WHERE kind = 'ucp' ORDER BY rowid DESC`).first<Json>();
    assert.deepEqual(JSON.parse(row!.payload_json).order.fulfillment.events[0].line_items, []);
  });
});
