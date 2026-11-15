// Prices a UCP session with the storefront's own code.
//
// One source of truth for what a basket costs: priceBasket in _orders.ts
// (catalogue price from D1, tax from _tax.ts, minor units from _money.ts) —
// the same function the storefront checkout and ACP call. This module only
// writes its answer in UCP's words: line items, totals and messages.
//
// Totals follow Google's "Tax-inclusive pricing" rule for a shop that prices
// gross (the default): `subtotal` already includes tax, is labelled so, and
// there is no separate `tax` line. A net-priced shop shows subtotal, tax and
// total. Either way there is exactly one `subtotal` and one `total`
// (totals.json).
//
// Message codes are the ones Google publishes ("Error codes", read
// 2026-11-14): an unknown code gets a generic error screen there.

import type { Env } from "./_env";
import { priceBasket, type PricedBasket, type PricingError } from "./_orders";
import { getSettingString } from "./_settings";
import { taxContext } from "./_tax";
import { errorMessage, type UcpMessage } from "./_ucp-http";
import { lineId, type UcpItem } from "./_ucp-session";

export interface Total {
  type: string;
  amount: number;
  display_text?: string;
}

/** What a session's priced_json holds. */
export interface UcpPriced {
  ready: boolean;
  lineItems: Array<Record<string, unknown>>;
  totals: Total[];
  messages: UcpMessage[];
  totalMinor: number;
}

/** Shown, and not dismissible, when the shop asks buyers to waive the right
 *  of withdrawal for a download (setting legal.digital_waiver). */
export const WAIVER_TEXT =
  "This is a digital download delivered immediately after payment. By completing the purchase you agree to " +
  "immediate delivery and acknowledge that you lose your right of withdrawal once the download is available.";

/** Where the platform gives the country: Google's own path for a missing
 *  destination ("Missing-information handling"). */
export const DESTINATION_PATH = "$.fulfillment.methods[0].destinations[0]";

/** An item problem means no session can be sold as asked. */
const ITEM_ERRORS = new Set<PricingError["error"]>(["empty_basket", "unknown_sku", "product_unavailable", "price_not_available"]);

export function isItemError(error: PricingError): boolean {
  return ITEM_ERRORS.has(error.error);
}

function pricingMessage(error: PricingError, items: UcpItem[]): UcpMessage {
  if (isItemError(error)) {
    const index = items.findIndex((i) => "sku" in error && i.id === error.sku);
    const content = error.error === "price_not_available" ? "This product is not sold in that currency." : "This product is not available.";
    return errorMessage("item_unavailable", content, "recoverable", index >= 0 ? `$.line_items[${index}]` : "$.line_items");
  }
  if (error.error === "country_required") {
    return errorMessage("missing_fulfillment_info", "The buyer's country is needed to work out the tax.", "recoverable", DESTINATION_PATH);
  }
  return errorMessage("address_undeliverable", "This shop does not sell to that country.", "recoverable", DESTINATION_PATH);
}

export async function priceUcp(
  env: Env,
  input: { items: UcpItem[]; currency: string; country: string | null; origin: string },
): Promise<{ state: UcpPriced; basket: PricedBasket | null; error: PricingError | null }> {
  const ctx = await taxContext(env);
  const priced = await priceBasket(env, ctx, {
    lines: input.items.map((i) => ({ sku: i.id, quantity: i.quantity })),
    currency: input.currency,
    billingCountry: input.country,
    // The call comes from the platform's servers, not the buyer, so the
    // address Cloudflare sees is no evidence of where the buyer is.
    ipCountry: null,
    vatId: "",
    vatIdValid: false,
  });

  if ("error" in priced) {
    const lineItems = input.items.map((item, i) => ({ id: lineId(i), item: { id: item.id }, quantity: item.quantity, totals: [] }));
    const state = { ready: false, lineItems, totals: [], messages: [pricingMessage(priced, input.items)], totalMinor: 0 };
    return { state, basket: null, error: priced };
  }

  const gross = ctx.pricingMode === "gross";
  const lineItems = priced.items.map((line, index) => {
    const product = priced.products[index]!;
    return {
      id: lineId(index),
      item: {
        id: line.sku,
        title: line.name,
        // The price the catalogue and the feed show: gross in gross mode (the
        // line total divides exactly), net otherwise.
        price: gross ? line.total_minor / line.quantity : line.unit_minor,
        ...(product.image_url ? { image_url: new URL(product.image_url, input.origin).toString() } : {}),
      },
      quantity: line.quantity,
      totals: breakdown(gross, line.total_minor - line.tax_minor, line.tax_minor, line.total_minor),
    };
  });

  const messages: UcpMessage[] = [];
  if ((await getSettingString(env, "legal.digital_waiver")) !== "false") {
    // A disclosure: the platform MUST show it and MUST NOT let it be
    // dismissed (checkout.md, "Warning Presentation").
    messages.push({ type: "warning", code: "digital_waiver", presentation: "disclosure", content_type: "plain", content: WAIVER_TEXT });
  }
  return {
    state: { ready: true, lineItems, totals: breakdown(gross, priced.totalMinor - priced.taxMinor, priced.taxMinor, priced.totalMinor), messages, totalMinor: priced.totalMinor },
    basket: priced,
    error: null,
  };
}

/** Subtotal, tax and total — or, priced gross, a subtotal that includes the
 *  tax and no tax line (Google, "Tax-inclusive pricing"). */
export function breakdown(gross: boolean, net: number, tax: number, total: number): Total[] {
  if (gross) {
    return [
      { type: "subtotal", display_text: "Subtotal (including taxes)", amount: total },
      { type: "total", display_text: "Total", amount: total },
    ];
  }
  return [
    { type: "subtotal", display_text: "Subtotal", amount: net },
    { type: "tax", display_text: "Tax", amount: tax },
    { type: "total", display_text: "Total", amount: total },
  ];
}
