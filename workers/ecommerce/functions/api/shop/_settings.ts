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
  // Which payment providers to offer, and in what order the buttons appear.
  // Empty means "whatever SHOP_GATEWAYS says", which is how a shop that has
  // never opened the payments screen behaves.
  "gateways.order": [],

  // ── Modules ───────────────────────────────────────────────────────────────
  //
  // What this shop does, as opposed to what it is configured with. A module
  // that is off is off even when its secrets are present — that is the whole
  // point of a switch, and it is how a seller stops the shop emailing anyone
  // while they test, or stops it invoicing while their accountant decides how
  // they want it done.
  //
  // Every one of these is read where the work happens, not only where it is
  // drawn. A toggle that changes nothing is worse than no toggle.
  "modules.invoices": true, // issue an invoice and a credit note per order
  "modules.emails": true, // the buyer's own "here is your book" email
  "modules.admin_notices": true, // "you sold something", to the owner
  "modules.webhooks": true, // the outgoing webhooks in webhooks.endpoints
  "modules.tracking": false, // server-side GA4 / Meta, off until asked for
  "modules.turnstile": true, // the anti-spam check, when a secret is set
  "modules.resend": true, // the public "I lost my download" form
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

/** Whether a module is switched on. Unknown module: off, because a name nobody
 *  declared is a typo, and a typo must not turn something on. */
export async function moduleOn(env: Env, name: string): Promise<boolean> {
  const key = `modules.${name}`;
  if (!(key in SETTING_DEFAULTS)) return false;
  return Boolean(await getSetting(env, key));
}

/** The modules, with what each one needs in order to be usable at all.
 *
 *  Separating "off" from "cannot work" is the point: a switch offered for
 *  something with no credentials behind it is a switch that lies. */
export interface ModuleState {
  name: string;
  on: boolean;
  /** Null when the module can run; otherwise what is missing, in words. */
  blockedBy: string | null;
}

export async function moduleStates(env: Env): Promise<ModuleState[]> {
  const s = await allSettings(env);
  const on = (name: string): boolean => Boolean(s[`modules.${name}`]);
  const mail = env.SHOP_MAIL_URL && env.SHOP_MAIL_FROM ? null : "SHOP_MAIL_URL and SHOP_MAIL_FROM";

  return [
    { name: "invoices", on: on("invoices"), blockedBy: null },
    { name: "emails", on: on("emails"), blockedBy: mail },
    {
      name: "admin_notices",
      on: on("admin_notices"),
      blockedBy: mail ?? (env.SHOP_MAIL_ADMIN ? null : "SHOP_MAIL_ADMIN"),
    },
    {
      name: "webhooks",
      on: on("webhooks"),
      blockedBy: Array.isArray(s["webhooks.endpoints"]) && s["webhooks.endpoints"].length > 0
        ? null
        : "an endpoint in webhooks.endpoints",
    },
    {
      name: "tracking",
      on: on("tracking"),
      blockedBy:
        (env.GA4_MEASUREMENT_ID && env.GA4_API_SECRET) || (env.META_PIXEL_ID && env.META_ACCESS_TOKEN)
          ? null
          : "GA4 or Meta credentials",
    },
    { name: "turnstile", on: on("turnstile"), blockedBy: env.TURNSTILE_SECRET ? null : "TURNSTILE_SECRET" },
    { name: "resend", on: on("resend"), blockedBy: null },
  ];
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
