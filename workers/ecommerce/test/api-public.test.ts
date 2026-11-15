// The endpoints a buyer touches. Each is called the way Pages calls it, with a
// real request and the real bindings.

import { env } from "./_env";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as listProducts } from "../functions/api/shop/products/index";
import { onRequestGet as oneProduct } from "../functions/api/shop/products/[sku]";
import { onRequestPost as checkout } from "../functions/api/shop/checkout";
import { onRequestGet as orderStatus } from "../functions/api/shop/orders/[id]/status";
import { onRequestGet as download } from "../functions/api/shop/download/[token]";
import { onRequestPost as resend } from "../functions/api/shop/resend";
import { onRequestGet as invoicePage } from "../functions/api/shop/invoices/[number]";
import { issueTokens } from "../functions/api/shop/_downloads";
import { fulfilOrder } from "../functions/api/shop/_fulfil";
import { getOrderItems } from "../functions/api/shop/_orders";
import type { OrderRow, ProductRow } from "../functions/api/shop/_types";
import { assertPartial, bodyOf, ctx, freshShop, get, ORIGIN, post, seedProduct } from "./_helpers";

const shopEnv = () => ({ ...env, STRIPE_SECRET_KEY: "sk_test_x", SHOP_GATEWAYS: "stripe" });
const realFetch = globalThis.fetch;

/** Stripe, replaced by something that always agrees. */
function stripeAgrees(): void {
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ id: "cs_test_1", url: "https://checkout.stripe.com/pay/cs_test_1" }))) as typeof fetch;
}

async function settle(context: unknown): Promise<void> {
  await (context as { settled(): Promise<unknown> }).settled();
}

beforeEach(async () => {
  await freshShop();
  stripeAgrees();
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("the catalogue", () => {
  it("lists what is on sale, and nothing about the files", async () => {
    await seedProduct({ sku: "EBOOK-1", name: "A Book", priceMinor: 2400 });
    const { id } = await seedProduct({ sku: "DRAFT-1" });
    await env.SHOP_DB.prepare(`UPDATE products SET status = 'draft' WHERE id = ?`).bind(id).run();

    const res = (await listProducts(ctx(get("/api/shop/products")) as never)) as Response;
    const body = await bodyOf<{ products: Array<Record<string, unknown>>; shop: Record<string, unknown> }>(res);

    assert.equal((body.products).length, 1);
    assertPartial(body.products[0], { sku: "EBOOK-1", prices: { EUR: 2400 } });
    // A file key is where the bytes live; a buyer has no business with it.
    assert.ok(!(JSON.stringify(body.products[0])).includes("file_key"));
    assertPartial(body.shop, { name: "Example Press", baseCurrency: "EUR" });
    assert.ok((res.headers.get("cache-control")).includes("max-age=60"));
  });

  it("answers for one product, and 404s for anything else", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const found = (await oneProduct(ctx(get("/api/shop/products/EBOOK-1"), { sku: "EBOOK-1" }) as never)) as Response;
    assert.equal(found.status, 200);

    const missing = (await oneProduct(ctx(get("/api/shop/products/NOPE"), { sku: "NOPE" }) as never)) as Response;
    assert.equal(missing.status, 404);
  });

  it("says what is wrong rather than crashing when nothing is bound", async () => {
    const res = (await listProducts({
      ...ctx(get("/api/shop/products")),
      env: { ...env, SHOP_DB: undefined },
    } as never)) as Response;
    assert.equal(res.status, 503);
    assertPartial(await bodyOf<{ message: string }>(res), { error: "not_configured" });
  });
});

describe("checkout", () => {
  const goodBody = {
    items: [{ sku: "EBOOK-1", quantity: 1 }],
    gateway: "stripe",
    email: "buyer@example.com",
    name: "A Buyer",
    country: "DE",
    consentWaiver: "1",
  };

  it("creates an order and hands back a payment URL", async () => {
    await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
    const context = ctx(post("/api/shop/checkout", goodBody));
    const res = (await checkout({ ...context, env: shopEnv() } as never)) as Response;
    const body = await bodyOf<{ orderId: string; orderKey: string; redirectUrl: string }>(res);

    assert.equal(res.status, 200);
    assert.ok((body.redirectUrl).includes("checkout.stripe.com"));
    assert.ok((body.orderKey.length) > 20);

    const order = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`)
      .bind(body.orderId)
      .first<OrderRow>();
    assertPartial(order, { status: "pending", total_minor: 2400, gateway_ref: "cs_test_1" });
  });

  it("ignores a price the browser sends", async () => {
    await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
    const context = ctx(post("/api/shop/checkout", { ...goodBody, price: 1, totalMinor: 1, amount: 1 }));
    const res = (await checkout({ ...context, env: shopEnv() } as never)) as Response;
    const body = await bodyOf<{ orderId: string }>(res);
    const order = await env.SHOP_DB.prepare(`SELECT total_minor FROM orders WHERE id = ?`)
      .bind(body.orderId)
      .first<{ total_minor: number }>();
    assert.equal(order?.total_minor, 2400);
  });

  it("takes a single sku, which is the form on a product page", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const form = new URLSearchParams({
      sku: "EBOOK-1", quantity: "1", email: "buyer@example.com", country: "DE",
      gateway: "stripe", consentWaiver: "on",
    });
    const request = new Request(`${ORIGIN}/api/shop/checkout`, {
      method: "POST",
      headers: { "content-type": "application/x-www-form-urlencoded", accept: "text/html", origin: ORIGIN },
      body: form,
    });
    const res = (await checkout({ ...ctx(request), env: shopEnv() } as never)) as Response;
    // A browser posting a form wants a redirect, not JSON.
    assert.equal(res.status, 303);
    assert.ok((res.headers.get("location")).includes("checkout.stripe.com"));
  });

  it("refuses the things a buyer can get wrong, one message each", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const cases: Array<[Record<string, unknown>, string]> = [
      [{ ...goodBody, email: "not-an-email" }, "invalid_email"],
      [{ ...goodBody, items: [] }, "invalid_basket"],
      [{ ...goodBody, items: [{ sku: "NOPE", quantity: 1 }] }, "unknown_sku"],
      [{ ...goodBody, country: "" }, "country_required"],
      [{ ...goodBody, country: "Germany" }, "invalid_country"],
      [{ ...goodBody, consentWaiver: "" }, "waiver_required"],
      [{ ...goodBody, gateway: "bitcoin" }, "unknown_gateway"],
      [{ ...goodBody, items: [{ sku: "EBOOK-1", quantity: 999 }] }, "invalid_basket"],
    ];
    for (const [body, error] of cases) {
      const res = (await checkout({ ...ctx(post("/api/shop/checkout", body)), env: shopEnv() } as never)) as Response;
      assertPartial(await bodyOf<{ error: string }>(res), { error }, JSON.stringify(body));
    }
  });

  it("refuses a body it cannot read", async () => {
    const request = new Request(`${ORIGIN}/api/shop/checkout`, {
      method: "POST",
      headers: { "content-type": "application/json", origin: ORIGIN },
      body: "{oh no",
    });
    const res = (await checkout({ ...ctx(request), env: shopEnv() } as never)) as Response;
    assert.equal(res.status, 400);
  });

  it("refuses to sell before the shop is configured", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    await env.SHOP_DB.prepare(`DELETE FROM settings WHERE key = 'seller.name'`).run();
    const { invalidateSettings } = await import("../functions/api/shop/_settings");
    invalidateSettings();

    const res = (await checkout({ ...ctx(post("/api/shop/checkout", goodBody)), env: shopEnv() } as never)) as Response;
    assert.equal(res.status, 503);
  });

  it("leaves the order pending, and says nothing about keys, when the provider refuses", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    globalThis.fetch = (async () =>
      new Response(JSON.stringify({ error: { message: "Invalid API Key provided: sk_test_abcd" } }), {
        status: 401,
      })) as typeof fetch;

    const res = (await checkout({ ...ctx(post("/api/shop/checkout", goodBody)), env: shopEnv() } as never)) as Response;
    const body = await bodyOf<{ error: string; message: string }>(res);
    assert.equal(res.status, 502);
    assert.equal(body.error, "gateway_error");
    // The provider's wording quotes our own configuration back at us.
    assert.ok(!(body.message).includes("sk_test"));

    const order = await env.SHOP_DB.prepare(`SELECT status, gateway_ref FROM orders`).first<{ status: string; gateway_ref: string | null }>();
    assertPartial(order, { status: "pending", gateway_ref: null });

    const logged = await env.SHOP_DB.prepare(`SELECT detail_json FROM audit_log WHERE action = 'checkout.gateway_error'`).first<{ detail_json: string }>();
    assert.ok((logged?.detail_json).includes("Invalid API Key"));
  });

  it("refuses when the anti-spam check does not pass", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    globalThis.fetch = (async (url: unknown) => {
      if (String(url).includes("challenges.cloudflare.com")) {
        return new Response(JSON.stringify({ success: false }));
      }
      return new Response(JSON.stringify({ id: "cs_1", url: "https://x" }));
    }) as unknown as typeof fetch;

    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", { ...goodBody, turnstileToken: "bad" })),
      env: { ...shopEnv(), TURNSTILE_SECRET: "secret" },
    } as never)) as Response;
    assert.equal(res.status, 403);
  });
});

describe("the thank-you page's question", () => {
  async function buy(): Promise<{ orderId: string; orderKey: string }> {
    await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: [{ sku: "EBOOK-1", quantity: 1 }], gateway: "stripe",
        email: "buyer@example.com", country: "DE", consentWaiver: "1",
      })),
      env: shopEnv(),
    } as never)) as Response;
    return bodyOf(res);
  }

  it("answers only with the key that came with the purchase", async () => {
    const { orderId, orderKey } = await buy();

    const ok = (await orderStatus(ctx(get(`/api/shop/orders/${orderId}/status?k=${orderKey}`), { id: orderId }) as never)) as Response;
    assertPartial(await bodyOf<{ status: string; invoiceUrl: string | null }>(ok), { status: "pending", invoiceUrl: null });

    const wrong = (await orderStatus(ctx(get(`/api/shop/orders/${orderId}/status?k=nope`), { id: orderId }) as never)) as Response;
    assert.equal(wrong.status, 404);

    // The same answer for an order that does not exist, so the endpoint cannot
    // be used to find out which ids do.
    const missing = (await orderStatus(ctx(get(`/api/shop/orders/none/status?k=${orderKey}`), { id: "none" }) as never)) as Response;
    assert.equal(missing.status, 404);
  });

  it("reports the links as sent by email rather than handing them over", async () => {
    const { orderId, orderKey } = await buy();
    const order = (await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(orderId).first<OrderRow>())!;
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });

    const res = (await orderStatus(ctx(get(`/api/shop/orders/${orderId}/status?k=${orderKey}`), { id: orderId }) as never)) as Response;
    const body = await bodyOf<{ status: string; downloads: Array<{ url: string; remaining: number }>; linksSentByEmail: boolean }>(res);
    assert.equal(body.status, "paid");
    assert.equal(body.linksSentByEmail, true);
    assertPartial(body.downloads[0], { url: "", remaining: 3 });
  });

  it("links the paid order's invoice with the key the caller sent, and that link opens it", async () => {
    const { orderId, orderKey } = await buy();
    const order = (await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(orderId).first<OrderRow>())!;
    const { invoiceNumber } = await fulfilOrder(env, order, { origin: ORIGIN, actor: "webhook:stripe" });

    const res = (await orderStatus(ctx(get(`/api/shop/orders/${orderId}/status?k=${orderKey}`), { id: orderId }) as never)) as Response;
    const { invoiceUrl } = await bodyOf<{ invoiceUrl: string }>(res);
    assert.equal(invoiceUrl, `/api/shop/invoices/${encodeURIComponent(invoiceNumber!)}?k=${encodeURIComponent(orderKey)}`);
    const page = (await invoicePage(ctx(get(invoiceUrl), { number: encodeURIComponent(invoiceNumber!) }) as never)) as Response;
    assert.equal(page.status, 200);
    assert.equal(page.headers.get("content-type"), "text/html; charset=utf-8");
  });
});

describe("downloading", () => {
  async function paidToken(): Promise<string> {
    const { id } = await seedProduct({ sku: "EBOOK-1" });
    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: [{ sku: "EBOOK-1", quantity: 1 }], gateway: "stripe",
        email: "buyer@example.com", country: "DE", consentWaiver: "1",
      })),
      env: shopEnv(),
    } as never)) as Response;
    const { orderId } = await bodyOf<{ orderId: string }>(res);

    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'paid' WHERE id = ?`).bind(orderId).run();
    const items = await getOrderItems(env, orderId);
    const product = (await env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`).bind(id).first<ProductRow>())!;
    const [token] = await issueTokens(env, items, new Map([[id, product]]));
    return token!.token;
  }

  it("streams the file and names it properly", async () => {
    const token = await paidToken();
    const res = (await download(ctx(get(`/api/shop/download/${token}`), { token }) as never)) as Response;

    assert.equal(res.status, 200);
    assert.equal(res.headers.get("content-type"), "application/pdf");
    assert.ok((res.headers.get("content-disposition")).includes("filename*=UTF-8''"));
    assert.ok((await res.text()).includes("%PDF"));
  });

  it("serves a range without charging a second use", async () => {
    const token = await paidToken();
    await download(ctx(get(`/api/shop/download/${token}`), { token }) as never);

    const ranged = (await download(
      ctx(get(`/api/shop/download/${token}`, { range: "bytes=0-3" }), { token }) as never,
    )) as Response;
    assert.equal(ranged.status, 206);

    const row = await env.SHOP_DB.prepare(`SELECT uses FROM download_tokens`).first<{ uses: number }>();
    assert.equal(row?.uses, 1);
  });

  it("says which way the link is dead", async () => {
    const token = await paidToken();
    await env.SHOP_DB.prepare(`UPDATE download_tokens SET revoked = 1`).run();
    const res = (await download(ctx(get(`/api/shop/download/${token}`), { token }) as never)) as Response;
    assert.equal(res.status, 410);
    assertPartial(await bodyOf<{ error: string }>(res), { error: "revoked" });
  });

  it("refuses a token that is not even the right shape", async () => {
    const res = (await download(ctx(get("/api/shop/download/short"), { token: "short" }) as never)) as Response;
    assert.equal(res.status, 400);
  });

  it("says so when there is nowhere for files to be", async () => {
    const res = (await download({
      ...ctx(get("/api/shop/download/x".repeat(10)), { token: "x".repeat(30) }),
      env: { ...env, SHOP_FILES: undefined },
    } as never)) as Response;
    assert.equal(res.status, 503);
  });
});

describe("asking for the links again", () => {
  it("answers the same whether or not the address ever bought anything", async () => {
    await seedProduct({ sku: "EBOOK-1" });

    const known = ctx(post("/api/shop/resend", { email: "buyer@example.com" }));
    const first = (await resend(known as never)) as Response;
    await settle(known);

    const unknown = ctx(post("/api/shop/resend", { email: "nobody@example.com" }));
    const second = (await resend(unknown as never)) as Response;
    await settle(unknown);

    assert.equal(first.status, second.status);
    assert.equal(await first.text(), await second.text());
  });

  it("queues fresh links for an address that did buy", async () => {
    const { id } = await seedProduct({ sku: "EBOOK-1" });
    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: [{ sku: "EBOOK-1", quantity: 1 }], gateway: "stripe",
        email: "buyer@example.com", country: "DE", consentWaiver: "1",
      })),
      env: shopEnv(),
    } as never)) as Response;
    const { orderId } = await bodyOf<{ orderId: string }>(res);
    const order = (await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(orderId).first<OrderRow>())!;
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    await env.SHOP_DB.prepare(`DELETE FROM outbox`).run();

    const context = ctx(post("/api/shop/resend", { email: "Buyer@Example.com" }));
    await resend(context as never);
    await settle(context);

    const queued = await env.SHOP_DB.prepare(
      `SELECT COUNT(*) AS n FROM outbox WHERE target = 'buyer@example.com'`,
    ).first<{ n: number }>();
    assert.equal(queued?.n, 1);
    assert.notEqual(id, undefined);
  });

  it("refuses an address that is not one", async () => {
    const res = (await resend(ctx(post("/api/shop/resend", { email: "nope" })) as never)) as Response;
    assert.equal(res.status, 422);
  });
});

describe("the buyer's invoice", () => {
  it("is served from the frozen snapshot, to whoever has the order key", async () => {
    await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
    const checkoutRes = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: [{ sku: "EBOOK-1", quantity: 1 }], gateway: "stripe",
        email: "buyer@example.com", country: "DE", consentWaiver: "1",
      })),
      env: shopEnv(),
    } as never)) as Response;
    const { orderId, orderKey } = await bodyOf<{ orderId: string; orderKey: string }>(checkoutRes);
    const order = (await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(orderId).first<OrderRow>())!;
    const { invoiceNumber } = await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });

    const res = (await invoicePage(
      ctx(get(`/api/shop/invoices/${encodeURIComponent(invoiceNumber!)}?k=${orderKey}`), { number: invoiceNumber! }) as never,
    )) as Response;
    assert.equal(res.status, 200);
    assert.ok((res.headers.get("content-type")).includes("text/html"));
    // Nothing may execute inside an invoice, and nothing may index it.
    assert.ok((res.headers.get("content-security-policy")).includes("default-src 'none'"));
    assert.ok((res.headers.get("x-robots-tag")).includes("noindex"));
    assert.ok((await res.text()).includes(invoiceNumber!));

    const without = (await invoicePage(
      ctx(get(`/api/shop/invoices/${encodeURIComponent(invoiceNumber!)}?k=wrong`), { number: invoiceNumber! }) as never,
    )) as Response;
    assert.equal(without.status, 404);
  });

  it("404s for a number that was never issued", async () => {
    const res = (await invoicePage(ctx(get("/api/shop/invoices/FV%2F2026%2F999999?k=x"), { number: "FV/2026/999999" }) as never)) as Response;
    assert.equal(res.status, 404);
  });
});
