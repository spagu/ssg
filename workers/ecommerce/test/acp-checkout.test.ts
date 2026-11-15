// ACP checkout sessions (GO-163): create, update, retrieve, cancel — the
// protocol's shapes and errors, its authentication, idempotency at the route,
// and that ACP switched off is a 404 everywhere.

import { env } from "./_env";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import * as createRoute from "../functions/api/shop/acp/checkout_sessions/index.ts";
import * as sessionRoute from "../functions/api/shop/acp/checkout_sessions/[id].ts";
import * as completeRoute from "../functions/api/shop/acp/checkout_sessions/[id]/complete.ts";
import * as cancelRoute from "../functions/api/shop/acp/checkout_sessions/[id]/cancel.ts";
import { cancelSession, updateSession } from "../functions/api/shop/_acp-checkout";
import { hmacBase64url } from "../functions/api/shop/_acp-auth";
import { canonicalJson } from "../functions/api/shop/_acp-http";
import { putSettings } from "../functions/api/shop/_settings";
import { freshShop, seedProduct } from "./_helpers";
import { acpRequest, call, enableAcp, lossyD1 } from "./acp-helpers";

type Json = Record<string, any>;
const DE = { address: { name: "A Buyer", line_one: "1 Example Way", city: "Berlin", state: "BE", country: "DE", postal_code: "10115" } };

async function create(body: unknown, headers: Record<string, string> = {}): Promise<{ res: Response; json: Json }> {
  const res = await call(createRoute.onRequestPost as never, acpRequest("POST", "/checkout_sessions", body, headers));
  return { res, json: (await res.json()) as Json };
}
async function update(id: string, body: unknown): Promise<{ res: Response; json: Json }> {
  const res = await call(sessionRoute.onRequestPost as never, acpRequest("POST", `/checkout_sessions/${id}`, body), { id });
  return { res, json: (await res.json()) as Json };
}
async function retrieve(id: string): Promise<{ res: Response; json: Json }> {
  const res = await call(sessionRoute.onRequestGet as never, acpRequest("GET", `/checkout_sessions/${id}`), { id });
  return { res, json: (await res.json()) as Json };
}
async function cancel(id: string): Promise<{ res: Response; json: Json }> {
  const res = await call(cancelRoute.onRequestPost as never, acpRequest("POST", `/checkout_sessions/${id}/cancel`, {}), { id });
  return { res, json: (await res.json()) as Json };
}

beforeEach(async () => {
  await freshShop();
  enableAcp();
  await seedProduct({ sku: "EBOOK-1", name: "A Book", priceMinor: 2400 });
});

afterEach(() => {
  env.SHOP_DB.state.failOn = null;
});

describe("creating a session", () => {
  it("answers 201 with the authoritative cart, priced by the shop", async () => {
    const { res, json } = await create({ items: [{ id: "EBOOK-1", quantity: 1 }], fulfillment_details: DE, currency: "eur", capabilities: {} });
    assert.equal(res.status, 201);
    assert.match(json.id, /^cs_[0-9a-f]{32}$/);
    assert.equal(json.status, "ready_for_payment");
    assert.equal(json.currency, "eur");
    assert.deepEqual(json.protocol, { version: "2026-04-17" });
    const handler = json.capabilities.payment.handlers[0];
    assert.equal(handler.name, "dev.acp.tokenized.card");
    assert.equal(handler.psp, "stripe");
    assert.equal(handler.config.merchant_id, "acct_test_example");
    // Gross 24.00 at the German ebook rate (7 %): 22.43 net + 1.57 tax.
    assert.deepEqual(json.totals, [
      { type: "subtotal", display_text: "Subtotal", amount: 2243 },
      { type: "tax", display_text: "Tax", amount: 157 },
      { type: "total", display_text: "Total", amount: 2400 },
    ]);
    const line = json.line_items[0];
    assert.equal(line.id, "EBOOK-1");
    assert.equal(line.unit_amount, 2400);
    assert.equal(line.availability_status, "in_stock");
    assert.equal(line.disclosures[0].type, "disclaimer");
    assert.equal(json.fulfillment_options[0].type, "digital");
    assert.deepEqual(json.selected_fulfillment_options, [{ type: "digital", option_id: "digital", item_ids: ["EBOOK-1"] }]);
    assert.deepEqual(json.links.map((l: Json) => l.type), ["terms_of_use", "privacy_policy"]);
    assert.equal(json.links[0].url, "https://shop.example.com/shop/terms/");
    assert.equal(json.fulfillment_details.address.country, "DE");
    assert.equal(json.messages.at(-1).resolution, "requires_buyer_review");
    assert.ok(res.headers.get("idempotency-key"));
    assert.match(res.headers.get("request-id") ?? "", /^req_/);
    assert.equal(res.headers.get("cache-control"), "no-store");
  });

  it("reads `line_items`, merges duplicates and keeps buyer, locale and image", async () => {
    await env.SHOP_DB.prepare(`UPDATE products SET image_url = '/images/book.png' WHERE sku = 'EBOOK-1'`).run();
    const { json } = await create({
      line_items: [{ id: "EBOOK-1", quantity: 2 }, { id: "EBOOK-1" }],
      buyer: { email: "buyer@example.com", first_name: "Ann", last_name: "Buyer", ignored: "x" },
      fulfillment_details: { name: "Ann", email: "buyer@example.com", ...DE },
      locale: "de-DE",
    });
    assert.equal(json.line_items.length, 1);
    assert.equal(json.line_items[0].quantity, 3);
    assert.deepEqual(json.line_items[0].images, ["https://shop.example.com/images/book.png"]);
    assert.deepEqual(json.buyer, { email: "buyer@example.com", first_name: "Ann", last_name: "Buyer" });
    assert.equal(json.locale, "de-DE");
    assert.equal(json.totals[2].amount, 7200);
  });

  it("takes the locale from Accept-Language and drops a malformed one", async () => {
    const a = await create({ items: [{ id: "EBOOK-1" }] }, { "accept-language": "fr-FR,fr;q=0.8" });
    assert.equal(a.json.locale, "fr-FR");
    const b = await create({ items: [{ id: "EBOOK-1" }] }, { "accept-language": "not a tag" });
    assert.equal(b.json.locale, undefined);
  });

  it("is not ready without the buyer's country, and says what is missing", async () => {
    const { json } = await create({ items: [{ id: "EBOOK-1" }] });
    assert.equal(json.status, "not_ready_for_payment");
    assert.deepEqual(json.totals, []);
    assert.equal(json.messages[0].type, "error");
    assert.equal(json.messages[0].code, "missing");
    assert.equal(json.messages[0].param, "$.fulfillment_details.address.country");
  });

  it("turns what cannot be sold into messages, not errors", async () => {
    await seedProduct({ sku: "DRAFT-1", priceMinor: 100 });
    await env.SHOP_DB.prepare(`UPDATE products SET status = 'draft' WHERE sku = 'DRAFT-1'`).run();
    const cases: Array<[Json, string, string]> = [
      [{ items: [{ id: "EBOOK-1" }, { id: "NOPE" }], fulfillment_details: DE }, "not_found", "$.line_items[1]"],
      [{ items: [{ id: "DRAFT-1" }], fulfillment_details: DE }, "out_of_stock", "$.line_items[0]"],
      [{ items: [{ id: "EBOOK-1" }], fulfillment_details: DE, currency: "USD" }, "invalid", "$.currency"],
    ];
    for (const [body, code, param] of cases) {
      const { res, json } = await create(body);
      assert.equal(res.status, 201);
      assert.equal(json.status, "not_ready_for_payment");
      assert.deepEqual([json.messages[0].code, json.messages[0].param], [code, param]);
    }
  });

  it("refuses countries the shop does not serve or cannot tax", async () => {
    await putSettings(env, { "shop.countries_allowed": ["DE", "FR"] });
    const notServed = await create({ items: [{ id: "EBOOK-1" }], fulfillment_details: { address: { ...DE.address, country: "IT" } } });
    assert.equal(notServed.json.messages[0].code, "region_restricted");
    await env.SHOP_DB.prepare(`DELETE FROM vat_rates WHERE country = 'FR'`).run();
    const noRate = await create({ items: [{ id: "EBOOK-1" }], fulfillment_details: { address: { ...DE.address, country: "FR" } } });
    assert.equal(noRate.json.messages[0].code, "region_restricted");
    assert.equal(noRate.json.messages[0].resolution, "requires_buyer_input");
  });

  it("warns when the agent's price is not the catalogue's, and charges the catalogue's", async () => {
    const { json } = await create({ items: [{ id: "EBOOK-1", unit_amount: 1999 }], fulfillment_details: DE });
    assert.equal(json.status, "ready_for_payment");
    const warning = json.messages.find((m: Json) => m.type === "warning");
    assert.equal(warning.code, "price_change");
    assert.equal(warning.param, "$.line_items[0]");
    assert.equal(json.totals[2].amount, 2400);
  });

  it("prices net catalogues the same way, without a description", async () => {
    await putSettings(env, { "pricing.mode": "net" });
    await env.SHOP_DB.prepare(`UPDATE products SET description = NULL`).run();
    const { json } = await create({ items: [{ id: "EBOOK-1", quantity: 2 }], fulfillment_details: DE });
    assert.equal(json.line_items[0].unit_amount, 2400);
    assert.equal(json.line_items[0].description, undefined);
    assert.deepEqual(json.totals.map((t: Json) => t.amount), [4800, 336, 5136]);
  });

  it("leaves the waiver out when the shop does not ask for one", async () => {
    await putSettings(env, { "legal.digital_waiver": false, "legal.privacy_url": "" });
    const { json } = await create({ items: [{ id: "EBOOK-1" }], fulfillment_details: DE });
    assert.equal(json.line_items[0].disclosures, undefined);
    assert.deepEqual(json.messages, []);
    assert.deepEqual(json.links.map((l: Json) => l.type), ["terms_of_use"]);
  });

  it("answers malformed input with the protocol's Error and the field's path", async () => {
    const cases: Array<[unknown, string]> = [
      [{}, "$.line_items"],
      [{ items: [] }, "$.line_items"],
      [{ items: ["x"] }, "$.line_items[0].id"],
      [{ items: [{ id: "  " }] }, "$.line_items[0].id"],
      [{ items: [{ id: "EBOOK-1", quantity: 0 }] }, "$.line_items[0].quantity"],
      [{ items: [{ id: "EBOOK-1", quantity: 11 }] }, "$.line_items[0].quantity"],
      [{ items: [{ id: "EBOOK-1", quantity: 1.5 }] }, "$.line_items[0].quantity"],
      [{ items: [{ id: "EBOOK-1", unit_amount: -1 }] }, "$.line_items[0].unit_amount"],
      [{ items: [{ id: "EBOOK-1" }], buyer: "x" }, "$.buyer"],
      [{ items: [{ id: "EBOOK-1" }], buyer: { email: "nope" } }, "$.buyer.email"],
      [{ items: [{ id: "EBOOK-1" }], fulfillment_details: [] }, "$.fulfillment_details"],
      [{ items: [{ id: "EBOOK-1" }], fulfillment_details: { address: "x" } }, "$.fulfillment_details.address"],
      [{ items: [{ id: "EBOOK-1" }], fulfillment_details: { address: { country: "Germany" } } }, "$.fulfillment_details.address.country"],
      [{ items: [{ id: "EBOOK-1" }], fulfillment_details: { address: { country: 7 } } }, "$.fulfillment_details.address.country"],
      [{ items: [{ id: "EBOOK-1" }], currency: "EURO" }, "$.currency"],
      [{ items: [{ id: "EBOOK-1" }], currency: 5 }, "$.currency"],
      [{ items: Array.from({ length: 21 }, (_, i) => ({ id: `S-${i}` })) }, "$.line_items"],
    ];
    for (const [body, param] of cases) {
      const { res, json } = await create(body);
      assert.equal(res.status, 400, JSON.stringify(body));
      assert.deepEqual({ type: json.type, code: json.code, param: json.param }, { type: "invalid_request", code: "invalid", param });
      assert.equal(typeof json.message, "string");
    }
  });

  it("refuses a body that is not a JSON object", async () => {
    for (const raw of ["{not json", "[1,2]", "null"]) {
      const { res, json } = await create(raw);
      assert.equal(res.status, 400);
      assert.equal(json.code, "invalid_json");
    }
  });
});

describe("retrieving, updating and cancelling", () => {
  it("returns the stored state, and 404 for an unknown id", async () => {
    const { json: made } = await create({ items: [{ id: "EBOOK-1" }], fulfillment_details: DE });
    const { res, json } = await retrieve(made.id);
    assert.equal(res.status, 200);
    assert.deepEqual(json, made);
    assert.equal(res.headers.get("idempotency-key"), null);
    const missing = await retrieve("cs_missing");
    assert.equal(missing.res.status, 404);
    assert.deepEqual(missing.json, { type: "invalid_request", code: "not_found", message: "No such checkout session." });
  });

  it("applies changes, leaves absent fields alone and clears on null", async () => {
    const { json: made } = await create({ items: [{ id: "EBOOK-1" }], buyer: { email: "a@example.com" } });
    assert.equal(made.status, "not_ready_for_payment");
    const withCountry = await update(made.id, { fulfillment_details: DE, selected_fulfillment_options: [{ type: "digital", option_id: "digital", item_ids: ["EBOOK-1"] }] });
    assert.equal(withCountry.res.status, 200);
    assert.equal(withCountry.json.status, "ready_for_payment");
    assert.equal(withCountry.json.buyer.email, "a@example.com");
    const more = await update(made.id, { line_items: [{ id: "EBOOK-1", quantity: 2 }], buyer: null });
    assert.equal(more.json.totals[2].amount, 4800);
    assert.equal(more.json.buyer, undefined);
    assert.equal(more.json.fulfillment_details.address.country, "DE");
    const cleared = await update(made.id, { fulfillment_details: null, selected_fulfillment_options: null });
    assert.equal(cleared.json.status, "not_ready_for_payment");
    assert.equal(cleared.json.fulfillment_details, undefined);
  });

  it("rejects bad updates and unknown sessions", async () => {
    const { json: made } = await create({ items: [{ id: "EBOOK-1" }] });
    for (const [body, param] of [
      [{ selected_fulfillment_options: "x" }, "$.selected_fulfillment_options"],
      [{ selected_fulfillment_options: [{ option_id: "ship" }] }, "$.selected_fulfillment_options[0].option_id"],
      [{ items: [] }, "$.line_items"],
      [{ buyer: { email: "" } }, "$.buyer.email"],
      [{ fulfillment_details: 3 }, "$.fulfillment_details"],
    ] as Array<[unknown, string]>) {
      const { res, json } = await update(made.id, body);
      assert.equal(res.status, 400);
      assert.equal(json.param, param);
    }
    assert.equal((await update("cs_missing", {})).res.status, 404);
  });

  it("loses no write when two updates race", async () => {
    const { json: made } = await create({ items: [{ id: "EBOOK-1" }] });
    const [a, b] = await Promise.all([
      update(made.id, { buyer: { email: "racer@example.com" } }),
      update(made.id, { fulfillment_details: DE }),
    ]);
    assert.equal(a.res.status, 200);
    assert.equal(b.res.status, 200);
    const { json } = await retrieve(made.id);
    assert.equal(json.buyer.email, "racer@example.com");
    assert.equal(json.fulfillment_details.address.country, "DE");
    assert.equal(json.status, "ready_for_payment");
  });

  it("gives up with 409 when every attempt loses the race", async () => {
    const { json: made } = await create({ items: [{ id: "EBOOK-1" }] });
    const lossy = { ...env, SHOP_DB: lossyD1(env.SHOP_DB, /^UPDATE acp_sessions/) };
    const input = { request: new Request("https://shop.example.com/"), env: lossy as never, body: {}, id: made.id, origin: "https://shop.example.com", waitUntil: () => {} };
    const out = await updateSession(input);
    assert.equal(out.status, 409);
    assert.equal((out.body as Json).code, "concurrent_update");
    const cancelled = await cancelSession(input);
    assert.equal(cancelled.status, 409);
    assert.equal((cancelled.body as Json).code, "invalid_session_state");
  });

  it("cancels once, then answers 405", async () => {
    const { json: made } = await create({ items: [{ id: "EBOOK-1" }], fulfillment_details: DE });
    const first = await cancel(made.id);
    assert.equal(first.res.status, 200);
    assert.equal(first.json.status, "canceled");
    const again = await cancel(made.id);
    assert.equal(again.res.status, 405);
    assert.equal(again.json.code, "not_cancelable");
    const late = await update(made.id, { fulfillment_details: DE });
    assert.equal(late.res.status, 409);
    assert.equal(late.json.code, "invalid_session_state");
    assert.equal(late.res.headers.get("retry-after"), null);
    assert.equal((await cancel("cs_missing")).res.status, 404);
  });

  it("answers 409 with Retry-After while a completion is running", async () => {
    const { json: made } = await create({ items: [{ id: "EBOOK-1" }], fulfillment_details: DE });
    await env.SHOP_DB.prepare(`UPDATE acp_sessions SET status = 'complete_in_progress' WHERE id = ?`).bind(made.id).run();
    const busy = await cancel(made.id);
    assert.equal(busy.res.status, 409);
    assert.equal(busy.res.headers.get("retry-after"), "2");
    assert.equal((await update(made.id, {})).res.status, 409);
  });

  it("expires an abandoned session", async () => {
    const { json: made } = await create({ items: [{ id: "EBOOK-1" }], fulfillment_details: DE });
    await env.SHOP_DB.prepare(`UPDATE acp_sessions SET expires_at = '2000-01-01T00:00:00.000Z' WHERE id = ?`).bind(made.id).run();
    const { json } = await retrieve(made.id);
    assert.equal(json.status, "expired");
    assert.equal((await cancel(made.id)).res.status, 405);
  });
});

describe("authentication and versioning", () => {
  const body = { items: [{ id: "EBOOK-1" }] };

  it("refuses a missing or wrong bearer token", async () => {
    const none = await call(createRoute.onRequestPost as never, acpRequest("POST", "/checkout_sessions", body, {}, ["authorization"]));
    assert.equal(none.status, 401);
    for (const headers of [{ authorization: "" }, { authorization: "Bearer wrong" }, { authorization: "Basic abc" }]) {
      const { res, json } = await create(body, headers);
      assert.equal(res.status, 401);
      assert.equal(json.code, "unauthorized");
      assert.equal(res.headers.get("www-authenticate"), "Bearer");
    }
  });

  it("requires a supported API-Version and lists the supported ones", async () => {
    const missing = await call(createRoute.onRequestPost as never, acpRequest("POST", "/checkout_sessions", body, {}, ["api-version"]));
    const m = (await missing.json()) as Json;
    assert.equal(missing.status, 400);
    assert.equal(m.code, "missing_api_version");
    assert.deepEqual(m.supported_versions, ["2026-04-17", "2026-01-30", "2026-01-16"]);
    const old = await create(body, { "api-version": "2025-09-29" });
    assert.equal(old.json.code, "unsupported_api_version");
    assert.equal((await create(body, { "api-version": "2026-01-16" })).res.status, 201);
  });

  it("is closed, not open, when a secret is missing", async () => {
    for (const missing of ["SHOP_ACP_API_KEY", "STRIPE_SECRET_KEY", "SHOP_ACP_MERCHANT_ID"]) {
      enableAcp({ [missing]: undefined });
      const { res, json } = await create(body);
      assert.equal(res.status, 503);
      assert.deepEqual([json.type, json.code], ["service_unavailable", "not_configured"]);
    }
  });

  it("checks Signature and Timestamp when a signing secret is set", async () => {
    enableAcp({ SHOP_ACP_SIGNING_SECRET: "sign-secret-x" });
    const raw = JSON.stringify({ items: [{ quantity: 1, id: "EBOOK-1" }] });
    const now = new Date().toISOString();
    const rawSig = await hmacBase64url("sign-secret-x", `${now}.${raw}`);
    const canonSig = await hmacBase64url("sign-secret-x", `${now}.${canonicalJson(JSON.parse(raw))}`);
    const base64 = Buffer.from(rawSig, "base64url").toString("base64");
    const stale = new Date(Date.now() - 600_000).toISOString();
    const cases: Array<[Record<string, string>, number]> = [
      [{}, 401],
      [{ timestamp: now }, 401],
      [{ signature: rawSig, timestamp: "yesterday" }, 401],
      [{ signature: await hmacBase64url("sign-secret-x", `${stale}.${raw}`), timestamp: stale }, 401],
      [{ signature: await hmacBase64url("other", `${now}.${raw}`), timestamp: now }, 401],
      [{ signature: rawSig, timestamp: now }, 201],
      [{ signature: canonSig, timestamp: now }, 201],
      [{ signature: base64, timestamp: now }, 201],
    ];
    for (const [headers, status] of cases) {
      const res = await call(createRoute.onRequestPost as never, acpRequest("POST", "/checkout_sessions", raw, headers));
      assert.equal(res.status, status, JSON.stringify(headers));
      if (status === 401) assert.equal(((await res.json()) as Json).code, "invalid_signature");
    }
    // A GET has no body; the signature covers the timestamp and "".
    const made = (await (await call(createRoute.onRequestPost as never, acpRequest("POST", "/checkout_sessions", raw, { signature: rawSig, timestamp: now }))).json()) as Json;
    const getSig = await hmacBase64url("sign-secret-x", `${now}.`);
    const got = await call(sessionRoute.onRequestGet as never, acpRequest("GET", `/checkout_sessions/${made.id}`, undefined, { signature: getSig, timestamp: now }), { id: made.id });
    assert.equal(got.status, 200);
  });
});

describe("the route around every operation", () => {
  it("is a 404 on every route while ACP is off", async () => {
    for (const value of [undefined, "1", "yes", "TRUE"]) {
      enableAcp({ SHOP_ACP_ENABLED: value });
      const routes: Array<[unknown, Request]> = [
        [createRoute.onRequestPost, acpRequest("POST", "/checkout_sessions", {})],
        [sessionRoute.onRequestGet, acpRequest("GET", "/checkout_sessions/cs_x")],
        [sessionRoute.onRequestPost, acpRequest("POST", "/checkout_sessions/cs_x", {})],
        [completeRoute.onRequestPost, acpRequest("POST", "/checkout_sessions/cs_x/complete", {})],
        [cancelRoute.onRequestPost, acpRequest("POST", "/checkout_sessions/cs_x/cancel", {})],
      ];
      for (const [handler, request] of routes) {
        const res = await call(handler as never, request, { id: "cs_x" });
        assert.equal(res.status, 404);
      }
    }
    const rows = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM acp_sessions`).first<{ n: number }>();
    assert.equal(rows?.n, 0);
  });

  it("answers 503 without a database or a configured shop, and 413 for a huge body", async () => {
    const noDb = await createRoute.onRequestPost({ ...(await import("./_helpers")).ctx(acpRequest("POST", "/checkout_sessions", {})), env: { ...env, SHOP_DB: undefined } } as never);
    assert.equal(noDb.status, 503);
    const huge = await create(JSON.stringify({ items: [{ id: "x".repeat(70_000) }] }));
    assert.equal(huge.res.status, 413);
    await putSettings(env, { "seller.name": "" });
    const unconfigured = await create({ items: [{ id: "EBOOK-1" }] });
    assert.equal(unconfigured.res.status, 503);
    assert.equal(unconfigured.json.code, "shop_not_configured");
  });

  it("requires Idempotency-Key, replays a repeat and refuses a changed body", async () => {
    const noKey = await call(createRoute.onRequestPost as never, acpRequest("POST", "/checkout_sessions", { items: [{ id: "EBOOK-1" }] }, {}, ["idempotency-key"]));
    assert.equal(noKey.status, 400);
    assert.equal(((await noKey.json()) as Json).code, "idempotency_key_required");

    const key = { "idempotency-key": "same-key-1" };
    const first = await create({ items: [{ id: "EBOOK-1", quantity: 1 }] }, key);
    const replay = await create({ items: [{ quantity: 1, id: "EBOOK-1" }] }, key);
    assert.equal(replay.res.status, 201);
    assert.equal(replay.res.headers.get("idempotent-replayed"), "true");
    assert.match(replay.res.headers.get("request-id") ?? "", /^req_/);
    assert.deepEqual(replay.json, first.json);
    const rows = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM acp_sessions`).first<{ n: number }>();
    assert.equal(rows?.n, 1);

    const changed = await create({ items: [{ id: "EBOOK-1", quantity: 2 }] }, key);
    assert.equal(changed.res.status, 422);
    assert.equal(changed.json.code, "idempotency_conflict");
  });

  it("answers 500 without detail when an operation fails, and the retry runs fresh", async () => {
    env.SHOP_DB.state.failOn = /^INSERT INTO acp_sessions/;
    const key = { "idempotency-key": "retry-after-500" };
    const failed = await create({ items: [{ id: "EBOOK-1" }] }, key);
    assert.equal(failed.res.status, 500);
    assert.deepEqual(failed.json, { type: "processing_error", code: "internal_error", message: "The request could not be processed." });
    assert.equal(failed.res.headers.get("idempotency-key"), "retry-after-500");
    const logged = await env.SHOP_DB.prepare(`SELECT action FROM audit_log WHERE action = 'acp.error'`).first<{ action: string }>();
    assert.equal(logged?.action, "acp.error");
    env.SHOP_DB.state.failOn = null;
    const retried = await create({ items: [{ id: "EBOOK-1" }] }, key);
    assert.equal(retried.res.status, 201);
    assert.equal(retried.res.headers.get("idempotent-replayed"), null);
  });
});
