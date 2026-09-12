// GET   /api/shop/admin/orders/:id — everything about one order
// PATCH /api/shop/admin/orders/:id — the owner's private note

import type { Env } from "../../_env";
import { audit } from "../../_audit";
import { fail, json, nowISO, readJson, str } from "../../_lib";
import type { InvoiceRow, OrderItemRow, OrderRow, PaymentRow } from "../../_types";
import type { AdminData } from "../_middleware";

interface TokenSummary {
  order_item_id: string;
  expires_at: string;
  uses: number;
  max_uses: number;
  revoked: number;
}

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env, params }) => {
  const id = String(params.id ?? "");
  const order = await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ? OR number = ?`)
    .bind(id, id)
    .first<OrderRow>();
  if (!order) return fail(request, "not_found", "No such order.", 404);

  const [items, payments, invoices, tokens, customer] = await Promise.all([
    env.SHOP_DB.prepare(`SELECT * FROM order_items WHERE order_id = ?`).bind(order.id).all<OrderItemRow>(),
    env.SHOP_DB.prepare(`SELECT * FROM payments WHERE order_id = ? ORDER BY created_at`)
      .bind(order.id)
      .all<PaymentRow>(),
    env.SHOP_DB.prepare(`SELECT * FROM invoices WHERE order_id = ? ORDER BY issued_at`)
      .bind(order.id)
      .all<InvoiceRow>(),
    // The token itself was never stored, only its hash, so the panel can show
    // how a download is going but can never hand the link out again (S-12).
    env.SHOP_DB.prepare(
      `SELECT t.order_item_id, t.expires_at, t.uses, t.max_uses, t.revoked
         FROM download_tokens t
         JOIN order_items i ON i.id = t.order_item_id
        WHERE i.order_id = ?`,
    )
      .bind(order.id)
      .all<TokenSummary>(),
    order.customer_id
      ? env.SHOP_DB.prepare(`SELECT id, email, name, vat_id, vat_id_valid, country FROM customers WHERE id = ?`)
          .bind(order.customer_id)
          .first()
      : Promise.resolve(null),
  ]);

  return json({
    order: {
      ...order,
      key_hash: undefined, // never leaves the database (S-11)
      taxEvidence: safeParse(order.tax_evidence),
    },
    customer,
    items: items.results ?? [],
    payments: (payments.results ?? []).map((p) => ({ ...p, raw_json: undefined })),
    invoices: (invoices.results ?? []).map((i) => ({ ...i, snapshot_json: undefined })),
    tokens: tokens.results ?? [],
  });
};

function safeParse(value: string | null): unknown {
  if (!value) return null;
  try {
    return JSON.parse(value);
  } catch {
    return null;
  }
}

export const onRequestPatch: PagesFunction<Env, string, AdminData> = async ({ request, env, params, data }) => {
  const id = String(params.id ?? "");
  const order = await env.SHOP_DB.prepare(`SELECT id, number FROM orders WHERE id = ?`)
    .bind(id)
    .first<{ id: string; number: string }>();
  if (!order) return fail(request, "not_found", "No such order.", 404);

  const body = await readJson<{ notes?: string }>(request);
  if (!body || typeof body.notes !== "string") {
    return fail(request, "invalid_body", "Send { notes: \"…\" }.", 400);
  }

  // A note is the only field a human may change here. Amounts, status and tax
  // are decided by the payment, not by the panel.
  const notes = str(body.notes, 4000);
  await env.SHOP_DB.prepare(`UPDATE orders SET notes = ?, updated_at = ? WHERE id = ?`)
    .bind(notes || null, nowISO(), order.id)
    .run();
  await audit(env, `admin:${data.admin.sub}`, "order.note", order.id, { number: order.number });
  return json({ ok: true, notes });
};
