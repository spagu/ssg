// Stripe, over the REST API with raw fetch. No SDK, so this deploys by Direct
// Upload and adds nothing to the supply chain of a shop that holds money.

import type { Env } from "./_env";
import {
  hmacHex,
  parseSignatureHeader,
  redact,
  registerGateway,
  withinTolerance,
  type CheckoutInput,
  type CheckoutResult,
  type GatewayEvent,
  type PaymentGateway,
  type VerifiedWebhook,
} from "./_gateway";
import { timingSafeEqual } from "./_lib";
import type { PaymentRow } from "./_types";

const API = "https://api.stripe.com";

async function stripeForm(
  env: Env,
  path: string,
  form: URLSearchParams,
  idempotencyKey?: string,
): Promise<{ ok: boolean; status: number; body: Record<string, unknown> }> {
  const headers: Record<string, string> = {
    authorization: `Bearer ${env.STRIPE_SECRET_KEY ?? ""}`,
    "content-type": "application/x-www-form-urlencoded",
  };
  // Stripe deduplicates on this key for 24 hours, so a retry after a timeout
  // cannot create a second session or a second refund.
  if (idempotencyKey) headers["idempotency-key"] = idempotencyKey;

  const res = await fetch(`${API}${path}`, { method: "POST", headers, body: form });
  const body = (await res.json()) as Record<string, unknown>;
  return { ok: res.ok, status: res.status, body };
}

class StripeGateway implements PaymentGateway {
  readonly name = "stripe";

  async createCheckout(input: CheckoutInput, env: Env): Promise<CheckoutResult> {
    const { order, items, customer } = input;
    const form = new URLSearchParams();
    form.set("mode", "payment");
    // How the webhook finds the order again, and the second half of the check
    // that the session we hear about is the session we made (S-08).
    form.set("client_reference_id", order.id);
    form.set("metadata[order_number]", order.number);
    form.set("success_url", input.successUrl);
    form.set("cancel_url", input.cancelUrl);
    form.set("customer_email", customer.email);
    form.set("locale", stripeLocale(input.locale));
    // An abandoned session should expire rather than sit open forever; the
    // expiry event moves our order to cancelled.
    form.set("expires_at", String(Math.floor(Date.now() / 1000) + 24 * 3600));

    items.forEach((item, i) => {
      // We already computed the amount the buyer owes. In gross pricing the
      // unit amount includes tax, so what Stripe charges is exactly our total.
      const unit =
        input.pricingMode === "gross"
          ? Math.round(item.total_minor / item.quantity)
          : item.unit_minor;
      form.set(`line_items[${i}][quantity]`, String(item.quantity));
      form.set(`line_items[${i}][price_data][currency]`, order.currency.toLowerCase());
      form.set(`line_items[${i}][price_data][unit_amount]`, String(unit));
      form.set(`line_items[${i}][price_data][product_data][name]`, item.name);
    });

    if (input.providerTax) {
      form.set("automatic_tax[enabled]", "true");
      form.set("tax_id_collection[enabled]", "true");
    }

    const { ok, body } = await stripeForm(env, "/v1/checkout/sessions", form, order.id);
    if (!ok) {
      const err = (body.error as { message?: string } | undefined)?.message ?? "stripe rejected the session";
      throw new Error(err);
    }
    const url = body.url as string | undefined;
    const id = body.id as string | undefined;
    if (!url || !id) throw new Error("stripe returned no checkout url");
    return { redirectUrl: url, providerRef: id };
  }

  async verifyWebhook(request: Request, rawBody: string, env: Env): Promise<VerifiedWebhook | null> {
    const secret = env.STRIPE_WEBHOOK_SECRET;
    const header = request.headers.get("stripe-signature");
    if (!secret || !header) return null;

    const { t, v1 } = parseSignatureHeader(header);
    if (!t || v1.length === 0) return null;
    // An old signature is a replay, however valid its HMAC (S-05).
    if (!withinTolerance(t)) return null;

    const expected = await hmacHex(secret, `${t}.${rawBody}`);
    if (!v1.some((sig) => timingSafeEqual(sig, expected))) return null;

    let parsed: { id?: string; type?: string; data?: { object?: Record<string, unknown> } };
    try {
      parsed = JSON.parse(rawBody);
    } catch {
      return null;
    }
    if (!parsed.id || !parsed.type) return null;

    return {
      eventId: parsed.id,
      eventType: parsed.type,
      event: mapEvent(parsed.type, parsed.data?.object ?? {}),
    };
  }

  async refund(payment: PaymentRow, amountMinor: number, env: Env): Promise<{ refundRef: string }> {
    const form = new URLSearchParams();
    form.set("payment_intent", payment.gateway_ref);
    form.set("amount", String(amountMinor));
    const { ok, body } = await stripeForm(env, "/v1/refunds", form, `refund:${payment.id}:${amountMinor}`);
    if (!ok) {
      const err = (body.error as { message?: string } | undefined)?.message ?? "stripe refused the refund";
      throw new Error(err);
    }
    return { refundRef: String(body.id ?? "") };
  }
}

/** Stripe's event types, mapped onto the four things we act on. */
function mapEvent(type: string, object: Record<string, unknown>): GatewayEvent {
  const raw = redact(object);
  switch (type) {
    case "checkout.session.completed":
    case "checkout.session.async_payment_succeeded": {
      // A completed session whose payment is still processing (SEPA, some
      // bank transfers) is not money in the bank: the async_payment_succeeded
      // event is. Treating "complete" as paid is how a shop ships an ebook for
      // a transfer that later bounces.
      if (type === "checkout.session.completed" && object.payment_status !== "paid") {
        return { kind: "ignored", eventType: type };
      }
      return {
        kind: "paid",
        providerRef: String(object.id ?? ""),
        paymentRef: String(object.payment_intent ?? object.id ?? ""),
        amountMinor: Number(object.amount_total ?? 0),
        currency: String(object.currency ?? "").toUpperCase(),
        cardCountry: null,
        raw,
      };
    }
    case "checkout.session.async_payment_failed":
      return { kind: "failed", providerRef: String(object.id ?? ""), reason: "async_payment_failed", raw };
    case "checkout.session.expired":
      return { kind: "failed", providerRef: String(object.id ?? ""), reason: "expired", raw };
    case "charge.refunded": {
      const refunded = Number(object.amount_refunded ?? 0);
      return {
        kind: "refunded",
        providerRef: String(object.payment_intent ?? ""),
        paymentRef: String(object.id ?? ""),
        amountMinor: refunded,
        currency: String(object.currency ?? "").toUpperCase(),
        raw,
      };
    }
    default:
      return { kind: "ignored", eventType: type };
  }
}

/** Stripe accepts a fixed set of locale codes; anything else is an error, so an
 *  unknown language falls back to letting Stripe decide. */
function stripeLocale(locale: string): string {
  const known = new Set(["auto", "bg", "cs", "da", "de", "el", "en", "es", "et", "fi", "fr", "hu", "id", "it", "ja", "lt", "lv", "ms", "mt", "nb", "nl", "pl", "pt", "ro", "ru", "sk", "sl", "sv", "th", "tr", "vi", "zh"]);
  const short = (locale || "").slice(0, 2).toLowerCase();
  return known.has(short) ? short : "auto";
}

registerGateway("stripe", () => new StripeGateway());

export { StripeGateway, mapEvent as mapStripeEvent };
