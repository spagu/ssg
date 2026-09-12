// Reading prices for a set of products.
//
// Its own module because both the public catalogue and the admin list need it
// and both got it wrong the same way: `WHERE product_id IN (?, ?, …)`, one bind
// per product. D1 refuses a statement with too many bound parameters, so the
// query did not slow down as a shop grew — it started failing outright, at a
// size a demo never reaches and a real shop passes in its first afternoon.
//
// Paging the lists fixed the worst of it. This fixes the rest: even one page
// can hold more products than a single statement may name.

import type { Env } from "./_env";
import type { PriceRow } from "./_types";

/** How many ids one statement may name.
 *
 *  D1's own limit is higher than this, and deliberately not what is used: a
 *  number chosen to fit exactly is a number that breaks when a caller adds one
 *  more condition to the same statement. */
const CHUNK = 50;

/** Currency to minor units, per product id. Products with no price are absent
 *  rather than present and empty, so a caller can tell "free" from "unpriced" —
 *  the shop refuses to sell the second one. */
export async function pricesFor(env: Env, productIds: string[]): Promise<Map<string, Record<string, number>>> {
  const byProduct = new Map<string, Record<string, number>>();
  if (productIds.length === 0) return byProduct;

  for (let i = 0; i < productIds.length; i += CHUNK) {
    const chunk = productIds.slice(i, i + CHUNK);
    const { results } = await env.SHOP_DB.prepare(
      `SELECT * FROM prices WHERE product_id IN (${chunk.map(() => "?").join(",")})`,
    )
      .bind(...chunk)
      .all<PriceRow>();

    for (const row of results ?? []) {
      const entry = byProduct.get(row.product_id) ?? {};
      entry[row.currency] = row.amount_minor;
      byProduct.set(row.product_id, entry);
    }
  }
  return byProduct;
}
