// Prices an ACP session with the storefront's own code.
//
// There is one source of truth for what a basket costs: priceBasket in
// _orders.ts (catalogue price from D1, tax from _tax.ts, minor units from
// _money.ts). This module only asks it and writes the answer in the
// protocol's words — line items, totals and messages. An agent's idea of a
// price (Item.unit_amount) is compared, never used.
//
// Inventory: the shop sells digital files, so a product that exists, is
// active and has a price in the currency can always be sold. Stock for
// physical goods would be the merchant's own system; this shop refuses
// physical products at checkout today (priceBasket), here as well.

import type { Env } from "./_env";
import { priceBasket, type PricedBasket, type PricingError } from "./_orders";
import type { AcpItem } from "./_acp-session";
import { getSettingString } from "./_settings";
import { taxContext } from "./_tax";

export interface Total {
  type: string;
  display_text: string;
  amount: number;
}

export type Message = Record<string, unknown> & { type: "info" | "warning" | "error"; content_type: "plain"; content: string };

/** What a session's priced_json holds. */
export interface PricedState {
  ready: boolean;
  lineItems: Array<Record<string, unknown>>;
  totals: Total[];
  messages: Message[];
  totalMinor: number;
}

/** Shown with every line when the shop asks buyers to waive the right of
 *  withdrawal for a digital download (setting legal.digital_waiver). */
export const WAIVER_TEXT =
  "This is a digital download delivered immediately after payment. By completing the purchase you agree to " +
  "immediate delivery and acknowledge that you lose your right of withdrawal once the download is available.";

type Where = { code: string; param: (err: PricingError, items: AcpItem[]) => string; text: string; resolution: string };

const at = (err: PricingError, items: AcpItem[]): string =>
  `$.line_items[${items.findIndex((i) => "sku" in err && i.id === err.sku)}]`;
const country = (): string => "$.fulfillment_details.address.country";

/** The storefront's pricing errors as the protocol's MessageError. */
const PRICING_MESSAGES: Record<PricingError["error"], Where> = {
  empty_basket: { code: "invalid", param: () => "$.line_items", text: "The basket is empty.", resolution: "recoverable" },
  unknown_sku: { code: "not_found", param: at, text: "This shop has no such product.", resolution: "recoverable" },
  product_unavailable: { code: "out_of_stock", param: at, text: "This product is not on sale.", resolution: "recoverable" },
  price_not_available: { code: "invalid", param: () => "$.currency", text: "This product is not sold in that currency.", resolution: "recoverable" },
  country_required: { code: "missing", param: country, text: "The buyer's country is needed to work out the tax.", resolution: "requires_buyer_input" },
  tax_rate_missing: { code: "region_restricted", param: country, text: "This shop does not sell to that country yet.", resolution: "requires_buyer_input" },
  country_not_served: { code: "region_restricted", param: country, text: "This shop does not sell to that country.", resolution: "requires_buyer_input" },
};

export async function priceSession(
  env: Env,
  input: { items: AcpItem[]; currency: string; country: string | null; origin: string },
): Promise<{ state: PricedState; basket: PricedBasket | null }> {
  const ctx = await taxContext(env);
  const priced = await priceBasket(env, ctx, {
    lines: input.items.map((i) => ({ sku: i.id, quantity: i.quantity })),
    currency: input.currency,
    billingCountry: input.country,
    // The request comes from the platform's servers, not the buyer, so the
    // address Cloudflare sees is no evidence of where the buyer is.
    ipCountry: null,
    vatId: "",
    vatIdValid: false,
  });

  if ("error" in priced) {
    const where = PRICING_MESSAGES[priced.error];
    const message: Message = {
      type: "error", code: where.code, severity: "high", resolution: where.resolution,
      param: where.param(priced, input.items), content_type: "plain", content: where.text,
    };
    const lineItems = input.items.map((i) => ({ id: i.id, item: { id: i.id }, quantity: i.quantity, totals: [] }));
    return { state: { ready: false, lineItems, totals: [], messages: [message], totalMinor: 0 }, basket: null };
  }

  const waiver = (await getSettingString(env, "legal.digital_waiver")) !== "false";
  const messages: Message[] = [];
  const lineItems = priced.items.map((line, index) => {
    // The price the catalogue and the feed show: gross in gross mode (the
    // line total divides exactly), net otherwise.
    const catalogue = ctx.pricingMode === "gross" ? line.total_minor / line.quantity : line.unit_minor;
    const asked = input.items[index]!.unit_amount;
    if (asked !== undefined && asked !== catalogue) {
      messages.push({
        type: "warning", code: "price_change", severity: "medium", resolution: "requires_buyer_review",
        param: `$.line_items[${index}]`, content_type: "plain",
        content: `The price of ${line.name} is ${catalogue} in minor units of ${priced.currency}, not ${asked}.`,
      });
    }
    const product = priced.products[index]!;
    return {
      id: line.sku,
      item: { id: line.sku, name: line.name, unit_amount: catalogue },
      quantity: line.quantity,
      name: line.name,
      ...(product.description ? { description: product.description } : {}),
      ...(product.image_url ? { images: [new URL(product.image_url, input.origin).toString()] } : {}),
      unit_amount: catalogue,
      ...(waiver ? { disclosures: [{ type: "disclaimer", content_type: "plain", content: WAIVER_TEXT }] } : {}),
      product_id: product.id,
      sku: line.sku,
      availability_status: "in_stock",
      totals: lineTotals(line.total_minor - line.tax_minor, line.tax_minor, line.total_minor),
    };
  });
  if (waiver) {
    messages.push({ type: "info", severity: "high", resolution: "requires_buyer_review", content_type: "plain", content: WAIVER_TEXT });
  }

  return {
    state: {
      ready: true,
      lineItems,
      totals: lineTotals(priced.totalMinor - priced.taxMinor, priced.taxMinor, priced.totalMinor),
      messages,
      totalMinor: priced.totalMinor,
    },
    basket: priced,
  };
}

/** Subtotal (net), tax and total. Net is total minus tax, so the three always
 *  add up, whichever way the catalogue is priced. */
function lineTotals(net: number, tax: number, total: number): Total[] {
  return [
    { type: "subtotal", display_text: "Subtotal", amount: net },
    { type: "tax", display_text: "Tax", amount: tax },
    { type: "total", display_text: "Total", amount: total },
  ];
}
