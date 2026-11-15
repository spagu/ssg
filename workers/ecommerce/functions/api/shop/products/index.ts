// GET /api/shop/products — the catalogue the storefront reads.
//
// Public, cacheable, and deliberately thin: what a page needs to show a price
// and a buy button. Nothing about files, keys or stock.
//
// Paged, and answerable by product code. A storefront page shows the handful of
// products that are on it, so it asks for those by name — `?sku=A,B,C` — rather
// than downloading a catalogue to find three prices. Without that it gets a
// page, because a shop with five thousand books must not answer with five
// thousand books, and the earlier version both did that and looked their prices
// up with one bind per product, which SQLite refuses past its variable limit.

import type { Env } from "../_env";
import { offeredGateways } from "../_gateways";
import { pricesFor } from "../_catalogue";
import { cached, fail, SKU_RE, str } from "../_lib";
import { drainInBackground } from "../_outbox";
import { ensureSchema } from "../_schema";
import { allSettings } from "../_settings";
import type { ProductRow } from "../_types";

interface PublicProduct {
  sku: string;
  name: string;
  description: string | null;
  kind: string;
  imageUrl: string | null;
  fileName: string | null;
  fileSize: number | null;
  prices: Record<string, number>;
}

/** The most a single answer will carry. A page asking for more than this is a
 *  page that should be asking by code. */
const MAX_LIMIT = 200;
const DEFAULT_LIMIT = 50;
const MAX_SKUS = 50;

export const onRequestGet: PagesFunction<Env> = async ({ request, env, waitUntil }) => {
  if (!env.SHOP_DB) return fail(request, "not_configured", "The shop database is not bound. Bind SHOP_DB.", 503);
  await ensureSchema(env);

  // Ordinary public traffic is what keeps the outbox moving on a shop whose
  // owner is not looking at the panel (see _outbox).
  drainInBackground(env, { waitUntil });

  const q = new URL(request.url).searchParams;
  const wanted = str(q.get("sku"), 2000)
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean)
    .filter((s) => SKU_RE.test(s))
    .slice(0, MAX_SKUS);

  const limit = wanted.length
    ? wanted.length
    : Math.min(MAX_LIMIT, Math.max(1, Number.parseInt(q.get("limit") ?? String(DEFAULT_LIMIT), 10) || DEFAULT_LIMIT));
  const cursor = str(q.get("cursor"), 300);

  const where = ["status = 'active'"];
  const binds: unknown[] = [];
  if (wanted.length) {
    where.push(`sku IN (${wanted.map(() => "?").join(",")})`);
    binds.push(...wanted);
  } else if (cursor) {
    // Ordered by name, so the cursor is a name and the id breaks the tie
    // between two products sharing one.
    const at = cursor.indexOf("|");
    const [name, id] = at === -1 ? [cursor, ""] : [cursor.slice(0, at), cursor.slice(at + 1)];
    where.push("(name > ? OR (name = ? AND id > ?))");
    binds.push(name, name, id);
  }

  const [listed, counted] = await Promise.all([
    env.SHOP_DB.prepare(
      `SELECT * FROM products WHERE ${where.join(" AND ")} ORDER BY name, id LIMIT ?`,
    )
      .bind(...binds, limit + 1)
      .all<ProductRow>(),
    env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM products WHERE status = 'active'`).first<{ n: number }>(),
  ]);

  const rows = listed.results ?? [];
  const products = rows.slice(0, limit);
  const last = products.at(-1);

  // In chunks: even one page can hold more products than a single statement may
  // name. See _catalogue.
  const byProduct = await pricesFor(env, products.map((p) => p.id));

  const settings = await allSettings(env);
  const body = {
    shop: {
      name: settings["shop.name"],
      baseCurrency: settings["currency.base"],
      pricingMode: settings["pricing.mode"],
      gateways: await offeredGateways(env),
      termsUrl: settings["legal.terms_url"],
      privacyUrl: settings["legal.privacy_url"],
      digitalWaiver: Boolean(settings["legal.digital_waiver"]),
      // So a theme does not link to a form this shop has switched off.
      resend: settings["modules.resend"] !== false,
    },
    products: products.map(
      (p): PublicProduct => ({
        sku: p.sku,
        name: p.name,
        description: p.description,
        kind: p.kind,
        imageUrl: p.image_url,
        fileName: p.file_name,
        fileSize: p.file_size,
        prices: byProduct.get(p.id) ?? {},
      }),
    ),
    total: counted?.n ?? 0,
    nextCursor: !wanted.length && rows.length > limit && last ? `${last.name}|${last.id}` : null,
  };

  // A minute is short enough that a price change appears quickly and long
  // enough that a busy page does not query per visitor.
  return cached(body, 60);
};
