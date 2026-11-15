// UCP checkout sessions (GO-167): create, get, update, cancel and the
// business profile — the protocol's shapes and errors, its authentication,
// idempotency and rate limits at the route, and UCP switched off as a 404.

import { env } from "./_env";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import * as createRoute from "../functions/api/shop/ucp/checkout-sessions/index.ts";
import * as sessionRoute from "../functions/api/shop/ucp/checkout-sessions/[id].ts";
import * as completeRoute from "../functions/api/shop/ucp/checkout-sessions/[id]/complete.ts";
import * as cancelRoute from "../functions/api/shop/ucp/checkout-sessions/[id]/cancel.ts";
import * as profileRoute from "../functions/.well-known/ucp.ts";
import { updateSession, cancelSession } from "../functions/api/shop/_ucp-checkout";
import { putSettings } from "../functions/api/shop/_settings";
import { freshShop, seedProduct } from "./_helpers";
import { call, lossyD1 } from "./acp-helpers";
import { enableUcp, HANDLER, PUBLISHABLE, SECRETS, ucpRequest, UCP_KEY } from "./ucp-helpers";

type Json = Record<string, any>;
const DEST = { methods: [{ type: "shipping", destinations: [{ id: "d1", address_locality: "Berlin", postal_code: "10115", address_country: "DE" }], selected_destination_id: "d1" }] };
const BUYER = { email: "buyer@example.com", first_name: "Ann", last_name: "Buyer" };
const LINES = [{ item: { id: "EBOOK-1" }, quantity: 2 }];
const seen: string[] = [];

async function send(handler: unknown, request: Request, params: Record<string, string> = {}): Promise<{ res: Response; json: Json }> {
  const res = await call(handler as never, request, params);
  const text = await res.text();
  seen.push(text, JSON.stringify([...res.headers]));
  let json: Json = {};
  try {
    json = JSON.parse(text) as Json;
  } catch {
    // A 404 from a route that is switched off is plain text.
  }
  return { res, json };
}
const create = (body: unknown, headers: Record<string, string> = {}, drop: string[] = []) =>
  send(createRoute.onRequestPost, ucpRequest("POST", "/checkout-sessions", body, headers, drop));
const get = (id: string) => send(sessionRoute.onRequestGet, ucpRequest("GET", `/checkout-sessions/${id}`), { id });
const put = (id: string, body: unknown, headers: Record<string, string> = {}) =>
  send(sessionRoute.onRequestPut, ucpRequest("PUT", `/checkout-sessions/${id}`, body, headers), { id });
const cancel = (id: string) => send(cancelRoute.onRequestPost, ucpRequest("POST", `/checkout-sessions/${id}/cancel`, {}), { id });

beforeEach(async () => {
  await freshShop({ "legal.terms_url": "/terms/", "legal.privacy_url": "https://shop.example.com/privacy/" });
  enableUcp();
  await seedProduct({ sku: "EBOOK-1", name: "A Book", priceMinor: 2400 });
});

afterEach(() => {
  env.SHOP_DB.state.failOn = null;
});

describe("creating a session", () => {
  it("answers 201 with the shop's own price, in the protocol's shapes", async () => {
    const { res, json } = await create({ line_items: LINES, fulfillment: DEST, context: { language: "de-DE" } });
    assert.equal(res.status, 201);
    assert.match(json.id, /^chk_[0-9a-f]{32}$/);
    assert.equal(json.status, "incomplete");
    assert.equal(json.currency, "EUR");
    assert.equal(json.ucp.version, "2026-04-08");
    assert.equal(json.ucp.status, "success");
    assert.deepEqual(json.ucp.capabilities, { "dev.ucp.shopping.checkout": [{ version: "2026-04-08" }] });
    const handler = json.ucp.payment_handlers["com.google.pay"][0];
    assert.equal(handler.id, HANDLER);
    assert.equal(handler.version, "2026-01-23");
    assert.equal(handler.config.environment, "TEST");
    assert.deepEqual(handler.config.merchant_info, { merchant_name: "Example Press", merchant_id: "BCR2DN4TEXAMPLE", merchant_origin: "shop.example.com" });
    assert.deepEqual(handler.config.allowed_payment_methods[0].tokenization_specification, {
      type: "PAYMENT_GATEWAY",
      parameters: { gateway: "stripe", "stripe:version": "2018-10-31", "stripe:publishableKey": PUBLISHABLE },
    });
    // Gross pricing: one subtotal that includes the tax, no tax line.
    assert.deepEqual(json.line_items, [
      { id: "li_1", item: { id: "EBOOK-1", title: "A Book", price: 2400 }, quantity: 2, totals: [
        { type: "subtotal", display_text: "Subtotal (including taxes)", amount: 4800 },
        { type: "total", display_text: "Total", amount: 4800 },
      ] },
    ]);
    assert.deepEqual(json.totals.map((t: Json) => [t.type, t.amount]), [["subtotal", 4800], ["total", 4800]]);
    // The waiver is a disclosure, and the missing email a recoverable error.
    assert.equal(json.messages[0].presentation, "disclosure");
    assert.equal(json.messages[0].code, "digital_waiver");
    assert.deepEqual(json.messages[1], { type: "error", code: "missing_buyer_info", content: json.messages[1].content, severity: "recoverable", path: "$.buyer.email" });
    assert.deepEqual(json.links, [
      { type: "terms_of_service", url: "https://shop.example.com/terms/" },
      { type: "privacy_policy", url: "https://shop.example.com/privacy/" },
    ]);
    assert.ok(json.expires_at > new Date().toISOString());
    assert.equal(res.headers.get("request-id")?.startsWith("req-"), true);
    assert.ok(res.headers.get("idempotency-key"));
  });

  it("is ready to complete once a buyer and a country are known", async () => {
    const { json } = await create({ line_items: LINES, buyer: BUYER, context: { address_country: "de" } });
    assert.equal(json.status, "ready_for_complete");
    assert.deepEqual(json.buyer, BUYER);
    const row = await env.SHOP_DB.prepare(`SELECT country, details_json FROM ucp_sessions WHERE id = ?`).bind(json.id).first<Json>();
    assert.equal(row?.country, "DE");
    assert.equal(JSON.parse(row!.details_json).countrySource, "context");
  });

  it("asks for the destination when no country is given", async () => {
    const { json } = await create({ line_items: LINES, buyer: BUYER, fulfillment: { methods: [{ type: "shipping", destinations: [{ address_country: "Germany" }] }, "x"] } });
    assert.equal(json.status, "incomplete");
    assert.equal(json.messages[0].code, "missing_fulfillment_info");
    assert.equal(json.messages[0].path, "$.fulfillment.methods[0].destinations[0]");
  });

  it("says a country the shop does not serve is undeliverable", async () => {
    await putSettings(env, { "shop.countries_allowed": ["DE"] });
    const { json } = await create({ line_items: LINES, buyer: BUYER, context: { address_country: "FR" } });
    assert.equal(json.messages[0].code, "address_undeliverable");
  });

  it("creates nothing when no product can be sold, answering the error envelope", async () => {
    const { res, json } = await create({ line_items: [{ item: { id: "EBOOK-1" } }, { item: { id: "NOPE" }, quantity: 1 }], context: { address_country: "DE" } });
    assert.equal(res.status, 200);
    assert.deepEqual(json.ucp, { version: "2026-04-08", status: "error", capabilities: { "dev.ucp.shopping.checkout": [{ version: "2026-04-08" }] } });
    assert.deepEqual(json.messages, [{ type: "error", code: "item_unavailable", content: "This product is not available.", severity: "unrecoverable", path: "$.line_items[1]" }]);
    assert.equal(json.continue_url, "https://shop.example.com/");
    assert.equal(await count(`SELECT COUNT(*) AS n FROM ucp_sessions`), 0);
  });

  it("takes context.currency when the shop sells in it, and its base currency when not", async () => {
    await seedProduct({ sku: "USD-1", name: "Dollar Book", priceMinor: 1999, currency: "USD" });
    await env.SHOP_DB.prepare(`INSERT INTO prices (product_id, currency, amount_minor) SELECT id, 'USD', 2600 FROM products WHERE sku = 'EBOOK-1'`).run();
    const usd = await create({ line_items: LINES, context: { currency: "usd", address_country: "DE" } });
    assert.equal(usd.json.currency, "USD");
    assert.equal(usd.json.totals.at(-1).amount, 5200);
    const fallback = await create({ line_items: LINES, context: { currency: "GBP", address_country: "DE" } });
    assert.equal(fallback.json.currency, "EUR");
    const unpriced = await create({ line_items: [{ item: { id: "USD-1" } }], context: { address_country: "DE" } });
    assert.equal(unpriced.json.messages[0].content, "This product is not sold in that currency.");
  });

  it("shows subtotal, tax and total for a net-priced shop, and drops the waiver when it is off", async () => {
    await putSettings(env, { "pricing.mode": "net", "legal.digital_waiver": "false" });
    const { json } = await create({ line_items: LINES, buyer: BUYER, context: { address_country: "DE" } });
    assert.deepEqual(json.totals.map((t: Json) => t.type), ["subtotal", "tax", "total"]);
    assert.equal(json.line_items[0].item.price, 2400);
    assert.equal(json.messages.length, 0);
  });

  it("refuses malformed bodies with a 400 and the field's path", async () => {
    const cases: Array<[unknown, string]> = [
      [{}, "$.line_items"],
      [{ line_items: [{ item: {} }] }, "$.line_items[0].item.id"],
      [{ line_items: [{ item: { id: "EBOOK-1" }, quantity: 0 }] }, "$.line_items[0].quantity"],
      [{ line_items: LINES, buyer: "x" }, "$.buyer"],
      [{ line_items: LINES, buyer: { email: "nope" } }, "$.buyer.email"],
    ];
    for (const [body, path] of cases) {
      const { res, json } = await create(body);
      assert.equal(res.status, 400, path);
      assert.equal(json.code, "invalid_request");
      assert.equal(json.path, path);
    }
    const notJson = await create("[1]");
    assert.equal(notJson.res.status, 400);
    const huge = await create({ line_items: LINES, pad: "x".repeat(70_000) });
    assert.equal(huge.res.status, 413);
  });

  it("merges two lines for one product and reads the language from Accept-Language", async () => {
    const { json } = await create({ line_items: [{ item: { id: "EBOOK-1" }, quantity: 9 }, { item: { id: "EBOOK-1" }, quantity: 5 }] }, { "accept-language": "pl-PL,pl;q=0.9" });
    assert.equal(json.line_items[0].quantity, 10);
    const row = await env.SHOP_DB.prepare(`SELECT locale FROM ucp_sessions WHERE id = ?`).bind(json.id).first<Json>();
    assert.equal(row?.locale, "pl-PL");
  });
});

describe("getting, updating and cancelling", () => {
  it("gets a session, and answers an unknown one with not_found", async () => {
    const made = await create({ line_items: LINES });
    const { res, json } = await get(made.json.id);
    assert.equal(res.status, 200);
    assert.equal(json.id, made.json.id);
    const missing = await get("chk_missing");
    assert.equal(missing.res.status, 200);
    assert.equal(missing.json.ucp.status, "error");
    assert.equal(missing.json.messages[0].code, "not_found");
  });

  it("replaces the session on PUT: what is left out is cleared", async () => {
    const made = await create({ line_items: LINES, buyer: BUYER, fulfillment: DEST });
    assert.equal(made.json.status, "ready_for_complete");
    const hydrated = await put(made.json.id, { line_items: [{ id: "li_1", item: { id: "EBOOK-1" }, quantity: 1 }], buyer: BUYER, fulfillment: DEST });
    assert.equal(hydrated.json.status, "ready_for_complete");
    assert.equal(hydrated.json.totals.at(-1).amount, 2400);
    const cleared = await put(made.json.id, { line_items: LINES, fulfillment: DEST });
    assert.equal(cleared.json.status, "incomplete");
    assert.equal(cleared.json.buyer, undefined);
    const bad = await put(made.json.id, { line_items: LINES, buyer: [] });
    assert.equal(bad.res.status, 400);
    const noLines = await put(made.json.id, { buyer: BUYER });
    assert.equal(noLines.res.status, 400);
    assert.equal((await put("chk_missing", { line_items: LINES })).json.messages[0].code, "not_found");
  });

  it("retries an update that lost a race, and gives up after three", async () => {
    const made = await create({ line_items: LINES });
    const lossy = { ...env, SHOP_DB: lossyD1(env.SHOP_DB, /^UPDATE ucp_sessions/) };
    const result = await updateSession({ request: new Request("https://x.example"), env: lossy as never, body: { line_items: LINES }, id: made.json.id, origin: "https://shop.example.com", waitUntil: () => {} });
    assert.equal(result.status, 409);
    assert.equal((result.body as Json).code, "concurrent_update");
    assert.equal(result.headers?.["retry-after"], "1");
  });

  it("cancels once; a canceled session cancels again as it is, a completed one cannot", async () => {
    const made = await create({ line_items: LINES });
    const first = await cancel(made.json.id);
    assert.equal(first.json.status, "canceled");
    const again = await cancel(made.json.id);
    assert.equal(again.res.status, 200);
    assert.equal(again.json.status, "canceled");
    const update = await put(made.json.id, { line_items: LINES });
    assert.equal(update.res.status, 409);
    assert.equal(update.json.code, "invalid_state");
    await env.SHOP_DB.prepare(`UPDATE ucp_sessions SET status = 'completed' WHERE id = ?`).bind(made.json.id).run();
    assert.equal((await cancel(made.json.id)).res.status, 409);
    assert.equal((await cancel("chk_missing")).json.messages[0].code, "not_found");
  });

  it("refuses to cancel a session being paid for, and one that lost the race", async () => {
    const made = await create({ line_items: LINES });
    const lossy = { ...env, SHOP_DB: lossyD1(env.SHOP_DB, /^UPDATE ucp_sessions SET status = \?/) };
    const lost = await cancelSession({ request: new Request("https://x.example"), env: lossy as never, body: {}, id: made.json.id, origin: "https://shop.example.com", waitUntil: () => {} });
    assert.equal(lost.status, 409);
    await env.SHOP_DB.prepare(`UPDATE ucp_sessions SET status = 'complete_in_progress' WHERE id = ?`).bind(made.json.id).run();
    const busy = await cancel(made.json.id);
    assert.equal(busy.res.status, 409);
    assert.equal(busy.res.headers.get("retry-after"), "2");
  });

  it("expires an overdue session to canceled, and its pending order with it", async () => {
    const made = await create({ line_items: LINES });
    await env.SHOP_DB.prepare(`UPDATE ucp_sessions SET expires_at = '2000-01-01T00:00:00.000Z' WHERE id = ?`).bind(made.json.id).run();
    assert.equal((await get(made.json.id)).json.status, "canceled");
  });
});

describe("the route's gates", () => {
  it("is a 404 everywhere — profile included — unless SHOP_UCP_ENABLED is exactly \"true\"", async () => {
    for (const value of [undefined, "1", "TRUE", "yes"]) {
      enableUcp({ SHOP_UCP_ENABLED: value });
      assert.equal((await create({ line_items: LINES })).res.status, 404);
      assert.equal((await get("chk_x")).res.status, 404);
      assert.equal((await put("chk_x", {})).res.status, 404);
      assert.equal((await cancel("chk_x")).res.status, 404);
      assert.equal((await send(completeRoute.onRequestPost, ucpRequest("POST", "/checkout-sessions/chk_x/complete", {}), { id: "chk_x" })).res.status, 404);
      assert.equal((await send(profileRoute.onRequestGet, new Request("https://shop.example.com/.well-known/ucp"))).res.status, 404);
    }
    assert.equal(await count(`SELECT COUNT(*) AS n FROM ucp_sessions`), 0);
  });

  it("is closed (503), never open, when anything a sale needs is missing", async () => {
    const missing: Array<Record<string, string | undefined>> = [
      { SHOP_UCP_API_KEY: undefined },
      { STRIPE_SECRET_KEY: undefined },
      { SHOP_UCP_STRIPE_PUBLISHABLE_KEY: "sk_test_wrong" },
      { SHOP_UCP_SIGNING_KEY: undefined },
      { SHOP_UCP_SIGNING_KEY: "{not json" },
      { SHOP_UCP_SIGNING_KEY: JSON.stringify({ kty: "EC", crv: "P-256", x: "a", y: "b" }) },
      { SHOP_UCP_GPAY_ENVIRONMENT: "PRODUCTION", SHOP_UCP_GPAY_MERCHANT_ID: undefined },
    ];
    for (const overrides of missing) {
      enableUcp(overrides);
      const { res, json } = await create({ line_items: LINES });
      assert.equal(res.status, 503, JSON.stringify(overrides));
      assert.equal(json.code, "not_configured");
      assert.equal(res.headers.get("retry-after"), "300");
      assert.equal((await send(profileRoute.onRequestGet, new Request("https://shop.example.com/.well-known/ucp"))).res.status, 503);
    }
    enableUcp({ SHOP_UCP_GPAY_ENVIRONMENT: "PRODUCTION" });
    assert.equal((await create({ line_items: LINES })).res.status, 201);
  });

  it("refuses a missing or wrong X-API-Key, and a missing or plain-http UCP-Agent", async () => {
    assert.equal((await create({ line_items: LINES }, {}, ["x-api-key"])).res.status, 401);
    const wrong = await create({ line_items: LINES }, { "x-api-key": `${UCP_KEY}x` });
    assert.equal(wrong.res.status, 401);
    assert.equal(wrong.json.code, "unauthorized");
    assert.equal((await create({ line_items: LINES }, { authorization: `Bearer ${UCP_KEY}` }, ["x-api-key"])).res.status, 401);
    for (const agent of [null, 'profile="http://platform.example.com/p"', "profile=https://platform.example.com", 'profile="not a url"']) {
      const { res, json } = await create({ line_items: LINES }, agent ? { "ucp-agent": agent } : {}, agent ? [] : ["ucp-agent"]);
      assert.equal(res.status, 400, String(agent));
      assert.equal(json.code, "invalid_profile_url");
    }
    assert.equal(await count(`SELECT COUNT(*) AS n FROM ucp_sessions`), 0);
  });

  it("is a 503 without a database or before the shop can sell", async () => {
    const saved = env.SHOP_DB;
    (env as Json).SHOP_DB = undefined;
    try {
      assert.equal((await create({ line_items: LINES })).res.status, 503);
    } finally {
      (env as Json).SHOP_DB = saved;
    }
    await putSettings(env, { "seller.name": "" });
    const { res, json } = await create({ line_items: LINES });
    assert.equal(res.status, 503);
    assert.equal(json.code, "shop_not_configured");
  });

  it("limits each credential + address to its budget, with Retry-After", async () => {
    let last: Response | null = null;
    for (let i = 0; i < 121; i++) last = (await get("chk_missing")).res;
    assert.equal(last!.status, 429);
    assert.ok(Number(last!.headers.get("retry-after")) >= 1);
    // Another credential from the same address has its own budget (and is
    // refused for being wrong, not for being busy).
    assert.equal((await create({ line_items: LINES }, { "x-api-key": "someone-else" })).res.status, 401);
  });

  it("answers 500 without detail when an operation fails, and frees its key", async () => {
    env.SHOP_DB.state.failOn = /^INSERT INTO ucp_sessions/;
    const key = crypto.randomUUID();
    const { res, json } = await create({ line_items: LINES }, { "idempotency-key": key });
    assert.equal(res.status, 500);
    assert.deepEqual(json, { code: "internal_error", content: "The request could not be processed." });
    env.SHOP_DB.state.failOn = null;
    assert.equal((await create({ line_items: LINES }, { "idempotency-key": key })).res.status, 201);
  });
});

describe("idempotency", () => {
  it("replays the same body, refuses another with 409, and runs unkeyed calls as they come", async () => {
    const key = crypto.randomUUID();
    const body = { line_items: LINES, buyer: BUYER };
    const first = await create(body, { "idempotency-key": key });
    const replay = await create(body, { "idempotency-key": key });
    assert.equal(replay.res.status, 201);
    assert.equal(replay.res.headers.get("idempotent-replayed"), "true");
    assert.equal(replay.json.id, first.json.id);
    assert.equal(replay.res.headers.get("request-id")?.startsWith("req-"), true);
    // Another body: the raw bytes differ, even if only in key order.
    const conflict = await create({ buyer: BUYER, line_items: LINES }, { "idempotency-key": key });
    assert.equal(conflict.res.status, 409);
    assert.equal(conflict.json.code, "idempotency_conflict");
    const tooLong = await create(body, { "idempotency-key": "k".repeat(256) });
    assert.equal(tooLong.res.status, 400);
    const unkeyed1 = await create(body, {}, ["idempotency-key"]);
    const unkeyed2 = await create(body, {}, ["idempotency-key"]);
    assert.notEqual(unkeyed1.json.id, unkeyed2.json.id);
    assert.equal(await count(`SELECT COUNT(*) AS n FROM ucp_sessions`), 3);
  });

  it("answers 409 while the first request with a key still runs, and fails closed when it cannot record one", async () => {
    const key = crypto.randomUUID();
    const raw = JSON.stringify({ line_items: LINES });
    await env.SHOP_DB.prepare(`INSERT INTO ucp_idempotency (scope, key, body_hash, state, created_at) VALUES (?, ?, ?, 'in_flight', ?)`)
      .bind("POST /api/shop/ucp/checkout-sessions", key, await sha(raw), new Date().toISOString())
      .run();
    const busy = await create(raw, { "idempotency-key": key });
    assert.equal(busy.res.status, 409);
    assert.equal(busy.json.code, "idempotency_in_flight");
    env.SHOP_DB.state.failOn = /INTO ucp_idempotency/;
    const down = await create(raw);
    assert.equal(down.res.status, 503);
    assert.equal(down.json.code, "unavailable");
  });
});

describe("the business profile", () => {
  it("publishes the endpoint, the capabilities, the handler and the public key — never the private one", async () => {
    const { res, json } = await send(profileRoute.onRequestGet, new Request("https://shop.example.com/.well-known/ucp"));
    assert.equal(res.status, 200);
    assert.equal(res.headers.get("cache-control"), "public, max-age=300");
    assert.equal(json.ucp.version, "2026-04-08");
    assert.deepEqual(json.ucp.services["dev.ucp.shopping"][0], {
      version: "2026-04-08",
      spec: "https://ucp.dev/2026-04-08/specification/overview",
      transport: "rest",
      endpoint: "https://shop.example.com/api/shop/ucp",
      schema: "https://ucp.dev/2026-04-08/services/shopping/rest.openapi.json",
    });
    assert.deepEqual(Object.keys(json.ucp.capabilities), ["dev.ucp.shopping.checkout", "dev.ucp.shopping.order"]);
    assert.equal(json.ucp.payment_handlers["com.google.pay"][0].id, HANDLER);
    const [key] = json.signing_keys;
    assert.deepEqual(Object.keys(key).sort(), ["alg", "crv", "kid", "kty", "use", "x", "y"]);
    assert.equal(key.kid, "shop-2026-11");
    const unbound = { ...env, SHOP_DB: undefined };
    const down = await profileRoute.onRequestGet({ request: new Request("https://shop.example.com/.well-known/ucp"), env: unbound } as never);
    assert.equal(down.status, 503);
    // Configured without a merchant id in TEST: the field is simply absent.
    enableUcp({ SHOP_UCP_GPAY_MERCHANT_ID: undefined, SHOP_UCP_HANDLER_ID: undefined });
    await putSettings(env, { "seller.name": "", "shop.name": "" });
    const bare = (await send(profileRoute.onRequestGet, new Request("https://shop.example.com/.well-known/ucp"))).json;
    const handler = bare.ucp.payment_handlers["com.google.pay"][0];
    assert.equal(handler.id, "gpay");
    assert.deepEqual(handler.config.merchant_info, { merchant_name: "Shop", merchant_origin: "shop.example.com" });
  });

  it("never puts a secret in any answer", () => {
    for (const secret of SECRETS) assert.ok(!seen.join("\n").includes(secret), "a secret leaked");
  });
});

async function count(sql: string): Promise<number> {
  return (await env.SHOP_DB.prepare(sql).first<{ n: number }>())?.n ?? 0;
}

async function sha(raw: string): Promise<string> {
  const d = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(raw));
  return [...new Uint8Array(d)].map((b) => b.toString(16).padStart(2, "0")).join("");
}
