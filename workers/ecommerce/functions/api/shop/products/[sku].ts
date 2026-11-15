// GET /api/shop/products/:sku — one product, for a product page.

import type { Env } from "../_env";
import { cached, fail, SKU_RE } from "../_lib";
import { ensureSchema } from "../_schema";
import type { PriceRow, ProductRow } from "../_types";

export const onRequestGet: PagesFunction<Env> = async ({ request, env, params }) => {
  if (!env.SHOP_DB) return fail(request, "not_configured", "The shop database is not bound. Bind SHOP_DB.", 503);
  await ensureSchema(env);

  const sku = String(params.sku ?? "");
  if (!SKU_RE.test(sku)) return fail(request, "invalid_sku", "That is not a valid product code.", 400);

  const product = await env.SHOP_DB.prepare(`SELECT * FROM products WHERE sku = ? AND status = 'active'`)
    .bind(sku)
    .first<ProductRow>();
  if (!product) return fail(request, "not_found", "No such product.", 404);

  const { results } = await env.SHOP_DB.prepare(`SELECT * FROM prices WHERE product_id = ?`)
    .bind(product.id)
    .all<PriceRow>();
  const prices: Record<string, number> = {};
  for (const p of results ?? []) prices[p.currency] = p.amount_minor;

  return cached(
    {
      sku: product.sku,
      name: product.name,
      description: product.description,
      kind: product.kind,
      imageUrl: product.image_url,
      fileName: product.file_name,
      fileSize: product.file_size,
      prices,
    },
    60,
  );
};
