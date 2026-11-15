// Order events for the agent platform, and the Order object they carry.
//
// The protocol's webhook spec (openapi.agentic_checkout_webhook.yaml,
// 2026-04-17): the seller POSTs `{type: "order_create" | "order_update",
// data: <the full Order>}` to the platform's receiver, signed in a
// `Merchant-Signature: t=<unix seconds>,v1=<hex HMAC-SHA256 of "t.body">`
// header. The receiver rejects a stale timestamp (300 s recommended).
//
// Events go through the shop's outbox like every email and webhook: written
// next to the order change, delivered with the outbox's retries and backoff,
// visible in the panel when they keep failing. The signing secret is read at
// delivery time from SHOP_ACP_WEBHOOK_SECRET and never stored in the queue.

import type { Env } from "./_env";
import { hmacHex } from "./_gateway";
import { newId } from "./_lib";
import { getOrder, getOrderItems } from "./_orders";
import { enqueue } from "./_outbox";
import type { SessionRow } from "./_acp-session";

/** The storefront's order states in the protocol's words. Unknown values are
 *  passed through: the spec says receivers MUST accept them. */
const ORDER_STATUS: Record<string, string> = {
  pending: "created",
  paid: "confirmed",
  fulfilled: "completed",
  needs_review: "manual_review",
  cancelled: "canceled",
  failed: "canceled",
  refunded: "canceled",
};

/** Where the buyer can see the order: the storefront's own thank-you page,
 *  which needs the order key. */
export function permalink(origin: string, orderId: string, orderKey: string | null): string {
  const key = orderKey ? `&k=${encodeURIComponent(orderKey)}` : "";
  return `${origin}/shop/thanks/?order=${orderId}${key}`;
}

/** The Order for a completed session, or null when it has none. */
export async function acpOrder(env: Env, session: SessionRow, origin: string): Promise<Record<string, unknown> | null> {
  const order = session.order_id ? await getOrder(env, session.order_id) : null;
  if (!order || session.status !== "completed") return null;
  const items = await getOrderItems(env, order.id);
  const invoice = await env.SHOP_DB.prepare(`SELECT number FROM invoices WHERE order_id = ? AND kind = 'invoice'`)
    .bind(order.id)
    .first<{ number: string }>();
  const delivered = order.status === "paid" || order.status === "fulfilled";
  // Refunds as the Order's `adjustments` (the 2026-04-17 webhook spec: "Use
  // adjustments[] … which supports refunds"), with the total refunded as an
  // `amount_refunded` total, as its own example does. Absent until there is one.
  const { results } = await env.SHOP_DB.prepare(
    `SELECT id, amount_minor, currency, created_at FROM payments
      WHERE order_id = ? AND kind = 'refund' AND status = 'succeeded' ORDER BY created_at, rowid`,
  )
    .bind(order.id)
    .all<{ id: string; amount_minor: number; currency: string; created_at: string }>();
  const refunds = results;
  const refunded = refunds.reduce((sum, r) => sum + Math.abs(r.amount_minor), 0);
  return {
    type: "order",
    id: order.id,
    checkout_session_id: session.id,
    order_number: order.number,
    permalink_url: permalink(origin, order.id, session.order_key),
    status: ORDER_STATUS[order.status] ?? order.status,
    confirmation: {
      confirmation_number: order.number,
      ...(invoice ? { invoice_number: invoice.number } : {}),
    },
    line_items: items.map((i) => ({
      id: i.sku,
      title: i.name,
      product_id: i.product_id,
      quantity: { ordered: i.quantity, current: i.quantity, fulfilled: delivered ? i.quantity : 0 },
      unit_price: i.unit_minor,
      subtotal: i.total_minor - i.tax_minor,
      status: delivered ? "fulfilled" : "pending",
    })),
    totals: [
      { type: "subtotal", display_text: "Subtotal", amount: order.total_minor - order.tax_minor },
      { type: "tax", display_text: "Tax", amount: order.tax_minor },
      { type: "total", display_text: "Total", amount: order.total_minor },
      ...(refunds.length ? [{ type: "amount_refunded", display_text: "Refunded", amount: refunded }] : []),
    ],
    ...(refunds.length
      ? {
          adjustments: refunds.map((r) => ({
            id: `adj_${r.id}`,
            type: "refund",
            occurred_at: r.created_at,
            status: "completed",
            amount: Math.abs(r.amount_minor),
            currency: r.currency.toLowerCase(),
          })),
        }
      : {}),
  };
}

/** Queues an event for the platform. False when no receiver is configured —
 *  the order is still the shop's, the platform just is not told. */
export async function enqueueOrderEvent(
  env: Env,
  type: "order_create" | "order_update",
  order: Record<string, unknown>,
): Promise<boolean> {
  if (!env.SHOP_ACP_WEBHOOK_URL) return false;
  await enqueue(env, "acp", env.SHOP_ACP_WEBHOOK_URL, { type, data: order });
  return true;
}

/** Called by the outbox. Throws on anything but a 2xx, so the outbox retries. */
export async function deliverAcpEvent(env: Env, target: string, payload: Record<string, unknown>): Promise<void> {
  const secret = env.SHOP_ACP_WEBHOOK_SECRET;
  if (!secret) throw new Error("SHOP_ACP_WEBHOOK_SECRET is not set");
  let url: URL;
  try {
    url = new URL(target);
  } catch {
    throw new Error("invalid ACP webhook url");
  }
  if (url.protocol !== "https:") throw new Error("ACP webhook url must be https");

  const body = JSON.stringify(payload);
  const t = Math.floor(Date.now() / 1000);
  const res = await fetch(url.toString(), {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "merchant-signature": `t=${t},v1=${await hmacHex(secret, `${t}.${body}`)}`,
      "request-id": newId(),
      timestamp: new Date(t * 1000).toISOString(),
    },
    body,
    signal: AbortSignal.timeout(10_000),
  });
  if (!res.ok) throw new Error(`ACP webhook ${res.status}`);
}
