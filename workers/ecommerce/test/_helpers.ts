// Shared setup for the shop's tests.
//
// Each test file gets its own isolate and its own storage, so these build the
// shop from nothing rather than depending on what another test left behind.

import { env } from "./_env";
import {  } from "node:test";
import assert from "node:assert/strict";
import { invalidateSettings, putSettings } from "../functions/api/shop/_settings";
import { ensureSchema, resetSchemaCache } from "../functions/api/shop/_schema";
import { newId, nowISO } from "../functions/api/shop/_lib";
import type { Admin } from "../functions/api/shop/_auth";

export const ORIGIN = "https://shop.example.com";

/** Who the admin gate hands a handler (GO-155): an `auth` account holding
 *  every shop permission. `actor` is what the audit log records. The gate
 *  itself — modes, permissions, CSRF — is tested in admin-*.test.ts. */
export const owner: Admin = {
  mode: "auth",
  user: { id: "owner-1", email: "owner@example.com" },
  perms: ["ecommerce:*"],
  actor: "auth:owner-1 owner@example.com",
};

/** Clears the rate-limit counters.
 *
 *  They live in KV, which each test FILE gets rather than each test,
 *  so without this the twentieth test to call checkout is refused for the
 *  nineteen before it — all of them arriving from the same (absent) address. */
export async function resetBudgets(): Promise<void> {
  if (!env.SHOP_KV) return;
  const { keys } = await env.SHOP_KV.list({ prefix: "rl:" });
  await Promise.all(keys.map((k) => env.SHOP_KV!.delete(k.name)));
}

/** Empties every table the shop owns.
 *
 *  Each test FILE gets its own storage, not each test, so a fixture seeded by
 *  one test would collide with the next one's. Clearing is cheaper and clearer
 *  than making every fixture unique. */
export async function emptyShop(): Promise<void> {
  const { results } = await env.SHOP_DB.prepare(
    `SELECT name FROM sqlite_master WHERE type = 'table'
       AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '\\_cf\\_%' ESCAPE '\\'`,
  ).all<{ name: string }>();
  const tables = (results ?? []).map((r) => r.name);
  if (tables.length === 0) return;
  await env.SHOP_DB.batch(tables.map((t) => env.SHOP_DB.prepare(`DELETE FROM ${t}`)));
}

/** A shop that is configured well enough to sell something. */
export async function freshShop(overrides: Record<string, unknown> = {}): Promise<void> {
  resetSchemaCache();
  invalidateSettings();
  await ensureSchema(env);
  await emptyShop();
  await resetBudgets();
  // Emptying removed the migration bookkeeping and the seeded rates as well, so
  // the schema runs once more to put both back.
  resetSchemaCache();
  await ensureSchema(env);
  const failed = await putSettings(env, {
    "seller.name": "Example Press",
    "seller.address": "1 Example Street",
    "seller.country": "PL",
    "seller.vat_id": "PL1234567890",
    "seller.email": "help@example.com",
    "shop.name": "Example Press",
    "shop.url": ORIGIN,
    "currency.base": "EUR",
    "pricing.mode": "gross",
    "tax.mode": "table",
    ...overrides,
  });
  assert.deepEqual(failed, []);
}

export interface SeededProduct {
  id: string;
  sku: string;
}

/** A product on sale, with a file behind it. */
export async function seedProduct(
  opts: { sku?: string; name?: string; priceMinor?: number; currency?: string; withFile?: boolean } = {},
): Promise<SeededProduct> {
  const id = newId();
  const sku = opts.sku ?? "EBOOK-1";
  const withFile = opts.withFile !== false;
  const key = `products/${id}/book.pdf`;

  await env.SHOP_DB.prepare(
    `INSERT INTO products (id, sku, name, description, kind, status, tax_category,
                           file_key, file_name, file_size, file_sha256,
                           download_limit, download_days, image_url, created_at, updated_at)
     VALUES (?, ?, ?, 'A book.', 'digital', 'active', 'ebook', ?, ?, ?, ?, 3, 30, NULL, ?, ?)`,
  )
    .bind(
      id,
      sku,
      opts.name ?? "A Book",
      withFile ? key : null,
      withFile ? "book.pdf" : null,
      withFile ? 11 : null,
      withFile ? "0".repeat(64) : null,
      nowISO(),
      nowISO(),
    )
    .run();

  await env.SHOP_DB.prepare(`INSERT INTO prices (product_id, currency, amount_minor) VALUES (?, ?, ?)`)
    .bind(id, opts.currency ?? "EUR", opts.priceMinor ?? 2400)
    .run();

  if (withFile && env.SHOP_FILES) {
    await env.SHOP_FILES.put(key, new TextEncoder().encode("%PDF-1.4\nx\n"), {
      httpMetadata: { contentType: "application/pdf" },
    });
  }
  return { id, sku };
}

/** A request the endpoints will accept: same origin, JSON body. */
export function post(path: string, body: unknown, headers: Record<string, string> = {}): Request {
  return new Request(`${ORIGIN}${path}`, {
    method: "POST",
    headers: { "content-type": "application/json", origin: ORIGIN, ...headers },
    body: typeof body === "string" ? body : JSON.stringify(body),
  });
}

export function get(path: string, headers: Record<string, string> = {}): Request {
  return new Request(`${ORIGIN}${path}`, { headers });
}

/** The context shape a Pages Function is called with. `waitUntil` runs the
 *  promise immediately rather than dropping it: a test that ignored background
 *  work would pass while the outbox never drained. */
export function ctx<T extends Record<string, unknown> = Record<string, unknown>>(
  request: Request,
  params: Record<string, string> = {},
  data: T = {} as T,
): Parameters<PagesFunction<typeof env, string, T>>[0] & { settled: () => Promise<unknown> } {
  const pending: Promise<unknown>[] = [];
  return {
    request,
    env,
    params,
    data,
    functionPath: new URL(request.url).pathname,
    waitUntil: (p: Promise<unknown>) => pending.push(p),
    passThroughOnException: () => {},
    next: async () => new Response("next"),
    // Tests that care about background work await this.
    settled: () => Promise.allSettled(pending),
  } as never;
}

export async function bodyOf<T = Record<string, unknown>>(res: Response): Promise<T> {
  return (await res.json()) as T;
}

/** assertPartial asserts that `actual` contains everything `expected` names:
 *  objects are compared on the expected keys only, recursively; arrays must
 *  have the same length, element by element; anything else must be equal. */
export function assertPartial(actual: unknown, expected: unknown, path = "$"): void {
  if (Array.isArray(expected)) {
    assert.ok(Array.isArray(actual), `${path}: expected an array`);
    assert.equal(actual.length, expected.length, `${path}: length`);
    expected.forEach((e, i) => assertPartial(actual[i], e, `${path}[${i}]`));
    return;
  }
  if (expected !== null && typeof expected === "object" && !(expected instanceof RegExp)) {
    assert.ok(actual !== null && typeof actual === "object", `${path}: expected an object`);
    for (const [k, v] of Object.entries(expected)) assertPartial((actual as Record<string, unknown>)[k], v, `${path}.${k}`);
    return;
  }
  assert.deepEqual(actual, expected, path);
}
