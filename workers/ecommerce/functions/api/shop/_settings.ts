// Shop settings: the things the owner edits in the panel, as opposed to the
// bindings and secrets that live in wrangler.
//
// Stored as JSON values in one D1 table, read through a per-isolate cache with
// a short TTL so the hot path (a product listing, a checkout) does not pay a
// query per setting. The TTL is what makes a change in the panel appear within
// a few seconds without a deploy.

import type { Env } from "./_env";

export const SETTING_DEFAULTS: Record<string, unknown> = {
  "seller.name": "",
  "seller.address": "",
  "seller.country": "",
  "seller.vat_id": "",
  "seller.email": "",
  "seller.registry": "",
  "shop.name": "Shop",
  "shop.url": "",
  "shop.countries_allowed": [], // empty = every country the tax rules can price
  "currency.base": "EUR",
  "pricing.mode": "gross", // gross | net
  "tax.mode": "table", // none | table | stripe
  "tax.rounding": "half_up",
  // {series} is not decoration: credit notes use a different series, and a
  // format with the series spelled out would number them FV/… as well, which
  // collides with the invoice of the same number.
  "invoice.format": "{series}/{year}/{n:06}",
  "invoice.series": "FV",
  "invoice.credit_series": "KOR",
  "invoice.separate_email": false,
  "invoice.vat_in_local": false,
  "downloads.renew_on_resend": true,
  "legal.digital_waiver": true, // ask the buyer to waive withdrawal before download
  "legal.terms_url": "/shop/terms/",
  "legal.privacy_url": "/shop/privacy/",
  "webhooks.endpoints": [],
};

interface CacheEntry {
  values: Record<string, unknown>;
  at: number;
}

const TTL_MS = 5000;
let cache: CacheEntry | null = null;

/** Drops the cache. Called after a settings write, and by tests. */
export function invalidateSettings(): void {
  cache = null;
}

export async function allSettings(env: Env): Promise<Record<string, unknown>> {
  const now = Date.now();
  if (cache && now - cache.at < TTL_MS) return cache.values;

  const values: Record<string, unknown> = { ...SETTING_DEFAULTS };
  const { results } = await env.SHOP_DB.prepare(`SELECT key, value FROM settings`).all<{
    key: string;
    value: string;
  }>();
  for (const row of results ?? []) {
    try {
      values[row.key] = JSON.parse(row.value);
    } catch {
      // A value that is not JSON is a value written by hand; keep it as text
      // rather than losing it.
      values[row.key] = row.value;
    }
  }
  cache = { values, at: now };
  return values;
}

export async function getSetting(env: Env, key: string): Promise<unknown> {
  const all = await allSettings(env);
  return all[key];
}

export async function getSettingString(env: Env, key: string): Promise<string> {
  const v = await getSetting(env, key);
  return typeof v === "string" ? v : v == null ? "" : String(v);
}

export async function getSettingBool(env: Env, key: string): Promise<boolean> {
  return Boolean(await getSetting(env, key));
}

/** Writes settings, refusing keys nobody declared.
 *
 *  An unknown key is a typo — accepting it would leave a setting that looks
 *  saved in the panel and is never read anywhere. */
export async function putSettings(env: Env, patch: Record<string, unknown>): Promise<string[]> {
  const unknown = Object.keys(patch).filter((k) => !(k in SETTING_DEFAULTS));
  if (unknown.length > 0) return unknown;

  const now = new Date().toISOString();
  const stmt = env.SHOP_DB.prepare(
    `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
       ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
  );
  const batch = Object.entries(patch).map(([k, v]) => stmt.bind(k, JSON.stringify(v), now));
  if (batch.length > 0) await env.SHOP_DB.batch(batch);
  invalidateSettings();
  return [];
}

/** The seller block an invoice freezes. Missing pieces are the owner's problem
 *  to fix in the panel; the invoice records what was configured at the time. */
export interface SellerSnapshot {
  name: string;
  address: string;
  country: string;
  vatId: string;
  email: string;
  registry: string;
}

export async function sellerSnapshot(env: Env): Promise<SellerSnapshot> {
  const s = await allSettings(env);
  return {
    name: String(s["seller.name"] ?? ""),
    address: String(s["seller.address"] ?? ""),
    country: String(s["seller.country"] ?? "").toUpperCase(),
    vatId: String(s["seller.vat_id"] ?? ""),
    email: String(s["seller.email"] ?? ""),
    registry: String(s["seller.registry"] ?? ""),
  };
}

/** True when the shop has enough configuration to take money. The checkout
 *  refuses before creating an order rather than after taking one. */
export async function shopIsConfigured(env: Env): Promise<{ ok: boolean; missing: string[] }> {
  const s = await allSettings(env);
  const missing: string[] = [];
  if (!s["seller.name"]) missing.push("seller.name");
  if (!s["currency.base"]) missing.push("currency.base");
  if ((s["tax.mode"] ?? "table") === "table" && !s["seller.country"]) missing.push("seller.country");
  return { ok: missing.length === 0, missing };
}
