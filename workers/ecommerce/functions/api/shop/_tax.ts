// VAT.
//
// This is a mechanism, not tax advice. The seller sets the mode and the rates;
// the shop applies them, records which rate it applied and on what evidence,
// and refuses rather than guessing when it cannot tell.
//
// Three modes (settings 'tax.mode'):
//   none    prices are final, tax 0 — a seller below a threshold or outside scope
//   table   this module, from the vat_rates table (the default)
//   stripe  Stripe Tax computes it in Checkout; we record what it returns

import type { Env } from "./_env";
import { netFromGross, roundHalfUp, taxFromNet } from "./_money";
import { getSetting } from "./_settings";

export type TaxMode = "none" | "table" | "stripe";

export interface TaxEvidence {
  /** What the customer said their billing country is. Always present. */
  billing: string | null;
  /** What Cloudflare resolved from the IP at checkout. */
  ip: string | null;
  /** The card or account country, filled in later from the webhook. */
  card?: string | null;
  /** Set when billing and ip disagree; resolved once the card country arrives. */
  conflict?: boolean;
}

const EU = new Set([
  "AT", "BE", "BG", "HR", "CY", "CZ", "DK", "EE", "FI", "FR", "DE", "GR", "HU",
  "IE", "IT", "LV", "LT", "LU", "MT", "NL", "PL", "PT", "RO", "SK", "SI", "ES", "SE",
]);

export const isEU = (country: string): boolean => EU.has(country.toUpperCase());

/** VAT id shapes, country by country. Enough to reject a typo before anyone
 *  relies on it; only VIES can say whether a well-formed number is real, and
 *  that is a separate module (ECOM-019). */
const VAT_ID_RE: Record<string, RegExp> = {
  AT: /^ATU\d{8}$/, BE: /^BE0\d{9}$/, BG: /^BG\d{9,10}$/, HR: /^HR\d{11}$/,
  CY: /^CY\d{8}[A-Z]$/, CZ: /^CZ\d{8,10}$/, DK: /^DK\d{8}$/, EE: /^EE\d{9}$/,
  FI: /^FI\d{8}$/, FR: /^FR[A-Z0-9]{2}\d{9}$/, DE: /^DE\d{9}$/, GR: /^(EL|GR)\d{9}$/,
  HU: /^HU\d{8}$/, IE: /^IE(\d{7}[A-W]{1,2}|\d[A-Z*+]\d{5}[A-W])$/, IT: /^IT\d{11}$/,
  LV: /^LV\d{11}$/, LT: /^LT(\d{9}|\d{12})$/, LU: /^LU\d{8}$/, MT: /^MT\d{8}$/,
  NL: /^NL\d{9}B\d{2}$/, PL: /^PL\d{10}$/, PT: /^PT\d{9}$/, RO: /^RO\d{2,10}$/,
  SK: /^SK\d{10}$/, SI: /^SI\d{8}$/, ES: /^ES[A-Z0-9]\d{7}[A-Z0-9]$/, SE: /^SE\d{12}$/,
};

/** Well-formed for its own country prefix. A number whose prefix is not an EU
 *  country is not an EU VAT id, whatever else it may be. */
export function isWellFormedVatId(vatId: string): boolean {
  if (vatId.length < 4) return false;
  const prefix = vatId.slice(0, 2).toUpperCase();
  const country = prefix === "EL" ? "GR" : prefix;
  const re = VAT_ID_RE[country];
  return re ? re.test(vatId.toUpperCase()) : false;
}

export function vatIdCountry(vatId: string): string | null {
  const prefix = vatId.slice(0, 2).toUpperCase();
  const country = prefix === "EL" ? "GR" : prefix;
  return EU.has(country) ? country : null;
}

export interface TaxContext {
  mode: TaxMode;
  /** Where the seller is established — reverse charge needs to know. */
  sellerCountry: string;
  /** Whether the seller is VAT-registered at all. */
  sellerHasVat: boolean;
  /** Prices in the catalogue are gross (tax included) or net. */
  pricingMode: "gross" | "net";
}

export async function taxContext(env: Env): Promise<TaxContext> {
  const mode = ((await getSetting(env, "tax.mode")) as TaxMode) || "table";
  return {
    mode: mode === "none" || mode === "stripe" ? mode : "table",
    sellerCountry: String((await getSetting(env, "seller.country")) || "").toUpperCase(),
    sellerHasVat: Boolean(await getSetting(env, "seller.vat_id")),
    pricingMode: ((await getSetting(env, "pricing.mode")) as "gross" | "net") === "net" ? "net" : "gross",
  };
}

/** Which country's rate applies, and the evidence for it.
 *
 *  For digital services sold to a consumer the place of supply is the
 *  customer's country, and the EU expects two non-contradictory pieces of
 *  evidence. We have the billing country the customer typed and the country
 *  Cloudflare resolved from their address; the card country arrives later with
 *  the webhook and either settles or confirms a disagreement. */
export function resolveTaxCountry(billing: string | null, ip: string | null): { country: string | null; evidence: TaxEvidence } {
  const b = billing ? billing.toUpperCase() : null;
  const i = ip ? ip.toUpperCase() : null;
  const evidence: TaxEvidence = { billing: b, ip: i };
  if (b && i && b !== i) evidence.conflict = true;
  return { country: b ?? i, evidence };
}

export interface RateLookup {
  rateBp: number;
  kind: string;
}

/** The rate in force for a country and product category on a given date.
 *
 *  Falls back from the product's own category to the standard rate, and then
 *  refuses: a missing rate is a country the seller has not thought about, and
 *  charging 0% there is a decision nobody made. */
export async function lookupRate(
  env: Env,
  country: string,
  kind: string,
  atISO: string,
): Promise<RateLookup | null> {
  const row = await env.SHOP_DB.prepare(
    `SELECT rate_bp, kind FROM vat_rates
      WHERE country = ? AND kind IN (?, 'standard')
        AND valid_from <= ? AND (valid_to IS NULL OR valid_to > ?)
      ORDER BY CASE kind WHEN 'standard' THEN 1 ELSE 0 END, valid_from DESC
      LIMIT 1`,
  )
    .bind(country.toUpperCase(), kind, atISO, atISO)
    .first<{ rate_bp: number; kind: string }>();
  return row ? { rateBp: row.rate_bp, kind: row.kind } : null;
}

export interface TaxDecision {
  /** 0 when out of scope, exempt or reverse charged. */
  rateBp: number;
  reverseCharge: boolean;
  country: string | null;
  /** Why the rate is what it is — shown on the invoice as an annotation. */
  note: "standard" | "reverse_charge" | "outside_scope" | "not_registered" | "mode_none";
}

/** Decides the rate for one order.
 *
 *  The order of the questions is the order the rules apply in: a seller with no
 *  VAT registration charges none; a business customer in another member state
 *  reverse-charges; a customer outside the EU is outside the scope of EU VAT;
 *  everyone else pays their own country's rate. */
export async function decideTax(
  env: Env,
  ctx: TaxContext,
  opts: { country: string | null; vatId: string; vatIdValid: boolean; taxCategory: string; atISO: string },
): Promise<TaxDecision | { error: string }> {
  if (ctx.mode === "none") return { rateBp: 0, reverseCharge: false, country: opts.country, note: "mode_none" };
  if (ctx.mode === "stripe") return { rateBp: 0, reverseCharge: false, country: opts.country, note: "standard" };
  if (!ctx.sellerHasVat) {
    return { rateBp: 0, reverseCharge: false, country: opts.country, note: "not_registered" };
  }

  const country = opts.country?.toUpperCase() ?? null;
  if (!country) return { error: "country_required" };

  // B2B inside the EU, in a different member state: the customer accounts for
  // the tax, and the invoice says so.
  const idCountry = opts.vatId ? vatIdCountry(opts.vatId) : null;
  if (
    opts.vatId &&
    opts.vatIdValid &&
    idCountry &&
    isEU(ctx.sellerCountry) &&
    idCountry !== ctx.sellerCountry
  ) {
    return { rateBp: 0, reverseCharge: true, country: idCountry, note: "reverse_charge" };
  }

  // Outside the EU and the UK: no EU VAT on a digital service.
  if (!isEU(country) && country !== "GB") {
    return { rateBp: 0, reverseCharge: false, country, note: "outside_scope" };
  }

  const rate = await lookupRate(env, country, opts.taxCategory, opts.atISO);
  if (!rate) return { error: "tax_rate_missing" };
  return { rateBp: rate.rateBp, reverseCharge: false, country, note: "standard" };
}

export interface PricedLine {
  unitMinor: number;
  taxRateBp: number;
  taxMinor: number;
  totalMinor: number;
}

/** Turns a catalogue price into a line: net unit, tax and gross total.
 *
 *  In gross mode the catalogue price is what the customer pays and the tax is
 *  carved out of it — which is what a seller of ebooks across the EU normally
 *  wants: one price on the page, the VAT share differing by country. */
export function priceLine(catalogueMinor: number, quantity: number, rateBp: number, pricingMode: "gross" | "net"): PricedLine {
  if (pricingMode === "gross") {
    const grossTotal = catalogueMinor * quantity;
    const netTotal = netFromGross(grossTotal, rateBp);
    const tax = grossTotal - netTotal;
    return {
      unitMinor: roundHalfUp(netTotal / quantity),
      taxRateBp: rateBp,
      taxMinor: tax,
      totalMinor: grossTotal,
    };
  }
  const netTotal = catalogueMinor * quantity;
  const tax = taxFromNet(netTotal, rateBp);
  return { unitMinor: catalogueMinor, taxRateBp: rateBp, taxMinor: tax, totalMinor: netTotal + tax };
}

/** The sentence that goes on the invoice under the totals. */
export function taxNote(decision: Pick<TaxDecision, "note">, locale: string): string {
  const pl = locale.startsWith("pl");
  switch (decision.note) {
    case "reverse_charge":
      return pl
        ? "Odwrotne obciążenie — podatek rozlicza nabywca (art. 196 dyrektywy 2006/112/WE)."
        : "Reverse charge — VAT to be accounted for by the recipient (Article 196, Directive 2006/112/EC).";
    case "outside_scope":
      return pl ? "Usługa poza zakresem VAT UE." : "Supply outside the scope of EU VAT.";
    case "not_registered":
      return pl ? "Sprzedawca nie jest zarejestrowany jako podatnik VAT." : "The seller is not registered for VAT.";
    default:
      return "";
  }
}
