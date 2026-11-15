// Telling the agent platform an order came from that it was refunded.
//
// A refund is recorded in exactly one place: the payment provider's webhook
// (_webhook.ts, applyRefunded) — the admin panel's refund button only asks
// the provider, and the order changes when the provider confirms, through the
// same path as a refund started in the provider's dashboard. So that is where
// this is called, once per refund event (the webhook inbox drops repeats).
//
//   channel 'acp' → ACP `order_update` with the full Order, refunds as
//                   `adjustments` (_acp-webhook.ts)
//   channel 'ucp' → UCP order event with the full Order, the refund as an
//                   adjustment; Webhook-Id is the adjustment's id (Google:
//                   "the id of the primary event") (_ucp-order.ts)
//   anything else → nothing: a storefront order has no platform to tell.

import type { Env } from "./_env";
import { acpOrder, enqueueOrderEvent } from "./_acp-webhook";
import type { SessionRow } from "./_acp-session";
import { getOrder } from "./_orders";
import { adjustmentId, enqueueUcpEvent, ucpOrder } from "./_ucp-order";
import { sessionForOrder } from "./_ucp-session";

/** Queues the order update for the platform the order came from. Returns the
 *  protocol told, or null when there was nobody to tell (a web order, a
 *  session that is gone, no webhook URL configured). */
export async function announceRefund(env: Env, orderId: string, origin: string): Promise<"acp" | "ucp" | null> {
  const order = await getOrder(env, orderId);
  if (order?.channel === "acp") {
    const session = await env.SHOP_DB.prepare(`SELECT * FROM acp_sessions WHERE order_id = ?`).bind(orderId).first<SessionRow>();
    const data = session ? await acpOrder(env, session, origin) : null;
    return data && (await enqueueOrderEvent(env, "order_update", data)) ? "acp" : null;
  }
  if (order?.channel === "ucp") {
    const session = await sessionForOrder(env, orderId);
    const data = session ? await ucpOrder(env, session, origin) : null;
    const latest = await env.SHOP_DB.prepare(
      `SELECT id FROM payments WHERE order_id = ? AND kind = 'refund' AND status = 'succeeded' ORDER BY created_at DESC, rowid DESC LIMIT 1`,
    )
      .bind(orderId)
      .first<{ id: string }>();
    return data && latest && (await enqueueUcpEvent(env, data, adjustmentId(latest), origin)) ? "ucp" : null;
  }
  return null;
}
