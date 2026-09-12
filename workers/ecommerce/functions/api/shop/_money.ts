// Money. Integers in the currency's minor unit, never floats.
//
// 1999 EUR-minor is 19.99 €. Floats in prices produce rounding errors in VAT
// invoices, and the seller is the one who answers for those. Stripe and PayPal
// both count in minor units too, so this is also the shape the wire wants.

/** Currencies whose minor unit is not 1/100. The long tail (Kuwaiti dinar and
 *  friends, 3 decimals) is deliberately absent: the shop refuses a currency it
 *  does not know how to divide rather than guessing at two decimals. */
const ZERO_DECIMAL = new Set([
  "BIF", "CLP", "DJF", "GNF", "JPY", "KMF", "KRW", "MGA", "PYG",
  "RWF", "UGX", "VND", "VUV", "XAF", "XOF", "XPF",
]);

const THREE_DECIMAL = new Set(["BHD", "IQD", "JOD", "KWD", "LYD", "OMR", "TND"]);

export function currencyDecimals(currency: string): number {
  const c = currency.toUpperCase();
  if (ZERO_DECIMAL.has(c)) return 0;
  if (THREE_DECIMAL.has(c)) return 3;
  return 2;
}

export function isKnownCurrency(currency: string): boolean {
  return /^[A-Z]{3}$/.test(currency.toUpperCase());
}

/** Minor units to the decimal string a payment API wants ("19.99", "1000"). */
export function minorToDecimalString(amountMinor: number, currency: string): string {
  const d = currencyDecimals(currency);
  if (d === 0) return String(Math.round(amountMinor));
  const sign = amountMinor < 0 ? "-" : "";
  const abs = Math.abs(Math.round(amountMinor));
  const factor = 10 ** d;
  const whole = Math.floor(abs / factor);
  const frac = String(abs % factor).padStart(d, "0");
  return `${sign}${whole}.${frac}`;
}

/** The inverse, for reading amounts back from a gateway. Throws on nonsense
 *  rather than returning NaN, because a NaN amount would flow into a
 *  comparison that then silently passes. */
export function decimalStringToMinor(value: string, currency: string): number {
  const d = currencyDecimals(currency);
  const m = /^(-?)(\d+)(?:\.(\d+))?$/.exec(value.trim());
  if (!m) throw new Error(`not a decimal amount: ${value}`);
  const sign = m[1] === "-" ? -1 : 1;
  const whole = Number.parseInt(m[2] ?? "0", 10);
  const fracRaw = (m[3] ?? "").padEnd(d, "0").slice(0, d);
  const frac = d === 0 ? 0 : Number.parseInt(fracRaw || "0", 10);
  return sign * (whole * 10 ** d + frac);
}

/** Half-up away from zero: 0.5 rounds to 1, -0.5 to -1.
 *
 *  This is what most tax administrations mean by "rounded", and it is NOT what
 *  JavaScript's Math.round does for negative numbers (it rounds -0.5 to -0).
 *  Credit notes carry negative amounts, so the difference is not theoretical. */
export function roundHalfUp(value: number): number {
  return value < 0 ? -Math.round(-value) : Math.round(value);
}

/** Tax on a net amount, at a rate in basis points (2300 = 23%). */
export function taxFromNet(netMinor: number, rateBp: number): number {
  return roundHalfUp((netMinor * rateBp) / 10000);
}

/** Splits a gross amount into net and tax. Used when the shop prices in gross,
 *  which is what a seller of ebooks in the EU usually wants: one price on the
 *  page for every country, the VAT share differing underneath. */
export function netFromGross(grossMinor: number, rateBp: number): number {
  return roundHalfUp((grossMinor * 10000) / (10000 + rateBp));
}

/** Formats for a human, in the reader's locale. Only used in emails and
 *  invoices; the panel formats in the browser with Intl. */
export function formatMoney(amountMinor: number, currency: string, locale = "en"): string {
  const d = currencyDecimals(currency);
  try {
    return new Intl.NumberFormat(locale, {
      style: "currency",
      currency: currency.toUpperCase(),
      minimumFractionDigits: d,
      maximumFractionDigits: d,
    }).format(amountMinor / 10 ** d);
  } catch {
    return `${minorToDecimalString(amountMinor, currency)} ${currency.toUpperCase()}`;
  }
}
