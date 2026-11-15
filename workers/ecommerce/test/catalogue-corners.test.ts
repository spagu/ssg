// The catalogue, checkout and settings in the states the main suites do not
// reach: half-filled forms, unusual files, a database answering sparsely, and
// the defaults a shop falls back on when a setting is blank.

import { env } from "./_env";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  onRequestDelete as deleteProduct,
  onRequestGet as getProduct,
  onRequestPatch as patchProduct,
} from "../functions/api/shop/admin/products/[id]";
import { onRequestPost as uploadFile } from "../functions/api/shop/admin/products/[id]/file";
import { onRequestGet as adminProducts, onRequestPost as createProduct } from "../functions/api/shop/admin/products/index";
import { onRequestGet as oneProduct } from "../functions/api/shop/products/[sku]";
import { onRequestGet as listProducts } from "../functions/api/shop/products/index";
import { onRequestPost as checkout } from "../functions/api/shop/checkout";
import { onRequestGet as orderStatus } from "../functions/api/shop/orders/[id]/status";
import { createOrder, getOrderItems, priceBasket, upsertCustomer, type PricedBasket } from "../functions/api/shop/_orders";
import {
  allSettings,
  invalidateSettings,
  moduleStates,
  putSettings,
  sellerSnapshot,
  shopIsConfigured,
} from "../functions/api/shop/_settings";
import { ensureSchema, resetSchemaCache } from "../functions/api/shop/_schema";
import { taxContext } from "../functions/api/shop/_tax";
import { resetAccessCache, verifyAccessJwt } from "../functions/api/shop/_access";
import { base64url } from "../functions/api/shop/_lib";
import type { OrderRow, ProductRow } from "../functions/api/shop/_types";
import { fakeD1, sparseD1 } from "./fakes";
import { bodyOf, ctx, freshShop, get, ORIGIN, post, seedProduct, owner } from "./_helpers";

// The identity the admin gate hands a handler (_helpers.ts, `owner`).
const admin = owner;
const shopEnv = () => ({ ...env, STRIPE_SECRET_KEY: "sk_test_x", SHOP_GATEWAYS: "stripe" });
const realFetch = globalThis.fetch;

function asAdmin(request: Request, params: Record<string, string> = {}, over: Record<string, unknown> = {}) {
  return { ...ctx(request, params, { admin }), env: { ...shopEnv(), ...over } } as never;
}
function patch(path: string, body: unknown): Request {
  return new Request(`${ORIGIN}${path}`, {
    method: "PATCH",
    headers: { "content-type": "application/json", origin: ORIGIN },
    body: typeof body === "string" ? body : JSON.stringify(body),
  });
}
function upload(bytes: Uint8Array, name = "book.bin"): Request {
  const form = new FormData();
  form.append("file", new File([bytes], name));
  return new Request(`${ORIGIN}/x`, { method: "POST", body: form });
}
const productRow = async (id: string) =>
  env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`).bind(id).first<ProductRow>();

/** A database that answers the way the D1 types allow but the engine rarely
 *  does: `all()` without a results array, and a COUNT(*) with no row. The shop
 *  must read both as "nothing" rather than throw. */
function sparse(db: typeof env.SHOP_DB): typeof env.SHOP_DB {
  return sparseD1(db, /[^]/, { first: /COUNT\(\*\)/, run: false }) as typeof env.SHOP_DB;
}

beforeEach(async () => {
  await freshShop();
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ id: "cs_test_9", url: "https://checkout.stripe.com/pay/cs_test_9" }))) as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = realFetch;
  invalidateSettings();
});

describe("editing one product", () => {
  it("404s for a product that is not there, with or without an id", async () => {
    for (const params of [{}, { id: "nope" }]) {
      assert.equal(((await getProduct(asAdmin(get("/x"), params))) as Response).status, 404);
      assert.equal(((await patchProduct(asAdmin(patch("/x", {}), params))) as Response).status, 404);
      assert.equal(((await deleteProduct(asAdmin(get("/x"), params))) as Response).status, 404);
    }
  });

  it("shows its prices and how often it sold", async () => {
    const { id } = await seedProduct({ sku: "P-1", priceMinor: 1500 });
    const body = await bodyOf<{ product: { prices: Record<string, number> }; soldCount: number }>(
      (await getProduct(asAdmin(get("/x"), { id }))) as Response,
    );
    assert.deepEqual(body.product.prices, { EUR: 1500 });
    assert.equal(body.soldCount, 0);
  });

  it("refuses a body it cannot read", async () => {
    const { id } = await seedProduct();
    assert.equal(((await patchProduct(asAdmin(patch("/x", "{nope"), { id }))) as Response).status, 400);
  });

  it("will not put a product without a file on sale", async () => {
    const { id } = await seedProduct({ withFile: false });
    const res = (await patchProduct(asAdmin(patch("/x", { status: "active" }), { id }))) as Response;
    assert.equal(res.status, 422);
    assert.equal((await bodyOf(res)).error, "no_file");
  });

  it("archives, drafts anything else, and keeps the status when none is sent", async () => {
    const { id } = await seedProduct();
    await patchProduct(asAdmin(patch("/x", { status: "archived" }), { id }));
    assert.equal((await productRow(id))?.status, "archived");
    await patchProduct(asAdmin(patch("/x", { status: "whatever" }), { id }));
    assert.equal((await productRow(id))?.status, "draft");
    await patchProduct(asAdmin(patch("/x", { status: "active" }), { id }));
    await patchProduct(asAdmin(patch("/x", { name: "Renamed" }), { id }));
    assertPartialRow(await productRow(id), { status: "active", name: "Renamed" });
  });

  it("sets every field, and falls back where a field is sent blank", async () => {
    const { id } = await seedProduct({ name: "Keep" });
    await patchProduct(asAdmin(patch("/x", {
      description: "A long story.", taxCategory: "audiobook", downloadLimit: 7, downloadDays: 14, imageUrl: "/i.png",
    }), { id }));
    assertPartialRow(await productRow(id), {
      description: "A long story.", tax_category: "audiobook", download_limit: 7, download_days: 14, image_url: "/i.png",
    });

    await patchProduct(asAdmin(patch("/x", { name: "", description: "", taxCategory: "", imageUrl: "" }), { id }));
    assertPartialRow(await productRow(id), { name: "Keep", description: null, tax_category: "ebook", image_url: null });
  });

  it("sets, replaces and removes prices, and refuses nonsense", async () => {
    const { id } = await seedProduct({ priceMinor: 2400 });
    const prices = async () =>
      (await bodyOf<{ product: { prices: Record<string, number> } }>((await getProduct(asAdmin(get("/x"), { id }))) as Response))
        .product.prices;

    await patchProduct(asAdmin(patch("/x", { prices: { usd: 2900, EUR: 2500 } }), { id }));
    assert.deepEqual(await prices(), { EUR: 2500, USD: 2900 });
    await patchProduct(asAdmin(patch("/x", { prices: { USD: null } }), { id }));
    assert.deepEqual(await prices(), { EUR: 2500 });

    const badCurrency = (await patchProduct(asAdmin(patch("/x", { prices: { E1: 100 } }), { id }))) as Response;
    assert.equal((await bodyOf(badCurrency)).error, "invalid_currency");
    for (const amount of [-1, "lots"]) {
      const res = (await patchProduct(asAdmin(patch("/x", { prices: { EUR: amount } }), { id }))) as Response;
      assert.equal((await bodyOf(res)).error, "invalid_amount");
    }
  });

  it("deletes an unsold product and its file, with or without storage bound", async () => {
    const withFile = await seedProduct({ sku: "F-1" });
    const key = (await productRow(withFile.id))!.file_key!;
    const res = (await deleteProduct(asAdmin(get("/x"), { id: withFile.id }))) as Response;
    assert.deepEqual(await bodyOf(res), { archived: false, deleted: true });
    assert.equal(await env.SHOP_FILES.head(key), null);

    const noStorage = await seedProduct({ sku: "F-2" });
    await deleteProduct(asAdmin(get("/x"), { id: noStorage.id }, { SHOP_FILES: undefined }));
    assert.equal(await productRow(noStorage.id), null);

    const noFile = await seedProduct({ sku: "F-3", withFile: false });
    await deleteProduct(asAdmin(get("/x"), { id: noFile.id }));
    assert.equal(await productRow(noFile.id), null);
  });
});

function assertPartialRow(row: unknown, expected: Record<string, unknown>): void {
  for (const [k, v] of Object.entries(expected)) assert.equal((row as Record<string, unknown>)[k], v, k);
}

describe("uploading a product file", () => {
  const zipWith = (text: string): Uint8Array => {
    const bytes = new Uint8Array(120);
    bytes.set([0x50, 0x4b, 0x03, 0x04]);
    bytes.set(new TextEncoder().encode(text), 30);
    return bytes;
  };

  it("404s without a product id", async () => {
    const res = (await uploadFile(asAdmin(upload(new TextEncoder().encode("%PDF-1.4")), {}))) as Response;
    assert.equal(res.status, 404);
  });

  it("refuses a file over the configured limit", async () => {
    const { id } = await seedProduct();
    const big = new Uint8Array(1024 * 1024 + 1);
    const res = (await uploadFile(asAdmin(upload(big), { id }, { SHOP_MAX_FILE_MB: "1" }))) as Response;
    assert.equal(res.status, 413);
    assert.match((await bodyOf<{ message: string }>(res)).message, /1 MB/);
  });

  it("knows each ebook format by its first bytes, and refuses anything else", async () => {
    const cases: Array<[Uint8Array, string | null]> = [
      [zipWith("mimetypeapplication/epub+zip"), "application/epub+zip"],
      [zipWith("just a zip"), "application/zip"],
      [new TextEncoder().encode("BOOKMOBI and the rest"), "application/x-mobipocket-ebook"],
      [new TextEncoder().encode("TPZ3 and the rest"), "application/vnd.amazon.ebook"],
      [new TextEncoder().encode("plain text"), null],
    ];
    for (const [bytes, type] of cases) {
      const { id } = await seedProduct({ sku: `S-${Math.random().toString(36).slice(2, 8)}`, withFile: false });
      const res = (await uploadFile(asAdmin(upload(bytes), { id }))) as Response;
      if (type === null) assert.equal(res.status, 422);
      else assert.equal((await bodyOf<{ contentType: string }>(res)).contentType, type);
    }
  });

  it("names a file sent without a name after the product", async () => {
    // A browser sends filename="" for a file picked from somewhere without one.
    const { id } = await seedProduct({ sku: "NAMELESS", withFile: false });
    const b = "boundary-1";
    const request = new Request(`${ORIGIN}/x`, {
      method: "POST",
      headers: { "content-type": `multipart/form-data; boundary=${b}` },
      body: `--${b}\r\nContent-Disposition: form-data; name="file"; filename=""\r\n\r\n%PDF-1.4\r\n--${b}--\r\n`,
    });
    const res = (await uploadFile(asAdmin(request, { id }))) as Response;
    assert.equal((await bodyOf<{ fileName: string }>(res)).fileName, "NAMELESS.bin");
  });

  it("removes the replaced file when the database cannot say whether it is in use", async () => {
    const { id } = await seedProduct({ sku: "R-0" });
    const old = (await productRow(id))!.file_key!;
    await uploadFile({
      ...ctx(upload(new TextEncoder().encode("%PDF-1.4\nnew"), "n.pdf"), { id }, { admin }),
      env: { ...shopEnv(), SHOP_DB: sparse(env.SHOP_DB) },
    } as never);
    assert.equal(await env.SHOP_FILES.head(old), null);
  });

  it("removes the replaced file only when no live link still points at it", async () => {
    const { id } = await seedProduct({ sku: "R-1" });
    const first = (await productRow(id))!.file_key!;
    await uploadFile(asAdmin(upload(new TextEncoder().encode("%PDF-1.4\nfirst"), "a.pdf"), { id }));
    assert.equal(await env.SHOP_FILES.head(first), null);

    // The same bytes again: same key, nothing to remove.
    const second = (await productRow(id))!.file_key!;
    await uploadFile(asAdmin(upload(new TextEncoder().encode("%PDF-1.4\nfirst"), "a.pdf"), { id }));
    assert.notEqual(await env.SHOP_FILES.head(second), null);

    // A sold copy with a live token keeps the old file.
    const orderId = crypto.randomUUID();
    const itemId = crypto.randomUUID();
    await env.SHOP_DB.prepare(`INSERT INTO customers (id, email, email_lc, country, created_at) VALUES ('c1', 'b@example.com', 'b@example.com', 'DE', '2026-01-01')`).run();
    await env.SHOP_DB.prepare(
      `INSERT INTO orders (id, number, key_hash, status, customer_id, currency, subtotal_minor, tax_minor, total_minor,
                           reverse_charge, gateway, consent_marketing, consent_waiver, created_at, updated_at)
       VALUES (?, 'O-1', 'h', 'paid', 'c1', 'EUR', 1, 0, 1, 0, 'stripe', 0, 1, '2026-01-01', '2026-01-01')`,
    ).bind(orderId).run();
    await env.SHOP_DB.prepare(
      `INSERT INTO order_items (id, order_id, product_id, sku, name, quantity, unit_minor, tax_rate_bp, tax_minor, total_minor)
       VALUES (?, ?, ?, 'R-1', 'A Book', 1, 1, 0, 0, 1)`,
    ).bind(itemId, orderId, id).run();
    await env.SHOP_DB.prepare(
      `INSERT INTO download_tokens (token_hash, order_item_id, expires_at, max_uses, uses, revoked, created_at)
       VALUES ('t', ?, '2099-01-01', 3, 0, 0, '2026-01-01')`,
    ).bind(itemId).run();
    await uploadFile(asAdmin(upload(new TextEncoder().encode("%PDF-1.4\nthird"), "a.pdf"), { id }));
    assert.notEqual(await env.SHOP_FILES.head(second), null);
  });
});

describe("the panel's product list", () => {
  it("pages by every sort it offers, both ways", async () => {
    await seedProduct({ sku: "A-1", name: "Alpha" });
    await seedProduct({ sku: "B-1", name: "Beta" });
    for (const sort of ["name", "sku", "status", "created", "updated"]) {
      for (const dir of ["asc", "desc"]) {
        const first = await bodyOf<{ products: ProductRow[]; nextCursor: string }>(
          (await adminProducts(asAdmin(get(`/x?sort=${sort}&dir=${dir}&limit=1`)))) as Response,
        );
        assert.ok(first.nextCursor, `${sort} ${dir}`);
        const second = await bodyOf<{ products: ProductRow[]; nextCursor: string | null }>(
          (await adminProducts(asAdmin(get(`/x?sort=${sort}&dir=${dir}&limit=1&cursor=${encodeURIComponent(first.nextCursor)}`)))) as Response,
        );
        assert.equal(second.products.length, 1);
        assert.notEqual(second.products[0]!.id, first.products[0]!.id);
        assert.equal(second.nextCursor, null);
      }
    }
  });

  it("reads a limit it cannot parse as the default, and a cursor without an id as a value", async () => {
    await seedProduct({ sku: "A-1", name: "Alpha" });
    const body = await bodyOf<{ products: ProductRow[] }>(
      (await adminProducts(asAdmin(get("/x?limit=lots&sort=name&dir=asc&cursor=Aaa")))) as Response,
    );
    assert.equal(body.products.length, 1);
  });

  it("shows a product without any price as having none", async () => {
    const { id } = await seedProduct({ sku: "NO-PRICE" });
    await env.SHOP_DB.prepare(`DELETE FROM prices WHERE product_id = ?`).bind(id).run();
    const body = await bodyOf<{ products: Array<{ prices: Record<string, number> }> }>(
      (await adminProducts(asAdmin(get("/x")))) as Response,
    );
    assert.deepEqual(body.products[0]!.prices, {});
  });

  it("creates a product on sale straight away, and clamps what it cannot read", async () => {
    const res = (await createProduct(asAdmin(post("/x", { sku: "NEW-1", name: "New", status: "active", downloadLimit: "lots" })))) as Response;
    assert.equal(res.status, 201);
    const { product } = await bodyOf<{ product: ProductRow }>(res);
    assert.equal(product.status, "active");
    assert.equal(product.download_limit, 5);
  });
});

describe("the storefront's catalogue", () => {
  it("pages by name with a cursor, with or without the id half", async () => {
    await seedProduct({ sku: "A-1", name: "Alpha" });
    await seedProduct({ sku: "B-1", name: "Beta" });
    const first = await bodyOf<{ products: Array<{ sku: string }>; nextCursor: string }>(
      (await listProducts(ctx(get("/api/shop/products?limit=1")) as never)) as Response,
    );
    assert.equal(first.products[0]!.sku, "A-1");
    const second = await bodyOf<{ products: Array<{ sku: string }>; nextCursor: string | null }>(
      (await listProducts(ctx(get(`/api/shop/products?limit=1&cursor=${encodeURIComponent(first.nextCursor)}`)) as never)) as Response,
    );
    assert.equal(second.products[0]!.sku, "B-1");
    const byName = await bodyOf<{ products: Array<{ sku: string }> }>(
      (await listProducts(ctx(get("/api/shop/products?cursor=Alpha")) as never)) as Response,
    );
    // A cursor with no id half starts at that name, inclusive.
    assert.deepEqual(byName.products.map((p) => p.sku), ["A-1", "B-1"]);
  });

  it("reads a limit it cannot parse as the default, and lists a product with no price", async () => {
    const { id } = await seedProduct({ sku: "A-1" });
    await env.SHOP_DB.prepare(`DELETE FROM prices WHERE product_id = ?`).bind(id).run();
    const body = await bodyOf<{ products: Array<{ prices: Record<string, number> }> }>(
      (await listProducts(ctx(get("/api/shop/products?limit=many")) as never)) as Response,
    );
    assert.deepEqual(body.products[0]!.prices, {});
  });

  it("refuses a product code that is missing or not one", async () => {
    for (const params of [{}, { sku: "not a sku!" }]) {
      const res = (await oneProduct(ctx(get("/x"), params) as never)) as Response;
      assert.equal(res.status, 400);
    }
  });
});

describe("a database answering sparsely", () => {
  it("reads a missing results array and a missing count as nothing", async () => {
    const { id, sku } = await seedProduct({ sku: "S-1" });
    const thin = { ...shopEnv(), SHOP_DB: sparse(env.SHOP_DB) };

    const list = await bodyOf<{ products: unknown[]; total: number }>(
      (await listProducts({ ...ctx(get("/api/shop/products")), env: thin } as never)) as Response,
    );
    assert.deepEqual([list.products.length, list.total], [0, 0]);

    const panel = await bodyOf<{ products: unknown[]; total: number }>(
      (await adminProducts({ ...ctx(get("/x"), {}, { admin }), env: thin } as never)) as Response,
    );
    assert.deepEqual([panel.products.length, panel.total], [0, 0]);

    const one = await bodyOf<{ prices: Record<string, number> }>(
      (await oneProduct({ ...ctx(get("/x"), { sku }), env: thin } as never)) as Response,
    );
    assert.deepEqual(one.prices, {});

    const detail = await bodyOf<{ product: { prices: Record<string, number> }; soldCount: number }>(
      (await getProduct({ ...ctx(get("/x"), { id }, { admin }), env: thin } as never)) as Response,
    );
    assert.deepEqual([detail.product.prices, detail.soldCount], [{}, 0]);
    // Archive-or-delete reads "no count" as "never sold".
    const gone = await bodyOf((await deleteProduct({ ...ctx(get("/x"), { id }, { admin }), env: thin } as never)) as Response);
    assert.equal(gone.deleted, true);

    assert.deepEqual(await getOrderItems(thin, "any"), []);
    const { pricesFor } = await import("../functions/api/shop/_catalogue");
    assert.equal((await pricesFor(thin, [id])).size, 0);
    invalidateSettings();
    const settings = await allSettings(thin);
    assert.equal(settings["currency.base"], "EUR");
  });

  it("numbers the first order of the year as one when the count has no row", async () => {
    const { nextOrderNumber } = await import("../functions/api/shop/_orders");
    assert.match(await nextOrderNumber({ ...env, SHOP_DB: sparse(env.SHOP_DB) }), /^O-\d{4}-000001$/);
  });
});

describe("checkout's basket", () => {
  const body = { gateway: "stripe", email: "buyer@example.com", name: "A Buyer", country: "DE", consentWaiver: "1" };
  const send = async (extra: Record<string, unknown>, over: Record<string, unknown> = {}) =>
    (await checkout({ ...ctx(post("/api/shop/checkout", { ...body, ...extra })), env: { ...shopEnv(), ...over } } as never)) as Response;

  it("merges two lines naming one product, defaulting each to one copy", async () => {
    await seedProduct({ sku: "EBOOK-1", priceMinor: 1000 });
    const res = await send({ items: [{ sku: "EBOOK-1" }, { sku: "EBOOK-1", quantity: 2 }] });
    const { orderId } = await bodyOf<{ orderId: string }>(res);
    const items = await getOrderItems(env, orderId);
    assert.deepEqual(items.map((i) => i.quantity), [3]);
  });

  it("refuses a line that is not a product, or is nothing at all", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    for (const items of [[null], [{ sku: "bad sku!" }], [{}]]) {
      assert.equal((await bodyOf(await send({ items }))).error, "invalid_basket");
    }
  });

  it("refuses a currency that is not a code, and takes the first offered provider when none is named", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    assert.equal((await bodyOf(await send({ items: [{ sku: "EBOOK-1" }], currency: "E1R" }))).error, "invalid_currency");
    const res = await send({ items: [{ sku: "EBOOK-1" }], gateway: "" });
    assert.equal(res.status, 200);
    const { orderId } = await bodyOf<{ orderId: string }>(res);
    const order = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(orderId).first<OrderRow>();
    assert.equal(order?.gateway, "stripe");
  });

  it("records the country the tax was decided for when the buyer gave none", async () => {
    await putSettings(env, { "tax.mode": "none" });
    await seedProduct({ sku: "EBOOK-1" });
    const res = await send({ items: [{ sku: "EBOOK-1" }], country: "" });
    assert.equal(res.status, 200);
    const { orderId } = await bodyOf<{ orderId: string }>(res);
    const order = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(orderId).first<OrderRow>();
    assert.equal(order?.tax_country, null);
  });
});

describe("pricing and recording an order", () => {
  it("will not sell a product that is not digital", async () => {
    const { id } = await seedProduct({ sku: "BOX-1" });
    await env.SHOP_DB.prepare(`UPDATE products SET kind = 'physical' WHERE id = ?`).bind(id).run();
    const priced = await priceBasket(env, await taxContext(env), {
      lines: [{ sku: "BOX-1", quantity: 1 }], currency: "EUR", billingCountry: "DE", ipCountry: null, vatId: "", vatIdValid: false,
    });
    assert.deepEqual(priced, { error: "product_unavailable", sku: "BOX-1" });
  });

  it("records a reverse-charged order and a verified VAT number", async () => {
    await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
    const priced = (await priceBasket(env, await taxContext(env), {
      lines: [{ sku: "EBOOK-1", quantity: 1 }], currency: "EUR", billingCountry: "DE", ipCountry: null,
      vatId: "DE123456789", vatIdValid: true,
    })) as PricedBasket;
    assert.equal(priced.reverseCharge, true);
    const customer = await upsertCustomer(env, { email: "b2b@example.com", name: "Firm", vatId: "DE123456789", vatIdValid: true, country: "DE" });
    assert.equal(customer.vat_id_valid, 1);
    const unverified = await upsertCustomer(env, { email: "b2c@example.com", name: "", vatId: "", vatIdValid: false, country: "DE" });
    assert.equal(unverified.vat_id_valid, 0);
    const { order } = await createOrder(env, priced, customer, {
      gateway: "stripe", locale: "en", consentMarketing: true, consentWaiver: true, ipHash: null, userAgent: "t",
    });
    assert.equal(order.reverse_charge, 1);
  });
});

describe("the order status page", () => {
  it("is rate limited like everything a stranger can poll", async () => {
    const limiter = { limit: async () => ({ success: false }) };
    const res = (await orderStatus({ ...ctx(get("/x"), { id: "o" }), env: { ...env, RATE_LIMIT_STATUS: limiter } } as never)) as Response;
    assert.equal(res.status, 429);
  });

  it("answers 404 when the link has no id or no key", async () => {
    assert.equal(((await orderStatus(ctx(get("/x"), {}) as never)) as Response).status, 404);
    assert.equal(((await orderStatus(ctx(get("/x"), { id: "o" }) as never)) as Response).status, 404);
  });

  it("shows a paid order without download rows when the database answers with none", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", { items: [{ sku: "EBOOK-1" }], email: "b@example.com", country: "DE", consentWaiver: "1" })),
      env: shopEnv(),
    } as never)) as Response;
    const { orderId, orderKey } = await bodyOf<{ orderId: string; orderKey: string }>(res);
    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'paid' WHERE id = ?`).bind(orderId).run();
    const status = await bodyOf<{ downloads: unknown[]; linksSentByEmail: boolean }>(
      (await orderStatus({
        ...ctx(get(`/x?k=${encodeURIComponent(orderKey)}`), { id: orderId }),
        env: { ...env, SHOP_DB: sparse(env.SHOP_DB) },
      } as never)) as Response,
    );
    assert.deepEqual([status.downloads, status.linksSentByEmail], [[], true]);
  });
});

describe("settings left blank", () => {
  it("keeps a value written by hand that is not JSON", async () => {
    await env.SHOP_DB.prepare(`UPDATE settings SET value = 'Hand Written Press' WHERE key = 'seller.name'`).run();
    invalidateSettings();
    assert.equal((await allSettings(env))["seller.name"], "Hand Written Press");
  });

  it("reads a switch left blank as off", async () => {
    const { getSettingBool } = await import("../functions/api/shop/_settings");
    await putSettings(env, { "invoice.separate_email": null });
    assert.equal(await getSettingBool(env, "invoice.separate_email"), false);
  });

  it("refuses a key nobody declared", async () => {
    assert.deepEqual(await putSettings(env, { "seller.nmae": "x" }), ["seller.nmae"]);
  });

  it("freezes blanks as empty text, and says what is missing to sell", async () => {
    await putSettings(env, {
      "seller.name": null, "seller.address": null, "seller.country": null, "seller.vat_id": null,
      "seller.email": null, "seller.registry": null, "currency.base": null, "tax.mode": null,
    });
    assert.deepEqual(await sellerSnapshot(env), { name: "", address: "", country: "", vatId: "", email: "", registry: "" });
    assert.deepEqual(await shopIsConfigured(env), { ok: false, missing: ["seller.name", "currency.base", "seller.country"] });
    const ctxBlank = await taxContext(env);
    assert.deepEqual([ctxBlank.mode, ctxBlank.sellerCountry], ["table", ""]);
  });

  it("does not ask for a seller country when the provider decides the tax", async () => {
    await putSettings(env, { "seller.country": "", "tax.mode": "stripe" });
    assert.deepEqual((await shopIsConfigured(env)).missing, []);
  });

  it("unblocks tracking with either provider's credentials, and only with both halves", async () => {
    const tracking = async (over: Record<string, unknown>) =>
      (await moduleStates({ ...env, ...over } as never)).find((m) => m.name === "tracking")!.blockedBy;
    assert.equal(await tracking({ GA4_MEASUREMENT_ID: "G-1" }), "GA4 or Meta credentials");
    assert.equal(await tracking({ GA4_MEASUREMENT_ID: "G-1", GA4_API_SECRET: "s" }), null);
    assert.equal(await tracking({ META_PIXEL_ID: "p" }), "GA4 or Meta credentials");
    assert.equal(await tracking({ META_PIXEL_ID: "p", META_ACCESS_TOKEN: "t" }), null);
  });
});

describe("the migration runner", () => {
  /** A database that throws `message` for any statement matching `pattern`. */
  function failing(pattern: RegExp, message: string) {
    const db = fakeD1();
    const prepare = db.prepare;
    return {
      ...db,
      prepare: (sql: string) => {
        const stmt = prepare(sql);
        return pattern.test(sql) ? { ...stmt, run: async () => { throw new Error(message); } } : stmt;
      },
    };
  }

  it("treats a column that already exists as already migrated", async () => {
    resetSchemaCache();
    const db = failing(/idx_payments_ref/, "D1_ERROR: duplicate column name: kind");
    await ensureSchema({ ...env, SHOP_DB: db as never });
    const done = await db.prepare(`SELECT COUNT(*) AS n FROM schema_migrations`).first();
    assert.ok((done as { n: number }).n > 0);
    resetSchemaCache();
  });

  it("stops on any other failure, and seeds no rates when the count has no row", async () => {
    resetSchemaCache();
    const db = failing(/CREATE TABLE IF NOT EXISTS products/, "D1_ERROR: disk full");
    await assert.rejects(ensureSchema({ ...env, SHOP_DB: db as never }), /disk full/);
    resetSchemaCache();
    const thin = fakeD1();
    await ensureSchema({ ...env, SHOP_DB: sparse(thin as never) });
    const seeded = await thin.prepare(`SELECT COUNT(*) AS n FROM vat_rates`).first();
    assert.ok((seeded as { n: number }).n > 0);
    resetSchemaCache();
  });
});

describe("Cloudflare Access corners", () => {
  let pair: CryptoKeyPair;
  let jwks: string;
  const encode = (v: unknown) => base64url(new TextEncoder().encode(JSON.stringify(v)));
  async function sign(head: Record<string, unknown>, payload: Record<string, unknown>): Promise<string> {
    const h = encode(head);
    const b = encode(payload);
    const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", pair.privateKey, new TextEncoder().encode(`${h}.${b}`));
    return `${h}.${b}.${base64url(new Uint8Array(sig))}`;
  }
  const request = (jwt: string) => new Request(`${ORIGIN}/x`, { headers: { "cf-access-jwt-assertion": jwt } });
  const opts = { team: "myteam", aud: "aud-tag" };
  let fetches = 0;

  beforeEach(async () => {
    resetAccessCache();
    fetches = 0;
    pair = (await crypto.subtle.generateKey(
      { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
      true,
      ["sign", "verify"],
    )) as CryptoKeyPair;
    const pub = (await crypto.subtle.exportKey("jwk", pair.publicKey)) as JsonWebKey;
    // An EC key in the set is skipped, not imported as if it were RSA.
    jwks = JSON.stringify({ keys: [{ kty: "EC", kid: "ec-1", crv: "P-256", x: "x", y: "y" }, { ...pub, kid: "kid-1" }] });
    globalThis.fetch = (async () => {
      fetches++;
      return new Response(jwks);
    }) as unknown as typeof fetch;
  });

  it("accepts a single audience, fetches the keys once, and fills in a missing email and subject", async () => {
    const payload = { aud: "aud-tag", iss: "https://myteam.cloudflareaccess.com", exp: Math.floor(Date.now() / 1000) + 60 };
    const first = await verifyAccessJwt(request(await sign({ alg: "RS256", kid: "kid-1" }, payload)), opts);
    assert.deepEqual([first.ok, (first as { email: string }).email, (first as { sub: string }).sub], [true, "", ""]);
    await verifyAccessJwt(request(await sign({ alg: "RS256", kid: "kid-1" }, payload)), opts);
    assert.equal(fetches, 1);
  });

  it("refuses another algorithm, and a token that names no key", async () => {
    const payload = { aud: ["aud-tag"], iss: "https://myteam.cloudflareaccess.com" };
    const hs = await verifyAccessJwt(request(await sign({ alg: "HS256", kid: "kid-1" }, payload)), opts);
    assert.equal((hs as { code: string }).code, "unexpected_algorithm");
    const nokid = await verifyAccessJwt(request(await sign({ alg: "RS256" }, payload)), opts);
    assert.equal((nokid as { code: string }).code, "unknown_key");
    const ec = await verifyAccessJwt(request(await sign({ alg: "RS256", kid: "ec-1" }, payload)), opts);
    assert.equal((ec as { code: string }).code, "unknown_key");
  });
});
