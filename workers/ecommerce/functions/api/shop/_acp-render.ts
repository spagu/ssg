// A session row as the protocol's CheckoutSession (or CheckoutSessionWithOrder
// once it is completed). Everything an agent sees of a session is built here,
// from what is stored — nothing is recomputed on a read.

import type { Env } from "./_env";
import { ACP_PROTOCOL_VERSION } from "./_acp-http";
import { DIGITAL_OPTION_ID } from "./_acp-input";
import { paymentHandler } from "./_acp-payment";
import type { PricedState } from "./_acp-pricing";
import { parseJson, type SessionRow } from "./_acp-session";
import { acpOrder } from "./_acp-webhook";
import { allSettings } from "./_settings";

/** The one way this shop delivers: a download link, at no cost. */
const DIGITAL_OPTION = {
  type: "digital",
  id: DIGITAL_OPTION_ID,
  title: "Instant download",
  description: "A download link by email as soon as the payment succeeds.",
  totals: [{ type: "total", display_text: "Delivery", amount: 0 }],
};

export async function renderSession(env: Env, row: SessionRow, origin: string): Promise<Record<string, unknown>> {
  const state = parseJson<PricedState>(row.priced_json, { ready: false, lineItems: [], totals: [], messages: [], totalMinor: 0 });
  const buyer = parseJson<Record<string, string> | null>(row.buyer_json, null);
  const details = parseJson<Record<string, unknown> | null>(row.details_json, null);
  const order = await acpOrder(env, row, origin);
  return {
    id: row.id,
    protocol: { version: ACP_PROTOCOL_VERSION },
    capabilities: { payment: { handlers: [paymentHandler(env)] } },
    ...(buyer ? { buyer } : {}),
    status: row.status,
    currency: row.currency.toLowerCase(),
    ...(row.locale ? { locale: row.locale } : {}),
    line_items: state.lineItems,
    ...(details ? { fulfillment_details: details } : {}),
    fulfillment_options: [DIGITAL_OPTION],
    selected_fulfillment_options: [
      { type: "digital", option_id: DIGITAL_OPTION_ID, item_ids: state.lineItems.map((l) => String(l.id)) },
    ],
    totals: state.totals,
    messages: state.messages,
    links: await links(env, origin),
    created_at: row.created_at,
    updated_at: row.updated_at,
    expires_at: row.expires_at,
    ...(order ? { order } : {}),
  };
}

/** The shop's terms and privacy pages, absolute. A setting left empty is a
 *  link left out, not a link to nowhere. */
async function links(env: Env, origin: string): Promise<Array<{ type: string; url: string }>> {
  const settings = await allSettings(env);
  const out: Array<{ type: string; url: string }> = [];
  for (const [type, key] of [["terms_of_use", "legal.terms_url"], ["privacy_policy", "legal.privacy_url"]] as const) {
    const value = String(settings[key] || "");
    if (value) out.push({ type, url: new URL(value, origin).toString() });
  }
  return out;
}
