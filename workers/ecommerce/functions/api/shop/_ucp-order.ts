// Order events for the UCP platform, and the Order they carry.
//
// order.md ("Events"): the business POSTs the full current order — never a
// delta — to the webhook URL the platform gave it, signed (RFC 9421,
// _ucp-sign.ts), with the Standard Webhooks headers Webhook-Id and
// Webhook-Timestamp. Google's receiver (Merchant "Order lifecycle", version
// 2026-04-08, read 2026-11-14) also wants its API key in X-Goog-Api-Key and
// the same Idempotency-Key on every retry of one event.
//
// Events go through the shop's outbox like every email and webhook: written
// next to the order change, retried with backoff, visible in the panel when
// they keep failing. The webhook URL, its API key and the signing key are all
// read at delivery time; none of them is stored in the queue.

import type { Env } from "./_env";
import { getOrder, getOrderItems } from "./_orders";
import { enqueue } from "./_outbox";
import { taxContext } from "./_tax";
import type { PaymentRow } from "./_types";
import { ORDER_CAPABILITY, UCP_VERSION } from "./_ucp-http";
import { breakdown } from "./_ucp-pricing";
import { lineId, type UcpSessionRow } from "./_ucp-session";
import { signedHeaders } from "./_ucp-sign";
import { allowedScheme } from "./_ucp-testing";

/** Where the buyer can see the order: the storefront's own thank-you page,
 *  which needs the order key. */
export function permalink(origin: string, orderId: string, orderKey: string | null): string {
  const key = orderKey ? `&k=${encodeURIComponent(orderKey)}` : "";
  return `${origin}/shop/thanks/?order=${orderId}${key}`;
}

/** The order of a completed session as order.json, or null when it has none. */
export async function ucpOrder(env: Env, session: UcpSessionRow, origin: string): Promise<Record<string, unknown> | null> {
  const order = session.order_id ? await getOrder(env, session.order_id) : null;
  if (!order || session.status !== "completed") return null;
  const items = await getOrderItems(env, order.id);
  const { results } = await env.SHOP_DB.prepare(
    `SELECT * FROM payments WHERE order_id = ? AND kind = 'refund' AND status = 'succeeded' ORDER BY created_at, rowid`,
  )
    .bind(order.id)
    .all<PaymentRow>();
  const refunds = results;
  const gross = (await taxContext(env)).pricingMode === "gross";
  // A download is delivered the moment the order is paid; a full refund
  // takes the lines off the order (Google: status "removed", total 0, the
  // original quantity kept).
  const delivered = ["paid", "fulfilled", "refunded"].includes(order.status);
  const removed = order.status === "refunded";

  return {
    ucp: { version: UCP_VERSION, capabilities: { [ORDER_CAPABILITY]: [{ version: UCP_VERSION }] } },
    id: order.id,
    label: order.number,
    checkout_id: session.id,
    permalink_url: permalink(origin, order.id, session.order_key),
    currency: order.currency,
    line_items: items.map((i, index) => {
      const fulfilled = delivered ? i.quantity : 0;
      return {
        id: lineId(index),
        item: { id: i.sku, title: i.name, price: gross ? i.total_minor / i.quantity : i.unit_minor },
        quantity: removed ? { original: i.quantity, total: 0, fulfilled } : { total: i.quantity, fulfilled },
        totals: breakdown(gross, i.total_minor - i.tax_minor, i.tax_minor, i.total_minor),
        status: removed ? "removed" : delivered ? "fulfilled" : "processing",
      };
    }),
    totals: breakdown(gross, order.total_minor - order.tax_minor, order.tax_minor, order.total_minor),
    fulfillment: {
      expectations: [
        {
          id: "exp_1",
          line_items: items.map((i, index) => ({ id: lineId(index), quantity: i.quantity })),
          method_type: "digital",
          destination: order.tax_country ? { address_country: order.tax_country } : {},
          description: "Download link by email, immediately after payment",
          fulfillable_on: "now",
        },
      ],
    },
    ...(refunds.length
      ? {
          adjustments: refunds.map((r) => ({
            id: adjustmentId(r),
            type: "refund",
            occurred_at: r.created_at,
            status: "completed",
            totals: [{ type: "total", display_text: "Refund", amount: -Math.abs(r.amount_minor) }],
            description: "Refund",
          })),
        }
      : {}),
  };
}

export const adjustmentId = (refund: Pick<PaymentRow, "id">): string => `adj_${refund.id}`;

/** Queues an event. False when no webhook URL is configured — the order is
 *  still the shop's, the platform just is not told. `eventId` is the
 *  Webhook-Id: the order's id for its creation, the adjustment's for a
 *  refund (Google: "the id of the primary event"). */
export async function enqueueUcpEvent(env: Env, order: Record<string, unknown>, eventId: string, origin: string): Promise<boolean> {
  if (!env.SHOP_UCP_WEBHOOK_URL) return false;
  const shown = new URL(env.SHOP_UCP_WEBHOOK_URL);
  await enqueue(env, "ucp", `${shown.origin}${shown.pathname}`, {
    eventId,
    occurredAt: new Date().toISOString(),
    profile: `${origin}/.well-known/ucp`,
    order,
  });
  return true;
}

/** Called by the outbox. Throws on anything but a 2xx, so the outbox retries
 *  — with the same Webhook-Id and Idempotency-Key, as Google asks. */
export async function deliverUcpEvent(env: Env, payload: Record<string, unknown>): Promise<void> {
  if (!env.SHOP_UCP_WEBHOOK_URL) throw new Error("SHOP_UCP_WEBHOOK_URL is not set");
  const url = new URL(env.SHOP_UCP_WEBHOOK_URL);
  // http only for the conformance suite's local receiver, in test mode.
  if (!allowedScheme(env, url.protocol)) throw new Error("UCP webhook url must be https");

  const eventId = String(payload.eventId);
  const body = JSON.stringify(payload.order);
  const base = {
    "content-type": "application/json",
    "ucp-agent": `profile="${String(payload.profile)}"`,
    "idempotency-key": eventId,
  };
  const res = await fetch(url.toString(), {
    method: "POST",
    headers: {
      ...base,
      ...(await signedHeaders(env, url, body, base)),
      "webhook-id": eventId,
      // RFC 3339, as every Google example shows; the protocol's table says
      // "unix" (UCP.md, ambiguity 6).
      "webhook-timestamp": String(payload.occurredAt),
      ...(env.SHOP_UCP_WEBHOOK_API_KEY ? { "x-goog-api-key": env.SHOP_UCP_WEBHOOK_API_KEY } : {}),
    },
    body,
    signal: AbortSignal.timeout(10_000),
  });
  if (!res.ok) throw new Error(`UCP webhook ${res.status}`);
}
