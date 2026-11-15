// VAT. The tests are written as the questions an auditor would ask: which rate,
// on what evidence, and why not some other rate.

import { env } from "./_env";
import { beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  decideTax,
  isEU,
  isWellFormedVatId,
  lookupRate,
  priceLine,
  resolveTaxCountry,
  taxContext,
  taxNote,
  vatIdCountry,
} from "../functions/api/shop/_tax";
import { assertPartial, freshShop } from "./_helpers";

const NOW = "2026-06-01T00:00:00.000Z";

beforeEach(async () => {
  await freshShop();
});

describe("VAT identifiers", () => {
  it("accepts a well-formed number for its own country", () => {
    assert.equal(isWellFormedVatId("PL1234567890"), true);
    assert.equal(isWellFormedVatId("DE123456789"), true);
    assert.equal(isWellFormedVatId("ATU12345678"), true);
    assert.equal(isWellFormedVatId("EL123456789"), true); // Greece writes EL
  });

  it("rejects a typo, a wrong length and a non-EU prefix", () => {
    assert.equal(isWellFormedVatId("PL12345"), false);
    assert.equal(isWellFormedVatId("DE1234567890"), false);
    assert.equal(isWellFormedVatId("XX123456789"), false);
    assert.equal(isWellFormedVatId("PL"), false);
  });

  it("reads the country from the prefix", () => {
    assert.equal(vatIdCountry("PL1234567890"), "PL");
    assert.equal(vatIdCountry("EL123456789"), "GR");
    assert.equal(vatIdCountry("ZZ1"), null);
  });

  it("knows who is in the EU", () => {
    assert.equal(isEU("de"), true);
    assert.equal(isEU("GB"), false); // since 2021
    assert.equal(isEU("US"), false);
  });
});

describe("where the customer is", () => {
  it("prefers what the customer said over what the network suggests", () => {
    const { country, evidence } = resolveTaxCountry("DE", "PL");
    assert.equal(country, "DE");
    assert.equal(evidence.billing, "DE");
    assert.equal(evidence.ip, "PL");
    // Two pieces that disagree is the case the rules care about.
    assert.equal(evidence.conflict, true);
  });

  it("falls back to the network when the customer said nothing", () => {
    assert.equal(resolveTaxCountry(null, "FR").country, "FR");
  });

  it("has no answer when neither is known", () => {
    assert.equal(resolveTaxCountry(null, null).country, null);
  });

  it("records no conflict when the two agree", () => {
    assert.ok(!(resolveTaxCountry("IT", "IT").evidence.conflict));
  });
});

describe("the rate table", () => {
  it("prefers a category rate over the standard one", async () => {
    const ebook = await lookupRate(env, "DE", "ebook", NOW);
    assert.deepEqual(ebook, { rateBp: 700, kind: "ebook" });
  });

  it("falls back to standard for a category nobody priced", async () => {
    const other = await lookupRate(env, "DE", "software", NOW);
    assert.equal(other?.kind, "standard");
    assert.equal(other?.rateBp, 1900);
  });

  it("has nothing for a country it was never given", async () => {
    assert.equal(await lookupRate(env, "US", "ebook", NOW), null);
  });

  it("answers with the rate in force on the date asked about", async () => {
    await env.SHOP_DB.prepare(
      `UPDATE vat_rates SET valid_to = ? WHERE country = 'PL' AND kind = 'ebook'`,
    )
      .bind("2026-07-01T00:00:00.000Z")
      .run();
    await env.SHOP_DB.prepare(
      `INSERT INTO vat_rates (country, kind, rate_bp, valid_from, valid_to)
       VALUES ('PL', 'ebook', 800, '2026-07-01T00:00:00.000Z', NULL)`,
    ).run();

    assert.equal((await lookupRate(env, "PL", "ebook", NOW))?.rateBp, 500);
    assert.equal((await lookupRate(env, "PL", "ebook", "2026-08-01T00:00:00.000Z"))?.rateBp, 800);
  });
});

describe("the decision", () => {
  const opts = { country: "DE", vatId: "", vatIdValid: false, taxCategory: "ebook", atISO: NOW };

  it("charges the customer's own rate", async () => {
    const ctx = await taxContext(env);
    assert.deepEqual(await decideTax(env, ctx, opts), {
      rateBp: 700,
      reverseCharge: false,
      country: "DE",
      note: "standard",
    });
  });

  it("reverse-charges a verified business in another member state", async () => {
    const ctx = await taxContext(env);
    const out = await decideTax(env, ctx, { ...opts, vatId: "DE123456789", vatIdValid: true });
    assert.deepEqual(out, { rateBp: 0, reverseCharge: true, country: "DE", note: "reverse_charge" });
  });

  it("does not reverse-charge on an unverified number", async () => {
    // A well-formed number is a string the buyer typed. Only VIES can say it is
    // real, and until it does the seller carries the tax.
    const ctx = await taxContext(env);
    const out = await decideTax(env, ctx, { ...opts, vatId: "DE123456789", vatIdValid: false });
    assertPartial(out, { reverseCharge: false, rateBp: 700 });
  });

  it("does not reverse-charge a number from the seller's own country", async () => {
    const ctx = await taxContext(env);
    const out = await decideTax(env, ctx, {
      ...opts,
      country: "PL",
      vatId: "PL1234567890",
      vatIdValid: true,
    });
    assertPartial(out, { reverseCharge: false });
  });

  it("puts a customer outside the EU outside the scope", async () => {
    const ctx = await taxContext(env);
    const out = await decideTax(env, ctx, { ...opts, country: "US" });
    assert.deepEqual(out, { rateBp: 0, reverseCharge: false, country: "US", note: "outside_scope" });
  });

  it("still charges UK VAT, which is not EU VAT", async () => {
    const ctx = await taxContext(env);
    const out = await decideTax(env, ctx, { ...opts, country: "GB" });
    assertPartial(out, { note: "standard" });
  });

  it("refuses rather than charging zero where it has no rate", async () => {
    await env.SHOP_DB.prepare(`DELETE FROM vat_rates WHERE country = 'FI'`).run();
    const ctx = await taxContext(env);
    assert.deepEqual(await decideTax(env, ctx, { ...opts, country: "FI" }), { error: "tax_rate_missing" });
  });

  it("needs to know the country before it can decide anything", async () => {
    const ctx = await taxContext(env);
    assert.deepEqual(await decideTax(env, ctx, { ...opts, country: null }), { error: "country_required" });
  });

  it("charges nothing when the seller is not registered", async () => {
    await freshShop({ "seller.vat_id": "" });
    const ctx = await taxContext(env);
    assertPartial(await decideTax(env, ctx, opts), { rateBp: 0, note: "not_registered" });
  });

  it("charges nothing in mode none", async () => {
    await freshShop({ "tax.mode": "none" });
    const ctx = await taxContext(env);
    assertPartial(await decideTax(env, ctx, opts), { rateBp: 0, note: "mode_none" });
  });

  it("leaves the tax to Stripe in mode stripe", async () => {
    await freshShop({ "tax.mode": "stripe" });
    const ctx = await taxContext(env);
    assertPartial(await decideTax(env, ctx, opts), { rateBp: 0, note: "standard" });
  });
});

describe("pricing a line", () => {
  it("carves the tax out of a gross price", () => {
    assert.deepEqual(priceLine(2400, 1, 700, "gross"), {
      unitMinor: 2243,
      taxRateBp: 700,
      taxMinor: 157,
      totalMinor: 2400,
    });
  });

  it("adds the tax to a net price", () => {
    assert.deepEqual(priceLine(2000, 2, 2300, "net"), {
      unitMinor: 2000,
      taxRateBp: 2300,
      taxMinor: 920,
      totalMinor: 4920,
    });
  });

  it("keeps the gross total exact across a quantity", () => {
    // The unit price may not divide evenly once tax is carved out; what must
    // stay exact is what the customer is charged.
    const line = priceLine(999, 3, 2300, "gross");
    assert.equal(line.totalMinor, 2997);
    assert.equal(line.taxMinor + (line.totalMinor - line.taxMinor), 2997);
  });
});

describe("the note on the invoice", () => {
  it("explains a reverse charge in both languages", () => {
    assert.match(taxNote({ note: "reverse_charge" }, "pl"), /Odwrotne obciążenie/);
    assert.match(taxNote({ note: "reverse_charge" }, "en"), /[Rr]everse charge/);
  });

  it("explains the two other cases that need explaining", () => {
    for (const note of ["outside_scope", "not_registered"] as const) {
      assert.ok((taxNote({ note }, "en").length) > 0);
      assert.ok((taxNote({ note }, "pl").length) > 0);
    }
  });

  it("says nothing about an ordinary taxed sale", () => {
    // An invoice that charges the customer's own rate needs no annotation; a
    // sentence there would be noise on every invoice the shop ever issues.
    assert.equal(taxNote({ note: "standard" }, "en"), "");
    assert.equal(taxNote({ note: "mode_none" }, "pl"), "");
  });
});
