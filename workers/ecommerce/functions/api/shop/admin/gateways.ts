// GET  /api/shop/admin/gateways — what the shop can take money with
// POST /api/shop/admin/gateways — which to offer, in what order
//
// How other shop software does this, and why this does not:
//
// Magento, PrestaShop and WooCommerce all put the provider's API keys in a form
// in the admin panel and store them in the database. It is the obvious design
// and it is why "database dump" and "stolen payment credentials" so often turn
// out to be the same incident: the keys sit in every backup, they are readable
// by anything with a database connection, and the admin panel becomes the place
// worth breaking into. Shopify avoids it by not having the keys at all — the
// payment provider is the platform.
//
// On Cloudflare there is a third option, and it is the one taken here: the keys
// are wrangler **secrets**, encrypted at rest, injected into the running worker
// and readable by nothing else. Not this endpoint, not the panel, not a database
// export, not a support session with a screen shared.
//
// What is left for a panel to do is the rest of the job, which is the part
// sellers actually get stuck on: is it configured, is it test or live, what
// exact URL does the webhook go to, which events does it need, has one ever
// arrived, and does the key work right now. That is what this answers.

import { enabledGateways, isTestMode, paypalBase, type Env } from "../_env";
import { audit } from "../_audit";
import { requireOwner } from "../_auth";
import { fail, json, readJson } from "../_lib";
import { getSetting, putSettings } from "../_settings";
import type { AdminData } from "./_middleware";

interface GatewayInfo {
  name: string;
  label: string;
  /** Its secrets are present, so it could be used. */
  configured: boolean;
  /** It is being offered to buyers. */
  enabled: boolean;
  /** What is missing, in words, when it is not configured. */
  missing: string[];
  mode: "test" | "live" | "unknown";
  webhookUrl: string;
  /** The events the provider must be told to send. */
  events: string[];
  /** Where to set it up, so nobody has to search for the page. */
  dashboardUrl: string;
  lastEventAt: string | null;
  lastEventType: string | null;
  eventsSeen: number;
  paymentsTaken: number;
}

const LABELS: Record<string, string> = { stripe: "Stripe", paypal: "PayPal" };

const EVENTS: Record<string, string[]> = {
  stripe: [
    "checkout.session.completed",
    "checkout.session.async_payment_succeeded",
    "checkout.session.async_payment_failed",
    "checkout.session.expired",
    "charge.refunded",
  ],
  paypal: [
    "CHECKOUT.ORDER.APPROVED",
    "PAYMENT.CAPTURE.COMPLETED",
    "PAYMENT.CAPTURE.DENIED",
    "PAYMENT.CAPTURE.REFUNDED",
  ],
};

/** What each provider needs before it can be used at all. */
function missingFor(env: Env, name: string): string[] {
  if (name === "stripe") {
    return [
      env.STRIPE_SECRET_KEY ? "" : "STRIPE_SECRET_KEY",
      env.STRIPE_WEBHOOK_SECRET ? "" : "STRIPE_WEBHOOK_SECRET",
    ].filter(Boolean);
  }
  return [
    env.PAYPAL_CLIENT_ID ? "" : "PAYPAL_CLIENT_ID",
    env.PAYPAL_CLIENT_SECRET ? "" : "PAYPAL_CLIENT_SECRET",
    env.PAYPAL_WEBHOOK_ID ? "" : "PAYPAL_WEBHOOK_ID",
  ].filter(Boolean);
}

function modeFor(env: Env, name: string): "test" | "live" | "unknown" {
  if (name === "stripe") {
    if (env.STRIPE_SECRET_KEY?.startsWith("sk_test_")) return "test";
    if (env.STRIPE_SECRET_KEY?.startsWith("sk_live_")) return "live";
    return "unknown";
  }
  if (!env.PAYPAL_CLIENT_ID) return "unknown";
  return paypalBase(env).includes("sandbox") ? "test" : "live";
}

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env }) => {
  const origin = new URL(request.url).origin;
  const offered = enabledGateways(env);
  const order = (await getSetting(env, "gateways.order")) as unknown;
  const ordered = Array.isArray(order) ? order.map(String) : offered;

  const gateways: GatewayInfo[] = [];
  for (const name of ["stripe", "paypal"]) {
    const missing = missingFor(env, name);
    const [seen, taken] = await Promise.all([
      env.SHOP_DB.prepare(
        `SELECT COUNT(*) AS n, MAX(received_at) AS at FROM webhook_inbox WHERE gateway = ?`,
      )
        .bind(name)
        .first<{ n: number; at: string | null }>(),
      env.SHOP_DB.prepare(
        `SELECT COUNT(*) AS n FROM payments WHERE gateway = ? AND kind = 'charge' AND status = 'succeeded'`,
      )
        .bind(name)
        .first<{ n: number }>(),
    ]);
    const latest = await env.SHOP_DB.prepare(
      `SELECT event_type FROM webhook_inbox WHERE gateway = ? ORDER BY received_at DESC LIMIT 1`,
    )
      .bind(name)
      .first<{ event_type: string }>();

    gateways.push({
      name,
      label: LABELS[name] ?? name,
      configured: missing.length === 0,
      enabled: offered.includes(name),
      missing,
      mode: modeFor(env, name),
      webhookUrl: `${origin}/api/shop/webhooks/${name}`,
      events: EVENTS[name] ?? [],
      dashboardUrl:
        name === "stripe"
          ? "https://dashboard.stripe.com/webhooks"
          : "https://developer.paypal.com/dashboard/applications",
      lastEventAt: seen?.at ?? null,
      lastEventType: latest?.event_type ?? null,
      eventsSeen: seen?.n ?? 0,
      paymentsTaken: taken?.n ?? 0,
    });
  }

  return json({
    gateways,
    order: ordered,
    testMode: isTestMode(env),
    // Said plainly, because the first thing a seller looks for on this screen is
    // the box to paste a key into.
    note:
      "Keys are set with `wrangler pages secret put` and are never stored in this shop's database, " +
      "so nothing here can show or change one. A database dump therefore cannot contain them.",
  });
};

interface OrderBody {
  order?: string[];
}

export const onRequestPost: PagesFunction<Env, string, AdminData> = async ({ request, env, data }) => {
  const denied = requireOwner(request, data.admin);
  if (denied) return denied;

  const body = await readJson<OrderBody>(request);
  if (!body || !Array.isArray(body.order)) {
    return fail(request, "invalid_body", 'Send { order: ["stripe", "paypal"] }.', 400);
  }

  const known = new Set(["stripe", "paypal"]);
  const order = body.order.map(String).filter((name) => known.has(name));
  const unknownName = body.order.map(String).find((name) => !known.has(name));
  if (unknownName) {
    return fail(request, "unknown_gateway", `This shop has no payment provider called ${unknownName}.`, 422);
  }

  // A provider with no credentials cannot be offered: the button would take a
  // buyer to a 502 at the last step of a purchase.
  const unusable = order.find((name) => missingFor(env, name).length > 0);
  if (unusable) {
    return fail(
      request,
      "gateway_unavailable",
      `${LABELS[unusable] ?? unusable} needs ${missingFor(env, unusable).join(" and ")} before it can be offered.`,
      422,
    );
  }

  await putSettings(env, { "gateways.order": order });
  await audit(env, `admin:${data.admin.sub}`, "gateways.order", null, { order });
  return json({ order });
};
