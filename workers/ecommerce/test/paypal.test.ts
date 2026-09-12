// PayPal, in full: the token, the order, the capture, and every event type the
// shop acts on. PayPal is answered by a stand-in that behaves the way its
// documentation says it does.

import { env } from "cloudflare:test";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { gateway } from "../functions/api/shop/_gateways";
import { onRequestPost as paypalWebhook } from "../functions/api/shop/webhooks/paypal";
import type { CheckoutInput, GatewayEvent, VerifiedWebhook } from "../functions/api/shop/_gateway";
import type { CustomerRow, OrderItemRow, OrderRow, PaymentRow } from "../functions/api/shop/_types";
import { ctx, freshShop, ORIGIN } from "./_helpers";

const payEnv = () => ({
  ...env,
  SHOP_GATEWAYS: "paypal",
  PAYPAL_CLIENT_ID: "client",
  PAYPAL_CLIENT_SECRET: "secret",
  PAYPAL_WEBHOOK_ID: "wh-1",
  SHOP_PAYPAL_ENV: "sandbox",
});

const order: OrderRow = {
  id: "order-1", number: "O-2026-000001", key_hash: "h", status: "pending", customer_id: "c1",
  currency: "EUR", subtotal_minor: 2243, tax_minor: 157, total_minor: 2400, tax_country: "DE",
  tax_evidence: "{}", reverse_charge: 0, gateway: "paypal", gateway_ref: null, consent_marketing: 0,
  consent_waiver: 1, ip_hash: null, user_agent: "t", locale: "en", notes: null,
  created_at: "2026-01-01T00:00:00.000Z", paid_at: null, updated_at: "2026-01-01T00:00:00.000Z",
};
const items: OrderItemRow[] = [{
  id: "i1", order_id: "order-1", product_id: "p1", sku: "EBOOK-1", name: "A Book",
  quantity: 1, unit_minor: 2243, tax_rate_bp: 700, tax_minor: 157, total_minor: 2400,
}];
const customer: CustomerRow = {
  id: "c1", email: "buyer@example.com", email_lc: "buyer@example.com", name: "A Buyer",
  vat_id: null, vat_id_valid: null, country: "DE", created_at: "2026-01-01T00:00:00.000Z",
};
const input: CheckoutInput = {
  order, items, customer,
  successUrl: `${ORIGIN}/shop/thanks/`,
  cancelUrl: `${ORIGIN}/shop/cancel/`,
  locale: "en", pricingMode: "gross", providerTax: false,
};

let calls: Array<{ url: string; init: RequestInit }> = [];
const realFetch = globalThis.fetch;

/** PayPal, as far as these tests are concerned: a token endpoint, plus whatever
 *  the test says the API answers. */
function paypalAnswers(api: (url: string, init: RequestInit) => unknown, tokenOk = true): void {
  globalThis.fetch = (async (url: unknown, init: RequestInit = {}) => {
    const href = String(url);
    calls.push({ url: href, init });
    if (href.includes("/v1/oauth2/token")) {
      return tokenOk
        ? new Response(JSON.stringify({ access_token: `tok-${calls.length}`, expires_in: 3600 }))
        : new Response(JSON.stringify({ error: "invalid_client" }), { status: 401 });
    }
    const answer = api(href, init);
    return answer instanceof Response ? answer : new Response(JSON.stringify(answer));
  }) as unknown as typeof fetch;
}

function webhookRequest(body: string): Request {
  return new Request(`${ORIGIN}/api/shop/webhooks/paypal`, {
    method: "POST",
    headers: {
      "paypal-auth-algo": "SHA256withRSA",
      "paypal-cert-url": "https://api.paypal.com/cert",
      "paypal-transmission-id": "tid",
      "paypal-transmission-sig": "sig",
      "paypal-transmission-time": "2026-09-01T00:00:00Z",
    },
    body,
  });
}

/** Runs an event through verification and returns what the shop made of it. */
async function interpret(event: Record<string, unknown>, api: (url: string, init: RequestInit) => unknown): Promise<GatewayEvent | null> {
  const body = JSON.stringify(event);
  paypalAnswers((url, init) => {
    if (url.includes("verify-webhook-signature")) return { verification_status: "SUCCESS" };
    return api(url, init);
  });
  const verified: VerifiedWebhook | null = await gateway(payEnv(), "paypal")!.verifyWebhook(
    webhookRequest(body), body, payEnv(),
  );
  return verified?.event ?? null;
}

beforeEach(async () => {
  calls = [];
  await freshShop();
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("starting a payment", () => {
  it("creates an order with the amount broken down the way PayPal wants", async () => {
    paypalAnswers(() => ({
      id: "PAYPAL-1",
      links: [
        { rel: "self", href: "https://api-m.sandbox.paypal.com/v2/checkout/orders/PAYPAL-1" },
        { rel: "approve", href: "https://www.sandbox.paypal.com/checkoutnow?token=PAYPAL-1" },
      ],
    }));

    const result = await gateway(payEnv(), "paypal")!.createCheckout(input, payEnv());
    expect(result).toEqual({
      redirectUrl: "https://www.sandbox.paypal.com/checkoutnow?token=PAYPAL-1",
      providerRef: "PAYPAL-1",
    });

    const body = JSON.parse(String(calls.at(-1)?.init.body)) as {
      purchase_units: Array<{ amount: { value: string; breakdown: Record<string, unknown> }; custom_id: string }>;
      payment_source: { paypal: { experience_context: { shipping_preference: string } } };
    };
    expect(body.purchase_units[0]?.amount.value).toBe("24.00");
    // The order number, because it is what a human reading a PayPal statement
    // can match against an invoice.
    expect(body.purchase_units[0]?.custom_id).toBe("O-2026-000001");
    // A file has nowhere to be shipped to, and an address field the buyer has
    // to fill in is a field they can abandon the sale over.
    expect(body.payment_source.paypal.experience_context.shipping_preference).toBe("NO_SHIPPING");
  });

  it("reuses the access token rather than asking for one per call", async () => {
    paypalAnswers(() => ({ id: "PAYPAL-1", links: [{ rel: "approve", href: "https://x" }] }));
    await gateway(payEnv(), "paypal")!.createCheckout(input, payEnv());
    // Whatever the first call needed, the second must need no token of its own:
    // an OAuth round trip per API call doubles the latency of every checkout.
    calls = [];
    await gateway(payEnv(), "paypal")!.createCheckout(input, payEnv());
    expect(calls.filter((c) => c.url.includes("/v1/oauth2/token"))).toHaveLength(0);
    expect(calls).toHaveLength(1);
  });

  it("throws when PayPal will not authenticate us", async () => {
    paypalAnswers(() => ({}), false);
    await expect(gateway(payEnv(), "paypal")!.createCheckout(input, payEnv())).rejects.toThrow();
  });

  it("throws when PayPal answers without a link to send the buyer to", async () => {
    paypalAnswers(() => ({ id: "PAYPAL-1", links: [] }));
    await expect(gateway(payEnv(), "paypal")!.createCheckout(input, payEnv())).rejects.toThrow();
  });

  it("throws when PayPal refuses the order outright", async () => {
    paypalAnswers(() => new Response(JSON.stringify({ name: "INVALID_REQUEST" }), { status: 400 }));
    await expect(gateway(payEnv(), "paypal")!.createCheckout(input, payEnv())).rejects.toThrow();
  });
});

describe("an approval, which is not yet money", () => {
  it("captures, and reports what was actually captured", async () => {
    const event = await interpret(
      { id: "WH-1", event_type: "CHECKOUT.ORDER.APPROVED", resource: { id: "PAYPAL-1" } },
      (url) => {
        if (url.includes("/capture")) {
          return {
            status: "COMPLETED",
            payer: { address: { country_code: "DE" } },
            purchase_units: [
              { payments: { captures: [{ id: "CAP-1", amount: { value: "24.00", currency_code: "EUR" } }] } },
            ],
          };
        }
        return {};
      },
    );

    expect(event).toMatchObject({
      kind: "paid",
      providerRef: "PAYPAL-1",
      paymentRef: "CAP-1",
      amountMinor: 2400,
      currency: "EUR",
      // The payer's country is the third piece of VAT evidence.
      cardCountry: "DE",
    });
  });

  it("is a failure, not a payment, when the capture does not complete", async () => {
    const event = await interpret(
      { id: "WH-2", event_type: "CHECKOUT.ORDER.APPROVED", resource: { id: "PAYPAL-2" } },
      (url) => (url.includes("/capture") ? { status: "DECLINED" } : {}),
    );
    expect(event).toMatchObject({ kind: "failed", providerRef: "PAYPAL-2", reason: "capture_failed" });
  });
});

describe("the other events", () => {
  it("reads a completed capture, finding the order through the up link", async () => {
    const event = await interpret(
      {
        id: "WH-3",
        event_type: "PAYMENT.CAPTURE.COMPLETED",
        resource: {
          id: "CAP-9",
          amount: { value: "24.00", currency_code: "EUR" },
          links: [{ rel: "up", href: "https://api-m.paypal.com/v2/checkout/orders/PAYPAL-9" }],
        },
      },
      () => ({}),
    );
    expect(event).toMatchObject({ kind: "paid", providerRef: "PAYPAL-9", paymentRef: "CAP-9", amountMinor: 2400 });
  });

  it("falls back to the supplementary data when there is no up link", async () => {
    const event = await interpret(
      {
        id: "WH-4",
        event_type: "PAYMENT.CAPTURE.COMPLETED",
        resource: {
          id: "CAP-10",
          amount: { value: "19.00", currency_code: "EUR" },
          supplementary_data: { related_ids: { order_id: "PAYPAL-10" } },
        },
      },
      () => ({}),
    );
    expect(event).toMatchObject({ kind: "paid", providerRef: "PAYPAL-10", amountMinor: 1900 });
  });

  it("treats a denial and a reversal as failures", async () => {
    for (const type of ["PAYMENT.CAPTURE.DENIED", "PAYMENT.CAPTURE.REVERSED"]) {
      const event = await interpret({ id: `WH-${type}`, event_type: type, resource: { id: "CAP-11" } }, () => ({}));
      expect(event).toMatchObject({ kind: "failed", reason: type });
    }
  });

  it("reads a refund", async () => {
    const event = await interpret(
      {
        id: "WH-5",
        event_type: "PAYMENT.CAPTURE.REFUNDED",
        resource: { id: "CAP-12", amount: { value: "10.00", currency_code: "EUR" } },
      },
      () => ({}),
    );
    expect(event).toMatchObject({ kind: "refunded", amountMinor: 1000, currency: "EUR" });
  });

  it("ignores anything else", async () => {
    const event = await interpret(
      { id: "WH-6", event_type: "BILLING.SUBSCRIPTION.CREATED", resource: {} },
      () => ({}),
    );
    expect(event).toEqual({ kind: "ignored", eventType: "BILLING.SUBSCRIPTION.CREATED" });
  });
});

describe("refusing a webhook", () => {
  it("refuses one with no webhook id configured", async () => {
    const body = JSON.stringify({ id: "WH-7", event_type: "PAYMENT.CAPTURE.COMPLETED", resource: {} });
    paypalAnswers(() => ({ verification_status: "SUCCESS" }));
    const verified = await gateway({ ...payEnv(), PAYPAL_WEBHOOK_ID: undefined }, "paypal")!.verifyWebhook(
      webhookRequest(body), body, { ...payEnv(), PAYPAL_WEBHOOK_ID: undefined },
    );
    expect(verified).toBe(null);
  });

  it("refuses a body that is not JSON, and one with no event type", async () => {
    paypalAnswers(() => ({ verification_status: "SUCCESS" }));
    const g = gateway(payEnv(), "paypal")!;
    expect(await g.verifyWebhook(webhookRequest("not json"), "not json", payEnv())).toBe(null);

    const noType = JSON.stringify({ id: "WH-8" });
    expect(await g.verifyWebhook(webhookRequest(noType), noType, payEnv())).toBe(null);
  });

  it("refuses when PayPal itself says the signature is wrong", async () => {
    const body = JSON.stringify({ id: "WH-9", event_type: "PAYMENT.CAPTURE.COMPLETED", resource: {} });
    paypalAnswers(() => ({ verification_status: "FAILURE" }));
    expect(await gateway(payEnv(), "paypal")!.verifyWebhook(webhookRequest(body), body, payEnv())).toBe(null);
  });
});

describe("refunding through PayPal", () => {
  const payment: PaymentRow = {
    id: "pay-1", order_id: "order-1", gateway: "paypal", gateway_ref: "CAP-1", kind: "charge",
    status: "succeeded", amount_minor: 2400, currency: "EUR", raw_json: null, created_at: "2026-01-01",
  };

  it("sends the amount as a decimal in the right currency", async () => {
    paypalAnswers(() => ({ id: "REF-1" }));
    const out = await gateway(payEnv(), "paypal")!.refund(payment, 1000, payEnv());
    expect(out.refundRef).toBe("REF-1");

    const body = JSON.parse(String(calls.at(-1)?.init.body)) as { amount: { value: string; currency_code: string } };
    expect(body.amount).toEqual({ value: "10.00", currency_code: "EUR" });
  });

  it("throws when PayPal refuses", async () => {
    paypalAnswers(() => new Response(JSON.stringify({ name: "CAPTURE_FULLY_REFUNDED" }), { status: 422 }));
    await expect(gateway(payEnv(), "paypal")!.refund(payment, 2400, payEnv())).rejects.toThrow(/paypal/i);
  });
});

describe("the webhook endpoint", () => {
  it("answers 400 to something that does not verify, writing nothing", async () => {
    paypalAnswers(() => ({ verification_status: "FAILURE" }));
    const body = JSON.stringify({ id: "WH-10", event_type: "PAYMENT.CAPTURE.COMPLETED", resource: {} });
    const res = (await paypalWebhook({
      ...ctx(webhookRequest(body)),
      env: payEnv(),
    } as never)) as Response;
    expect(res.status).toBe(400);

    const inbox = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM webhook_inbox`).first<{ n: number }>();
    expect(inbox?.n).toBe(0);
  });

  it("acknowledges a verified event for an order it does not have", async () => {
    paypalAnswers(() => ({ verification_status: "SUCCESS" }));
    const body = JSON.stringify({
      id: "WH-11",
      event_type: "PAYMENT.CAPTURE.COMPLETED",
      resource: {
        id: "CAP-20",
        amount: { value: "24.00", currency_code: "EUR" },
        supplementary_data: { related_ids: { order_id: "PAYPAL-UNKNOWN" } },
      },
    });
    const context = ctx(webhookRequest(body));
    const res = (await paypalWebhook({ ...context, env: payEnv() } as never)) as Response;
    await (context as unknown as { settled(): Promise<unknown> }).settled();

    expect(res.status).toBe(200);
    expect(await res.json()).toMatchObject({ action: "unknown-order" });
  });
});
