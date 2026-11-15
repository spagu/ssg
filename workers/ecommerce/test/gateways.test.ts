// Payment providers. Nothing here reaches the network: fetch is replaced, so
// what is tested is the request we build and the answer we make of theirs.

import { env } from "./_env";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  gateway,
  gateways,
  hmacHex,
  parseSignatureHeader,
  redact,
  SIGNATURE_TOLERANCE_S,
  withinTolerance,
} from "../functions/api/shop/_gateway";
import "../functions/api/shop/_gateways";
import { mapStripeEvent } from "../functions/api/shop/_stripe";
import type { CheckoutInput } from "../functions/api/shop/_gateway";
import type { CustomerRow, OrderItemRow, OrderRow, PaymentRow } from "../functions/api/shop/_types";
import { assertPartial, freshShop } from "./_helpers";

const configured = {
  STRIPE_SECRET_KEY: "sk_test_x",
  STRIPE_WEBHOOK_SECRET: "whsec_test",
  PAYPAL_CLIENT_ID: "client",
  PAYPAL_CLIENT_SECRET: "secret",
  PAYPAL_WEBHOOK_ID: "wh-1",
};

const order: OrderRow = {
  id: "order-1", number: "O-2026-000001", key_hash: "h", status: "pending", customer_id: "c1",
  currency: "EUR", subtotal_minor: 2243, tax_minor: 157, total_minor: 2400, tax_country: "DE",
  tax_evidence: "{}", reverse_charge: 0, gateway: "stripe", gateway_ref: null, consent_marketing: 0,
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
  successUrl: "https://shop.example.com/shop/thanks/",
  cancelUrl: "https://shop.example.com/shop/cancel/",
  locale: "en", pricingMode: "gross", providerTax: false,
};

let calls: Array<{ url: string; init: RequestInit }> = [];
const realFetch = globalThis.fetch;

function respond(handler: (url: string, init: RequestInit) => Response | Promise<Response>): void {
  globalThis.fetch = (async (url: unknown, init: RequestInit = {}) => {
    calls.push({ url: String(url), init });
    return handler(String(url), init);
  }) as unknown as typeof fetch;
}

beforeEach(async () => {
  calls = [];
  await freshShop();
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("the registry", () => {
  it("offers only what is configured, in the order the owner listed", () => {
    assert.deepEqual(gateways({ ...env, ...configured, SHOP_GATEWAYS: "paypal,stripe" }).map((g) => g.name), [
      "paypal",
      "stripe",
    ]);
    assert.equal(gateway({ ...env, ...configured }, "stripe")?.name, "stripe");
    assert.equal(gateway({ ...env, STRIPE_SECRET_KEY: undefined, PAYPAL_CLIENT_ID: undefined }, "stripe"), null);
    assert.equal(gateway({ ...env, ...configured }, "bitcoin"), null);
  });
});

describe("signatures", () => {
  it("parses a Stripe-style header", () => {
    assert.deepEqual(parseSignatureHeader("t=123,v1=aaa,v1=bbb,v0=ccc"), { t: "123", v1: ["aaa", "bbb"] });
    assert.deepEqual(parseSignatureHeader(""), { t: "", v1: [] });
    assert.deepEqual(parseSignatureHeader("nonsense"), { t: "", v1: [] });
  });

  it("computes a stable HMAC", async () => {
    assert.equal(await hmacHex("secret", "payload"), await hmacHex("secret", "payload"));
    assert.notEqual(await hmacHex("secret", "payload"), await hmacHex("other", "payload"));
    assert.match(await hmacHex("secret", "payload"), /^[0-9a-f]{64}$/);
  });

  it("treats an old signature as a replay however valid its HMAC", () => {
    const now = Math.floor(Date.now() / 1000);
    assert.equal(withinTolerance(String(now)), true);
    assert.equal(withinTolerance(String(now - SIGNATURE_TOLERANCE_S - 1)), false);
    // A timestamp from the future is equally wrong.
    assert.equal(withinTolerance(String(now + SIGNATURE_TOLERANCE_S + 1)), false);
    assert.equal(withinTolerance("not-a-number"), false);
  });

  it("keeps a provider's payload out of the database at full size", () => {
    const redacted = redact({ id: "x", customer_details: { email: "a@example.com" }, nested: { deep: [1, 2, 3] } }) as Record<string, unknown>;
    assert.ok((JSON.stringify(redacted).length) < 4000);
  });
});

describe("Stripe", () => {
  it("asks for a session that names our order and cannot be made twice", async () => {
    respond(() => new Response(JSON.stringify({ id: "cs_test_1", url: "https://checkout.stripe.com/x" })));
    const result = await gateway({ ...env, ...configured }, "stripe")!.createCheckout(input, { ...env, ...configured });

    assert.deepEqual(result, { redirectUrl: "https://checkout.stripe.com/x", providerRef: "cs_test_1" });
    const body = String(calls[0]?.init.body);
    assert.ok((body).includes("client_reference_id=order-1"));
    // Gross pricing: the amount Stripe charges already has the tax inside it.
    assert.ok((body).includes("%5Bunit_amount%5D=2400"));
    assert.ok((body).includes("expires_at="));
    const headers = calls[0]?.init.headers as Record<string, string>;
    assert.equal(headers["idempotency-key"], "order-1");
  });

  it("passes the tax decision to Stripe when the shop asked it to", async () => {
    respond(() => new Response(JSON.stringify({ id: "cs_1", url: "https://x" })));
    await gateway({ ...env, ...configured }, "stripe")!.createCheckout(
      { ...input, providerTax: true },
      { ...env, ...configured },
    );
    assert.ok((String(calls[0]?.init.body)).includes("automatic_tax%5Benabled%5D=true"));
  });

  it("repeats Stripe's reason when it refuses", async () => {
    respond(() => new Response(JSON.stringify({ error: { message: "No such price" } }), { status: 400 }));
    await assert.rejects(gateway({ ...env, ...configured }, "stripe")!.createCheckout(input, { ...env, ...configured }), (err) => String((err as Error).message).includes("No such price"));
  });

  it("complains when Stripe answers with no URL", async () => {
    respond(() => new Response(JSON.stringify({ id: "cs_1" })));
    await assert.rejects(gateway({ ...env, ...configured }, "stripe")!.createCheckout(input, { ...env, ...configured }), /checkout url/);
  });

  it("refunds against the payment intent, once per amount", async () => {
    respond(() => new Response(JSON.stringify({ id: "re_1" })));
    const payment: PaymentRow = {
      id: "pay-1", order_id: "order-1", gateway: "stripe", gateway_ref: "pi_1", kind: "charge",
      status: "succeeded", amount_minor: 2400, currency: "EUR", raw_json: null, created_at: "2026-01-01",
    };
    const out = await gateway({ ...env, ...configured }, "stripe")!.refund(payment, 2400, { ...env, ...configured });
    assert.equal(out.refundRef, "re_1");
    assert.equal((calls[0]?.init.headers as Record<string, string>)["idempotency-key"], "refund:pay-1:2400");
  });

  it("passes Stripe's refusal of a refund on", async () => {
    respond(() => new Response(JSON.stringify({ error: { message: "Charge already refunded" } }), { status: 400 }));
    const payment: PaymentRow = {
      id: "pay-1", order_id: "order-1", gateway: "stripe", gateway_ref: "pi_1", kind: "charge",
      status: "succeeded", amount_minor: 2400, currency: "EUR", raw_json: null, created_at: "2026-01-01",
    };
    await assert.rejects(gateway({ ...env, ...configured }, "stripe")!.refund(payment, 2400, { ...env, ...configured }), (err) => String((err as Error).message).includes("already refunded"));
  });
});

describe("Stripe webhooks", () => {
  const secret = "whsec_test";

  async function signed(body: string, at = Math.floor(Date.now() / 1000)): Promise<Request> {
    const signature = await hmacHex(secret, `${at}.${body}`);
    return new Request("https://shop.example.com/api/shop/webhooks/stripe", {
      method: "POST",
      headers: { "stripe-signature": `t=${at},v1=${signature}` },
      body,
    });
  }

  const paidBody = JSON.stringify({
    id: "evt_1",
    type: "checkout.session.completed",
    data: { object: { id: "cs_1", payment_intent: "pi_1", payment_status: "paid", amount_total: 2400, currency: "eur" } },
  });

  it("verifies a signature it just made", async () => {
    const verified = await gateway({ ...env, ...configured }, "stripe")!.verifyWebhook(
      await signed(paidBody), paidBody, { ...env, ...configured },
    );
    assert.equal(verified?.eventId, "evt_1");
    assertPartial(verified?.event, { kind: "paid", providerRef: "cs_1", amountMinor: 2400, currency: "EUR" });
  });

  it("refuses one that is minutes old", async () => {
    // This is the replay hole: the HMAC is valid forever, so only the timestamp
    // stops a captured request being sent again tomorrow.
    const old = Math.floor(Date.now() / 1000) - SIGNATURE_TOLERANCE_S - 60;
    assert.equal(await gateway({ ...env, ...configured }, "stripe")!.verifyWebhook(
        await signed(paidBody, old), paidBody, { ...env, ...configured },
      ), null);
  });

  it("refuses a body that changed after signing", async () => {
    const request = await signed(paidBody);
    const tampered = paidBody.replace("2400", "1");
    assert.equal(await gateway({ ...env, ...configured }, "stripe")!.verifyWebhook(request, tampered, { ...env, ...configured }), null);
  });

  it("refuses a missing header, a missing secret, and rubbish", async () => {
    const g = gateway({ ...env, ...configured }, "stripe")!;
    const bare = new Request("https://shop.example.com/w", { method: "POST", body: paidBody });
    assert.equal(await g.verifyWebhook(bare, paidBody, { ...env, ...configured }), null);
    assert.equal(await g.verifyWebhook(await signed(paidBody), paidBody, { ...env, STRIPE_WEBHOOK_SECRET: undefined }), null);

    const noSig = new Request("https://shop.example.com/w", {
      method: "POST", headers: { "stripe-signature": "garbage" }, body: paidBody,
    });
    assert.equal(await g.verifyWebhook(noSig, paidBody, { ...env, ...configured }), null);

    const notJson = "not json";
    assert.equal(await g.verifyWebhook(await signed(notJson), notJson, { ...env, ...configured }), null);

    const noType = JSON.stringify({ id: "evt_2" });
    assert.equal(await g.verifyWebhook(await signed(noType), noType, { ...env, ...configured }), null);
  });
});

describe("what a Stripe event means", () => {
  it("is not paid until the payment says paid", () => {
    // A completed session for a bank transfer is not money; treating it as paid
    // is how a shop ships a book for a transfer that later bounces.
    assert.deepEqual(mapStripeEvent("checkout.session.completed", { id: "cs_1", payment_status: "unpaid" }), { kind: "ignored", eventType: "checkout.session.completed" });

    assertPartial(mapStripeEvent("checkout.session.async_payment_succeeded", {
        id: "cs_1", payment_intent: "pi_1", amount_total: 2400, currency: "eur",
      }), { kind: "paid" });
  });

  it("separates an abandoned basket from a failure", () => {
    assertPartial(mapStripeEvent("checkout.session.expired", { id: "cs_1" }), {
      kind: "failed", reason: "expired",
    });
    assertPartial(mapStripeEvent("checkout.session.async_payment_failed", { id: "cs_1" }), {
      kind: "failed", reason: "async_payment_failed",
    });
  });

  it("reads a refund off the charge", () => {
    assertPartial(mapStripeEvent("charge.refunded", { id: "ch_1", payment_intent: "pi_1", amount_refunded: 1000, currency: "eur" }), { kind: "refunded", providerRef: "pi_1", amountMinor: 1000 });
  });

  it("ignores everything else rather than erroring", () => {
    // Providers add event types; a 500 makes them retry forever.
    assert.deepEqual(mapStripeEvent("customer.created", {}), { kind: "ignored", eventType: "customer.created" });
  });
});

describe("PayPal", () => {
  const payEnv = { ...env, ...configured, SHOP_GATEWAYS: "paypal" };

  it("gets a token, then creates an order the buyer can approve", async () => {
    respond((url) => {
      if (url.includes("/v1/oauth2/token")) {
        return new Response(JSON.stringify({ access_token: "tok", expires_in: 3600 }));
      }
      return new Response(
        JSON.stringify({
          id: "PAYPAL-1",
          links: [{ rel: "approve", href: "https://www.sandbox.paypal.com/checkoutnow?token=PAYPAL-1" }],
        }),
      );
    });

    const result = await gateway(payEnv, "paypal")!.createCheckout(input, payEnv);
    assert.equal(result.providerRef, "PAYPAL-1");
    assert.ok((result.redirectUrl).includes("paypal.com"));

    const body = String(calls[1]?.init.body);
    // No shipping: a file has nowhere to be delivered to.
    assert.ok((body).includes("NO_SHIPPING"));
    assert.ok((body).includes("O-2026-000001"));
  });

  it("says so when PayPal will not give a token", async () => {
    respond(() => new Response(JSON.stringify({ error: "invalid_client" }), { status: 401 }));
    await assert.rejects(gateway(payEnv, "paypal")!.createCheckout(input, payEnv));
  });

  it("verifies a webhook by asking PayPal, and believes only SUCCESS", async () => {
    const body = JSON.stringify({
      id: "WH-1",
      event_type: "PAYMENT.CAPTURE.COMPLETED",
      resource: { id: "CAP-1", amount: { value: "24.00", currency_code: "EUR" }, supplementary_data: { related_ids: { order_id: "PAYPAL-1" } } },
    });
    const request = new Request("https://shop.example.com/api/shop/webhooks/paypal", {
      method: "POST",
      headers: {
        "paypal-transmission-id": "t", "paypal-transmission-time": "now", "paypal-cert-url": "https://api.paypal.com/cert",
        "paypal-auth-algo": "SHA256withRSA", "paypal-transmission-sig": "sig",
      },
      body,
    });

    respond((url) => {
      if (url.includes("/v1/oauth2/token")) return new Response(JSON.stringify({ access_token: "tok", expires_in: 3600 }));
      return new Response(JSON.stringify({ verification_status: "SUCCESS" }));
    });
    assert.notEqual(await gateway(payEnv, "paypal")!.verifyWebhook(request, body, payEnv), null);

    respond((url) => {
      if (url.includes("/v1/oauth2/token")) return new Response(JSON.stringify({ access_token: "tok", expires_in: 3600 }));
      return new Response(JSON.stringify({ verification_status: "FAILURE" }));
    });
    assert.equal(await gateway(payEnv, "paypal")!.verifyWebhook(request.clone(), body, payEnv), null);
  });

  it("refuses a webhook with no signature headers at all", async () => {
    const body = "{}";
    const bare = new Request("https://shop.example.com/w", { method: "POST", body });
    assert.equal(await gateway(payEnv, "paypal")!.verifyWebhook(bare, body, payEnv), null);
  });
});
