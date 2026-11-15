// A session row as the protocol's Checkout (checkout.json). Everything the
// platform sees of a session is built here from what is stored — nothing is
// re-priced on a read.

import type { Env } from "./_env";
import { allSettings } from "./_settings";
import { CHECKOUT_CAPABILITY, UCP_VERSION } from "./_ucp-http";
import { permalink } from "./_ucp-order";
import { googlePayHandler, HANDLER_NAME } from "./_ucp-payment";
import type { UcpPriced } from "./_ucp-pricing";
import { parseJson, type UcpDetails, type UcpSessionRow } from "./_ucp-session";

/** The `ucp` envelope of every checkout response: the version, the
 *  capability confirmed and the payment handler (response_checkout_schema
 *  requires `payment_handlers`). */
export async function envelope(env: Env, origin: string): Promise<Record<string, unknown>> {
  const settings = await allSettings(env);
  return {
    version: UCP_VERSION,
    status: "success",
    capabilities: { [CHECKOUT_CAPABILITY]: [{ version: UCP_VERSION }] },
    payment_handlers: { [HANDLER_NAME]: [googlePayHandler(env, origin, merchantName(settings))] },
  };
}

/** The name on the Google Pay sheet: the seller's legal name, else the shop's. */
export function merchantName(settings: Record<string, unknown>): string {
  return String(settings["seller.name"] || settings["shop.name"] || "Shop");
}

export async function renderSession(env: Env, row: UcpSessionRow, origin: string): Promise<Record<string, unknown>> {
  const state = parseJson<UcpPriced>(row.priced_json, { ready: false, lineItems: [], totals: [], messages: [], totalMinor: 0 });
  const buyer = parseJson<Record<string, string> | null>(row.buyer_json, null);
  const details = parseJson<UcpDetails>(row.details_json, {});
  const completed = row.status === "completed" && row.order_id;
  const label = completed
    ? await env.SHOP_DB.prepare(`SELECT number FROM orders WHERE id = ?`).bind(row.order_id).first<{ number: string }>()
    : null;
  return {
    ucp: await envelope(env, origin),
    id: row.id,
    status: row.status,
    currency: row.currency,
    ...(buyer && Object.keys(buyer).length ? { buyer } : {}),
    line_items: state.lineItems,
    totals: state.totals,
    messages: state.messages,
    links: await links(env, origin),
    expires_at: row.expires_at,
    // Always present, empty until the platform has paid: platforms (and the
    // conformance suite) read payment.instruments back on every update.
    payment: { instruments: details.instrument ? [details.instrument] : [] },
    ...(completed
      ? { order: { id: row.order_id, ...(label ? { label: label.number } : {}), permalink_url: permalink(origin, row.order_id!, row.order_key) } }
      : {}),
  };
}

/** The shop's terms and privacy pages, absolute. Google shows both on its
 *  checkout and asks for both ("links … must include privacy_policy and
 *  terms_of_service"); a setting left empty is a link left out rather than
 *  a link to nowhere — UCP.md tells the owner to set both. */
async function links(env: Env, origin: string): Promise<Array<{ type: string; url: string }>> {
  const settings = await allSettings(env);
  const out: Array<{ type: string; url: string }> = [];
  for (const [type, key] of [["terms_of_service", "legal.terms_url"], ["privacy_policy", "legal.privacy_url"]] as const) {
    const value = String(settings[key] || "");
    if (value) out.push({ type, url: new URL(value, origin).toString() });
  }
  return out;
}
