// The endpoints a buyer touches. Each is called the way Pages calls it, with a
// real request and the real bindings.

import { env } from "cloudflare:test";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
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
import { bodyOf, ctx, freshShop, get, ORIGIN, post, seedProduct } from "./_helpers";

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

    expect(body.products).toHaveLength(1);
    expect(body.products[0]).toMatchObject({ sku: "EBOOK-1", prices: { EUR: 2400 } });
    // A file key is where the bytes live; a buyer has no business with it.
    expect(JSON.stringify(body.products[0])).not.toContain("file_key");
    expect(body.shop).toMatchObject({ name: "Example Press", baseCurrency: "EUR" });
    expect(res.headers.get("cache-control")).toContain("max-age=60");
  });

  it("answers for one product, and 404s for anything else", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const found = (await oneProduct(ctx(get("/api/shop/products/EBOOK-1"), { sku: "EBOOK-1" }) as never)) as Response;
    expect(found.status).toBe(200);

    const missing = (await oneProduct(ctx(get("/api/shop/products/NOPE"), { sku: "NOPE" }) as never)) as Response;
    expect(missing.status).toBe(404);
  });

  it("says what is wrong rather than crashing when nothing is bound", async () => {
    const res = (await listProducts({
      ...ctx(get("/api/shop/products")),
      env: { ...env, SHOP_DB: undefined },
    } as never)) as Response;
    expect(res.status).toBe(503);
    expect(await bodyOf<{ message: string }>(res)).toMatchObject({ error: "not_configured" });
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

    expect(res.status).toBe(200);
    expect(body.redirectUrl).toContain("checkout.stripe.com");
    expect(body.orderKey.length).toBeGreaterThan(20);

    const order = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`)
      .bind(body.orderId)
      .first<OrderRow>();
    expect(order).toMatchObject({ status: "pending", total_minor: 2400, gateway_ref: "cs_test_1" });
  });

  it("ignores a price the browser sends", async () => {
    await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
    const context = ctx(post("/api/shop/checkout", { ...goodBody, price: 1, totalMinor: 1, amount: 1 }));
    const res = (await checkout({ ...context, env: shopEnv() } as never)) as Response;
    const body = await bodyOf<{ orderId: string }>(res);
    const order = await env.SHOP_DB.prepare(`SELECT total_minor FROM orders WHERE id = ?`)
      .bind(body.orderId)
      .first<{ total_minor: number }>();
    expect(order?.total_minor).toBe(2400);
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
    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toContain("checkout.stripe.com");
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
      expect(await bodyOf<{ error: string }>(res), JSON.stringify(body)).toMatchObject({ error });
    }
  });

  it("refuses a body it cannot read", async () => {
    const request = new Request(`${ORIGIN}/api/shop/checkout`, {
      method: "POST",
      headers: { "content-type": "application/json", origin: ORIGIN },
      body: "{oh no",
    });
    const res = (await checkout({ ...ctx(request), env: shopEnv() } as never)) as Response;
    expect(res.status).toBe(400);
  });

  it("refuses to sell before the shop is configured", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    await env.SHOP_DB.prepare(`DELETE FROM settings WHERE key = 'seller.name'`).run();
    const { invalidateSettings } = await import("../functions/api/shop/_settings");
    invalidateSettings();

    const res = (await checkout({ ...ctx(post("/api/shop/checkout", goodBody)), env: shopEnv() } as never)) as Response;
    expect(res.status).toBe(503);
  });

  it("leaves the order pending, and says nothing about keys, when the provider refuses", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    globalThis.fetch = (async () =>
      new Response(JSON.stringify({ error: { message: "Invalid API Key provided: sk_test_abcd" } }), {
        status: 401,
      })) as typeof fetch;

    const res = (await checkout({ ...ctx(post("/api/shop/checkout", goodBody)), env: shopEnv() } as never)) as Response;
    const body = await bodyOf<{ error: string; message: string }>(res);
    expect(res.status).toBe(502);
    expect(body.error).toBe("gateway_error");
    // The provider's wording quotes our own configuration back at us.
    expect(body.message).not.toContain("sk_test");

    const order = await env.SHOP_DB.prepare(`SELECT status, gateway_ref FROM orders`).first<{ status: string; gateway_ref: string | null }>();
    expect(order).toMatchObject({ status: "pending", gateway_ref: null });

    const logged = await env.SHOP_DB.prepare(`SELECT detail_json FROM audit_log WHERE action = 'checkout.gateway_error'`).first<{ detail_json: string }>();
    expect(logged?.detail_json).toContain("Invalid API Key");
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
    expect(res.status).toBe(403);
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
    expect(await bodyOf<{ status: string }>(ok)).toMatchObject({ status: "pending" });

    const wrong = (await orderStatus(ctx(get(`/api/shop/orders/${orderId}/status?k=nope`), { id: orderId }) as never)) as Response;
    expect(wrong.status).toBe(404);

    // The same answer for an order that does not exist, so the endpoint cannot
    // be used to find out which ids do.
    const missing = (await orderStatus(ctx(get(`/api/shop/orders/none/status?k=${orderKey}`), { id: "none" }) as never)) as Response;
    expect(missing.status).toBe(404);
  });

  it("reports the links as sent by email rather than handing them over", async () => {
    const { orderId, orderKey } = await buy();
    const order = (await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(orderId).first<OrderRow>())!;
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });

    const res = (await orderStatus(ctx(get(`/api/shop/orders/${orderId}/status?k=${orderKey}`), { id: orderId }) as never)) as Response;
    const body = await bodyOf<{ status: string; downloads: Array<{ url: string; remaining: number }>; linksSentByEmail: boolean }>(res);
    expect(body.status).toBe("paid");
    expect(body.linksSentByEmail).toBe(true);
    expect(body.downloads[0]).toMatchObject({ url: "", remaining: 3 });
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

    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toBe("application/pdf");
    expect(res.headers.get("content-disposition")).toContain("filename*=UTF-8''");
    expect(await res.text()).toContain("%PDF");
  });

  it("serves a range without charging a second use", async () => {
    const token = await paidToken();
    await download(ctx(get(`/api/shop/download/${token}`), { token }) as never);

    const ranged = (await download(
      ctx(get(`/api/shop/download/${token}`, { range: "bytes=0-3" }), { token }) as never,
    )) as Response;
    expect(ranged.status).toBe(206);

    const row = await env.SHOP_DB.prepare(`SELECT uses FROM download_tokens`).first<{ uses: number }>();
    expect(row?.uses).toBe(1);
  });

  it("says which way the link is dead", async () => {
    const token = await paidToken();
    await env.SHOP_DB.prepare(`UPDATE download_tokens SET revoked = 1`).run();
    const res = (await download(ctx(get(`/api/shop/download/${token}`), { token }) as never)) as Response;
    expect(res.status).toBe(410);
    expect(await bodyOf<{ error: string }>(res)).toMatchObject({ error: "revoked" });
  });

  it("refuses a token that is not even the right shape", async () => {
    const res = (await download(ctx(get("/api/shop/download/short"), { token: "short" }) as never)) as Response;
    expect(res.status).toBe(400);
  });

  it("says so when there is nowhere for files to be", async () => {
    const res = (await download({
      ...ctx(get("/api/shop/download/x".repeat(10)), { token: "x".repeat(30) }),
      env: { ...env, SHOP_FILES: undefined },
    } as never)) as Response;
    expect(res.status).toBe(503);
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

    expect(first.status).toBe(second.status);
    expect(await first.text()).toBe(await second.text());
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
    expect(queued?.n).toBe(1);
    expect(id).toBeDefined();
  });

  it("refuses an address that is not one", async () => {
    const res = (await resend(ctx(post("/api/shop/resend", { email: "nope" })) as never)) as Response;
    expect(res.status).toBe(422);
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
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toContain("text/html");
    // Nothing may execute inside an invoice, and nothing may index it.
    expect(res.headers.get("content-security-policy")).toContain("default-src 'none'");
    expect(res.headers.get("x-robots-tag")).toContain("noindex");
    expect(await res.text()).toContain(invoiceNumber!);

    const without = (await invoicePage(
      ctx(get(`/api/shop/invoices/${encodeURIComponent(invoiceNumber!)}?k=wrong`), { number: invoiceNumber! }) as never,
    )) as Response;
    expect(without.status).toBe(404);
  });

  it("404s for a number that was never issued", async () => {
    const res = (await invoicePage(ctx(get("/api/shop/invoices/FV%2F2026%2F999999?k=x"), { number: "FV/2026/999999" }) as never)) as Response;
    expect(res.status).toBe(404);
  });
});
