// PayPal, over REST v2 Orders with raw fetch.
//
// The shape differs from Stripe in one way that matters: PayPal's checkout
// ends with the buyer *approving* an order, and the money only moves when the
// merchant captures it. So "paid" here is the result of a capture we perform,
// not of the buyer returning to the site.

import { paypalBase, type Env } from "./_env";
import {
  redact,
  registerGateway,
  type CheckoutInput,
  type CheckoutResult,
  type GatewayEvent,
  type PaymentGateway,
  type VerifiedWebhook,
} from "./_gateway";
import { decimalStringToMinor, minorToDecimalString } from "./_money";
import type { PaymentRow } from "./_types";

/** Access tokens live about nine hours; cache per isolate and refresh a minute
 *  early rather than asking for one on every call. */
let tokenCache: { base: string; token: string; expiresAt: number } | null = null;

async function accessToken(env: Env): Promise<string> {
  const base = paypalBase(env);
  const now = Date.now();
  if (tokenCache && tokenCache.base === base && now < tokenCache.expiresAt) return tokenCache.token;

  const basic = btoa(`${env.PAYPAL_CLIENT_ID ?? ""}:${env.PAYPAL_CLIENT_SECRET ?? ""}`);
  const res = await fetch(`${base}/v1/oauth2/token`, {
    method: "POST",
    headers: { authorization: `Basic ${basic}`, "content-type": "application/x-www-form-urlencoded" },
    body: "grant_type=client_credentials",
  });
  if (!res.ok) throw new Error(`paypal auth failed: ${res.status}`);
  const body = (await res.json()) as { access_token?: string; expires_in?: number };
  if (!body.access_token) throw new Error("paypal returned no access token");
  tokenCache = {
    base,
    token: body.access_token,
    expiresAt: now + Math.max(60, (body.expires_in ?? 32400) - 60) * 1000,
  };
  return body.access_token;
}

/** Exposed so tests can start from a clean cache. */
export function resetPaypalToken(): void {
  tokenCache = null;
}

async function paypalJson(
  env: Env,
  path: string,
  init: { method: string; body?: unknown; requestId?: string },
): Promise<{ ok: boolean; status: number; body: Record<string, unknown> }> {
  const token = await accessToken(env);
  const headers: Record<string, string> = {
    authorization: `Bearer ${token}`,
    "content-type": "application/json",
  };
  // PayPal's idempotency header: a retried create or capture returns the first
  // result instead of doing it twice.
  if (init.requestId) headers["paypal-request-id"] = init.requestId;

  const res = await fetch(`${paypalBase(env)}${path}`, {
    method: init.method,
    headers,
    body: init.body === undefined ? undefined : JSON.stringify(init.body),
  });
  const text = await res.text();
  let body: Record<string, unknown> = {};
  try {
    body = text ? (JSON.parse(text) as Record<string, unknown>) : {};
  } catch {
    body = { raw: text };
  }
  return { ok: res.ok, status: res.status, body };
}

class PaypalGateway implements PaymentGateway {
  readonly name = "paypal";

  async createCheckout(input: CheckoutInput, env: Env): Promise<CheckoutResult> {
    const { order, items } = input;
    const currency = order.currency.toUpperCase();

    // PayPal insists the breakdown adds up to the total, to the cent. We send
    // net item totals plus tax so the buyer sees the same split as the invoice.
    const itemTotal = items.reduce((sum, i) => sum + i.unit_minor * i.quantity, 0);
    const taxTotal = order.tax_minor;

    const body = {
      intent: "CAPTURE",
      purchase_units: [
        {
          reference_id: order.id,
          custom_id: order.number,
          invoice_id: order.number,
          amount: {
            currency_code: currency,
            value: minorToDecimalString(order.total_minor, currency),
            breakdown: {
              item_total: { currency_code: currency, value: minorToDecimalString(itemTotal, currency) },
              tax_total: { currency_code: currency, value: minorToDecimalString(taxTotal, currency) },
            },
          },
          items: items.map((i) => ({
            name: i.name.slice(0, 127),
            quantity: String(i.quantity),
            unit_amount: { currency_code: currency, value: minorToDecimalString(i.unit_minor, currency) },
            tax: {
              currency_code: currency,
              value: minorToDecimalString(Math.round(i.tax_minor / i.quantity), currency),
            },
            category: "DIGITAL_GOODS",
          })),
        },
      ],
      payment_source: {
        paypal: {
          experience_context: {
            return_url: input.successUrl,
            cancel_url: input.cancelUrl,
            user_action: "PAY_NOW",
            // Nothing is shipped, so asking for an address would only add a
            // step and collect data we have no use for.
            shipping_preference: "NO_SHIPPING",
            locale: input.locale.slice(0, 5),
          },
        },
      },
    };

    const { ok, body: res } = await paypalJson(env, "/v2/checkout/orders", {
      method: "POST",
      body,
      requestId: order.id,
    });
    if (!ok) throw new Error(`paypal rejected the order: ${JSON.stringify(res).slice(0, 200)}`);

    const links = (res.links as Array<{ rel?: string; href?: string }> | undefined) ?? [];
    const approve = links.find((l) => l.rel === "payer-action" || l.rel === "approve")?.href;
    const id = res.id as string | undefined;
    if (!approve || !id) throw new Error("paypal returned no approval link");
    return { redirectUrl: approve, providerRef: id };
  }

  async verifyWebhook(request: Request, rawBody: string, env: Env): Promise<VerifiedWebhook | null> {
    const webhookId = env.PAYPAL_WEBHOOK_ID;
    if (!webhookId) return null;

    const need = [
      "paypal-auth-algo",
      "paypal-cert-url",
      "paypal-transmission-id",
      "paypal-transmission-sig",
      "paypal-transmission-time",
    ] as const;
    const headers: Record<string, string> = {};
    for (const h of need) {
      const v = request.headers.get(h);
      if (!v) return null;
      headers[h] = v;
    }

    let parsed: { id?: string; event_type?: string; resource?: Record<string, unknown> };
    try {
      parsed = JSON.parse(rawBody);
    } catch {
      return null;
    }
    if (!parsed.id || !parsed.event_type) return null;

    // PayPal verifies its own signature for us. That is a network call per
    // webhook, which is fine at ebook volumes; verifying the certificate chain
    // locally is a later optimisation, not a correctness question.
    const { ok, body } = await paypalJson(env, "/v1/notifications/verify-webhook-signature", {
      method: "POST",
      body: {
        auth_algo: headers["paypal-auth-algo"],
        cert_url: headers["paypal-cert-url"],
        transmission_id: headers["paypal-transmission-id"],
        transmission_sig: headers["paypal-transmission-sig"],
        transmission_time: headers["paypal-transmission-time"],
        webhook_id: webhookId,
        webhook_event: JSON.parse(rawBody),
      },
    });
    if (!ok || body.verification_status !== "SUCCESS") return null;

    return {
      eventId: parsed.id,
      eventType: parsed.event_type,
      event: await this.mapEvent(parsed.event_type, parsed.resource ?? {}, env),
    };
  }

  /** Maps an event, capturing an approved order on the way: the approval is
   *  the buyer's consent, the capture is the money. Both paths end in "paid",
   *  and the inbox makes it safe for both to arrive. */
  private async mapEvent(
    type: string,
    resource: Record<string, unknown>,
    env: Env,
  ): Promise<GatewayEvent> {
    const raw = redact(resource);
    switch (type) {
      case "CHECKOUT.ORDER.APPROVED": {
        const orderId = String(resource.id ?? "");
        const captured = await this.capture(orderId, env);
        if (!captured) return { kind: "failed", providerRef: orderId, reason: "capture_failed", raw };
        return { ...captured, raw };
      }
      case "PAYMENT.CAPTURE.COMPLETED": {
        const amount = (resource.amount as { value?: string; currency_code?: string } | undefined) ?? {};
        const currency = String(amount.currency_code ?? "").toUpperCase();
        const links = (resource.links as Array<{ rel?: string; href?: string }> | undefined) ?? [];
        return {
          kind: "paid",
          providerRef: providerRefFromCapture(resource, links),
          paymentRef: String(resource.id ?? ""),
          amountMinor: amount.value ? decimalStringToMinor(amount.value, currency) : 0,
          currency,
          cardCountry: null,
          raw,
        };
      }
      case "PAYMENT.CAPTURE.DENIED":
      case "PAYMENT.CAPTURE.REVERSED":
        return { kind: "failed", providerRef: String(resource.id ?? ""), reason: type, raw };
      case "PAYMENT.CAPTURE.REFUNDED": {
        const amount = (resource.amount as { value?: string; currency_code?: string } | undefined) ?? {};
        const currency = String(amount.currency_code ?? "").toUpperCase();
        return {
          kind: "refunded",
          providerRef: String(resource.id ?? ""),
          paymentRef: String(resource.id ?? ""),
          amountMinor: amount.value ? decimalStringToMinor(amount.value, currency) : 0,
          currency,
          raw,
        };
      }
      default:
        return { kind: "ignored", eventType: type };
    }
  }

  private async capture(orderId: string, env: Env): Promise<Omit<Extract<GatewayEvent, { kind: "paid" }>, "raw"> | null> {
    const { ok, body } = await paypalJson(env, `/v2/checkout/orders/${encodeURIComponent(orderId)}/capture`, {
      method: "POST",
      body: {},
      requestId: `capture:${orderId}`,
    });
    if (!ok || body.status !== "COMPLETED") return null;

    const units = (body.purchase_units as Array<Record<string, unknown>> | undefined) ?? [];
    const payments = (units[0]?.payments as { captures?: Array<Record<string, unknown>> } | undefined) ?? {};
    const capture = payments.captures?.[0];
    const amount = (capture?.amount as { value?: string; currency_code?: string } | undefined) ?? {};
    const currency = String(amount.currency_code ?? "").toUpperCase();
    const payer = (body.payer as { address?: { country_code?: string } } | undefined) ?? {};

    return {
      kind: "paid",
      providerRef: orderId,
      paymentRef: String(capture?.id ?? orderId),
      amountMinor: amount.value ? decimalStringToMinor(amount.value, currency) : 0,
      currency,
      cardCountry: payer.address?.country_code ?? null,
    };
  }

  async refund(payment: PaymentRow, amountMinor: number, env: Env): Promise<{ refundRef: string }> {
    const currency = payment.currency.toUpperCase();
    const { ok, body } = await paypalJson(
      env,
      `/v2/payments/captures/${encodeURIComponent(payment.gateway_ref)}/refund`,
      {
        method: "POST",
        body: { amount: { value: minorToDecimalString(amountMinor, currency), currency_code: currency } },
        requestId: `refund:${payment.id}:${amountMinor}`,
      },
    );
    if (!ok) throw new Error(`paypal refused the refund: ${JSON.stringify(body).slice(0, 200)}`);
    return { refundRef: String(body.id ?? "") };
  }
}

/** A capture event names the order it belongs to through an "up" link; without
 *  it we fall back to the supplementary data PayPal sometimes includes. */
function providerRefFromCapture(
  resource: Record<string, unknown>,
  links: Array<{ rel?: string; href?: string }>,
): string {
  const up = links.find((l) => l.rel === "up")?.href;
  const fromLink = up ? (/\/checkout\/orders\/([^/?]+)/.exec(up)?.[1] ?? "") : "";
  if (fromLink) return fromLink;
  const supplementary = resource.supplementary_data as
    | { related_ids?: { order_id?: string } }
    | undefined;
  return supplementary?.related_ids?.order_id ?? "";
}

registerGateway("paypal", () => new PaypalGateway());

export { PaypalGateway };
