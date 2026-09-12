// GET /api/shop/products — the catalogue the storefront reads.
//
// Public, cacheable, and deliberately thin: what a page needs to show a price
// and a buy button. Nothing about files, keys or stock.

import { enabledGateways, type Env } from "../_env";
import { cached, fail } from "../_lib";
import { drainInBackground } from "../_outbox";
import { ensureSchema } from "../_schema";
import { allSettings } from "../_settings";
import type { PriceRow, ProductRow } from "../_types";

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

export const onRequestGet: PagesFunction<Env> = async ({ request, env, waitUntil }) => {
  if (!env.SHOP_DB) return fail(request, "not_configured", "The shop database is not bound. Bind SHOP_DB.", 503);
  await ensureSchema(env);

  // Ordinary public traffic is what keeps the outbox moving on a shop whose
  // owner is not looking at the panel (see _outbox).
  drainInBackground(env, { waitUntil });

  const { results } = await env.SHOP_DB.prepare(
    `SELECT * FROM products WHERE status = 'active' ORDER BY name`,
  ).all<ProductRow>();
  const products = results ?? [];

  const prices = products.length
    ? (
        await env.SHOP_DB.prepare(
          `SELECT * FROM prices WHERE product_id IN (${products.map(() => "?").join(",")})`,
        )
          .bind(...products.map((p) => p.id))
          .all<PriceRow>()
      ).results ?? []
    : [];

  const byProduct = new Map<string, Record<string, number>>();
  for (const p of prices) {
    const entry = byProduct.get(p.product_id) ?? {};
    entry[p.currency] = p.amount_minor;
    byProduct.set(p.product_id, entry);
  }

  const settings = await allSettings(env);
  const body = {
    shop: {
      name: settings["shop.name"],
      baseCurrency: settings["currency.base"],
      pricingMode: settings["pricing.mode"],
      gateways: enabledGateways(env),
      termsUrl: settings["legal.terms_url"],
      privacyUrl: settings["legal.privacy_url"],
      digitalWaiver: Boolean(settings["legal.digital_waiver"]),
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
  };

  // A minute is short enough that a price change appears quickly and long
  // enough that a busy page does not query per visitor.
  return cached(body, 60);
};
