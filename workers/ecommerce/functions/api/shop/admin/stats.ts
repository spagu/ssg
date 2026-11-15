// GET /api/shop/admin/stats — the numbers the panel opens with.
//
// Deliberately small: revenue over a window, what needs attention, and the
// products that actually sell. A shop with a few products does not need a
// reporting engine, and every chart here would be a query on every page load.

import { channelRank } from "../_channels";
import type { Env } from "../_env";
import { json } from "../_lib";
import { getSettingString } from "../_settings";
import type { AdminData } from "./_middleware";

const WINDOWS: Record<string, number> = { "7d": 7, "30d": 30, "365d": 365 };

/** The statuses of an order that was paid: what gross is made of. */
const PAID = "('paid', 'fulfilled', 'refunded')";

/** sideBySide pairs two grouped results on one key, keeping a key that only
 *  one side has — a refund made in the window for an order paid before it
 *  still counts. The key is returned with the pair; sales keys come first. */
function sideBySide<S, R>(
  sales: S[] | undefined,
  refunds: R[] | undefined,
  keyOf: (row: S | R) => string,
): Array<[S | undefined, R | undefined, string]> {
  const pairs = new Map<string, [S | undefined, R | undefined, string]>();
  for (const row of sales ?? []) pairs.set(keyOf(row), [row, undefined, keyOf(row)]);
  for (const row of refunds ?? []) {
    const key = keyOf(row);
    const pair = pairs.get(key) ?? [undefined, undefined, key];
    pair[1] = row;
    pairs.set(key, pair);
  }
  return [...pairs.values()];
}

interface ChannelKey {
  channel: string;
  currency: string;
}

/** channelTotals: orders, gross and refunds per channel and currency (GO-165)
 *  — money in two currencies is never added up. Known channels first, in
 *  _channels.ts order, then currency. */
async function channelTotals(db: D1Database, since: string) {
  const sales = await db
    .prepare(
      `SELECT channel, currency, COUNT(*) AS orders, SUM(total_minor) AS gross
         FROM orders
        WHERE status IN ${PAID} AND paid_at >= ?
        GROUP BY channel, currency`,
    )
    .bind(since)
    .all<ChannelKey & { orders: number; gross: number | null }>();
  const refunds = await db
    .prepare(
      `SELECT o.channel AS channel, p.currency AS currency, COUNT(*) AS refunds, SUM(ABS(p.amount_minor)) AS refunded
         FROM payments p
         JOIN orders o ON o.id = p.order_id
        WHERE p.kind = 'refund' AND p.status = 'succeeded' AND p.created_at >= ?
        GROUP BY o.channel, p.currency`,
    )
    .bind(since)
    .all<ChannelKey & { refunds: number | null; refunded: number | null }>();

  return sideBySide(sales.results, refunds.results, (r) => `${r.channel}\u0000${r.currency}`)
    .map(([sale, back]) => {
      const { channel, currency } = (sale ?? back)!;
      const grossMinor = sale?.gross ?? 0;
      const refundedMinor = back?.refunded ?? 0;
      return {
        channel,
        currency,
        orders: sale?.orders ?? 0,
        grossMinor,
        refunds: back?.refunds ?? 0,
        refundedMinor,
        netMinor: grossMinor - refundedMinor,
      };
    })
    // Pairs are unique per channel and currency, so two never compare equal.
    .sort((a, b) => channelRank(a.channel) - channelRank(b.channel) || (`${a.channel}\u0000${a.currency}` < `${b.channel}\u0000${b.currency}` ? -1 : 1));
}

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env }) => {
  const requested = new URL(request.url).searchParams.get("window") ?? "30d";
  const days = WINDOWS[requested] ?? 30;
  const since = new Date(Date.now() - days * 86400000).toISOString();

  // Gross is every order that was paid in the window — paid, fulfilled or
  // refunded since — and refunds made in the window come off it once. A
  // refunded order left out of gross while its refund was still subtracted
  // took its money off twice (GO-165); _summary.ts counts it this way too. A
  // pending order is a hope, not revenue. VAT is what was kept: a full
  // refund's credit note gave its VAT back.
  const sales = await env.SHOP_DB.prepare(
    `SELECT currency,
            COUNT(*) AS orders,
            SUM(total_minor) AS gross,
            SUM(CASE WHEN status = 'refunded' THEN 0 ELSE tax_minor END) AS tax
       FROM orders
      WHERE status IN ${PAID} AND paid_at >= ?
      GROUP BY currency`,
  )
    .bind(since)
    .all<{ currency: string; orders: number; gross: number | null; tax: number | null }>();

  const refunds = await env.SHOP_DB.prepare(
    `SELECT currency, SUM(ABS(amount_minor)) AS refunded
       FROM payments
      WHERE kind = 'refund' AND status = 'succeeded' AND created_at >= ?
      GROUP BY currency`,
  )
    .bind(since)
    .all<{ currency: string; refunded: number | null }>();

  const byCurrency = sideBySide(sales.results, refunds.results, (r) => r.currency).map(([sale, back, currency]) => {
    const grossMinor = sale?.gross ?? 0;
    const refundedMinor = back?.refunded ?? 0;
    return {
      currency,
      orders: sale?.orders ?? 0,
      grossMinor,
      taxMinor: sale?.tax ?? 0,
      refundedMinor,
      netMinor: grossMinor - refundedMinor,
    };
  });

  const byChannel = await channelTotals(env.SHOP_DB, since);

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
    byChannel,
    needsReview: attention?.needsReview ?? 0,
    stalePending: attention?.stalePending ?? 0,
    stuckMessages: stuck?.n ?? 0,
    topProducts: top.results ?? [],
    byCountry: countries.results ?? [],
  });
};
