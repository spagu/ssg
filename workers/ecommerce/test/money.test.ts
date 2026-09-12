// Money is integers. These tests are about the places where that stops being
// obvious: currencies that are not 1/100, and rounding a negative amount.

import { describe, expect, it } from "vitest";
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
    expect(currencyDecimals("EUR")).toBe(2);
    expect(currencyDecimals("jpy")).toBe(0);
    expect(currencyDecimals("KWD")).toBe(3);
  });

  it("accepts any three letters as a code", () => {
    expect(isKnownCurrency("eur")).toBe(true);
    expect(isKnownCurrency("EURO")).toBe(false);
    expect(isKnownCurrency("12")).toBe(false);
  });
});

describe("minor units on the wire", () => {
  it("formats each shape the way a payment API wants it", () => {
    expect(minorToDecimalString(1999, "EUR")).toBe("19.99");
    expect(minorToDecimalString(1000, "JPY")).toBe("1000");
    expect(minorToDecimalString(1999, "KWD")).toBe("1.999");
    expect(minorToDecimalString(-250, "EUR")).toBe("-2.50");
    expect(minorToDecimalString(5, "EUR")).toBe("0.05");
  });

  it("reads them back", () => {
    expect(decimalStringToMinor("19.99", "EUR")).toBe(1999);
    expect(decimalStringToMinor("1000", "JPY")).toBe(1000);
    expect(decimalStringToMinor("1.999", "KWD")).toBe(1999);
    expect(decimalStringToMinor("-2.50", "EUR")).toBe(-250);
    // More decimals than the currency has are cut, not rounded up into it.
    expect(decimalStringToMinor("19.999", "EUR")).toBe(1999);
    expect(decimalStringToMinor("19", "EUR")).toBe(1900);
  });

  it("throws rather than returning NaN", () => {
    // A NaN amount would flow into a comparison against the order total and
    // pass silently, which is the whole reason this throws.
    expect(() => decimalStringToMinor("nineteen", "EUR")).toThrow();
    expect(() => decimalStringToMinor("", "EUR")).toThrow();
  });
});

describe("rounding", () => {
  it("goes half-up away from zero, unlike Math.round", () => {
    expect(roundHalfUp(0.5)).toBe(1);
    expect(roundHalfUp(1.5)).toBe(2);
    expect(roundHalfUp(-0.5)).toBe(-1);
    expect(Math.round(-0.5)).toBe(-0); // the behaviour this exists to avoid
    expect(roundHalfUp(-1.5)).toBe(-2);
    expect(roundHalfUp(2.4)).toBe(2);
  });
});

describe("tax arithmetic", () => {
  it("adds tax to a net amount", () => {
    expect(taxFromNet(10000, 2300)).toBe(2300);
    expect(taxFromNet(1999, 500)).toBe(100); // 99.95 → 100
  });

  it("carves tax out of a gross amount", () => {
    // Germany's ebook rate on €24.00 gross: €22.43 net, €1.57 tax.
    const net = netFromGross(2400, 700);
    expect(net).toBe(2243);
    expect(2400 - net).toBe(157);
  });

  it("leaves a zero-rated amount alone", () => {
    expect(taxFromNet(1999, 0)).toBe(0);
    expect(netFromGross(1999, 0)).toBe(1999);
  });
});

describe("formatting for a human", () => {
  it("uses the reader's locale", () => {
    expect(formatMoney(1999, "EUR", "en")).toMatch(/19\.99/);
    expect(formatMoney(1000, "JPY", "en")).toMatch(/1,000/);
  });

  it("falls back rather than throwing on a code Intl refuses", () => {
    // Intl accepts any three letters, so this needs a genuinely malformed code
    // — the branch exists so a bad row in the database cannot take a page down.
    expect(formatMoney(1999, "E$R", "en")).toBe("19.99 E$R");
  });
});
