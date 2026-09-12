// GET /api/shop/admin/settings — what the shop is configured to do
// PUT /api/shop/admin/settings — change some of it
//
// Settings are the owner's business decisions (who is selling, in what
// currency, under whose VAT number). Secrets and bindings are not here: those
// live in wrangler and are never readable through the API (S-02).

import type { Env } from "../_env";
import { audit } from "../_audit";
import { requireOwner } from "../_auth";
import { fail, json, readJson } from "../_lib";
import { isKnownCurrency } from "../_money";
import { allSettings, putSettings, SETTING_DEFAULTS } from "../_settings";
import type { AdminData } from "./_middleware";

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ env }) => {
  return json({ settings: await allSettings(env), keys: Object.keys(SETTING_DEFAULTS).sort() });
};

/** Checks the settings whose wrong value would show up as a wrong invoice
 *  rather than as an error. Everything else is free text the owner owns. */
function validate(patch: Record<string, unknown>): string | null {
  const enumOf = (key: string, allowed: string[]): string | null => {
    const v = patch[key];
    if (v === undefined) return null;
    return allowed.includes(String(v)) ? null : `${key} must be one of: ${allowed.join(", ")}`;
  };

  const enumError =
    enumOf("pricing.mode", ["gross", "net"]) ??
    enumOf("tax.mode", ["none", "table", "stripe"]) ??
    enumOf("tax.rounding", ["half_up"]);
  if (enumError) return enumError;

  const currency = patch["currency.base"];
  if (currency !== undefined && !isKnownCurrency(String(currency))) {
    return "currency.base must be a three-letter currency code.";
  }

  const country = patch["seller.country"];
  if (country !== undefined && String(country) !== "" && !/^[A-Za-z]{2}$/.test(String(country))) {
    return "seller.country must be a two-letter country code.";
  }

  // {n} is what makes an invoice number unique; a format without it would give
  // every invoice of the year the same number.
  const format = patch["invoice.format"];
  if (format !== undefined && !String(format).includes("{n")) {
    return "invoice.format must contain {n} or {n:06}.";
  }

  for (const key of ["shop.countries_allowed", "webhooks.endpoints"]) {
    const v = patch[key];
    if (v !== undefined && !Array.isArray(v)) return `${key} must be a list.`;
  }
  return null;
}

export const onRequestPut: PagesFunction<Env, string, AdminData> = async ({ request, env, data }) => {
  const denied = requireOwner(request, data.admin);
  if (denied) return denied;

  const patch = await readJson<Record<string, unknown>>(request);
  if (!patch || typeof patch !== "object" || Array.isArray(patch)) {
    return fail(request, "invalid_body", "Send an object of settings to change.", 400);
  }

  const problem = validate(patch);
  if (problem) return fail(request, "invalid_setting", problem, 422);

  const unknown = await putSettings(env, patch);
  if (unknown.length > 0) {
    return fail(request, "unknown_setting", `This shop has no setting called ${unknown.join(", ")}.`, 422);
  }

  // The values are the owner's own configuration, but seller.vat_id and the
  // addresses end up on invoices, so who changed what is worth keeping.
  await audit(env, `admin:${data.admin.sub}`, "settings.update", null, { keys: Object.keys(patch) });
  return json({ settings: await allSettings(env) });
};
