// Server-side conversion tracking: GA4 and Meta.
//
// Server-side because it survives ad blockers and needs no script on the thank
// you page; queued because Google being slow must not delay the email carrying
// the buyer's file.
//
// It only ever runs for an order whose buyer consented at checkout. The consent
// is recorded on the order itself, so the shop can show, later, why a given
// purchase was or was not reported.

import type { Env } from "./_env";
import { sha256hex } from "./_lib";

export interface PurchasePayload {
  orderId: string;
  orderNumber: string;
  currency: string;
  valueMinor: number;
  taxMinor: number;
  /** Minor units divided by the currency's factor, computed by the caller so
   *  this module does not need the currency table. */
  value: number;
  tax: number;
  email: string;
  items: Array<{ sku: string; name: string; quantity: number; price: number }>;
  /** The _ga client id when the buyer had analytics consent, else a random one:
   *  the conversion still counts, it just does not join a session. */
  clientId: string;
  userAgent?: string;
  ip?: string;
}

export async function deliverTracking(env: Env, target: string, payload: Record<string, unknown>): Promise<void> {
  const data = payload as unknown as PurchasePayload;
  if (target === "ga4") return sendGa4(env, data);
  if (target === "meta") return sendMeta(env, data);
  throw new Error(`unknown tracking target ${target}`);
}

async function sendGa4(env: Env, p: PurchasePayload): Promise<void> {
  if (!env.GA4_MEASUREMENT_ID || !env.GA4_API_SECRET) throw new Error("ga4_not_configured");
  const url =
    `https://www.google-analytics.com/mp/collect?measurement_id=${encodeURIComponent(env.GA4_MEASUREMENT_ID)}` +
    `&api_secret=${encodeURIComponent(env.GA4_API_SECRET)}`;
  const body = {
    client_id: p.clientId,
    events: [
      {
        name: "purchase",
        params: {
          transaction_id: p.orderNumber,
          currency: p.currency,
          value: p.value,
          tax: p.tax,
          items: p.items.map((i) => ({
            item_id: i.sku,
            item_name: i.name,
            quantity: i.quantity,
            price: i.price,
          })),
        },
      },
    ],
  };
  const res = await fetch(url, { method: "POST", body: JSON.stringify(body) });
  // GA4's Measurement Protocol answers 204 on success and, unhelpfully, also
  // on most malformed payloads. A non-2xx is still worth a retry.
  if (!res.ok) throw new Error(`ga4 ${res.status}`);
}

async function sendMeta(env: Env, p: PurchasePayload): Promise<void> {
  if (!env.META_PIXEL_ID || !env.META_ACCESS_TOKEN) throw new Error("meta_not_configured");
  const url = `https://graph.facebook.com/v19.0/${encodeURIComponent(env.META_PIXEL_ID)}/events`;
  const body = {
    data: [
      {
        event_name: "Purchase",
        event_time: Math.floor(Date.now() / 1000),
        // The same id the browser pixel would send, so a seller running both
        // does not count one sale twice.
        event_id: p.orderId,
        action_source: "website",
        user_data: {
          // Hashed, because Meta wants it hashed and because a queued row
          // should not be a plaintext address sitting in the database.
          em: [await sha256hex((p.email ?? "").trim().toLowerCase())],
          client_user_agent: p.userAgent,
          client_ip_address: p.ip,
        },
        custom_data: {
          currency: p.currency,
          value: p.value,
          order_id: p.orderNumber,
          contents: p.items.map((i) => ({ id: i.sku, quantity: i.quantity, item_price: i.price })),
        },
      },
    ],
    access_token: env.META_ACCESS_TOKEN,
  };
  const res = await fetch(url, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`meta ${res.status}`);
}

/** Which trackers this deployment can use. The caller skips the whole step when
 *  this is empty, so an unconfigured shop writes no rows to drain. */
export function enabledTrackers(env: Env): string[] {
  const out: string[] = [];
  if (env.GA4_MEASUREMENT_ID && env.GA4_API_SECRET) out.push("ga4");
  if (env.META_PIXEL_ID && env.META_ACCESS_TOKEN) out.push("meta");
  return out;
}
