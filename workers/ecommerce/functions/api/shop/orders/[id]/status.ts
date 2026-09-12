// GET /api/shop/orders/:id/status?k=… — what the thank-you page polls.
//
// The redirect back from a payment provider proves nothing: a buyer can open
// that URL themselves. So this reports the status the webhook wrote, and the
// page waits for it (S-09).

import type { Env } from "../../_env";
import { fail, json } from "../../_lib";
import { getOrder, getOrderItems, orderKeyMatches } from "../../_orders";
import { drainInBackground } from "../../_outbox";
import { guard } from "../../_ratelimit";
import { ensureSchema } from "../../_schema";

export const onRequestGet: PagesFunction<Env> = async ({ request, env, params, waitUntil }) => {
  if (!env.SHOP_DB) return fail(request, "not_configured", "The shop database is not bound.", 503);
  await ensureSchema(env);

  // Generous, because the thank-you page polls this while it waits for the
  // webhook — but not unlimited, because a wrong key answers 404 and that is a
  // thing worth doing slowly.
  const limited = await guard(env, request, "status");
  if (limited) return limited;

  const id = String(params.id ?? "");
  const key = new URL(request.url).searchParams.get("k") ?? "";

  const order = await getOrder(env, id);
  // One answer for "no such order" and "wrong key", so the endpoint cannot be
  // used to discover which order ids exist.
  if (!order || !(await orderKeyMatches(order, key))) {
    return fail(request, "not_found", "No order matches that link.", 404);
  }

  // A buyer refreshing the thank-you page is as good a reason as any to push
  // the queue along — their own email is usually the row at the front of it.
  drainInBackground(env, { waitUntil }, 3);

  const items = await getOrderItems(env, order.id);
  const ready = order.status === "paid" || order.status === "fulfilled";

  let downloads: Array<{ name: string; url: string; expiresAt: string; remaining: number }> = [];
  if (ready) {
    // Tokens are stored hashed, so the plaintext is only in the email. What the
    // page can show is what is still available, and a link to have it resent.
    const { results } = await env.SHOP_DB.prepare(
      `SELECT t.expires_at, t.uses, t.max_uses, i.name
         FROM download_tokens t JOIN order_items i ON i.id = t.order_item_id
        WHERE i.order_id = ? AND t.revoked = 0`,
    )
      .bind(order.id)
      .all<{ expires_at: string; uses: number; max_uses: number; name: string }>();
    downloads = (results ?? []).map((r) => ({
      name: r.name,
      url: "",
      expiresAt: r.expires_at,
      remaining: Math.max(0, r.max_uses - r.uses),
    }));
  }

  return json({
    status: order.status,
    number: order.number,
    currency: order.currency,
    totalMinor: order.total_minor,
    paidAt: order.paid_at,
    items: items.map((i) => ({ sku: i.sku, name: i.name, quantity: i.quantity, totalMinor: i.total_minor })),
    downloads,
    /** The email is where the actual links are; the page says so rather than
     *  pretending it can hand them over. */
    linksSentByEmail: ready,
  });
};
