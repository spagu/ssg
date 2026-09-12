// POST /api/shop/admin/orders/:id/resend — send the download email again.
//
// The buyer wrote in saying they never got it. Unlike the public resend, this
// one names the order and answers honestly, because the person asking has
// already proved who they are.

import type { Env } from "../../../_env";
import { audit } from "../../../_audit";
import { issueTokens } from "../../../_downloads";
import { shopOrigin } from "../../../_fulfil";
import { fail, json } from "../../../_lib";
import { resendMail } from "../../../_mail";
import { drainOutbox, enqueue } from "../../../_outbox";
import { allSettings } from "../../../_settings";
import type { CustomerRow, OrderItemRow, OrderRow, ProductRow } from "../../../_types";
import type { AdminData } from "../../_middleware";

export const onRequestPost: PagesFunction<Env, string, AdminData> = async ({ request, env, params, data }) => {
  const id = String(params.id ?? "");
  const order = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(id).first<OrderRow>();
  if (!order) return fail(request, "not_found", "No such order.", 404);
  if (!["paid", "fulfilled"].includes(order.status)) {
    return fail(request, "not_paid", "Only a paid order has links to send.", 409);
  }

  const customer = order.customer_id
    ? await env.SHOP_DB.prepare(`SELECT * FROM customers WHERE id = ?`).bind(order.customer_id).first<CustomerRow>()
    : null;
  if (!customer) return fail(request, "no_customer", "This order has no email address on it.", 409);

  const { results: items } = await env.SHOP_DB.prepare(`SELECT * FROM order_items WHERE order_id = ?`)
    .bind(order.id)
    .all<OrderItemRow>();

  const products = new Map<string, ProductRow>();
  for (const item of items ?? []) {
    const p = await env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`)
      .bind(item.product_id)
      .first<ProductRow>();
    if (p) products.set(item.product_id, p);
  }

  // The old links are cancelled first: a token whose plaintext we no longer
  // hold cannot be put in an email, so new ones have to replace them.
  await env.SHOP_DB.prepare(
    `UPDATE download_tokens SET revoked = 1
      WHERE order_item_id IN (SELECT id FROM order_items WHERE order_id = ?)`,
  )
    .bind(order.id)
    .run();

  const tokens = await issueTokens(env, items ?? [], products);
  if (tokens.length === 0) {
    return fail(request, "nothing_to_send", "No line of this order has a file attached to it.", 409);
  }

  const settings = await allSettings(env);
  const origin = await shopOrigin(env, request);
  const mail = resendMail({
    locale: order.locale ?? "en",
    shopName: String(settings["shop.name"] ?? "Shop"),
    orderNumber: order.number,
    totalMinor: order.total_minor,
    currency: order.currency,
    downloads: tokens.map((t) => ({
      name: t.productName,
      url: `${origin}/api/shop/download/${t.token}`,
      expiresAt: t.expiresAt,
      maxUses: t.maxUses,
    })),
    resendUrl: `${origin}/shop/resend/`,
  });
  await enqueue(env, "email", customer.email, mail as unknown as Record<string, unknown>);
  await drainOutbox(env, 3).catch(() => 0);

  await audit(env, `admin:${data.admin.sub}`, "order.resend", order.id, {
    number: order.number,
    links: tokens.length,
  });
  return json({ ok: true, sentTo: customer.email, links: tokens.length });
};
