// POST /api/shop/checkout — turn a basket into an order and a payment URL.
//
// The order of operations is the point of this file:
//
//   validate → Turnstile → price from the database → write the order →
//   ask the gateway → record the gateway's reference
//
// The gateway call is last because it is the only step that leaves the
// building. If it fails, we have an order in `pending` with no gateway
// reference — visible, sweepable, explainable — rather than a session open at
// Stripe that nothing in our database knows about.

import { enabledGateways, type Env } from "./_env";
import { audit } from "./_audit";
import { gateway } from "./_gateways";
import {
  COUNTRY_RE,
  CURRENCY_RE,
  fail,
  ipHash,
  isValidEmail,
  json,
  normaliseVatId,
  readBody,
  requestCountry,
  SKU_RE,
  str,
  userAgent,
  verifyTurnstile,
} from "./_lib";
import { createOrder, priceBasket, upsertCustomer, type BasketLine } from "./_orders";
import { guard } from "./_ratelimit";
import { ensureSchema } from "./_schema";
import { getSettingString, moduleOn, shopIsConfigured } from "./_settings";
import { isWellFormedVatId, taxContext } from "./_tax";

interface CheckoutBody {
  /** Either a single sku+quantity, or a basket. Both shapes accepted so the
   *  no-JavaScript form (one hidden field) and the cart (a JSON array) can use
   *  the same endpoint. */
  sku?: string;
  quantity?: number | string;
  items?: Array<{ sku?: string; quantity?: number | string }> | string;
  currency?: string;
  gateway?: string;
  email?: string;
  name?: string;
  country?: string;
  vatId?: string;
  locale?: string;
  consentMarketing?: boolean | string;
  consentWaiver?: boolean | string;
  turnstileToken?: string;
  "cf-turnstile-response"?: string;
}

const truthy = (v: unknown): boolean => v === true || v === "1" || v === "on" || v === "true";

/** Reads the basket from either shape, capping both the number of distinct
 *  lines and the quantity per line. An ebook shop has no use for a thousand
 *  copies, and an unbounded quantity is an unbounded amount. */
function readLines(body: CheckoutBody): BasketLine[] | null {
  const raw: Array<{ sku?: string; quantity?: number | string }> = [];

  if (typeof body.items === "string") {
    try {
      const parsed = JSON.parse(body.items);
      if (Array.isArray(parsed)) raw.push(...parsed);
    } catch {
      return null;
    }
  } else if (Array.isArray(body.items)) {
    raw.push(...body.items);
  }
  if (raw.length === 0 && body.sku) raw.push({ sku: body.sku, quantity: body.quantity });

  if (raw.length === 0 || raw.length > 20) return null;

  const lines: BasketLine[] = [];
  for (const entry of raw) {
    const sku = String(entry?.sku ?? "");
    if (!SKU_RE.test(sku)) return null;
    const qty = Number.parseInt(String(entry?.quantity ?? 1), 10);
    if (!Number.isFinite(qty) || qty < 1 || qty > 10) return null;
    // Two lines naming the same product are one line with more copies.
    const existing = lines.find((l) => l.sku === sku);
    if (existing) existing.quantity = Math.min(10, existing.quantity + qty);
    else lines.push({ sku, quantity: qty });
  }
  return lines;
}

export const onRequestPost: PagesFunction<Env> = async (context) => {
  const { request, env } = context;
  if (!env.SHOP_DB) return fail(request, "not_configured", "The shop database is not bound.", 503);
  await ensureSchema(env);

  const configured = await shopIsConfigured(env);
  if (!configured.ok) {
    return fail(
      request,
      "shop_not_configured",
      `The shop is missing settings: ${configured.missing.join(", ")}. Set them in the admin panel.`,
      503,
    );
  }

  // Before anything is read or looked up: an order write and a call to a
  // payment provider are the two most expensive things this shop does.
  const limited = await guard(env, request, "checkout");
  if (limited) return limited;

  const body = await readBody<CheckoutBody>(request);
  if (!body) return fail(request, "invalid_body", "The request body could not be read.", 400);

  const lines = readLines(body);
  if (!lines) return fail(request, "invalid_basket", "The basket is empty or malformed.", 422);

  const email = str(body.email, 254);
  if (!isValidEmail(email)) return fail(request, "invalid_email", "A valid email address is required.", 422);

  const name = str(body.name, 120);
  // Read at full length, then check. Truncating to two characters first would
  // turn "Germany" into "GE" — Georgia — and sell at the wrong rate to a buyer
  // who merely typed the name instead of the code.
  const country = str(body.country, 40).toUpperCase();
  if (country && !COUNTRY_RE.test(country)) {
    return fail(request, "invalid_country", "The country must be a two-letter code, such as DE.", 422);
  }

  const vatId = normaliseVatId(str(body.vatId, 20));
  if (vatId && !isWellFormedVatId(vatId)) {
    return fail(request, "invalid_vat_id", "That VAT number does not match the format for its country.", 422);
  }

  const requested = str(body.currency, 3).toUpperCase();
  if (requested && !CURRENCY_RE.test(requested)) {
    return fail(request, "invalid_currency", "The currency must be a three-letter code.", 422);
  }
  const currency = requested || (await getSettingString(env, "currency.base")) || "EUR";

  const gateways = enabledGateways(env);
  if (gateways.length === 0) {
    return fail(request, "no_gateway", "No payment provider is configured for this shop.", 503);
  }
  const gatewayName = str(body.gateway, 20).toLowerCase() || gateways[0]!;
  const provider = gateway(env, gatewayName);
  if (!provider) return fail(request, "unknown_gateway", "That payment provider is not available here.", 422);

  // Selling a digital file usually means asking the buyer to waive the right to
  // withdraw before the download starts. Whether to ask is a setting; when it
  // is on, an unticked box is a refusal to sell, not a detail.
  const waiverRequired = (await getSettingString(env, "legal.digital_waiver")) !== "false";
  const consentWaiver = truthy(body.consentWaiver);
  if (waiverRequired && !consentWaiver) {
    return fail(
      request,
      "waiver_required",
      "Please confirm you agree to immediate delivery and the loss of the right to withdraw.",
      422,
    );
  }

  const ip = request.headers.get("cf-connecting-ip");
  const token = str(body.turnstileToken ?? body["cf-turnstile-response"], 4096);
  // The secret being present is not the same as the check being wanted: a
  // seller testing a storefront turns the module off and keeps the secret.
  if (env.TURNSTILE_SECRET && (await moduleOn(env, "turnstile"))) {
    if (!token || !(await verifyTurnstile(env.TURNSTILE_SECRET, token, ip))) {
      return fail(request, "captcha_failed", "The anti-spam check did not pass. Please try again.", 403);
    }
  }

  const ctx = await taxContext(env);
  const priced = await priceBasket(env, ctx, {
    lines,
    currency,
    billingCountry: country || null,
    ipCountry: requestCountry(request),
    vatId,
    // VIES validation is a separate module; until it exists a well-formed
    // number is not treated as verified, so reverse charge does not apply on
    // the strength of a string the buyer typed.
    vatIdValid: false,
  });

  if ("error" in priced) {
    switch (priced.error) {
      case "unknown_sku":
        return fail(request, "unknown_sku", `No product with the code ${priced.sku}.`, 422);
      case "product_unavailable":
        return fail(request, "product_unavailable", `${priced.sku} is not on sale.`, 422);
      case "price_not_available":
        return fail(request, "price_not_available", `${priced.sku} has no price in ${priced.currency}.`, 422);
      case "country_required":
        return fail(request, "country_required", "A billing country is required to work out the tax.", 422);
      case "tax_rate_missing":
        return fail(
          request,
          "tax_rate_missing",
          `This shop has no VAT rate for ${priced.country}, so it cannot sell there yet.`,
          422,
        );
      case "country_not_served":
        return fail(request, "country_not_served", `This shop does not sell to ${priced.country}.`, 422);
      default:
        return fail(request, "invalid_basket", "The basket could not be priced.", 422);
    }
  }

  const customer = await upsertCustomer(env, {
    email,
    name,
    vatId,
    vatIdValid: null,
    country: country || priced.taxCountry,
  });

  const { order, items, orderKey } = await createOrder(env, priced, customer, {
    gateway: gatewayName,
    locale: str(body.locale, 10) || "en",
    consentMarketing: truthy(body.consentMarketing),
    consentWaiver,
    ipHash: await ipHash(request, env),
    userAgent: userAgent(request),
  });

  const origin = new URL(request.url).origin;
  const successUrl = `${origin}/shop/thanks/?order=${order.id}&k=${encodeURIComponent(orderKey)}`;
  const cancelUrl = `${origin}/shop/cancel/?order=${order.id}`;

  let redirectUrl: string;
  try {
    const result = await provider.createCheckout(
      {
        order,
        items,
        customer,
        successUrl,
        cancelUrl,
        locale: order.locale ?? "en",
        pricingMode: ctx.pricingMode,
        providerTax: ctx.mode === "stripe",
      },
      env,
    );
    redirectUrl = result.redirectUrl;
    await env.SHOP_DB.prepare(`UPDATE orders SET gateway_ref = ?, updated_at = ? WHERE id = ?`)
      .bind(result.providerRef, new Date().toISOString(), order.id)
      .run();
  } catch (e) {
    // The order stays pending with no reference: nothing was charged, and the
    // row explains itself in the panel.
    //
    // The provider's own wording goes to the audit log, not to the buyer.
    // Stripe answers a bad key with "Invalid API Key provided: sk_test_…",
    // which is the shop's configuration quoted back at whoever asked (S-29).
    const detail = e instanceof Error ? e.message : String(e);
    await audit(env, "system", "checkout.gateway_error", order.id, {
      gateway: gatewayName,
      number: order.number,
      error: detail.slice(0, 300),
    });
    return fail(
      request,
      "gateway_error",
      "The payment provider could not start this payment. Nothing was charged — please try again.",
      502,
    );
  }

  // A browser form post (no JavaScript) wants a redirect; the cart wants JSON.
  const wantsHtml = (request.headers.get("accept") ?? "").includes("text/html");
  if (wantsHtml) {
    return new Response(null, { status: 303, headers: { location: redirectUrl, "cache-control": "no-store" } });
  }
  return json({ orderId: order.id, orderKey, orderNumber: order.number, redirectUrl });
};
