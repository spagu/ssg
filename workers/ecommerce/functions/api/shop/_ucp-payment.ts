// Paying for a UCP session with Google Pay, processed by Stripe.
//
// Google Pay is the credential provider on Google's surfaces: the shop
// declares a `com.google.pay` payment handler (Google Pay, "Google Pay
// payment handler for UCP", version 2026-01-23, read 2026-11-14), Google
// shows its sheet, and the complete call carries
// `payment.instruments[].credential = {type: "PAYMENT_GATEWAY", token}` — the
// Google Pay tokenization data, encrypted for the gateway named in the
// handler's `tokenization_specification`.
//
// The gateway here is Stripe, configured as Google's processor table gives it
// ("gateway": "stripe", "stripe:version": "2018-10-31",
// "stripe:publishableKey"). With that configuration the token Google Pay
// returns is a Stripe card token (`{"id": "tok_…", …}` as a JSON string),
// which Stripe's API accepts in `card[token]` ("Create a PaymentMethod":
// "provide a Stripe token (e.g., for Apple Pay …) into the card hash"). So:
//
//   POST /v1/payment_intents  amount, currency, payment_method_types[]=card,
//        payment_method_data[type]=card, payment_method_data[card][token]=tok_…,
//        confirm=true            Idempotency-Key: ucp:<session>:<token hash>
//
// The idempotency key is what makes the capture idempotent: a retried
// completion with the same token gets Stripe's stored answer, never a second
// charge; a new token (after a decline) is a new attempt. PayPal also appears
// in Google's processor table, but its Google Pay flow needs PayPal's own
// client-side confirmation, so it is not offered here (UCP.md).

import type { Env } from "./_env";
import { intentOutcome, type ChargeOutcome } from "./_agent-charge";
import { sha256hex } from "./_lib";
import { stripeForm } from "./_stripe";
import type { OrderRow } from "./_types";

export const HANDLER_NAME = "com.google.pay";
export const HANDLER_VERSION = "2026-01-23";
const HANDLER_SPEC = "https://pay.google.com/gp/p/ucp/2026-01-23/";

/** Google's processor table gives this Stripe API version for the token. */
export const STRIPE_GPAY_VERSION = "2018-10-31";

export const handlerId = (env: Env): string => env.SHOP_UCP_HANDLER_ID || "gpay";
export const gpayEnvironment = (env: Env): "TEST" | "PRODUCTION" =>
  env.SHOP_UCP_GPAY_ENVIRONMENT === "PRODUCTION" ? "PRODUCTION" : "TEST";

/** The handler declaration, in the profile and in every checkout response
 *  (both are required by Google's handler page). */
export function googlePayHandler(env: Env, origin: string, merchantName: string): Record<string, unknown> {
  const merchantId = env.SHOP_UCP_GPAY_MERCHANT_ID;
  return {
    id: handlerId(env),
    version: HANDLER_VERSION,
    spec: HANDLER_SPEC,
    schema: `${HANDLER_SPEC}schemas/config.json`,
    config: {
      api_version: 2,
      api_version_minor: 0,
      environment: gpayEnvironment(env),
      merchant_info: {
        merchant_name: merchantName,
        ...(merchantId ? { merchant_id: merchantId } : {}),
        merchant_origin: new URL(origin).host,
      },
      allowed_payment_methods: [
        {
          type: "CARD",
          parameters: {
            allowed_auth_methods: ["PAN_ONLY", "CRYPTOGRAM_3DS"],
            allowed_card_networks: ["AMEX", "DISCOVER", "JCB", "MASTERCARD", "VISA"],
            // The country is the VAT evidence for a download; MIN carries it
            // (and the postal code) without asking for a street address.
            billing_address_required: true,
            billing_address_parameters: { format: "MIN" },
          },
          tokenization_specification: {
            type: "PAYMENT_GATEWAY",
            parameters: {
              gateway: "stripe",
              "stripe:version": STRIPE_GPAY_VERSION,
              "stripe:publishableKey": env.SHOP_UCP_STRIPE_PUBLISHABLE_KEY,
            },
          },
        },
      ],
    },
  };
}

export type InstrumentReading =
  | { ok: true; instrument: Record<string, unknown>; stripeToken: string; billingCountry: string | null }
  | { ok: false; missing: boolean };

const isObject = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === "object" && !Array.isArray(v);

/** The instrument to charge: the selected one (or the only one), from this
 *  shop's handler, carrying a Stripe card token. `missing` means there was no
 *  payment object at all — a malformed complete, not an ineligible card. */
export function readInstrument(env: Env, payment: unknown): InstrumentReading {
  const list = isObject(payment) && Array.isArray(payment.instruments) ? payment.instruments.filter(isObject) : [];
  if (list.length === 0) return { ok: false, missing: true };
  const chosen = list.find((i) => i.selected === true) ?? (list.length === 1 ? list[0] : undefined);
  if (!chosen || chosen.handler_id !== handlerId(env)) return { ok: false, missing: false };
  const credential = chosen.credential;
  if (!isObject(credential) || credential.type !== "PAYMENT_GATEWAY" || typeof credential.token !== "string") {
    return { ok: false, missing: false };
  }
  const stripeToken = stripeTokenOf(credential.token);
  if (!stripeToken) return { ok: false, missing: false };
  // Kept to echo back: everything but the credential, which the protocol
  // marks response-omitted (token_credential.json).
  const { credential: _secret, ...instrument } = chosen;
  const billing = isObject(chosen.billing_address) ? chosen.billing_address.address_country : undefined;
  const billingCountry = typeof billing === "string" && /^[A-Za-z]{2}$/.test(billing) ? billing.toUpperCase() : null;
  return { ok: true, instrument, stripeToken, billingCountry };
}

/** The Stripe token inside Google Pay's tokenization data. Anything else —
 *  another gateway's payload, a raw ECv2 envelope — cannot be charged here. */
export function stripeTokenOf(token: string): string | null {
  try {
    const parsed = JSON.parse(token) as unknown;
    const id = isObject(parsed) ? parsed.id : undefined;
    return typeof id === "string" && /^tok_[A-Za-z0-9_]{1,250}$/.test(id) ? id : null;
  } catch {
    return null;
  }
}

/** Charges the token. Declines are outcomes; Stripe failing throws. */
export async function chargeGooglePay(
  env: Env,
  input: { sessionId: string; order: OrderRow; stripeToken: string },
): Promise<ChargeOutcome> {
  const { order } = input;
  const form = new URLSearchParams();
  form.set("amount", String(order.total_minor));
  form.set("currency", order.currency.toLowerCase());
  // Card only: with automatic payment methods Stripe would want a return_url
  // for redirect methods this server-side confirmation cannot follow.
  form.set("payment_method_types[0]", "card");
  form.set("payment_method_data[type]", "card");
  form.set("payment_method_data[card][token]", input.stripeToken);
  form.set("confirm", "true");
  form.set("metadata[order_id]", order.id);
  form.set("metadata[order_number]", order.number);
  form.set("metadata[ucp_checkout_id]", input.sessionId);
  form.set("metadata[channel]", "ucp");

  const key = `ucp:${input.sessionId}:${(await sha256hex(input.stripeToken)).slice(0, 32)}`;
  const { status, body } = await stripeForm(env, "/v1/payment_intents", form, key);
  return intentOutcome(status, body);
}
