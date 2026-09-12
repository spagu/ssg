// Pricing a basket and turning it into an order. This is the file that proves
// the browser has no say in what anything costs.

import { env } from "cloudflare:test";
import { beforeEach, describe, expect, it } from "vitest";
import {
  allowedCountries,
  createOrder,
  findOrderByGatewayRef,
  getOrder,
  getOrderItems,
  nextOrderNumber,
  orderKeyMatches,
  priceBasket,
  upsertCustomer,
  type PricedBasket,
} from "../functions/api/shop/_orders";
import { taxContext } from "../functions/api/shop/_tax";
import { putSettings } from "../functions/api/shop/_settings";
import { freshShop, seedProduct } from "./_helpers";

const meta = {
  gateway: "stripe",
  locale: "en",
  consentMarketing: false,
  consentWaiver: true,
  ipHash: null,
  userAgent: "test",
};

async function price(
  lines: Array<{ sku: string; quantity: number }>,
  opts: Partial<{ currency: string; billingCountry: string | null; ipCountry: string | null; vatId: string; vatIdValid: boolean }> = {},
) {
  const ctx = await taxContext(env);
  return priceBasket(env, ctx, {
    lines,
    currency: opts.currency ?? "EUR",
    billingCountry: opts.billingCountry === undefined ? "DE" : opts.billingCountry,
    ipCountry: opts.ipCountry ?? null,
    vatId: opts.vatId ?? "",
    vatIdValid: opts.vatIdValid ?? false,
  });
}

beforeEach(async () => {
  await freshShop();
});

describe("pricing a basket", () => {
  it("prices from the database, whatever the basket claims", async () => {
    await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
    const priced = (await price([{ sku: "EBOOK-1", quantity: 1 }])) as PricedBasket;
    expect(priced.totalMinor).toBe(2400);
    expect(priced.taxMinor).toBe(157); // Germany's 7% ebook rate, carved out
    expect(priced.subtotalMinor).toBe(2243);
    expect(priced.taxCountry).toBe("DE");
  });

  it("adds up several lines and several copies", async () => {
    await seedProduct({ sku: "A", priceMinor: 1000 });
    await seedProduct({ sku: "B", priceMinor: 2500 });
    const priced = (await price([
      { sku: "A", quantity: 2 },
      { sku: "B", quantity: 1 },
    ])) as PricedBasket;
    expect(priced.totalMinor).toBe(4500);
    expect(priced.items).toHaveLength(2);
  });

  it("refuses an empty basket", async () => {
    expect(await price([])).toEqual({ error: "empty_basket" });
  });

  it("refuses a product that does not exist", async () => {
    expect(await price([{ sku: "NOPE", quantity: 1 }])).toEqual({ error: "unknown_sku", sku: "NOPE" });
  });

  it("refuses a draft, which is not for sale", async () => {
    const { id } = await seedProduct({ sku: "DRAFT" });
    await env.SHOP_DB.prepare(`UPDATE products SET status = 'draft' WHERE id = ?`).bind(id).run();
    expect(await price([{ sku: "DRAFT", quantity: 1 }])).toEqual({
      error: "product_unavailable",
      sku: "DRAFT",
    });
  });

  it("refuses a currency the product has no price in", async () => {
    await seedProduct({ sku: "EBOOK-1", currency: "EUR" });
    expect(await price([{ sku: "EBOOK-1", quantity: 1 }], { currency: "PLN" })).toEqual({
      error: "price_not_available",
      sku: "EBOOK-1",
      currency: "PLN",
    });
  });

  it("refuses a country it has no rate for rather than charging nothing", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    await env.SHOP_DB.prepare(`DELETE FROM vat_rates WHERE country = 'DE'`).run();
    expect(await price([{ sku: "EBOOK-1", quantity: 1 }])).toEqual({
      error: "tax_rate_missing",
      country: "DE",
    });
  });

  it("needs a country before it can price anything", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    expect(await price([{ sku: "EBOOK-1", quantity: 1 }], { billingCountry: null })).toEqual({
      error: "country_required",
    });
  });

  it("refuses a country the shop chose not to sell to", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    await putSettings(env, { "shop.countries_allowed": ["PL"] });
    expect(await price([{ sku: "EBOOK-1", quantity: 1 }])).toEqual({
      error: "country_not_served",
      country: "DE",
    });
  });

  it("zero-rates a verified business in another member state", async () => {
    await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
    const priced = (await price([{ sku: "EBOOK-1", quantity: 1 }], {
      vatId: "DE123456789",
      vatIdValid: true,
    })) as PricedBasket;
    expect(priced.reverseCharge).toBe(true);
    expect(priced.taxMinor).toBe(0);
    // Gross pricing with no tax means the customer pays the same number with
    // nothing carved out of it.
    expect(priced.totalMinor).toBe(2400);
  });

  it("records the evidence the decision rested on", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const priced = (await price([{ sku: "EBOOK-1", quantity: 1 }], {
      billingCountry: "DE",
      ipCountry: "PL",
    })) as PricedBasket;
    expect(priced.evidence).toMatchObject({ billing: "DE", ip: "PL", conflict: true });
  });
});

describe("the countries setting", () => {
  it("reads a list or a comma-separated string, and empty means everywhere", async () => {
    await putSettings(env, { "shop.countries_allowed": ["PL", "de"] });
    expect(await allowedCountries(env)).toEqual(["PL", "DE"]);

    await env.SHOP_DB.prepare(`UPDATE settings SET value = '"PL, FR"' WHERE key = 'shop.countries_allowed'`).run();
    const { invalidateSettings } = await import("../functions/api/shop/_settings");
    invalidateSettings();
    expect(await allowedCountries(env)).toEqual(["PL", "FR"]);

    await putSettings(env, { "shop.countries_allowed": [] });
    expect(await allowedCountries(env)).toEqual([]);
  });
});

describe("customers", () => {
  it("is one person however many times they come back", async () => {
    const first = await upsertCustomer(env, {
      email: "Buyer@Example.com",
      name: "A Buyer",
      vatId: "",
      vatIdValid: null,
      country: "DE",
    });
    const second = await upsertCustomer(env, {
      email: "buyer@example.com",
      name: "",
      vatId: "DE123456789",
      vatIdValid: true,
      country: "",
    });
    expect(second.id).toBe(first.id);
    // Nothing already known is erased by a later purchase that omitted it.
    expect(second.name).toBe("A Buyer");
    expect(second.country).toBe("DE");
    expect(second.vat_id).toBe("DE123456789");
  });
});

describe("orders", () => {
  it("numbers them within the year", async () => {
    const year = new Date().getUTCFullYear();
    expect(await nextOrderNumber(env)).toBe(`O-${year}-000001`);
  });

  it("stores only the hash of the key it hands the buyer", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const priced = (await price([{ sku: "EBOOK-1", quantity: 1 }])) as PricedBasket;
    const customer = await upsertCustomer(env, {
      email: "b@example.com", name: "B", vatId: "", vatIdValid: null, country: "DE",
    });
    const { order, items, orderKey } = await createOrder(env, priced, customer, meta);

    expect(order.key_hash).not.toContain(orderKey);
    expect(await orderKeyMatches(order, orderKey)).toBe(true);
    expect(await orderKeyMatches(order, "wrong")).toBe(false);
    expect(await orderKeyMatches(order, "")).toBe(false);

    expect(await getOrder(env, order.id)).toMatchObject({ id: order.id, status: "pending" });
    expect(await getOrderItems(env, order.id)).toHaveLength(items.length);
    expect(await getOrder(env, "no-such-order")).toBe(null);
  });

  it("finds an order by what the provider calls it", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const priced = (await price([{ sku: "EBOOK-1", quantity: 1 }])) as PricedBasket;
    const customer = await upsertCustomer(env, {
      email: "b@example.com", name: "B", vatId: "", vatIdValid: null, country: "DE",
    });
    const { order } = await createOrder(env, priced, customer, meta);
    await env.SHOP_DB.prepare(`UPDATE orders SET gateway_ref = 'cs_test_1' WHERE id = ?`).bind(order.id).run();

    expect((await findOrderByGatewayRef(env, "stripe", "cs_test_1"))?.id).toBe(order.id);
    expect(await findOrderByGatewayRef(env, "stripe", "cs_test_other")).toBe(null);
    expect(await findOrderByGatewayRef(env, "paypal", "cs_test_1")).toBe(null);
  });
});
