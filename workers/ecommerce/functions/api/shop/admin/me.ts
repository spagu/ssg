// GET /api/shop/admin/me — who am I, and what does this shop still need?
//
// The panel calls this first. It doubles as the shop's health check: an owner
// opening the panel on a fresh deployment should be told what is missing rather
// than left to discover it at the first sale.

import { authMode, enabledGateways, isTestMode, type Env } from "../_env";
import { json } from "../_lib";
import { rateLimitingAvailable } from "../_ratelimit";
import { allSettings, shopIsConfigured } from "../_settings";
import type { AdminData } from "./_middleware";

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ env, data }) => {
  const settings = await allSettings(env);
  const configured = await shopIsConfigured(env);
  const gateways = enabledGateways(env);

  // Each warning is something the owner can act on today, in the order it will
  // bite them.
  const warnings: string[] = [];
  if (!configured.ok) warnings.push(`Settings still missing: ${configured.missing.join(", ")}`);
  if (gateways.length === 0) warnings.push("No payment provider is configured, so nothing can be bought.");
  if (!env.SHOP_FILES) warnings.push("No file storage is bound (SHOP_FILES), so files cannot be delivered.");
  if (!env.SHOP_MAIL_URL || !env.SHOP_MAIL_FROM) {
    warnings.push("Email is not configured, so buyers will not receive their download links.");
  }
  if (!env.SHOP_KV) warnings.push("No KV namespace is bound; login throttling and range windows are off.");
  if (!rateLimitingAvailable(env)) {
    warnings.push(
      "Nothing can enforce a request budget here: checkout, resend and downloads are uncapped. " +
        "Bind SHOP_KV, or a Workers rate-limiting binding.",
    );
  }
  if (!env.SHOP_IP_SALT) warnings.push("SHOP_IP_SALT is not set, so no IP is recorded at all.");
  if (env.SHOP_ADMIN_BOOTSTRAP) {
    warnings.push("SHOP_ADMIN_BOOTSTRAP is still set. Delete it now that an owner exists.");
  }

  // VAT rates that nobody has looked at in a year are rates that have probably
  // moved (see the VAT plan).
  const newest = await env.SHOP_DB.prepare(`SELECT MAX(valid_from) AS v FROM vat_rates`).first<{ v: string | null }>();
  if (newest?.v && Date.now() - Date.parse(newest.v) > 365 * 86400000) {
    warnings.push("The VAT rates were last updated over a year ago. Check them.");
  }

  const pending = await env.SHOP_DB.prepare(
    `SELECT COUNT(*) AS n FROM outbox WHERE done_at IS NULL AND attempts >= 5`,
  ).first<{ n: number }>();
  if ((pending?.n ?? 0) > 0) warnings.push(`${pending?.n} queued message(s) have failed and need attention.`);

  return json({
    admin: data.admin,
    authMode: authMode(env),
    testMode: isTestMode(env),
    gateways,
    shopName: settings["shop.name"],
    baseCurrency: settings["currency.base"],
    warnings,
  });
};
