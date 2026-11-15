// The panel's API. The first thing tested is that none of it answers to someone
// who has not signed in.

import { env } from "./_env";
import { AUTH_VARS, ask, shopEnv as gateEnv, throughGate } from "./admin-fixtures";
import type { Admin } from "../functions/api/shop/_auth";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { onRequest as adminGate } from "../functions/api/shop/admin/_middleware";
import { onRequestGet as me } from "../functions/api/shop/admin/me";
import { onRequestGet as stats } from "../functions/api/shop/admin/stats";
import { onRequestGet as listProducts, onRequestPost as createProduct } from "../functions/api/shop/admin/products/index";
import {
  onRequestDelete as deleteProduct,
  onRequestGet as getProduct,
  onRequestPatch as patchProduct,
} from "../functions/api/shop/admin/products/[id]";
import { onRequestPost as uploadFile } from "../functions/api/shop/admin/products/[id]/file";
import { onRequestGet as listOrders } from "../functions/api/shop/admin/orders/index";
import { onRequestGet as getOrder, onRequestPatch as noteOrder } from "../functions/api/shop/admin/orders/[id]";
import { onRequestPost as markPaid } from "../functions/api/shop/admin/orders/[id]/mark-paid";
import { onRequestPost as cancelOrder } from "../functions/api/shop/admin/orders/[id]/cancel";
import { onRequestPost as refundOrder } from "../functions/api/shop/admin/orders/[id]/refund";
import { onRequestPost as resendOrder } from "../functions/api/shop/admin/orders/[id]/resend";
import { onRequestGet as getSettings, onRequestPut as putSettingsEndpoint } from "../functions/api/shop/admin/settings";
import {
  onRequestDelete as deleteRate,
  onRequestGet as listRates,
  onRequestPost as setRate,
} from "../functions/api/shop/admin/vat-rates";
import { onRequestGet as listOutbox, onRequestPost as pushOutbox } from "../functions/api/shop/admin/outbox";
import { onRequestGet as auditLog } from "../functions/api/shop/admin/audit";
import { onRequestGet as exportCsv } from "../functions/api/shop/admin/export";
import { onRequestPost as checkout } from "../functions/api/shop/checkout";
import { fulfilOrder } from "../functions/api/shop/_fulfil";
import { enqueue } from "../functions/api/shop/_outbox";
import type { OrderRow, ProductRow } from "../functions/api/shop/_types";
import { assertPartial, bodyOf, ctx, freshShop, get, ORIGIN, post, seedProduct, owner } from "./_helpers";

const shopEnv = () => ({ ...env, STRIPE_SECRET_KEY: "sk_test_x", SHOP_GATEWAYS: "stripe" });
const realFetch = globalThis.fetch;

/** A context for a handler that sits behind the gate, with the identity the
 *  middleware would have put there. */
function asAdmin(request: Request, params: Record<string, string> = {}, who: Admin = owner) {
  return { ...ctx(request, params, { admin: who }), env: shopEnv() };
}

async function buyAndPay(sku = "EBOOK-1"): Promise<{ order: OrderRow; productId: string }> {
  const { id } = await seedProduct({ sku, priceMinor: 2400 });
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ id: "cs_test_1", url: "https://checkout.stripe.com/x" }))) as typeof fetch;

  const res = (await checkout({
    ...ctx(post("/api/shop/checkout", {
      items: [{ sku, quantity: 1 }], gateway: "stripe",
      email: "buyer@example.com", name: "A Buyer", country: "DE", consentWaiver: "1",
    })),
    env: shopEnv(),
  } as never)) as Response;
  const { orderId } = await bodyOf<{ orderId: string }>(res);
  const order = (await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(orderId).first<OrderRow>())!;
  return { order, productId: id };
}

beforeEach(async () => {
  await freshShop();
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("the gate", () => {
  it("refuses everything behind it without a token", async () => {
    const res = (await adminGate({
      ...ctx(get("/api/shop/admin/products")),
      env: shopEnv(),
      functionPath: "/api/shop/admin/products",
    } as never)) as Response;
    assert.equal(res.status, 401);
  });

  it("says what is wrong when the database is not bound", async () => {
    const res = (await adminGate({
      ...ctx(get("/api/shop/admin/me")),
      env: { ...env, SHOP_DB: undefined },
      functionPath: "/api/shop/admin/me",
    } as never)) as Response;
    assert.equal(res.status, 503);
  });
});

describe("the overview", () => {
  it("tells the owner what is still missing", async () => {
    const res = (await me(asAdmin(get("/api/shop/admin/me")) as never)) as Response;
    const body = await bodyOf<{ warnings: string[]; gateways: string[]; testMode: boolean }>(res);
    assert.deepEqual(body.gateways, ["stripe"]);
    assert.equal(body.testMode, true);
    assert.match(body.warnings.join(" "), /Email is not configured/);
  });

  it("counts only settled money", async () => {
    const { order } = await buyAndPay();
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });

    const res = (await stats(asAdmin(get("/api/shop/admin/stats?window=30d")) as never)) as Response;
    const body = await bodyOf<{ byCurrency: Array<{ currency: string; grossMinor: number }>; topProducts: unknown[] }>(res);
    assertPartial(body.byCurrency[0], { currency: "EUR", grossMinor: 2400 });
    assert.equal((body.topProducts).length, 1);
  });

  it("falls back to thirty days for a window nobody offers", async () => {
    const res = (await stats(asAdmin(get("/api/shop/admin/stats?window=forever")) as never)) as Response;
    assertPartial(await bodyOf<{ window: string }>(res), { window: "30d" });
  });
});

describe("products", () => {
  it("creates, prices, files and activates one", async () => {
    const created = (await createProduct(
      asAdmin(post("/api/shop/admin/products", { sku: "EBOOK-9", name: "New Book" })) as never,
    )) as Response;
    assert.equal(created.status, 201);
    const { product } = await bodyOf<{ product: ProductRow }>(created);
    assert.equal(product.status, "draft");

    // A product with no file cannot go on sale.
    const tooSoon = (await patchProduct(
      asAdmin(post(`/api/shop/admin/products/${product.id}`, { status: "active" }), { id: product.id }) as never,
    )) as Response;
    assertPartial(await bodyOf<{ error: string }>(tooSoon), { error: "no_file" });

    const form = new FormData();
    form.append("file", new File([new TextEncoder().encode("%PDF-1.4\nbook\n")], "book.pdf", { type: "application/pdf" }));
    const upload = (await uploadFile({
      ...ctx(new Request(`${ORIGIN}/api/shop/admin/products/${product.id}/file`, { method: "POST", body: form }), { id: product.id }, { admin: owner }),
      env: shopEnv(),
    } as never)) as Response;
    assertPartial(await bodyOf<{ contentType: string }>(upload), { contentType: "application/pdf" });

    const activated = (await patchProduct(
      asAdmin(post(`/api/shop/admin/products/${product.id}`, { status: "active", prices: { EUR: 1900 } }), { id: product.id }) as never,
    )) as Response;
    assertPartial(await bodyOf<{ product: ProductRow }>(activated), { product: { status: "active" } });

    const listed = (await listProducts(asAdmin(get("/api/shop/admin/products")) as never)) as Response;
    const body = await bodyOf<{ products: Array<{ prices: Record<string, number> }> }>(listed);
    assert.deepEqual(body.products[0]?.prices, { EUR: 1900 });
  });

  it("refuses a file that is not a book", async () => {
    const { id } = await seedProduct({ sku: "EBOOK-1", withFile: false });
    const form = new FormData();
    form.append("file", new File([new TextEncoder().encode("MZ ")], "book.exe", { type: "application/pdf" }));
    const res = (await uploadFile({
      ...ctx(new Request(`${ORIGIN}/x`, { method: "POST", body: form }), { id }, { admin: owner }),
      env: shopEnv(),
    } as never)) as Response;
    // The extension and the declared type both said PDF; the bytes did not.
    assertPartial(await bodyOf<{ error: string }>(res), { error: "unsupported_file" });
  });

  it("refuses a file larger than the cap", async () => {
    const { id } = await seedProduct({ sku: "EBOOK-1", withFile: false });
    const form = new FormData();
    form.append("file", new File([new Uint8Array(2 * 1024 * 1024)], "book.pdf"));
    const res = (await uploadFile({
      ...ctx(new Request(`${ORIGIN}/x`, { method: "POST", body: form }), { id }, { admin: owner }),
      env: { ...shopEnv(), SHOP_MAX_FILE_MB: "1" },
    } as never)) as Response;
    assert.equal(res.status, 413);
  });

  it("refuses a duplicate code, a bad code and a missing name", async () => {
    await seedProduct({ sku: "TAKEN" });
    for (const [body, status] of [
      [{ sku: "TAKEN", name: "Another" }, 409],
      [{ sku: "not a sku!", name: "X" }, 422],
      [{ sku: "FINE", name: "" }, 422],
    ] as Array<[Record<string, unknown>, number]>) {
      const res = (await createProduct(asAdmin(post("/api/shop/admin/products", body)) as never)) as Response;
      assert.equal(res.status, status);
    }
  });

  it("archives a product someone has bought, and deletes one nobody has", async () => {
    const { order, productId } = await buyAndPay();
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });

    const archived = (await deleteProduct(
      asAdmin(new Request(`${ORIGIN}/x`, { method: "DELETE" }), { id: productId }) as never,
    )) as Response;
    assert.deepEqual(await bodyOf(archived), { archived: true, deleted: false });

    const { id: unsold } = await seedProduct({ sku: "UNSOLD" });
    const deleted = (await deleteProduct(
      asAdmin(new Request(`${ORIGIN}/x`, { method: "DELETE" }), { id: unsold }) as never,
    )) as Response;
    assert.deepEqual(await bodyOf(deleted), { archived: false, deleted: true });
  });

  // Was: "keeps staff out of the catalogue" (the removed staff role). Now the
  // gate refuses an account without ecommerce:products, before the handler runs.
  it("keeps an account without ecommerce:products out of the catalogue", async () => {
    const request = await ask("/api/shop/admin/products", { method: "POST", perms: ["ecommerce:read"], panel: true, body: "{}" });
    const { res } = await throughGate(request, gateEnv(AUTH_VARS));
    assert.equal(res.status, 403);
    assert.equal((await bodyOf<{ permission: string }>(res)).permission, "ecommerce:products");
  });

  it("404s for a product that is not there", async () => {
    const res = (await getProduct(asAdmin(get("/api/shop/admin/products/nope"), { id: "nope" }) as never)) as Response;
    assert.equal(res.status, 404);
  });

  it("refuses a price in a currency that is not one", async () => {
    const { id } = await seedProduct({ sku: "EBOOK-1" });
    const res = (await patchProduct(
      asAdmin(post(`/x`, { prices: { EURO: 100 } }), { id }) as never,
    )) as Response;
    assertPartial(await bodyOf<{ error: string }>(res), { error: "invalid_currency" });
  });
});

describe("orders", () => {
  it("lists, filters and opens one", async () => {
    const { order } = await buyAndPay();

    const all = (await listOrders(asAdmin(get("/api/shop/admin/orders")) as never)) as Response;
    assert.equal(((await bodyOf<{ orders: unknown[] }>(all)).orders).length, 1);

    const byEmail = (await listOrders(asAdmin(get("/api/shop/admin/orders?q=buyer@example.com")) as never)) as Response;
    assert.equal(((await bodyOf<{ orders: unknown[] }>(byEmail)).orders).length, 1);

    const byStatus = (await listOrders(asAdmin(get("/api/shop/admin/orders?status=refunded")) as never)) as Response;
    assert.equal(((await bodyOf<{ orders: unknown[] }>(byStatus)).orders).length, 0);

    const bogus = (await listOrders(asAdmin(get("/api/shop/admin/orders?status=elsewhere")) as never)) as Response;
    assert.equal(bogus.status, 422);

    const detail = (await getOrder(asAdmin(get(`/x`), { id: order.id }) as never)) as Response;
    const body = await bodyOf<{ order: Record<string, unknown>; customer: Record<string, unknown> }>(detail);
    // The order key never leaves the database, not even to the owner.
    assert.equal(body.order.key_hash, undefined);
    assert.equal(body.customer.email, "buyer@example.com");
  });

  it("marks an order paid by hand, once", async () => {
    const { order } = await buyAndPay();
    const res = (await markPaid(
      asAdmin(post("/x", { reference: "transfer-1" }), { id: order.id }) as never,
    )) as Response;
    const body = await bodyOf<{ changed: boolean; invoiceNumber: string; linksIssued: number }>(res);
    assertPartial(body, { changed: true, linksIssued: 1 });
    assert.ok((body.invoiceNumber).includes("FV"));

    const again = (await markPaid(asAdmin(post("/x", {}), { id: order.id }) as never)) as Response;
    assert.equal(again.status, 409);
  });

  it("sends the links again on request", async () => {
    const { order } = await buyAndPay();
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    await env.SHOP_DB.prepare(`DELETE FROM outbox`).run();

    const res = (await resendOrder(asAdmin(post("/x", {}), { id: order.id }) as never)) as Response;
    assertPartial(await bodyOf<{ sentTo: string; links: number }>(res), {
      sentTo: "buyer@example.com",
      links: 1,
    });
  });

  it("will not resend for an order nobody paid for", async () => {
    const { order } = await buyAndPay();
    const res = (await resendOrder(asAdmin(post("/x", {}), { id: order.id }) as never)) as Response;
    assert.equal(res.status, 409);
  });

  it("cancels a pending order and refuses to cancel a paid one", async () => {
    const { order } = await buyAndPay();
    const cancelled = (await cancelOrder(asAdmin(post("/x", { reason: "abandoned" }), { id: order.id }) as never)) as Response;
    assert.equal(cancelled.status, 200);

    const again = (await cancelOrder(asAdmin(post("/x", {}), { id: order.id }) as never)) as Response;
    assert.equal(again.status, 409);
  });

  it("keeps a private note", async () => {
    const { order } = await buyAndPay();
    const res = (await noteOrder(asAdmin(post("/x", { notes: "Wrote in about VAT." }), { id: order.id }) as never)) as Response;
    assertPartial(await bodyOf<{ notes: string }>(res), { notes: "Wrote in about VAT." });

    const bad = (await noteOrder(asAdmin(post("/x", { notes: 42 }), { id: order.id }) as never)) as Response;
    assert.equal(bad.status, 400);
  });

  it("refuses a refund larger than what is left", async () => {
    const { order } = await buyAndPay();
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    await env.SHOP_DB.prepare(
      `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, created_at)
       VALUES ('p1', ?, 'stripe', 'pi_1', 'charge', 'succeeded', 2400, 'EUR', ?)`,
    )
      .bind(order.id, new Date().toISOString())
      .run();

    const res = (await refundOrder(asAdmin(post("/x", { amountMinor: 9999 }), { id: order.id }) as never)) as Response;
    assert.equal(res.status, 422);
  });

  it("asks the provider for a refund and leaves the order to the webhook", async () => {
    const { order } = await buyAndPay();
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    await env.SHOP_DB.prepare(
      `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, created_at)
       VALUES ('p1', ?, 'stripe', 'pi_1', 'charge', 'succeeded', 2400, 'EUR', ?)`,
    )
      .bind(order.id, new Date().toISOString())
      .run();
    globalThis.fetch = (async () => new Response(JSON.stringify({ id: "re_1" }))) as typeof fetch;

    const res = (await refundOrder(asAdmin(post("/x", { amountMinor: 2400 }), { id: order.id }) as never)) as Response;
    assertPartial(await bodyOf<{ refundRef: string }>(res), { refundRef: "re_1" });

    // The order is still paid: the provider's webhook is what changes it, so a
    // refund started here and one started in Stripe's dashboard take one path.
    const after = await env.SHOP_DB.prepare(`SELECT status FROM orders WHERE id = ?`)
      .bind(order.id)
      .first<{ status: string }>();
    assert.equal(after?.status, "paid");
  });

  it("reports a provider that refuses without repeating what it said", async () => {
    const { order } = await buyAndPay();
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    await env.SHOP_DB.prepare(
      `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, created_at)
       VALUES ('p1', ?, 'stripe', 'pi_1', 'charge', 'succeeded', 2400, 'EUR', ?)`,
    )
      .bind(order.id, new Date().toISOString())
      .run();
    globalThis.fetch = (async () =>
      new Response(JSON.stringify({ error: { message: "Charge ch_123 has already been refunded" } }), {
        status: 400,
      })) as typeof fetch;

    const res = (await refundOrder(asAdmin(post("/x", {}), { id: order.id }) as never)) as Response;
    assert.equal(res.status, 502);
    assert.ok(!(await res.text()).includes("ch_123"));

    const logged = await env.SHOP_DB.prepare(
      `SELECT detail_json FROM audit_log WHERE action = 'order.refund_failed'`,
    ).first<{ detail_json: string }>();
    assert.ok((logged?.detail_json).includes("already been refunded"));
  });

  it("will not refund an order that was never paid", async () => {
    const { order } = await buyAndPay();
    const res = (await refundOrder(asAdmin(post("/x", {}), { id: order.id }) as never)) as Response;
    assert.equal(res.status, 409);
  });
});

describe("settings and rates", () => {
  it("reads and writes, and refuses a key nobody declared", async () => {
    const read = (await getSettings(asAdmin(get("/api/shop/admin/settings")) as never)) as Response;
    assert.ok(((await bodyOf<{ keys: string[] }>(read)).keys).includes("seller.name"));

    const ok = (await putSettingsEndpoint(
      asAdmin(new Request(`${ORIGIN}/x`, { method: "PUT", headers: { "content-type": "application/json" }, body: JSON.stringify({ "shop.name": "Renamed" }) })) as never,
    )) as Response;
    assert.equal(ok.status, 200);

    const unknown = (await putSettingsEndpoint(
      asAdmin(new Request(`${ORIGIN}/x`, { method: "PUT", headers: { "content-type": "application/json" }, body: JSON.stringify({ "shop.nmae": "typo" }) })) as never,
    )) as Response;
    assertPartial(await bodyOf<{ error: string }>(unknown), { error: "unknown_setting" });
  });

  it("refuses settings that would produce wrong invoices", async () => {
    const cases = [
      { "pricing.mode": "whatever" },
      { "currency.base": "EURO" },
      { "seller.country": "Germany" },
      { "invoice.format": "no placeholder" },
      { "shop.countries_allowed": "PL" },
    ];
    for (const patch of cases) {
      const res = (await putSettingsEndpoint(
        asAdmin(new Request(`${ORIGIN}/x`, { method: "PUT", headers: { "content-type": "application/json" }, body: JSON.stringify(patch) })) as never,
      )) as Response;
      assert.equal(res.status, 422, JSON.stringify(patch));
    }
  });

  // Was: "keeps staff out of the settings" (the removed staff role).
  it("keeps an account without ecommerce:settings out of the settings", async () => {
    const request = await ask("/api/shop/admin/settings", { method: "PUT", perms: ["ecommerce:read", "ecommerce:products"], panel: true, body: "{}" });
    const { res } = await throughGate(request, gateEnv(AUTH_VARS));
    assert.equal(res.status, 403);
    assert.equal((await bodyOf<{ permission: string }>(res)).permission, "ecommerce:settings");
  });

  it("supersedes a rate rather than overwriting it", async () => {
    const res = (await setRate(
      asAdmin(post("/x", { country: "PL", kind: "ebook", rateBp: 800, validFrom: "2027-01-01T00:00:00.000Z" })) as never,
    )) as Response;
    assert.equal(res.status, 201);

    const listed = (await listRates(asAdmin(get("/api/shop/admin/vat-rates?country=PL")) as never)) as Response;
    const rates = (await bodyOf<{ rates: Array<{ kind: string; valid_to: string | null; rate_bp: number }> }>(listed)).rates;
    const ebooks = rates.filter((r) => r.kind === "ebook");
    assert.equal((ebooks).length, 2);
    // The old rate is closed off rather than deleted, so an old invoice can
    // still explain itself.
    assert.equal(ebooks.some((r) => r.valid_to !== null), true);
  });

  it("refuses a rate that is not a rate", async () => {
    for (const body of [
      { country: "Poland", kind: "ebook", rateBp: 500 },
      { country: "PL", kind: "ebook", rateBp: 50000 },
      { country: "PL", kind: "ebook", rateBp: 500, validFrom: "not a date" },
    ]) {
      const res = (await setRate(asAdmin(post("/x", body)) as never)) as Response;
      assert.equal(res.status, 422, JSON.stringify(body));
    }
  });

  it("removes a rate, and says so when there is none to remove", async () => {
    const gone = (await deleteRate(
      asAdmin(new Request(`${ORIGIN}/x?country=PL&kind=ebook&validFrom=2026-01-01T00:00:00.000Z`, { method: "DELETE" })) as never,
    )) as Response;
    assert.equal(gone.status, 200);

    const again = (await deleteRate(
      asAdmin(new Request(`${ORIGIN}/x?country=PL&kind=ebook&validFrom=2026-01-01T00:00:00.000Z`, { method: "DELETE" })) as never,
    )) as Response;
    assert.equal(again.status, 404);

    const incomplete = (await deleteRate(
      asAdmin(new Request(`${ORIGIN}/x?country=PL`, { method: "DELETE" })) as never,
    )) as Response;
    assert.equal(incomplete.status, 422);
  });
});

describe("the log", () => {
  it("lists the queue without showing what is in the messages", async () => {
    await enqueue(env, "email", "buyer@example.com", { subject: "Your book", downloadUrl: "https://secret" });
    const res = (await listOutbox(asAdmin(get("/api/shop/admin/outbox")) as never)) as Response;
    const text = await res.text();
    assert.ok((text).includes("buyer@example.com"));
    // The payload holds a download link. It stays in the database.
    assert.ok(!(text).includes("https://secret"));
  });

  it("pushes the queue and retries one message", async () => {
    const id = await enqueue(env, "email", "buyer@example.com", {});
    const drained = (await pushOutbox(asAdmin(post("/x", { action: "drain" })) as never)) as Response;
    assertPartial(await bodyOf<{ sent: number }>(drained), { sent: 0 });

    const retried = (await pushOutbox(asAdmin(post("/x", { id })) as never)) as Response;
    assert.equal(retried.status, 200);

    const missing = (await pushOutbox(asAdmin(post("/x", { id: "no-such-id" })) as never)) as Response;
    assert.equal(missing.status, 404);
  });

  it("shows who did what", async () => {
    const { order } = await buyAndPay();
    await markPaid(asAdmin(post("/x", {}), { id: order.id }) as never);

    const res = (await auditLog(asAdmin(get("/api/shop/admin/audit?limit=10")) as never)) as Response;
    const entries = (await bodyOf<{ entries: Array<{ action: string; actor: string }> }>(res)).entries;
    assert.equal(entries.some((e) => e.action === "order.mark_paid" && e.actor === owner.actor), true);
  });
});

describe("the export", () => {
  it("writes a CSV the accountant can open safely", async () => {
    const { order } = await buyAndPay();
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    await env.SHOP_DB.prepare(`UPDATE customers SET name = ? WHERE email_lc = 'buyer@example.com'`)
      .bind("=cmd|'/c calc'!A1")
      .run();

    const res = (await exportCsv(asAdmin(get("/api/shop/admin/export?type=orders")) as never)) as Response;
    assert.ok((res.headers.get("content-type")).includes("text/csv"));
    assert.ok((res.headers.get("content-disposition")).includes("attachment"));

    // Read as bytes: decoding to text strips the byte-order mark, which is the
    // one byte Excel actually needs.
    const bytes = new Uint8Array(await res.clone().arrayBuffer());
    assert.deepEqual([...bytes.slice(0, 3)], [0xef, 0xbb, 0xbf]);

    const text = await res.text();
    assert.ok((text).includes("24.00")); // a decimal amount, not 2400
    assert.ok((text).includes(order.number));
  });

  it("exports invoices and the VAT summary too", async () => {
    const { order } = await buyAndPay();
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });

    for (const type of ["invoices", "vat"]) {
      const res = (await exportCsv(asAdmin(get(`/api/shop/admin/export?type=${type}`)) as never)) as Response;
      assert.equal(res.status, 200);
      assert.ok(((await res.text()).split("\r\n").length) > 1);
    }
  });

  it("refuses a type and a date range it does not understand", async () => {
    const badType = (await exportCsv(asAdmin(get("/api/shop/admin/export?type=everything")) as never)) as Response;
    assert.equal(badType.status, 422);

    const badRange = (await exportCsv(asAdmin(get("/api/shop/admin/export?type=orders&from=yesterday")) as never)) as Response;
    assert.equal(badRange.status, 422);
  });
});
