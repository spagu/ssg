// GET    /api/shop/admin/products/:id
// PATCH  /api/shop/admin/products/:id — edit fields, set prices
// DELETE /api/shop/admin/products/:id — archive, or really delete if unsold

import type { Env } from "../../_env";
import { audit } from "../../_audit";
import { requireOwner } from "../../_auth";
import { CURRENCY_RE, fail, json, nowISO, readJson, str } from "../../_lib";
import { isKnownCurrency } from "../../_money";
import type { PriceRow, ProductRow } from "../../_types";
import type { AdminData } from "../_middleware";
import { actorOf, clampInt } from "./index";

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env, params }) => {
  const product = await env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`)
    .bind(String(params.id ?? ""))
    .first<ProductRow>();
  if (!product) return fail(request, "not_found", "No such product.", 404);

  const { results } = await env.SHOP_DB.prepare(`SELECT * FROM prices WHERE product_id = ?`)
    .bind(product.id)
    .all<PriceRow>();
  const prices: Record<string, number> = {};
  for (const p of results ?? []) prices[p.currency] = p.amount_minor;

  const sold = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM order_items WHERE product_id = ?`)
    .bind(product.id)
    .first<{ n: number }>();

  return json({ product: { ...product, prices }, soldCount: sold?.n ?? 0 });
};

interface PatchBody {
  name?: string;
  description?: string;
  status?: string;
  taxCategory?: string;
  downloadLimit?: number;
  downloadDays?: number;
  imageUrl?: string;
  /** Currency code to minor units. An entry set to null removes that price. */
  prices?: Record<string, number | null>;
}

export const onRequestPatch: PagesFunction<Env, string, AdminData> = async ({ request, env, params, data }) => {
  const denied = requireOwner(request, data.admin);
  if (denied) return denied;

  const id = String(params.id ?? "");
  const product = await env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`).bind(id).first<ProductRow>();
  if (!product) return fail(request, "not_found", "No such product.", 404);

  const body = await readJson<PatchBody>(request);
  if (!body) return fail(request, "invalid_body", "The request body could not be read.", 400);

  // A product with no file cannot be sold: activating it would sell a download
  // that does not exist.
  const nextStatus = body.status === undefined ? product.status : body.status === "active" ? "active" : body.status === "archived" ? "archived" : "draft";
  if (nextStatus === "active" && !product.file_key) {
    return fail(request, "no_file", "Attach a file before putting this product on sale.", 422);
  }

  await env.SHOP_DB.prepare(
    `UPDATE products SET name = ?, description = ?, status = ?, tax_category = ?,
            download_limit = ?, download_days = ?, image_url = ?, updated_at = ?
      WHERE id = ?`,
  )
    .bind(
      body.name === undefined ? product.name : str(body.name, 200) || product.name,
      body.description === undefined ? product.description : str(body.description, 20000) || null,
      nextStatus,
      body.taxCategory === undefined ? product.tax_category : str(body.taxCategory, 32) || "ebook",
      body.downloadLimit === undefined ? product.download_limit : clampInt(body.downloadLimit, 1, 100, 5),
      body.downloadDays === undefined ? product.download_days : clampInt(body.downloadDays, 1, 3650, 30),
      body.imageUrl === undefined ? product.image_url : str(body.imageUrl, 500) || null,
      nowISO(),
      id,
    )
    .run();

  if (body.prices) {
    for (const [rawCurrency, amount] of Object.entries(body.prices)) {
      const currency = rawCurrency.toUpperCase();
      if (!CURRENCY_RE.test(currency) || !isKnownCurrency(currency)) {
        return fail(request, "invalid_currency", `${rawCurrency} is not a currency code.`, 422);
      }
      if (amount === null) {
        await env.SHOP_DB.prepare(`DELETE FROM prices WHERE product_id = ? AND currency = ?`)
          .bind(id, currency)
          .run();
        continue;
      }
      const minor = Number.parseInt(String(amount), 10);
      if (!Number.isFinite(minor) || minor < 0) {
        return fail(request, "invalid_amount", `The price for ${currency} must be a whole number of minor units.`, 422);
      }
      await env.SHOP_DB.prepare(
        `INSERT INTO prices (product_id, currency, amount_minor) VALUES (?, ?, ?)
           ON CONFLICT(product_id, currency) DO UPDATE SET amount_minor = excluded.amount_minor`,
      )
        .bind(id, currency, minor)
        .run();
    }
  }

  await audit(env, actorOf(data), "product.update", id, { sku: product.sku, status: nextStatus });
  const updated = await env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`).bind(id).first<ProductRow>();
  return json({ product: updated });
};

export const onRequestDelete: PagesFunction<Env, string, AdminData> = async ({ request, env, params, data }) => {
  const denied = requireOwner(request, data.admin);
  if (denied) return denied;

  const id = String(params.id ?? "");
  const product = await env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`).bind(id).first<ProductRow>();
  if (!product) return fail(request, "not_found", "No such product.", 404);

  const sold = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM order_items WHERE product_id = ?`)
    .bind(id)
    .first<{ n: number }>();

  // A product someone has bought is part of their order and their invoice
  // forever. It gets archived, never removed.
  if ((sold?.n ?? 0) > 0) {
    await env.SHOP_DB.prepare(`UPDATE products SET status = 'archived', updated_at = ? WHERE id = ?`)
      .bind(nowISO(), id)
      .run();
    await audit(env, actorOf(data), "product.archive", id, { sku: product.sku, reason: "has orders" });
    return json({ archived: true, deleted: false });
  }

  if (product.file_key && env.SHOP_FILES) await env.SHOP_FILES.delete(product.file_key);
  await env.SHOP_DB.batch([
    env.SHOP_DB.prepare(`DELETE FROM prices WHERE product_id = ?`).bind(id),
    env.SHOP_DB.prepare(`DELETE FROM products WHERE id = ?`).bind(id),
  ]);
  await audit(env, actorOf(data), "product.delete", id, { sku: product.sku });
  return json({ archived: false, deleted: true });
};
