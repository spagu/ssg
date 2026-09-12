// GET  /api/shop/admin/products — the catalogue, drafts included
// POST /api/shop/admin/products — create one

import type { Env } from "../../_env";
import { audit } from "../../_audit";
import { requireOwner } from "../../_auth";
import { fail, json, newId, nowISO, readJson, SKU_RE, str } from "../../_lib";
import type { PriceRow, ProductRow } from "../../_types";
import type { AdminData } from "../_middleware";

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ env }) => {
  const { results } = await env.SHOP_DB.prepare(`SELECT * FROM products ORDER BY updated_at DESC`).all<ProductRow>();
  const products = results ?? [];

  const prices = products.length
    ? (
        await env.SHOP_DB.prepare(`SELECT * FROM prices WHERE product_id IN (${products.map(() => "?").join(",")})`)
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

  return json({
    products: products.map((p) => ({ ...p, prices: byProduct.get(p.id) ?? {} })),
  });
};

interface ProductBody {
  sku?: string;
  name?: string;
  description?: string;
  status?: string;
  taxCategory?: string;
  downloadLimit?: number;
  downloadDays?: number;
  imageUrl?: string;
}

export const onRequestPost: PagesFunction<Env, string, AdminData> = async ({ request, env, data }) => {
  const denied = requireOwner(request, data.admin);
  if (denied) return denied;

  const body = await readJson<ProductBody>(request);
  if (!body) return fail(request, "invalid_body", "The request body could not be read.", 400);

  const sku = str(body.sku, 64);
  if (!SKU_RE.test(sku)) {
    return fail(request, "invalid_sku", "A product code may use letters, digits, dot, dash and underscore.", 422);
  }
  const name = str(body.name, 200);
  if (!name) return fail(request, "invalid_name", "A product needs a name.", 422);

  const existing = await env.SHOP_DB.prepare(`SELECT id FROM products WHERE sku = ?`).bind(sku).first<{ id: string }>();
  if (existing) return fail(request, "sku_taken", "A product with that code already exists.", 409);

  const now = nowISO();
  const id = newId();
  await env.SHOP_DB.prepare(
    `INSERT INTO products (id, sku, name, description, kind, status, tax_category, download_limit, download_days, image_url, created_at, updated_at)
     VALUES (?, ?, ?, ?, 'digital', ?, ?, ?, ?, ?, ?, ?)`,
  )
    .bind(
      id,
      sku,
      name,
      str(body.description, 20000) || null,
      body.status === "active" ? "active" : "draft",
      str(body.taxCategory, 32) || "ebook",
      clampInt(body.downloadLimit, 1, 100, 5),
      clampInt(body.downloadDays, 1, 3650, 30),
      str(body.imageUrl, 500) || null,
      now,
      now,
    )
    .run();

  await audit(env, actorOf(data), "product.create", id, { sku, name });
  const product = await env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`).bind(id).first<ProductRow>();
  return json({ product }, 201);
};

export function clampInt(value: unknown, min: number, max: number, fallback: number): number {
  const n = Number.parseInt(String(value ?? ""), 10);
  if (!Number.isFinite(n)) return fallback;
  return Math.min(max, Math.max(min, n));
}

export function actorOf(data: AdminData): string {
  return `admin:${data.admin.sub}`;
}
