// GET /api/shop/admin/stats — the numbers the panel opens with.
//
// Deliberately small: revenue over a window, what needs attention, and the
// products that actually sell. A shop with a few products does not need a
// reporting engine, and every chart here would be a query on every page load.

import type { Env } from "../_env";
import { json } from "../_lib";
import { getSettingString } from "../_settings";
import type { AdminData } from "./_middleware";

interface Totals {
  orders: number;
  gross: number;
  tax: number;
}

const WINDOWS: Record<string, number> = { "7d": 7, "30d": 30, "365d": 365 };

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env }) => {
  const requested = new URL(request.url).searchParams.get("window") ?? "30d";
  const days = WINDOWS[requested] ?? 30;
  const since = new Date(Date.now() - days * 86400000).toISOString();

  // Only settled money counts. A pending order is a hope, not revenue, and a
  // refunded one is money that left again.
  const base = await env.SHOP_DB.prepare(
    `SELECT currency,
            COUNT(*) AS orders,
            SUM(total_minor) AS gross,
            SUM(tax_minor) AS tax
       FROM orders
      WHERE status IN ('paid', 'fulfilled') AND paid_at >= ?
      GROUP BY currency`,
  )
    .bind(since)
    .all<{ currency: string; orders: number; gross: number; tax: number }>();

  const refunds = await env.SHOP_DB.prepare(
    `SELECT currency, SUM(ABS(amount_minor)) AS refunded
       FROM payments
      WHERE kind = 'refund' AND status = 'succeeded' AND created_at >= ?
      GROUP BY currency`,
  )
    .bind(since)
    .all<{ currency: string; refunded: number }>();
  const refundBy = new Map((refunds.results ?? []).map((r) => [r.currency, r.refunded]));

  const byCurrency = (base.results ?? []).map((r) => ({
    currency: r.currency,
    orders: r.orders,
    grossMinor: r.gross ?? 0,
    taxMinor: r.tax ?? 0,
    refundedMinor: refundBy.get(r.currency) ?? 0,
    netMinor: (r.gross ?? 0) - (refundBy.get(r.currency) ?? 0),
  }));

  const attention = await env.SHOP_DB.prepare(
    `SELECT
       SUM(CASE WHEN status = 'needs_review' THEN 1 ELSE 0 END) AS needsReview,
       SUM(CASE WHEN status = 'pending' AND created_at < ? THEN 1 ELSE 0 END) AS stalePending
     FROM orders`,
  )
    .bind(new Date(Date.now() - 86400000).toISOString())
    .first<{ needsReview: number; stalePending: number }>();

  const stuck = await env.SHOP_DB.prepare(
    `SELECT COUNT(*) AS n FROM outbox WHERE done_at IS NULL AND attempts >= 3`,
  ).first<{ n: number }>();

  const top = await env.SHOP_DB.prepare(
    `SELECT i.sku, i.name, SUM(i.quantity) AS units, SUM(i.total_minor) AS revenue
       FROM order_items i
       JOIN orders o ON o.id = i.order_id
      WHERE o.status IN ('paid', 'fulfilled') AND o.paid_at >= ?
      GROUP BY i.sku, i.name
      ORDER BY revenue DESC
      LIMIT 10`,
  )
    .bind(since)
    .all<{ sku: string; name: string; units: number; revenue: number }>();

  // Where the buyers are matters for VAT: an owner whose sales cross a country
  // threshold needs to see that before their accountant does.
  const countries = await env.SHOP_DB.prepare(
    `SELECT tax_country AS country, COUNT(*) AS orders, SUM(total_minor) AS gross
       FROM orders
      WHERE status IN ('paid', 'fulfilled') AND paid_at >= ? AND tax_country IS NOT NULL
      GROUP BY tax_country
      ORDER BY gross DESC
      LIMIT 20`,
  )
    .bind(since)
    .all<{ country: string; orders: number; gross: number }>();

  return json({
    window: requested in WINDOWS ? requested : "30d",
    since,
    baseCurrency: await getSettingString(env, "currency.base"),
    byCurrency,
    needsReview: attention?.needsReview ?? 0,
    stalePending: attention?.stalePending ?? 0,
    stuckMessages: stuck?.n ?? 0,
    topProducts: top.results ?? [],
    byCountry: countries.results ?? [],
  });
};
