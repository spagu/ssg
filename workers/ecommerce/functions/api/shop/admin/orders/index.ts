// GET /api/shop/admin/orders — the order list, filtered and paged.
//
// Cursor paging on created_at, not OFFSET: an offset page shifts under you as
// new orders arrive, and shows the same order twice.

import type { Env } from "../../_env";
import { fail, json, str } from "../../_lib";
import type { OrderRow } from "../../_types";
import type { AdminData } from "../_middleware";

const STATUSES = new Set([
  "pending",
  "paid",
  "fulfilled",
  "refunded",
  "cancelled",
  "needs_review",
  "failed",
]);

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env }) => {
  const url = new URL(request.url);
  const limit = Math.min(100, Math.max(1, Number.parseInt(url.searchParams.get("limit") ?? "25", 10) || 25));
  const cursor = str(url.searchParams.get("cursor"), 80);
  const status = str(url.searchParams.get("status"), 20);
  const search = str(url.searchParams.get("q"), 254).toLowerCase();

  if (status && !STATUSES.has(status)) {
    return fail(request, "invalid_status", "That is not an order status.", 422);
  }

  const where: string[] = [];
  const binds: unknown[] = [];
  if (status) {
    where.push("o.status = ?");
    binds.push(status);
  }
  if (search) {
    // An order number or an email, whichever the owner pasted into the box.
    where.push("(LOWER(o.number) = ? OR c.email_lc LIKE ?)");
    binds.push(search, `%${search}%`);
  }
  if (cursor) {
    // Timestamp and id together, so two orders placed in the same millisecond
    // cannot make a page repeat one and lose the other.
    const at = cursor.indexOf("|");
    const [stamp, id] = at === -1 ? [cursor, ""] : [cursor.slice(0, at), cursor.slice(at + 1)];
    where.push("(o.created_at < ? OR (o.created_at = ? AND o.id < ?))");
    binds.push(stamp, stamp, id);
  }

  const sql = `SELECT o.*, c.email AS customer_email, c.name AS customer_name, c.country AS customer_country
                 FROM orders o
                 LEFT JOIN customers c ON c.id = o.customer_id
                ${where.length ? `WHERE ${where.join(" AND ")}` : ""}
                ORDER BY o.created_at DESC, o.id DESC
                LIMIT ?`;

  type Row = OrderRow & { customer_email: string | null; customer_name: string | null; customer_country: string | null };
  // One row over the limit tells us whether there is another page without a
  // second COUNT query over the whole table.
  const { results } = await env.SHOP_DB.prepare(sql)
    .bind(...binds, limit + 1)
    .all<Row>();
  const rows = results ?? [];
  const page = rows.slice(0, limit);
  const last = page.at(-1);

  return json({
    orders: page.map((o) => ({
      id: o.id,
      number: o.number,
      status: o.status,
      currency: o.currency,
      totalMinor: o.total_minor,
      taxMinor: o.tax_minor,
      taxCountry: o.tax_country,
      reverseCharge: o.reverse_charge === 1,
      gateway: o.gateway,
      email: o.customer_email,
      name: o.customer_name,
      country: o.customer_country,
      createdAt: o.created_at,
      paidAt: o.paid_at,
    })),
    nextCursor: rows.length > limit && last ? `${last.created_at}|${last.id}` : null,
  });
};
