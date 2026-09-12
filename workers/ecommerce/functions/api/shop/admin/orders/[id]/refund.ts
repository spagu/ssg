// POST /api/shop/admin/orders/:id/refund — give the money back.
//
// The refund is asked of the provider; the order is NOT marked refunded here.
// The provider's webhook does that, through exactly the same path a refund
// started in Stripe's own dashboard takes (S-19). One path, one set of bugs.

import type { Env } from "../../../_env";
import { audit } from "../../../_audit";
import { requireOwner } from "../../../_auth";
import { gateway } from "../../../_gateways";
import { fail, json, readJson } from "../../../_lib";
import type { OrderRow, PaymentRow } from "../../../_types";
import type { AdminData } from "../../_middleware";

export const onRequestPost: PagesFunction<Env, string, AdminData> = async ({ request, env, params, data }) => {
  const denied = requireOwner(request, data.admin);
  if (denied) return denied;

  const id = String(params.id ?? "");
  const order = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(id).first<OrderRow>();
  if (!order) return fail(request, "not_found", "No such order.", 404);
  if (!["paid", "fulfilled"].includes(order.status)) {
    return fail(request, "not_refundable", `An order that is ${order.status} cannot be refunded.`, 409);
  }

  const charge = await env.SHOP_DB.prepare(
    `SELECT * FROM payments WHERE order_id = ? AND kind = 'charge' AND status = 'succeeded'
      ORDER BY created_at DESC LIMIT 1`,
  )
    .bind(order.id)
    .first<PaymentRow>();
  if (!charge) return fail(request, "no_payment", "No successful payment is recorded for this order.", 409);

  const body = (await readJson<{ amountMinor?: number }>(request)) ?? {};
  const requested = body.amountMinor === undefined ? order.total_minor : Number.parseInt(String(body.amountMinor), 10);
  if (!Number.isFinite(requested) || requested <= 0) {
    return fail(request, "invalid_amount", "The refund amount must be a positive whole number of minor units.", 422);
  }

  // Refunds already granted come off what is left. Refunding twice because two
  // people clicked the button is real money.
  const already = await env.SHOP_DB.prepare(
    `SELECT COALESCE(SUM(ABS(amount_minor)), 0) AS n FROM payments
      WHERE order_id = ? AND kind = 'refund' AND status = 'succeeded'`,
  )
    .bind(order.id)
    .first<{ n: number }>();
  const remaining = order.total_minor - (already?.n ?? 0);
  if (requested > remaining) {
    return fail(
      request,
      "amount_too_large",
      `Only ${remaining} minor units are left to refund on this order.`,
      422,
    );
  }

  const provider = order.gateway ? gateway(env, order.gateway) : null;
  if (!provider) {
    return fail(request, "gateway_unavailable", "The payment provider for this order is not configured.", 503);
  }

  let refundRef: string;
  try {
    ({ refundRef } = await provider.refund(charge, requested, env));
  } catch (err) {
    // The provider's own wording is the useful part; it is logged, not shown,
    // in case it carries anything we would rather not print (S-29).
    await audit(env, `admin:${data.admin.sub}`, "order.refund_failed", order.id, {
      number: order.number,
      amount: requested,
      error: String(err).slice(0, 300),
    });
    return fail(request, "refund_failed", "The payment provider refused the refund. Check its dashboard.", 502);
  }

  await audit(env, `admin:${data.admin.sub}`, "order.refund_requested", order.id, {
    number: order.number,
    amount: requested,
    gateway: order.gateway,
    refundRef,
  });

  return json({
    ok: true,
    refundRef,
    amountMinor: requested,
    message: "The refund was sent to the provider. The order updates when the provider confirms it.",
  });
};
