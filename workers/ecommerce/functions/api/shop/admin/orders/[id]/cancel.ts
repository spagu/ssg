// POST /api/shop/admin/orders/:id/cancel — put an abandoned basket to rest.
//
// Only an order nobody has paid for. A paid order is refunded, never cancelled:
// cancelling one would leave money received against an order that claims it was
// never wanted.

import type { Env } from "../../../_env";
import { audit } from "../../../_audit";
import { fail, json, nowISO, readJson, str } from "../../../_lib";
import type { OrderRow } from "../../../_types";
import type { AdminData } from "../../_middleware";

export const onRequestPost: PagesFunction<Env, string, AdminData> = async ({ request, env, params, data }) => {
  const id = String(params.id ?? "");
  const order = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(id).first<OrderRow>();
  if (!order) return fail(request, "not_found", "No such order.", 404);

  const body = (await readJson<{ reason?: string }>(request)) ?? {};
  const reason = str(body.reason, 300);

  const res = await env.SHOP_DB.prepare(
    `UPDATE orders SET status = 'cancelled', updated_at = ? WHERE id = ? AND status IN ('pending', 'failed')`,
  )
    .bind(nowISO(), order.id)
    .run();
  if ((res.meta?.changes ?? 0) === 0) {
    return fail(request, "not_cancellable", `An order that is ${order.status} cannot be cancelled.`, 409);
  }

  await audit(env, `admin:${data.admin.sub}`, "order.cancel", order.id, { number: order.number, reason });
  return json({ ok: true });
};
