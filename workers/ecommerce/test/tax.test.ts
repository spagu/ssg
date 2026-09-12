// VAT. The tests are written as the questions an auditor would ask: which rate,
// on what evidence, and why not some other rate.

import { env } from "cloudflare:test";
import { beforeEach, describe, expect, it } from "vitest";
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
import { freshShop } from "./_helpers";

const NOW = "2026-06-01T00:00:00.000Z";

beforeEach(async () => {
  await freshShop();
});

describe("VAT identifiers", () => {
  it("accepts a well-formed number for its own country", () => {
    expect(isWellFormedVatId("PL1234567890")).toBe(true);
    expect(isWellFormedVatId("DE123456789")).toBe(true);
    expect(isWellFormedVatId("ATU12345678")).toBe(true);
    expect(isWellFormedVatId("EL123456789")).toBe(true); // Greece writes EL
  });

  it("rejects a typo, a wrong length and a non-EU prefix", () => {
    expect(isWellFormedVatId("PL12345")).toBe(false);
    expect(isWellFormedVatId("DE1234567890")).toBe(false);
    expect(isWellFormedVatId("XX123456789")).toBe(false);
    expect(isWellFormedVatId("PL")).toBe(false);
  });

  it("reads the country from the prefix", () => {
    expect(vatIdCountry("PL1234567890")).toBe("PL");
    expect(vatIdCountry("EL123456789")).toBe("GR");
    expect(vatIdCountry("ZZ1")).toBe(null);
  });

  it("knows who is in the EU", () => {
    expect(isEU("de")).toBe(true);
    expect(isEU("GB")).toBe(false); // since 2021
    expect(isEU("US")).toBe(false);
  });
});

describe("where the customer is", () => {
  it("prefers what the customer said over what the network suggests", () => {
    const { country, evidence } = resolveTaxCountry("DE", "PL");
    expect(country).toBe("DE");
    expect(evidence.billing).toBe("DE");
    expect(evidence.ip).toBe("PL");
    // Two pieces that disagree is the case the rules care about.
    expect(evidence.conflict).toBe(true);
  });

  it("falls back to the network when the customer said nothing", () => {
    expect(resolveTaxCountry(null, "FR").country).toBe("FR");
  });

  it("has no answer when neither is known", () => {
    expect(resolveTaxCountry(null, null).country).toBe(null);
  });

  it("records no conflict when the two agree", () => {
    expect(resolveTaxCountry("IT", "IT").evidence.conflict).toBeFalsy();
  });
});

describe("the rate table", () => {
  it("prefers a category rate over the standard one", async () => {
    const ebook = await lookupRate(env, "DE", "ebook", NOW);
    expect(ebook).toEqual({ rateBp: 700, kind: "ebook" });
  });

  it("falls back to standard for a category nobody priced", async () => {
    const other = await lookupRate(env, "DE", "software", NOW);
    expect(other?.kind).toBe("standard");
    expect(other?.rateBp).toBe(1900);
  });

  it("has nothing for a country it was never given", async () => {
    expect(await lookupRate(env, "US", "ebook", NOW)).toBe(null);
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

    expect((await lookupRate(env, "PL", "ebook", NOW))?.rateBp).toBe(500);
    expect((await lookupRate(env, "PL", "ebook", "2026-08-01T00:00:00.000Z"))?.rateBp).toBe(800);
  });
});

describe("the decision", () => {
  const opts = { country: "DE", vatId: "", vatIdValid: false, taxCategory: "ebook", atISO: NOW };

  it("charges the customer's own rate", async () => {
    const ctx = await taxContext(env);
    expect(await decideTax(env, ctx, opts)).toEqual({
      rateBp: 700,
      reverseCharge: false,
      country: "DE",
      note: "standard",
    });
  });

  it("reverse-charges a verified business in another member state", async () => {
    const ctx = await taxContext(env);
    const out = await decideTax(env, ctx, { ...opts, vatId: "DE123456789", vatIdValid: true });
    expect(out).toEqual({ rateBp: 0, reverseCharge: true, country: "DE", note: "reverse_charge" });
  });

  it("does not reverse-charge on an unverified number", async () => {
    // A well-formed number is a string the buyer typed. Only VIES can say it is
    // real, and until it does the seller carries the tax.
    const ctx = await taxContext(env);
    const out = await decideTax(env, ctx, { ...opts, vatId: "DE123456789", vatIdValid: false });
    expect(out).toMatchObject({ reverseCharge: false, rateBp: 700 });
  });

  it("does not reverse-charge a number from the seller's own country", async () => {
    const ctx = await taxContext(env);
    const out = await decideTax(env, ctx, {
      ...opts,
      country: "PL",
      vatId: "PL1234567890",
      vatIdValid: true,
    });
    expect(out).toMatchObject({ reverseCharge: false });
  });

  it("puts a customer outside the EU outside the scope", async () => {
    const ctx = await taxContext(env);
    const out = await decideTax(env, ctx, { ...opts, country: "US" });
    expect(out).toEqual({ rateBp: 0, reverseCharge: false, country: "US", note: "outside_scope" });
  });

  it("still charges UK VAT, which is not EU VAT", async () => {
    const ctx = await taxContext(env);
    const out = await decideTax(env, ctx, { ...opts, country: "GB" });
    expect(out).toMatchObject({ note: "standard" });
  });

  it("refuses rather than charging zero where it has no rate", async () => {
    await env.SHOP_DB.prepare(`DELETE FROM vat_rates WHERE country = 'FI'`).run();
    const ctx = await taxContext(env);
    expect(await decideTax(env, ctx, { ...opts, country: "FI" })).toEqual({ error: "tax_rate_missing" });
  });

  it("needs to know the country before it can decide anything", async () => {
    const ctx = await taxContext(env);
    expect(await decideTax(env, ctx, { ...opts, country: null })).toEqual({ error: "country_required" });
  });

  it("charges nothing when the seller is not registered", async () => {
    await freshShop({ "seller.vat_id": "" });
    const ctx = await taxContext(env);
    expect(await decideTax(env, ctx, opts)).toMatchObject({ rateBp: 0, note: "not_registered" });
  });

  it("charges nothing in mode none", async () => {
    await freshShop({ "tax.mode": "none" });
    const ctx = await taxContext(env);
    expect(await decideTax(env, ctx, opts)).toMatchObject({ rateBp: 0, note: "mode_none" });
  });

  it("leaves the tax to Stripe in mode stripe", async () => {
    await freshShop({ "tax.mode": "stripe" });
    const ctx = await taxContext(env);
    expect(await decideTax(env, ctx, opts)).toMatchObject({ rateBp: 0, note: "standard" });
  });
});

describe("pricing a line", () => {
  it("carves the tax out of a gross price", () => {
    expect(priceLine(2400, 1, 700, "gross")).toEqual({
      unitMinor: 2243,
      taxRateBp: 700,
      taxMinor: 157,
      totalMinor: 2400,
    });
  });

  it("adds the tax to a net price", () => {
    expect(priceLine(2000, 2, 2300, "net")).toEqual({
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
    expect(line.totalMinor).toBe(2997);
    expect(line.taxMinor + (line.totalMinor - line.taxMinor)).toBe(2997);
  });
});

describe("the note on the invoice", () => {
  it("explains a reverse charge in both languages", () => {
    expect(taxNote({ note: "reverse_charge" }, "pl")).toMatch(/Odwrotne obciążenie/);
    expect(taxNote({ note: "reverse_charge" }, "en")).toMatch(/[Rr]everse charge/);
  });

  it("explains the two other cases that need explaining", () => {
    for (const note of ["outside_scope", "not_registered"] as const) {
      expect(taxNote({ note }, "en").length).toBeGreaterThan(0);
      expect(taxNote({ note }, "pl").length).toBeGreaterThan(0);
    }
  });

  it("says nothing about an ordinary taxed sale", () => {
    // An invoice that charges the customer's own rate needs no annotation; a
    // sentence there would be noise on every invoice the shop ever issues.
    expect(taxNote({ note: "standard" }, "en")).toBe("");
    expect(taxNote({ note: "mode_none" }, "pl")).toBe("");
  });
});
