// Fulfilment: everything that happens between "the money arrived" and "the
// buyer has their book", including the parts that only happen when the shop is
// configured for them.

import { env } from "cloudflare:test";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { fulfilOrder, invoiceNumberToPath, markNeedsReview, shopOrigin } from "../functions/api/shop/_fulfil";
import { createOrder, priceBasket, upsertCustomer, type PricedBasket } from "../functions/api/shop/_orders";
import { taxContext } from "../functions/api/shop/_tax";
import { putSettings } from "../functions/api/shop/_settings";
import type { OrderRow } from "../functions/api/shop/_types";
import { freshShop, ORIGIN, seedProduct } from "./_helpers";

const realFetch = globalThis.fetch;

async function order(opts: { consentMarketing?: boolean; withFile?: boolean } = {}): Promise<OrderRow> {
  await seedProduct({ sku: "EBOOK-1", priceMinor: 2400, withFile: opts.withFile !== false });
  const ctx = await taxContext(env);
  const priced = (await priceBasket(env, ctx, {
    lines: [{ sku: "EBOOK-1", quantity: 1 }],
    currency: "EUR",
    billingCountry: "DE",
    ipCountry: null,
    vatId: "",
    vatIdValid: false,
  })) as PricedBasket;
  const customer = await upsertCustomer(env, {
    email: "buyer@example.com", name: "A Buyer", vatId: "", vatIdValid: null, country: "DE",
  });
  const created = await createOrder(env, priced, customer, {
    gateway: "stripe",
    locale: "en",
    consentMarketing: opts.consentMarketing ?? false,
    consentWaiver: true,
    ipHash: null,
    userAgent: "t",
  });
  return created.order;
}

async function queued(kind: string): Promise<number> {
  const row = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM outbox WHERE kind = ?`)
    .bind(kind)
    .first<{ n: number }>();
  return row?.n ?? 0;
}

beforeEach(async () => {
  await freshShop();
  globalThis.fetch = (async () => {
    throw new Error("fulfilment must not reach the network");
  }) as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("fulfilling an order", () => {
  it("issues an invoice, a link and an email", async () => {
    const result = await fulfilOrder(env, await order(), { origin: ORIGIN, actor: "test" });
    expect(result.changed).toBe(true);
    expect(result.tokensIssued).toBe(1);
    expect(result.invoiceNumber).toContain("FV");
    expect(await queued("email")).toBe(1);
  });

  it("puts the invoice link in the email only when it has the order key", async () => {
    const withKey = await fulfilOrder(env, await order(), { origin: ORIGIN, actor: "test", orderKey: "the-key" });
    expect(withKey.changed).toBe(true);
    const row = await env.SHOP_DB.prepare(`SELECT payload_json FROM outbox WHERE kind = 'email'`)
      .first<{ payload_json: string }>();
    // The invoice needs the key, and only the buyer's own copy of the email has
    // one, so a shop that cannot supply it sends links without it.
    expect(row?.payload_json).toContain("k=the-key");
  });

  it("does nothing the second time", async () => {
    const first = await order();
    await fulfilOrder(env, first, { origin: ORIGIN, actor: "test" });
    const second = await fulfilOrder(env, first, { origin: ORIGIN, actor: "test" });

    expect(second.changed).toBe(false);
    expect(await queued("email")).toBe(1);
    const invoices = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM invoices`).first<{ n: number }>();
    expect(invoices?.n).toBe(1);
  });

  it("refuses to fulfil an order that is refunded or cancelled", async () => {
    const row = await order();
    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'cancelled' WHERE id = ?`).bind(row.id).run();
    const result = await fulfilOrder(env, row, { origin: ORIGIN, actor: "test" });
    expect(result).toEqual({ changed: false, tokensIssued: 0, queued: 0 });
  });

  it("tells the owner as well when there is an address to tell", async () => {
    await fulfilOrder(env, await order(), { origin: ORIGIN, actor: "test" });
    expect(await queued("email")).toBe(1);

    await freshShop();
    const withNotice = { ...env, SHOP_MAIL_ADMIN: "owner@example.com" };
    await fulfilOrder(withNotice, await order(), { origin: ORIGIN, actor: "test" });
    expect(await queued("email")).toBe(2);

    const notice = await env.SHOP_DB.prepare(
      `SELECT payload_json FROM outbox WHERE target = 'owner@example.com'`,
    ).first<{ payload_json: string }>();
    // The owner's copy says a sale happened, not who made it.
    expect(notice?.payload_json).not.toContain("buyer@example.com");
  });

  it("tracks a purchase only with the buyer's consent", async () => {
    const tracked = { ...env, GA4_MEASUREMENT_ID: "G-1", GA4_API_SECRET: "s" };

    await fulfilOrder(tracked, await order({ consentMarketing: false }), { origin: ORIGIN, actor: "test" });
    expect(await queued("tracking")).toBe(0);

    await freshShop();
    await fulfilOrder(tracked, await order({ consentMarketing: true }), { origin: ORIGIN, actor: "test" });
    expect(await queued("tracking")).toBe(1);
  });

  it("calls the shop's own webhooks", async () => {
    await putSettings(env, {
      "webhooks.endpoints": [{ url: "https://listener.example/hook", events: ["order.paid"], secret: "s" }],
    });
    await fulfilOrder(env, await order(), { origin: ORIGIN, actor: "test" });
    expect(await queued("webhook")).toBe(1);

    const row = await env.SHOP_DB.prepare(`SELECT payload_json FROM outbox WHERE kind = 'webhook'`)
      .first<{ payload_json: string }>();
    const payload = JSON.parse(row!.payload_json) as { type: string; data: { order: { totalMinor: number } } };
    expect(payload.type).toBe("order.paid");
    expect(payload.data.order.totalMinor).toBe(2400);
  });

  it("delivers what it can when a line has no file", async () => {
    const result = await fulfilOrder(env, await order({ withFile: false }), { origin: ORIGIN, actor: "test" });
    // The order is still paid and still invoiced; there is simply nothing to
    // download, which the owner sees in the panel.
    expect(result.changed).toBe(true);
    expect(result.tokensIssued).toBe(0);
    expect(result.invoiceNumber).toBeDefined();
  });
});

describe("an order that needs a human", () => {
  it("is parked, logged, and reported to whoever is listening", async () => {
    await putSettings(env, {
      "webhooks.endpoints": [{ url: "https://listener.example/hook", events: ["order.needs_review"] }],
    });
    const row = await order();
    await markNeedsReview({ ...env, SHOP_MAIL_ADMIN: "owner@example.com" }, row, "paid 1, expected 2400", {
      origin: ORIGIN,
      actor: "webhook:stripe",
    });

    const after = await env.SHOP_DB.prepare(`SELECT status FROM orders WHERE id = ?`)
      .bind(row.id)
      .first<{ status: string }>();
    expect(after?.status).toBe("needs_review");

    const logged = await env.SHOP_DB.prepare(`SELECT * FROM audit_log WHERE action = 'order.needs_review'`)
      .first<{ detail_json: string }>();
    expect(logged?.detail_json).toContain("expected 2400");

    expect(await queued("email")).toBe(1);
    expect(await queued("webhook")).toBe(1);
  });
});

describe("the shop's own address", () => {
  it("prefers what the owner configured over the host that was asked", async () => {
    const request = new Request("https://preview-123.pages.dev/api/shop/checkout");
    expect(await shopOrigin(env, request)).toBe(ORIGIN);

    await putSettings(env, { "shop.url": "" });
    // Without one, an email sent from a preview deployment points at the
    // preview — which is right, because that is where the links work.
    expect(await shopOrigin(env, request)).toBe("https://preview-123.pages.dev");
  });

  it("trims a trailing slash so links do not double up", async () => {
    await putSettings(env, { "shop.url": "https://books.example.com/" });
    expect(await shopOrigin(env, new Request("https://x.example/"))).toBe("https://books.example.com");
  });
});

describe("invoice numbers in a URL", () => {
  it("escapes the slashes a default number contains", () => {
    expect(invoiceNumberToPath("FV/2026/000042")).toBe("FV%2F2026%2F000042");
  });
});
