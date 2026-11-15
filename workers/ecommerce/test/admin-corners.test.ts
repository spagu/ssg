// The panel's order, queue, report and settings endpoints, at their edges:
// a missing route parameter, a body that is not JSON, a filter or a page that
// asks for something unusual, and a database answer with nothing in it.
//
// Handlers are called directly with the identity the middleware would have put
// in `data`, as api-admin.test.ts does; the gate itself is tested there.

import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { env } from "./_env";
import { onRequestGet as listOrders } from "../functions/api/shop/admin/orders/index";
import { onRequestGet as getOrder, onRequestPatch as noteOrder } from "../functions/api/shop/admin/orders/[id]";
import { onRequestPost as cancelOrder } from "../functions/api/shop/admin/orders/[id]/cancel";
import { onRequestPost as markPaid } from "../functions/api/shop/admin/orders/[id]/mark-paid";
import { onRequestPost as refundOrder } from "../functions/api/shop/admin/orders/[id]/refund";
import { onRequestPost as resendOrder } from "../functions/api/shop/admin/orders/[id]/resend";
import { onRequestGet as listOutbox, onRequestPost as pushOutbox } from "../functions/api/shop/admin/outbox";
import { onRequestGet as stats } from "../functions/api/shop/admin/stats";
import { onRequestGet as auditLog } from "../functions/api/shop/admin/audit";
import { onRequestGet as exportCsv } from "../functions/api/shop/admin/export";
import {
  onRequestDelete as deleteRate,
  onRequestGet as listRates,
  onRequestPost as setRate,
} from "../functions/api/shop/admin/vat-rates";
import { onRequestGet as me } from "../functions/api/shop/admin/me";
import { onRequestPut as putSettingsEndpoint } from "../functions/api/shop/admin/settings";
import { newId, nowISO } from "../functions/api/shop/_lib";
import { invalidateSettings } from "../functions/api/shop/_settings";
import { bodyOf, ctx, freshShop, get, ORIGIN, post, seedProduct } from "./_helpers";
import { sparseD1 } from "./fakes";

/** The identity the password mode gives: every shop permission. */
const admin = { mode: "password", user: null, perms: ["ecommerce:*"], actor: "password" };
const realFetch = globalThis.fetch;

type Handler = (c: never) => Response | Promise<Response>;
type Shop = typeof env;

/** Calls a handler behind the gate, as the panel would reach it. */
async function call(
  handler: Handler,
  request: Request,
  params: Record<string, string> = {},
  shop: Shop = env,
  who: Record<string, unknown> = admin,
): Promise<Response> {
  return handler({ ...ctx(request, params, { admin: who }), env: shop } as never);
}

/** A raw request with no JSON body at all, which readJson cannot read. */
const noBody = (path: string) => new Request(`${ORIGIN}${path}`, { method: "POST", body: "not json" });

/** An env whose database answers the statements matching `pattern` the way a
 *  D1 result without its optional parts would: no `results`, no `meta`, no
 *  row. Everything else runs for real. */
function sparse(pattern: RegExp, rows?: Record<string, unknown>[]): Shop {
  return { ...env, SHOP_DB: sparseD1(env.SHOP_DB, pattern, { rows }) } as Shop;
}

interface OrderSeed {
  status?: string;
  number?: string;
  customer?: boolean;
  gateway?: string | null;
  total?: number;
  taxEvidence?: string | null;
  paidAt?: string | null;
  createdAt?: string;
  locale?: string | null;
  reverseCharge?: number;
  taxCountry?: string | null;
}

/** An order written straight to the database, in whatever state a test needs. */
async function seedOrder(o: OrderSeed = {}): Promise<string> {
  const id = newId();
  let customerId: string | null = null;
  if (o.customer !== false) {
    customerId = newId();
    await env.SHOP_DB.prepare(
      `INSERT INTO customers (id, email, email_lc, name, country, created_at) VALUES (?, ?, ?, 'A Buyer', 'DE', ?)`,
    )
      .bind(customerId, "buyer@example.com", "buyer@example.com", nowISO())
      .run();
  }
  const created = o.createdAt ?? nowISO();
  await env.SHOP_DB.prepare(
    `INSERT INTO orders (id, number, key_hash, status, customer_id, currency, subtotal_minor, tax_minor, total_minor,
                         tax_country, tax_evidence, reverse_charge, gateway, gateway_ref, locale, created_at, paid_at, updated_at)
     VALUES (?, ?, 'h', ?, ?, 'EUR', ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
  )
    .bind(
      id,
      o.number ?? `ORD-${id.slice(0, 8)}`,
      o.status ?? "paid",
      customerId,
      o.total ?? 2400,
      o.total ?? 2400,
      o.taxCountry === undefined ? "DE" : o.taxCountry,
      o.taxEvidence ?? null,
      o.reverseCharge ?? 0,
      o.gateway === undefined ? "stripe" : o.gateway,
      `pi_${id}`,
      o.locale === undefined ? "en" : o.locale,
      created,
      o.paidAt === undefined ? created : o.paidAt,
      created,
    )
    .run();
  return id;
}

async function addLine(orderId: string, productId: string, sku = "EBOOK-1"): Promise<void> {
  await env.SHOP_DB.prepare(
    `INSERT INTO order_items (id, order_id, product_id, sku, name, quantity, unit_minor, tax_rate_bp, tax_minor, total_minor)
     VALUES (?, ?, ?, ?, 'A Book', 1, 2400, 1900, 383, 2400)`,
  )
    .bind(newId(), orderId, productId, sku)
    .run();
}

async function addCharge(orderId: string, kind = "charge", amount = 2400): Promise<void> {
  await env.SHOP_DB.prepare(
    `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, created_at)
     VALUES (?, ?, 'stripe', ?, ?, 'succeeded', ?, 'EUR', ?)`,
  )
    .bind(newId(), orderId, `ref-${newId()}`, kind, amount, nowISO())
    .run();
}

beforeEach(async () => {
  await freshShop();
});

afterEach(() => {
  globalThis.fetch = realFetch;
  env.SHOP_DB.state.failOn = null;
});

describe("one order in the panel", () => {
  it("404s when the route names no order at all", async () => {
    assert.equal((await call(getOrder as Handler, get("/x"))).status, 404);
    assert.equal((await call(noteOrder as Handler, post("/x", { notes: "a" }))).status, 404);
  });

  it("shows an order without a customer, and reads its tax evidence only when it is JSON", async () => {
    const id = await seedOrder({ customer: false, taxEvidence: "{not json" });
    const body = await bodyOf<{ customer: unknown; order: { taxEvidence: unknown; key_hash?: string } }>(
      await call(getOrder as Handler, get("/x"), { id }),
    );
    assert.equal(body.customer, null);
    assert.equal(body.order.taxEvidence, null);
    assert.equal(body.order.key_hash, undefined);

    const withEvidence = await seedOrder({ taxEvidence: JSON.stringify({ billing: "DE" }) });
    const read = await bodyOf<{ order: { taxEvidence: unknown } }>(
      await call(getOrder as Handler, get("/x"), { id: withEvidence }),
    );
    assert.deepEqual(read.order.taxEvidence, { billing: "DE" });
  });

  it("finds an order by its number as well as its id", async () => {
    await seedOrder({ number: "ORD-NUMBERED" });
    const res = await call(getOrder as Handler, get("/x"), { id: "ORD-NUMBERED" });
    assert.equal(res.status, 200);
  });

  it("answers with empty lists when the database returns no rows array", async () => {
    const id = await seedOrder();
    const shop = sparse(/FROM (order_items|payments|invoices|download_tokens)/);
    const body = await bodyOf<{ items: unknown[]; payments: unknown[]; invoices: unknown[]; tokens: unknown[] }>(
      await call(getOrder as Handler, get("/x"), { id }, shop),
    );
    assert.deepEqual([body.items, body.payments, body.invoices, body.tokens], [[], [], [], []]);
  });

  it("refuses a note that is not a note, and clears one sent empty", async () => {
    const id = await seedOrder();
    assert.equal((await call(noteOrder as Handler, noBody("/x"), { id })).status, 400);
    assert.equal((await call(noteOrder as Handler, post("/x", { notes: 5 }), { id })).status, 400);

    const res = await call(noteOrder as Handler, post("/x", { notes: "" }), { id });
    assert.equal(res.status, 200);
    const row = await env.SHOP_DB.prepare(`SELECT notes FROM orders WHERE id = ?`).bind(id).first<{ notes: unknown }>();
    assert.equal(row?.notes, null);
    const logged = await env.SHOP_DB.prepare(`SELECT actor FROM audit_log WHERE action = 'order.note'`).first<{
      actor: string;
    }>();
    assert.equal(logged?.actor, "password");
  });
});

describe("cancelling an order", () => {
  it("404s without an order", async () => {
    assert.equal((await call(cancelOrder as Handler, post("/x", {}))).status, 404);
  });

  it("cancels an unpaid one with no reason given, and records who did", async () => {
    const id = await seedOrder({ status: "pending", paidAt: null });
    const res = await call(cancelOrder as Handler, noBody("/x"), { id });
    assert.equal(res.status, 200);
    const row = await env.SHOP_DB.prepare(`SELECT status FROM orders WHERE id = ?`).bind(id).first<{ status: string }>();
    assert.equal(row?.status, "cancelled");
  });

  it("refuses when the database reports no row changed", async () => {
    const id = await seedOrder({ status: "pending", paidAt: null });
    const res = await call(cancelOrder as Handler, post("/x", { reason: "r" }), { id }, sparse(/^UPDATE orders/));
    assert.equal(res.status, 409);
  });
});

describe("marking an order paid by hand", () => {
  it("404s without an order, and refuses one that is not waiting for money", async () => {
    assert.equal((await call(markPaid as Handler, post("/x", {}))).status, 404);
    const paid = await seedOrder({ status: "refunded" });
    assert.equal((await call(markPaid as Handler, post("/x", {}), { id: paid })).status, 409);
  });

  it("names the payment after the order when no reference is given", async () => {
    const { id: productId } = await seedProduct();
    const id = await seedOrder({ status: "needs_review", number: "ORD-MANUAL", paidAt: null });
    await addLine(id, productId);

    const res = await call(markPaid as Handler, noBody("/x"), { id });
    assert.equal(res.status, 200);
    const body = await bodyOf<{ changed: boolean; invoiceNumber: string | null }>(res);
    assert.equal(body.changed, true);
    const pay = await env.SHOP_DB.prepare(`SELECT gateway_ref FROM payments WHERE order_id = ?`)
      .bind(id)
      .first<{ gateway_ref: string }>();
    assert.equal(pay?.gateway_ref, "manual-ORD-MANUAL");
  });

  it("says there is no invoice number when invoicing is switched off", async () => {
    await freshShop({ "modules.invoices": false });
    const id = await seedOrder({ status: "pending", paidAt: null });
    const body = await bodyOf<{ invoiceNumber: string | null }>(
      await call(markPaid as Handler, post("/x", { reference: "bank-1" }), { id }),
    );
    assert.equal(body.invoiceNumber, null);
  });
});

describe("refunding an order", () => {
  const stripeEnv = (): Shop => ({ ...env, STRIPE_SECRET_KEY: "sk_test_x", SHOP_GATEWAYS: "stripe" }) as Shop;

  it("404s without an order, and refuses one nobody paid for", async () => {
    assert.equal((await call(refundOrder as Handler, post("/x", {}), {}, stripeEnv())).status, 404);
    const pending = await seedOrder({ status: "pending" });
    assert.equal((await call(refundOrder as Handler, post("/x", {}), { id: pending }, stripeEnv())).status, 409);
  });

  it("refuses when no successful charge is recorded", async () => {
    const id = await seedOrder();
    const res = await call(refundOrder as Handler, post("/x", {}), { id }, stripeEnv());
    assert.equal((await bodyOf<{ error: string }>(res)).error, "no_payment");
  });

  it("refunds the whole order when no amount is named", async () => {
    const id = await seedOrder();
    await addCharge(id);
    globalThis.fetch = (async () => new Response(JSON.stringify({ id: "re_1" }))) as typeof fetch;
    const body = await bodyOf<{ amountMinor: number; refundRef: string }>(
      await call(refundOrder as Handler, noBody("/x"), { id }, stripeEnv()),
    );
    assert.deepEqual([body.amountMinor, body.refundRef], [2400, "re_1"]);
  });

  it("refuses an amount that is not a positive number, or more than is left", async () => {
    const id = await seedOrder();
    await addCharge(id);
    await addCharge(id, "refund", -2000);
    const zero = await call(refundOrder as Handler, post("/x", { amountMinor: 0 }), { id }, stripeEnv());
    assert.equal((await bodyOf<{ error: string }>(zero)).error, "invalid_amount");
    const words = await call(refundOrder as Handler, post("/x", { amountMinor: "lots" }), { id }, stripeEnv());
    assert.equal((await bodyOf<{ error: string }>(words)).error, "invalid_amount");
    const tooMuch = await call(refundOrder as Handler, post("/x", { amountMinor: 500 }), { id }, stripeEnv());
    assert.equal((await bodyOf<{ error: string }>(tooMuch)).error, "amount_too_large");
  });

  it("counts nothing as already refunded when the sum comes back empty", async () => {
    const id = await seedOrder();
    await addCharge(id);
    globalThis.fetch = (async () => new Response(JSON.stringify({ id: "re_2" }))) as typeof fetch;
    const res = await call(refundOrder as Handler, post("/x", { amountMinor: 2400 }), { id }, sparse(/COALESCE\(SUM/));
    // sparse() spreads env, which has no Stripe key: the amount check passed,
    // and it is the provider that is missing.
    assert.equal((await bodyOf<{ error: string }>(res)).error, "gateway_unavailable");
  });

  it("refuses when the order names no provider, or one that is not configured", async () => {
    const none = await seedOrder({ gateway: null });
    await addCharge(none);
    const a = await call(refundOrder as Handler, post("/x", { amountMinor: 100 }), { id: none }, stripeEnv());
    assert.equal(a.status, 503);

    const paypal = await seedOrder({ gateway: "paypal" });
    await addCharge(paypal);
    const b = await call(refundOrder as Handler, post("/x", { amountMinor: 100 }), { id: paypal }, stripeEnv());
    assert.equal(b.status, 503);
  });

  it("says the provider refused, and logs it, when the refund call fails", async () => {
    const id = await seedOrder();
    await addCharge(id);
    globalThis.fetch = (async () =>
      new Response(JSON.stringify({ error: { message: "charge already refunded" } }), { status: 400 })) as typeof fetch;
    const res = await call(refundOrder as Handler, post("/x", {}), { id }, stripeEnv());
    assert.equal(res.status, 502);
    const logged = await env.SHOP_DB.prepare(`SELECT detail_json FROM audit_log WHERE action = 'order.refund_failed'`)
      .first<{ detail_json: string }>();
    assert.match(logged!.detail_json, /already refunded/);
  });
});

describe("sending the links again", () => {
  it("404s without an order, and refuses an unpaid one or one without a buyer", async () => {
    assert.equal((await call(resendOrder as Handler, post("/x", {}))).status, 404);
    const pending = await seedOrder({ status: "pending" });
    assert.equal((await call(resendOrder as Handler, post("/x", {}), { id: pending })).status, 409);
    const anonymous = await seedOrder({ customer: false });
    const res = await call(resendOrder as Handler, post("/x", {}), { id: anonymous });
    assert.equal((await bodyOf<{ error: string }>(res)).error, "no_customer");
  });

  it("has nothing to send when no line has a file, or the product is gone", async () => {
    const { id: noFile } = await seedProduct({ sku: "NOFILE", withFile: false });
    const id = await seedOrder();
    await addLine(id, noFile, "NOFILE");
    await addLine(id, newId(), "GONE");
    const res = await call(resendOrder as Handler, post("/x", {}), { id });
    assert.equal((await bodyOf<{ error: string }>(res)).error, "nothing_to_send");
  });

  it("has nothing to send when the database returns no lines", async () => {
    const id = await seedOrder();
    const res = await call(resendOrder as Handler, post("/x", {}), { id }, sparse(/SELECT \* FROM order_items/));
    assert.equal((await bodyOf<{ error: string }>(res)).error, "nothing_to_send");
  });

  it("queues the email in English when the order has no locale, even if the queue cannot drain", async () => {
    const { id: productId } = await seedProduct();
    const id = await seedOrder({ locale: null });
    await addLine(id, productId);
    env.SHOP_DB.state.failOn = /next_attempt <= \?/;
    const res = await call(resendOrder as Handler, post("/x", {}), { id });
    env.SHOP_DB.state.failOn = null;
    assert.equal(res.status, 200);
    const queued = await env.SHOP_DB.prepare(`SELECT payload_json FROM outbox`).first<{ payload_json: string }>();
    assert.ok(queued);
  });

  it("calls the shop 'Shop' when the owner cleared its name", async () => {
    const { id: productId } = await seedProduct();
    const id = await seedOrder();
    await addLine(id, productId);
    await env.SHOP_DB.prepare(`UPDATE settings SET value = 'null' WHERE key = 'shop.name'`).run();
    invalidateSettings();
    const res = await call(resendOrder as Handler, post("/x", {}), { id });
    assert.equal(res.status, 200);
    const queued = await env.SHOP_DB.prepare(`SELECT payload_json FROM outbox`).first<{ payload_json: string }>();
    assert.match(queued!.payload_json, /Shop/);
  });
});

describe("the order list, sorted and paged", () => {
  async function pages(sort: string): Promise<string[]> {
    const seen: string[] = [];
    let cursor = "";
    for (let i = 0; i < 5; i++) {
      const q = `/api/shop/admin/orders?limit=1&sort=${sort}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`;
      const body = await bodyOf<{ orders: { number: string }[]; nextCursor: string | null }>(
        await call(listOrders as Handler, get(q)),
      );
      seen.push(...body.orders.map((o) => o.number));
      if (!body.nextCursor) break;
      cursor = body.nextCursor;
    }
    return seen;
  }

  it("pages through every sort key without losing an order", async () => {
    await seedOrder({ number: "A-1", status: "paid", paidAt: "2026-10-01T00:00:00.000Z" });
    await seedOrder({ number: "A-2", status: "pending", paidAt: null });
    for (const sort of ["number", "status", "total", "date"]) {
      assert.deepEqual((await pages(sort)).sort(), ["A-1", "A-2"], sort);
    }
  });

  // Known bug, reported for GO-159: the cursor after a paid order compares
  // paid_at with `<`, which a NULL never satisfies, so the unpaid orders that
  // sort after it fall off the second page. One page (limit=100) keeps them —
  // listing.test.ts covers that. Runs, but is not counted as a failure.
  it("pages by payment date without dropping the unpaid orders", { todo: "cursor drops NULL paid_at rows" }, async () => {
    await seedOrder({ number: "A-1", status: "paid", paidAt: "2026-10-01T00:00:00.000Z" });
    await seedOrder({ number: "A-2", status: "pending", paidAt: null });
    assert.deepEqual((await pages("paid")).sort(), ["A-1", "A-2"]);
  });

  it("falls back to 25 a page for a limit that is not a number", async () => {
    await seedOrder();
    const body = await bodyOf<{ orders: unknown[] }>(await call(listOrders as Handler, get("/x?limit=lots")));
    assert.equal(body.orders.length, 1);
  });

  it("answers an empty page when the database returns nothing", async () => {
    const body = await bodyOf<{ orders: unknown[]; total: number; nextCursor: unknown }>(
      await call(listOrders as Handler, get("/x"), {}, sparse(/FROM orders o/)),
    );
    assert.deepEqual([body.orders, body.total, body.nextCursor], [[], 0, null]);
  });
});

describe("the outbox", () => {
  async function queue(n: number, attempts = 0): Promise<void> {
    for (let i = 0; i < n; i++) {
      await env.SHOP_DB.prepare(
        `INSERT INTO outbox (id, kind, target, payload_json, attempts, next_attempt, created_at)
         VALUES (?, 'email', 'a@example.com', '{}', ?, ?, ?)`,
      )
        .bind(newId(), attempts, `2026-01-0${i + 1}T00:00:00.000Z`, nowISO())
        .run();
    }
  }

  it("reports zero of everything when nothing was ever queued", async () => {
    const body = await bodyOf<{ total: number; nextCursor: unknown }>(
      await call(listOutbox as Handler, get("/x?limit=junk")),
    );
    assert.deepEqual([body.total, body.nextCursor], [0, null]);
    const stuck = await bodyOf<{ total: number }>(await call(listOutbox as Handler, get("/x?stuck=1")));
    assert.equal(stuck.total, 0);
  });

  it("pages through what is queued, with or without the tie-breaking id", async () => {
    await queue(3, 4);
    const first = await bodyOf<{ messages: { id: string }[]; nextCursor: string; total: number }>(
      await call(listOutbox as Handler, get("/x?limit=1&stuck=1")),
    );
    assert.equal(first.total, 3);
    const second = await bodyOf<{ messages: { id: string }[] }>(
      await call(listOutbox as Handler, get(`/x?limit=1&cursor=${encodeURIComponent(first.nextCursor)}`)),
    );
    assert.notEqual(second.messages[0]!.id, first.messages[0]!.id);
    // A cursor that is only a timestamp starts at that timestamp: every id
    // sorts after the empty one.
    const coarse = await bodyOf<{ messages: unknown[] }>(
      await call(listOutbox as Handler, get("/x?cursor=2026-01-02T00:00:00.000Z")),
    );
    assert.equal(coarse.messages.length, 2);
  });

  it("answers zeroes when the database returns no rows and no counts", async () => {
    const body = await bodyOf<{ messages: unknown[]; counts: unknown; total: number }>(
      await call(listOutbox as Handler, get("/x?stuck=1"), {}, sparse(/FROM outbox/)),
    );
    assert.deepEqual([body.messages, body.counts, body.total], [[], { pending: 0, stuck: 0, done: 0 }, 0]);
    const all = await bodyOf<{ total: number }>(await call(listOutbox as Handler, get("/x"), {}, sparse(/FROM outbox/)));
    assert.equal(all.total, 0);
  });

  it("drains the queue for a body it cannot read, and retries one message by id", async () => {
    const drained = await call(pushOutbox as Handler, noBody("/x"));
    assert.equal(drained.status, 200);

    assert.equal((await call(pushOutbox as Handler, post("/x", { id: "nope" }))).status, 404);

    await queue(1, 9);
    const row = await env.SHOP_DB.prepare(`SELECT id FROM outbox`).first<{ id: string }>();
    const retried = await call(pushOutbox as Handler, post("/x", { id: row!.id }));
    assert.equal(retried.status, 200);
    const logged = await env.SHOP_DB.prepare(`SELECT subject FROM audit_log WHERE action = 'outbox.retry'`).first<{
      subject: string;
    }>();
    assert.equal(logged?.subject, row!.id);
  });
});

describe("the numbers", () => {
  it("reads the default window, and any window it does not know as 30 days", async () => {
    const a = await bodyOf<{ window: string }>(await call(stats as Handler, get("/x")));
    const b = await bodyOf<{ window: string }>(await call(stats as Handler, get("/x?window=forever")));
    assert.deepEqual([a.window, b.window], ["30d", "30d"]);
  });

  it("nets refunds off the currency they were made in, and leaves others alone", async () => {
    const eur = await seedOrder();
    await addCharge(eur, "refund", -400);
    await env.SHOP_DB.prepare(`UPDATE orders SET currency = 'USD' WHERE id = ?`).bind(await seedOrder()).run();
    const body = await bodyOf<{ byCurrency: { currency: string; netMinor: number; refundedMinor: number }[] }>(
      await call(stats as Handler, get("/x?window=7d")),
    );
    const by = Object.fromEntries(body.byCurrency.map((r) => [r.currency, r]));
    assert.deepEqual([by.EUR!.netMinor, by.USD!.refundedMinor], [2000, 0]);
  });

  it("reports zeroes when the database answers with nothing at all", async () => {
    const body = await bodyOf<Record<string, unknown>>(
      await call(stats as Handler, get("/x"), {}, sparse(/FROM (orders|payments|outbox|order_items)/)),
    );
    assert.deepEqual(
      [body.byCurrency, body.needsReview, body.stalePending, body.stuckMessages, body.topProducts, body.byCountry],
      [[], 0, 0, 0, [], []],
    );
  });

  it("treats a currency row without sums as zero", async () => {
    const shop = sparse(/GROUP BY currency/, [{ currency: "EUR", orders: 1, gross: null, tax: null }]);
    const body = await bodyOf<{ byCurrency: { grossMinor: number; taxMinor: number; netMinor: number }[] }>(
      await call(stats as Handler, get("/x"), {}, shop),
    );
    assert.deepEqual(body.byCurrency[0], {
      currency: "EUR",
      orders: 1,
      grossMinor: 0,
      taxMinor: 0,
      refundedMinor: 0,
      netMinor: 0,
    });
  });
});

describe("the audit log", () => {
  async function entry(detail: string | null, at: string): Promise<void> {
    await env.SHOP_DB.prepare(
      `INSERT INTO audit_log (id, actor, action, subject, detail_json, created_at) VALUES (?, 'password', 'x', 's', ?, ?)`,
    )
      .bind(newId(), detail, at)
      .run();
  }

  it("shows a missing or unreadable detail as nothing", async () => {
    await entry(null, "2026-01-02T00:00:00.000Z");
    await entry("{broken", "2026-01-01T00:00:00.000Z");
    const body = await bodyOf<{ entries: { detail: unknown }[] }>(await call(auditLog as Handler, get("/x?limit=x")));
    assert.deepEqual(
      body.entries.map((e) => e.detail),
      [null, null],
    );
  });

  it("accepts a cursor that carries only a timestamp", async () => {
    await entry("{}", "2026-01-02T00:00:00.000Z");
    await entry("{}", "2026-01-01T00:00:00.000Z");
    const body = await bodyOf<{ entries: unknown[] }>(
      await call(auditLog as Handler, get("/x?cursor=2026-01-02T00:00:00.000Z")),
    );
    assert.equal(body.entries.length, 1);
  });

  it("answers an empty page when the database returns no rows", async () => {
    const body = await bodyOf<{ entries: unknown[] }>(
      await call(auditLog as Handler, get("/x"), {}, sparse(/FROM audit_log/)),
    );
    assert.deepEqual(body.entries, []);
  });
});

describe("the accountant's export", () => {
  it("exports orders by default, over the last year, with reverse charge spelled out", async () => {
    await seedOrder({ reverseCharge: 1 });
    const res = await call(exportCsv as Handler, get("/x"));
    assert.equal(res.status, 200);
    const csv = await res.text();
    assert.match(csv, /,"yes","EUR",/);
    assert.match(res.headers.get("content-disposition")!, /shop-orders-\d{4}-\d\d-\d\d-\d{4}-\d\d-\d\d\.csv/);
  });

  it("exports the VAT summary, and takes a 'to' with a time as it is", async () => {
    const { id: productId } = await seedProduct();
    const id = await seedOrder({ paidAt: "2026-06-01T10:00:00.000Z", createdAt: "2026-06-01T10:00:00.000Z" });
    await addLine(id, productId);
    const res = await call(exportCsv as Handler, get("/x?type=vat&from=2026-01-01&to=2026-12-31T00:00:00.000Z"));
    const csv = await res.text();
    assert.match(csv, /"DE","19\.00","1","EUR","20\.17","3\.83"/);
  });

  it("refuses dates it cannot read", async () => {
    const res = await call(exportCsv as Handler, get("/x?from=yesterday"));
    assert.equal(res.status, 422);
  });

  it("formats a row without a currency as euros and leaves empty amounts empty", async () => {
    const shop = sparse(/FROM orders o\s+JOIN order_items/, [
      { tax_country: "DE", tax_rate_bp: null, orders: 1, net: 100, tax: null },
    ]);
    const res = await call(exportCsv as Handler, get("/x?type=vat"), {}, shop);
    assert.match(await res.text(), /"DE","0\.00","1","","1\.00",""/);
  });

  it("exports an empty file when the database returns no rows", async () => {
    const res = await call(exportCsv as Handler, get("/x?type=invoices"), {}, sparse(/FROM invoices/));
    const csv = await res.text();
    assert.equal(csv.trim().split("\r\n").length, 1);
    const logged = await env.SHOP_DB.prepare(`SELECT detail_json FROM audit_log WHERE action = 'export'`).first<{
      detail_json: string;
    }>();
    assert.equal(JSON.parse(logged!.detail_json).rows, 0);
  });
});

describe("VAT rates by hand", () => {
  it("lists one country's rates when asked", async () => {
    const body = await bodyOf<{ rates: { country: string }[] }>(await call(listRates as Handler, get("/x?country=de")));
    assert.ok(body.rates.length > 0);
    assert.ok(body.rates.every((r) => r.country === "DE"));
  });

  it("answers an empty table when the database returns no rows", async () => {
    const body = await bodyOf<{ rates: unknown[] }>(await call(listRates as Handler, get("/x"), {}, sparse(/FROM vat_rates/)));
    assert.deepEqual(body.rates, []);
  });

  it("refuses a body, a rate or a date it cannot use", async () => {
    assert.equal((await call(setRate as Handler, noBody("/x"))).status, 400);
    const missing = await call(setRate as Handler, post("/x", { country: "DE" }));
    assert.equal((await bodyOf<{ error: string }>(missing)).error, "invalid_rate");
    const tooHigh = await call(setRate as Handler, post("/x", { country: "DE", rateBp: 10001 }));
    assert.equal((await bodyOf<{ error: string }>(tooHigh)).error, "invalid_rate");
    const badDate = await call(setRate as Handler, post("/x", { country: "DE", rateBp: 1900, validFrom: "soon" }));
    assert.equal((await bodyOf<{ error: string }>(badDate)).error, "invalid_date");
  });

  it("files a rate under 'standard' from today when kind and date are left out", async () => {
    const res = await call(setRate as Handler, post("/x", { country: "mt", rateBp: 1800 }));
    assert.equal(res.status, 201);
    const body = await bodyOf<{ kind: string; country: string; validFrom: string }>(res);
    assert.deepEqual([body.kind, body.country], ["standard", "MT"]);
    assert.ok(!Number.isNaN(Date.parse(body.validFrom)));
  });

  it("refuses a delete that does not name one rate, and one the database did not remove", async () => {
    assert.equal((await call(deleteRate as Handler, new Request(`${ORIGIN}/x?country=DE`, { method: "DELETE" }))).status, 422);
    const res = await call(
      deleteRate as Handler,
      new Request(`${ORIGIN}/x?country=DE&kind=standard&validFrom=2020-01-01`, { method: "DELETE" }),
      {},
      sparse(/^DELETE FROM vat_rates/),
    );
    assert.equal(res.status, 404);
  });
});

describe("removing a VAT rate", () => {
  it("removes the one named, and records it", async () => {
    await call(setRate as Handler, post("/x", { country: "MT", kind: "ebook", rateBp: 500, validFrom: "2026-01-01" }));
    const res = await call(
      deleteRate as Handler,
      new Request(`${ORIGIN}/x?country=mt&kind=ebook&validFrom=2026-01-01`, { method: "DELETE" }),
    );
    assert.equal(res.status, 200);
    const logged = await env.SHOP_DB.prepare(`SELECT subject FROM audit_log WHERE action = 'vat.delete'`).first<{
      subject: string;
    }>();
    assert.equal(logged?.subject, "MT/ebook");
  });
});

describe("the overview's warnings", () => {
  it("lists everything a bare deployment is missing", async () => {
    await env.SHOP_DB.prepare(`DELETE FROM settings`).run();
    invalidateSettings();
    const shop = {
      SHOP_DB: env.SHOP_DB,
      SHOP_GATEWAYS: "stripe",
      SHOP_TEST: "1",
    } as unknown as Shop;
    const body = await bodyOf<{ warnings: string[] }>(await call(me as Handler, get("/x"), {}, shop));
    const all = body.warnings.join("\n");
    for (const expected of [
      /Settings still missing/,
      /No payment provider/,
      /No file storage/,
      /No KV namespace/,
      /request budget/,
      /SHOP_IP_SALT/,
    ]) {
      assert.match(all, expected);
    }
  });

  it("lists nothing for an account that may not read the shop", async () => {
    const res = await call(me as Handler, get("/x"), {}, env, { ...admin, perms: [] });
    const body = await bodyOf<{ warnings: string[]; permissions: string[] }>(res);
    assert.deepEqual([body.warnings, body.permissions], [[], []]);
  });

  it("names the auth worker's endpoints in auth mode", async () => {
    const shop = {
      ...env,
      SHOP_AUTH_ISSUER: "https://example.com",
      SHOP_AUTH_AUDIENCE: "shop",
      SHOP_AUTH_PUBLIC_KEY: "{}",
      SHOP_AUTH_LOGIN_URL: "/login",
    } as Shop;
    const who = { mode: "auth", user: { id: "u1", email: "o@example.com" }, perms: ["ecommerce:read"], actor: "auth:u1" };
    const body = await bodyOf<{ loginUrl: string | null; logoutUrl: string | null; refreshUrl: string | null }>(
      await call(me as Handler, get("/x"), {}, shop, who),
    );
    assert.ok(body.loginUrl);
    assert.deepEqual([body.logoutUrl, body.refreshUrl], ["/api/auth/logout", "/api/auth/refresh"]);
  });

  it("warns about half-configured mail, stale VAT rates, and messages that keep failing", async () => {
    await env.SHOP_DB.prepare(`UPDATE vat_rates SET valid_from = '2000-01-01T00:00:00.000Z'`).run();
    await env.SHOP_DB.prepare(
      `INSERT INTO outbox (id, kind, target, payload_json, attempts, next_attempt, created_at)
       VALUES (?, 'email', 'a@example.com', '{}', 7, ?, ?)`,
    )
      .bind(newId(), nowISO(), nowISO())
      .run();
    const shop = { ...env, SHOP_MAIL_URL: "https://mail.example.com/send" } as Shop;
    const body = await bodyOf<{ warnings: string[] }>(await call(me as Handler, get("/x"), {}, shop));
    const all = body.warnings.join("\n");
    assert.match(all, /Email is not configured/);
    assert.match(all, /over a year ago/);
    assert.match(all, /1 queued message\(s\) have failed/);
  });

  it("is quiet about VAT and the queue when the database has nothing to say", async () => {
    const shop = {
      ...sparse(/MAX\(valid_from\)|FROM outbox/),
      SHOP_MAIL_URL: "https://mail.example.com/send",
      SHOP_MAIL_FROM: "shop@example.com",
    } as Shop;
    const body = await bodyOf<{ warnings: string[] }>(await call(me as Handler, get("/x"), {}, shop));
    assert.doesNotMatch(body.warnings.join("\n"), /Email|VAT rates|queued/);
  });
});

describe("settings the shop does not have", () => {
  it("accepts a value an enumerated setting allows, and records the change", async () => {
    const res = await call(putSettingsEndpoint as Handler, new Request(`${ORIGIN}/x`, {
      method: "PUT",
      body: JSON.stringify({ "pricing.mode": "net" }),
    }));
    assert.equal(res.status, 200);
    assert.equal((await bodyOf<{ settings: Record<string, unknown> }>(res)).settings["pricing.mode"], "net");
    const logged = await env.SHOP_DB.prepare(`SELECT actor FROM audit_log WHERE action = 'settings.update'`).first<{
      actor: string;
    }>();
    assert.equal(logged?.actor, "password");
  });

  it("refuses a value outside an enumerated setting", async () => {
    const res = await call(putSettingsEndpoint as Handler, new Request(`${ORIGIN}/x`, {
      method: "PUT",
      body: JSON.stringify({ "tax.rounding": "bankers" }),
    }));
    assert.match((await bodyOf<{ message: string }>(res)).message, /tax\.rounding must be one of/);
  });

  it("names the keys it does not know", async () => {
    const res = await call(putSettingsEndpoint as Handler, new Request(`${ORIGIN}/x`, {
      method: "PUT",
      body: JSON.stringify({ "shop.colour": "blue" }),
    }));
    assert.equal(res.status, 422);
    assert.equal((await bodyOf<{ error: string }>(res)).error, "unknown_setting");
  });
});
