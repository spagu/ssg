// Money is integers. These tests are about the places where that stops being
// obvious: currencies that are not 1/100, and rounding a negative amount.

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  currencyDecimals,
  decimalStringToMinor,
  formatMoney,
  isKnownCurrency,
  minorToDecimalString,
  netFromGross,
  roundHalfUp,
  taxFromNet,
} from "../functions/api/shop/_money";

describe("currency decimals", () => {
  it("knows the three shapes", () => {
    assert.equal(currencyDecimals("EUR"), 2);
    assert.equal(currencyDecimals("jpy"), 0);
    assert.equal(currencyDecimals("KWD"), 3);
  });

  it("accepts any three letters as a code", () => {
    assert.equal(isKnownCurrency("eur"), true);
    assert.equal(isKnownCurrency("EURO"), false);
    assert.equal(isKnownCurrency("12"), false);
  });
});

describe("minor units on the wire", () => {
  it("formats each shape the way a payment API wants it", () => {
    assert.equal(minorToDecimalString(1999, "EUR"), "19.99");
    assert.equal(minorToDecimalString(1000, "JPY"), "1000");
    assert.equal(minorToDecimalString(1999, "KWD"), "1.999");
    assert.equal(minorToDecimalString(-250, "EUR"), "-2.50");
    assert.equal(minorToDecimalString(5, "EUR"), "0.05");
  });

  it("reads them back", () => {
    assert.equal(decimalStringToMinor("19.99", "EUR"), 1999);
    assert.equal(decimalStringToMinor("1000", "JPY"), 1000);
    assert.equal(decimalStringToMinor("1.999", "KWD"), 1999);
    assert.equal(decimalStringToMinor("-2.50", "EUR"), -250);
    // More decimals than the currency has are cut, not rounded up into it.
    assert.equal(decimalStringToMinor("19.999", "EUR"), 1999);
    assert.equal(decimalStringToMinor("19", "EUR"), 1900);
  });

  it("throws rather than returning NaN", () => {
    // A NaN amount would flow into a comparison against the order total and
    // pass silently, which is the whole reason this throws.
    assert.throws(() => decimalStringToMinor("nineteen", "EUR"));
    assert.throws(() => decimalStringToMinor("", "EUR"));
  });
});

describe("rounding", () => {
  it("goes half-up away from zero, unlike Math.round", () => {
    assert.equal(roundHalfUp(0.5), 1);
    assert.equal(roundHalfUp(1.5), 2);
    assert.equal(roundHalfUp(-0.5), -1);
    assert.equal(Math.round(-0.5), -0); // the behaviour this exists to avoid
    assert.equal(roundHalfUp(-1.5), -2);
    assert.equal(roundHalfUp(2.4), 2);
  });
});

describe("tax arithmetic", () => {
  it("adds tax to a net amount", () => {
    assert.equal(taxFromNet(10000, 2300), 2300);
    assert.equal(taxFromNet(1999, 500), 100); // 99.95 → 100
  });

  it("carves tax out of a gross amount", () => {
    // Germany's ebook rate on €24.00 gross: €22.43 net, €1.57 tax.
    const net = netFromGross(2400, 700);
    assert.equal(net, 2243);
    assert.equal(2400 - net, 157);
  });

  it("leaves a zero-rated amount alone", () => {
    assert.equal(taxFromNet(1999, 0), 0);
    assert.equal(netFromGross(1999, 0), 1999);
  });
});

describe("formatting for a human", () => {
  it("uses the reader's locale", () => {
    assert.match(formatMoney(1999, "EUR", "en"), /19\.99/);
    assert.match(formatMoney(1000, "JPY", "en"), /1,000/);
  });

  it("falls back rather than throwing on a code Intl refuses", () => {
    // Intl accepts any three letters, so this needs a genuinely malformed code
    // — the branch exists so a bad row in the database cannot take a page down.
    assert.equal(formatMoney(1999, "E$R", "en"), "19.99 E$R");
  });
});
