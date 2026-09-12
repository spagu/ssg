// POST /api/shop/admin/orders/:id/mark-paid — a bank transfer arrived, or a
// webhook never did.
//
// This runs exactly the same fulfilment the webhook runs, so an order paid by
// hand gets its invoice, its links and its email like any other. The audit
// entry records who decided it, because no provider vouches for this one.

import type { Env } from "../../../_env";
import { audit } from "../../../_audit";
import { requireOwner } from "../../../_auth";
import { fulfilOrder, shopOrigin } from "../../../_fulfil";
import { fail, json, newId, nowISO, readJson, str } from "../../../_lib";
import type { OrderRow } from "../../../_types";
import type { AdminData } from "../../_middleware";

export const onRequestPost: PagesFunction<Env, string, AdminData> = async ({ request, env, params, data }) => {
  const denied = requireOwner(request, data.admin);
  if (denied) return denied;

  const id = String(params.id ?? "");
  const order = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(id).first<OrderRow>();
  if (!order) return fail(request, "not_found", "No such order.", 404);
  if (!["pending", "needs_review"].includes(order.status)) {
    return fail(request, "not_pending", `An order that is ${order.status} cannot be marked paid.`, 409);
  }

  const body = (await readJson<{ reference?: string }>(request)) ?? {};
  const reference = str(body.reference, 120) || `manual-${order.number}`;
  const actor = `admin:${data.admin.sub}`;

  // A payment row with gateway "manual" keeps the money trail complete: the
  // order's total is accounted for by something, and a later refund knows
  // there is nothing to ask a provider for.
  await env.SHOP_DB.prepare(
    `INSERT OR IGNORE INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, raw_json, created_at)
     VALUES (?, ?, 'manual', ?, 'charge', 'succeeded', ?, ?, ?, ?)`,
  )
    .bind(
      newId(),
      order.id,
      reference,
      order.total_minor,
      order.currency,
      JSON.stringify({ markedBy: data.admin.email, reference }),
      nowISO(),
    )
    .run();

  const result = await fulfilOrder(env, order, { origin: await shopOrigin(env, request), actor });
  await audit(env, actor, "order.mark_paid", order.id, {
    number: order.number,
    reference,
    changed: result.changed,
  });

  return json({
    ok: true,
    changed: result.changed,
    invoiceNumber: result.invoiceNumber ?? null,
    linksIssued: result.tokensIssued,
  });
};
