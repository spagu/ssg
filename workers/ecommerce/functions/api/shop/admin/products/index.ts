// GET  /api/shop/admin/products — the catalogue, drafts included
// POST /api/shop/admin/products — create one
//
// Filtered, sorted and paged, for the same reason the order list is: a shop
// with five thousand products is a shop whose product list must not be one
// response. The earlier version fetched every product and then every price with
// `WHERE product_id IN (?, ?, …)` — one bind per product — which SQLite refuses
// past its variable limit. At a few hundred products it worked; at a few
// thousand the screen simply stopped loading.

import type { Env } from "../../_env";
import { pricesFor } from "../../_catalogue";
import { audit } from "../../_audit";
import { requireOwner } from "../../_auth";
import { fail, json, newId, nowISO, readJson, SKU_RE, str } from "../../_lib";
import type { ProductRow } from "../../_types";
import type { AdminData } from "../_middleware";

const STATUSES = new Set(["draft", "active", "archived"]);

/** What may be sorted on. A whitelist, because the value ends up in the SQL. */
const SORTS: Record<string, string> = {
  updated: "p.updated_at",
  name: "p.name",
  sku: "p.sku",
  status: "p.status",
  created: "p.created_at",
};

function cursorValue(row: ProductRow, sort: string): string {
  switch (sort) {
    case "name":
      return row.name;
    case "sku":
      return row.sku;
    case "status":
      return row.status;
    case "created":
      return row.created_at;
    default:
      return row.updated_at;
  }
}

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env }) => {
  const q = new URL(request.url).searchParams;
  const limit = Math.min(100, Math.max(1, Number.parseInt(q.get("limit") ?? "25", 10) || 25));
  const cursor = str(q.get("cursor"), 300);
  const status = str(q.get("status"), 20);
  const search = str(q.get("q"), 200).toLowerCase();
  const sort = str(q.get("sort"), 20) || "updated";
  const dir = str(q.get("dir"), 4).toLowerCase() === "asc" ? "asc" : "desc";

  if (status && !STATUSES.has(status)) {
    return fail(request, "invalid_status", "A product is draft, active or archived.", 422);
  }
  if (!(sort in SORTS)) {
    return fail(request, "invalid_sort", `Sort by one of: ${Object.keys(SORTS).join(", ")}.`, 422);
  }

  const where: string[] = [];
  const binds: unknown[] = [];
  if (status) {
    where.push("p.status = ?");
    binds.push(status);
  }
  if (search) {
    where.push("(LOWER(p.sku) LIKE ? OR LOWER(p.name) LIKE ?)");
    binds.push(`%${search}%`, `%${search}%`);
  }

  const filterSql = where.length ? `WHERE ${where.join(" AND ")}` : "";
  const filterBinds = [...binds];

  const column = SORTS[sort]!;
  const comparison = dir === "asc" ? ">" : "<";
  if (cursor) {
    const at = cursor.indexOf("|");
    const [value, id] = at === -1 ? [cursor, ""] : [cursor.slice(0, at), cursor.slice(at + 1)];
    where.push(`(${column} ${comparison} ? OR (${column} = ? AND p.id ${comparison} ?))`);
    binds.push(value, value, id);
  }

  const [listed, counted] = await Promise.all([
    env.SHOP_DB.prepare(
      `SELECT * FROM products p
       ${where.length ? `WHERE ${where.join(" AND ")}` : ""}
       ORDER BY ${column} ${dir.toUpperCase()}, p.id ${dir.toUpperCase()}
       LIMIT ?`,
    )
      .bind(...binds, limit + 1)
      .all<ProductRow>(),
    env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM products p ${filterSql}`)
      .bind(...filterBinds)
      .first<{ n: number }>(),
  ]);

  const rows = listed.results ?? [];
  const page = rows.slice(0, limit);
  const last = page.at(-1);

  // Prices for this page only, and in chunks even then: never one bind per
  // product the shop owns. See _catalogue.
  const byProduct = await pricesFor(env, page.map((p) => p.id));

  return json({
    products: page.map((p) => ({ ...p, prices: byProduct.get(p.id) ?? {} })),
    total: counted?.n ?? 0,
    sort,
    dir,
    nextCursor: rows.length > limit && last ? `${cursorValue(last, sort)}|${last.id}` : null,
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
