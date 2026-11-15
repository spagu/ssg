// Pricing a basket and turning it into an order. This is the file that proves
// the browser has no say in what anything costs.

import { env } from "./_env";
import { beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
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
import { assertPartial, freshShop, seedProduct } from "./_helpers";

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
    assert.equal(priced.totalMinor, 2400);
    assert.equal(priced.taxMinor, 157); // Germany's 7% ebook rate, carved out
    assert.equal(priced.subtotalMinor, 2243);
    assert.equal(priced.taxCountry, "DE");
  });

  it("adds up several lines and several copies", async () => {
    await seedProduct({ sku: "A", priceMinor: 1000 });
    await seedProduct({ sku: "B", priceMinor: 2500 });
    const priced = (await price([
      { sku: "A", quantity: 2 },
      { sku: "B", quantity: 1 },
    ])) as PricedBasket;
    assert.equal(priced.totalMinor, 4500);
    assert.equal((priced.items).length, 2);
  });

  it("refuses an empty basket", async () => {
    assert.deepEqual(await price([]), { error: "empty_basket" });
  });

  it("refuses a product that does not exist", async () => {
    assert.deepEqual(await price([{ sku: "NOPE", quantity: 1 }]), { error: "unknown_sku", sku: "NOPE" });
  });

  it("refuses a draft, which is not for sale", async () => {
    const { id } = await seedProduct({ sku: "DRAFT" });
    await env.SHOP_DB.prepare(`UPDATE products SET status = 'draft' WHERE id = ?`).bind(id).run();
    assert.deepEqual(await price([{ sku: "DRAFT", quantity: 1 }]), {
      error: "product_unavailable",
      sku: "DRAFT",
    });
  });

  it("refuses a currency the product has no price in", async () => {
    await seedProduct({ sku: "EBOOK-1", currency: "EUR" });
    assert.deepEqual(await price([{ sku: "EBOOK-1", quantity: 1 }], { currency: "PLN" }), {
      error: "price_not_available",
      sku: "EBOOK-1",
      currency: "PLN",
    });
  });

  it("refuses a country it has no rate for rather than charging nothing", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    await env.SHOP_DB.prepare(`DELETE FROM vat_rates WHERE country = 'DE'`).run();
    assert.deepEqual(await price([{ sku: "EBOOK-1", quantity: 1 }]), {
      error: "tax_rate_missing",
      country: "DE",
    });
  });

  it("needs a country before it can price anything", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    assert.deepEqual(await price([{ sku: "EBOOK-1", quantity: 1 }], { billingCountry: null }), {
      error: "country_required",
    });
  });

  it("refuses a country the shop chose not to sell to", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    await putSettings(env, { "shop.countries_allowed": ["PL"] });
    assert.deepEqual(await price([{ sku: "EBOOK-1", quantity: 1 }]), {
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
    assert.equal(priced.reverseCharge, true);
    assert.equal(priced.taxMinor, 0);
    // Gross pricing with no tax means the customer pays the same number with
    // nothing carved out of it.
    assert.equal(priced.totalMinor, 2400);
  });

  it("records the evidence the decision rested on", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const priced = (await price([{ sku: "EBOOK-1", quantity: 1 }], {
      billingCountry: "DE",
      ipCountry: "PL",
    })) as PricedBasket;
    assertPartial(priced.evidence, { billing: "DE", ip: "PL", conflict: true });
  });
});

describe("the countries setting", () => {
  it("reads a list or a comma-separated string, and empty means everywhere", async () => {
    await putSettings(env, { "shop.countries_allowed": ["PL", "de"] });
    assert.deepEqual(await allowedCountries(env), ["PL", "DE"]);

    await env.SHOP_DB.prepare(`UPDATE settings SET value = '"PL, FR"' WHERE key = 'shop.countries_allowed'`).run();
    const { invalidateSettings } = await import("../functions/api/shop/_settings");
    invalidateSettings();
    assert.deepEqual(await allowedCountries(env), ["PL", "FR"]);

    await putSettings(env, { "shop.countries_allowed": [] });
    assert.deepEqual(await allowedCountries(env), []);
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
    assert.equal(second.id, first.id);
    // Nothing already known is erased by a later purchase that omitted it.
    assert.equal(second.name, "A Buyer");
    assert.equal(second.country, "DE");
    assert.equal(second.vat_id, "DE123456789");
  });
});

describe("orders", () => {
  it("numbers them within the year", async () => {
    const year = new Date().getUTCFullYear();
    assert.equal(await nextOrderNumber(env), `O-${year}-000001`);
  });

  it("stores only the hash of the key it hands the buyer", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const priced = (await price([{ sku: "EBOOK-1", quantity: 1 }])) as PricedBasket;
    const customer = await upsertCustomer(env, {
      email: "b@example.com", name: "B", vatId: "", vatIdValid: null, country: "DE",
    });
    const { order, items, orderKey } = await createOrder(env, priced, customer, meta);

    assert.ok(!(order.key_hash).includes(orderKey));
    assert.equal(await orderKeyMatches(order, orderKey), true);
    assert.equal(await orderKeyMatches(order, "wrong"), false);
    assert.equal(await orderKeyMatches(order, ""), false);

    assertPartial(await getOrder(env, order.id), { id: order.id, status: "pending" });
    assert.equal((await getOrderItems(env, order.id)).length, items.length);
    assert.equal(await getOrder(env, "no-such-order"), null);
  });

  it("finds an order by what the provider calls it", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const priced = (await price([{ sku: "EBOOK-1", quantity: 1 }])) as PricedBasket;
    const customer = await upsertCustomer(env, {
      email: "b@example.com", name: "B", vatId: "", vatIdValid: null, country: "DE",
    });
    const { order } = await createOrder(env, priced, customer, meta);
    await env.SHOP_DB.prepare(`UPDATE orders SET gateway_ref = 'cs_test_1' WHERE id = ?`).bind(order.id).run();

    assert.equal((await findOrderByGatewayRef(env, "stripe", "cs_test_1"))?.id, order.id);
    assert.equal(await findOrderByGatewayRef(env, "stripe", "cs_test_other"), null);
    assert.equal(await findOrderByGatewayRef(env, "paypal", "cs_test_1"), null);
  });
});
