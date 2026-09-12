// GET    /api/shop/admin/vat-rates — the rate table
// POST   /api/shop/admin/vat-rates — add or supersede a rate
// DELETE /api/shop/admin/vat-rates?country=..&kind=..&validFrom=.. — remove one
//
// Rates are dated, never overwritten: an invoice from March must still be able
// to explain the rate it used, even after the rate changes in July.

import type { Env } from "../_env";
import { audit } from "../_audit";
import { requireOwner } from "../_auth";
import { COUNTRY_RE, fail, json, readJson, str } from "../_lib";
import type { AdminData } from "./_middleware";

interface RateRow {
  country: string;
  kind: string;
  rate_bp: number;
  valid_from: string;
  valid_to: string | null;
}

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env }) => {
  const country = str(new URL(request.url).searchParams.get("country"), 40).toUpperCase();
  const query = country
    ? env.SHOP_DB.prepare(`SELECT * FROM vat_rates WHERE country = ? ORDER BY kind, valid_from DESC`).bind(country)
    : env.SHOP_DB.prepare(`SELECT * FROM vat_rates ORDER BY country, kind, valid_from DESC`);
  const { results } = await query.all<RateRow>();
  return json({ rates: results ?? [] });
};

interface RateBody {
  country?: string;
  kind?: string;
  rateBp?: number;
  validFrom?: string;
}

export const onRequestPost: PagesFunction<Env, string, AdminData> = async ({ request, env, data }) => {
  const denied = requireOwner(request, data.admin);
  if (denied) return denied;

  const body = await readJson<RateBody>(request);
  if (!body) return fail(request, "invalid_body", "The request body could not be read.", 400);

  // Full length first, then checked: truncating would quietly turn a typed
  // country name into a different country's code.
  const country = str(body.country, 40).toUpperCase();
  if (!COUNTRY_RE.test(country)) return fail(request, "invalid_country", "A two-letter country code is required.", 422);

  const kind = str(body.kind, 32) || "standard";
  const rateBp = Number.parseInt(String(body.rateBp ?? ""), 10);
  if (!Number.isFinite(rateBp) || rateBp < 0 || rateBp > 10000) {
    return fail(request, "invalid_rate", "The rate is in basis points, from 0 to 10000 (2300 = 23%).", 422);
  }

  const validFrom = str(body.validFrom, 40) || new Date().toISOString();
  if (Number.isNaN(Date.parse(validFrom))) {
    return fail(request, "invalid_date", "validFrom must be a date.", 422);
  }

  // The rate that was in force until now is closed off at the moment the new
  // one starts, so a lookup by date has exactly one answer.
  await env.SHOP_DB.prepare(
    `UPDATE vat_rates SET valid_to = ?
      WHERE country = ? AND kind = ? AND valid_from < ? AND (valid_to IS NULL OR valid_to > ?)`,
  )
    .bind(validFrom, country, kind, validFrom, validFrom)
    .run();

  await env.SHOP_DB.prepare(
    `INSERT INTO vat_rates (country, kind, rate_bp, valid_from, valid_to) VALUES (?, ?, ?, ?, NULL)
       ON CONFLICT(country, kind, valid_from) DO UPDATE SET rate_bp = excluded.rate_bp, valid_to = NULL`,
  )
    .bind(country, kind, rateBp, validFrom)
    .run();

  await audit(env, `admin:${data.admin.sub}`, "vat.set", `${country}/${kind}`, { rateBp, validFrom });
  return json({ ok: true, country, kind, rateBp, validFrom }, 201);
};

export const onRequestDelete: PagesFunction<Env, string, AdminData> = async ({ request, env, data }) => {
  const denied = requireOwner(request, data.admin);
  if (denied) return denied;

  const q = new URL(request.url).searchParams;
  const country = str(q.get("country"), 40).toUpperCase();
  const kind = str(q.get("kind"), 32);
  const validFrom = str(q.get("validFrom"), 40);
  if (!COUNTRY_RE.test(country) || !kind || !validFrom) {
    return fail(request, "invalid_query", "country, kind and validFrom are all required.", 422);
  }

  const res = await env.SHOP_DB.prepare(
    `DELETE FROM vat_rates WHERE country = ? AND kind = ? AND valid_from = ?`,
  )
    .bind(country, kind, validFrom)
    .run();
  if ((res.meta?.changes ?? 0) === 0) return fail(request, "not_found", "No such rate.", 404);

  await audit(env, `admin:${data.admin.sub}`, "vat.delete", `${country}/${kind}`, { validFrom });
  return json({ ok: true });
};
