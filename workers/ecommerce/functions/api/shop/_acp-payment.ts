// Paying for an ACP session with a Stripe Shared Payment Token (SPT).
//
// The agent platform holds the buyer's card; it grants the seller's Stripe
// account a token scoped to one amount, currency and expiry (the protocol's
// delegate_payment step), and sends that token in `payment_data` on complete.
// The seller turns it into a PaymentIntent, confirmed in the same call
// (Stripe, "Shared payment tokens — Sellers", read 2026-11-14):
//
//   POST /v1/payment_intents  amount, currency,
//        payment_method_data[shared_payment_granted_token]=spt_…, confirm=true
//   Stripe-Version: <a preview version> (SHOP_ACP_STRIPE_VERSION)
//
// It goes through the shop's own Stripe module (stripeForm in _stripe.ts):
// the same key, no SDK, and an Idempotency-Key, which is what makes the
// capture idempotent. The key is the session plus a hash of the token, so a
// retried completion with the same token gets Stripe's stored answer — the
// same PaymentIntent, never a second charge — while a new token (after a
// decline) is a new attempt.

import type { Env } from "./_env";
import { acpError, type AcpResult } from "./_acp-http";
import { intentOutcome, type ChargeOutcome } from "./_agent-charge";
import { sha256hex } from "./_lib";
import { stripeForm } from "./_stripe";
import type { OrderRow } from "./_types";

/** The handler's id in this shop's sessions; `payment_data.handler_id` must
 *  name it when it names one. */
export const HANDLER_ID = "card_tokenized";

/** The Stripe-Version Stripe's SPT documentation used on 2026-11-14. */
export const DEFAULT_STRIPE_VERSION = "2026-09-30.preview";

/** The payment handler this shop declares in `capabilities.payment.handlers`:
 *  the RFC's own "card tokenization, delegated, same PSP" example (§8.1). */
export function paymentHandler(env: Env): Record<string, unknown> {
  return {
    id: HANDLER_ID,
    name: "dev.acp.tokenized.card",
    version: "2026-01-22",
    spec: "https://acp.dev/specs/handlers/card",
    requires_delegate_payment: true,
    requires_pci_compliance: false,
    psp: "stripe",
    config_schema: "https://acp.dev/schemas/handlers/card/config.json",
    instrument_schemas: ["https://acp.dev/schemas/handlers/card/instrument.json"],
    config: {
      merchant_id: env.SHOP_ACP_MERCHANT_ID,
      psp: "stripe",
      accepted_brands: ["visa", "mastercard", "amex"],
    },
  };
}

/** The token from `payment_data`. The schema's shape is
 *  `{handler_id, instrument: {type, credential: {type: "spt", token}}}`; the
 *  same release's own complete example still sends `{token, provider:
 *  "stripe"}`, so that is read too. */
export function readToken(value: unknown): string | AcpResult {
  const invalid = acpError(400, "invalid_request", "invalid", "payment_data needs a Stripe shared payment token.", "$.payment_data");
  if (value === null || typeof value !== "object") return invalid;
  const data = value as Record<string, unknown>;
  const instrument = data.instrument as { credential?: { type?: unknown; token?: unknown } } | undefined;
  if (instrument && typeof instrument === "object") {
    if (data.handler_id !== undefined && data.handler_id !== HANDLER_ID) return invalid;
    const credential = instrument.credential;
    if (!credential || credential.type !== "spt") return invalid;
    return tokenOrInvalid(credential.token, invalid);
  }
  if (data.provider !== "stripe") return invalid;
  return tokenOrInvalid(data.token, invalid);
}

function tokenOrInvalid(token: unknown, invalid: AcpResult): string | AcpResult {
  return typeof token === "string" && /^[A-Za-z0-9_]{1,255}$/.test(token) ? token : invalid;
}

export { recordCharge, type ChargeOutcome } from "./_agent-charge";

/** Charges the token. A refusal (card declined, token spent, a 3-D Secure
 *  step this flow cannot perform) is an outcome; Stripe being unreachable or
 *  failing (5xx) throws, so the caller answers 5xx and the agent retries. */
export async function chargeToken(
  env: Env,
  input: { sessionId: string; order: OrderRow; token: string },
): Promise<ChargeOutcome> {
  const { order } = input;
  const form = new URLSearchParams();
  form.set("amount", String(order.total_minor));
  form.set("currency", order.currency.toLowerCase());
  form.set("payment_method_data[shared_payment_granted_token]", input.token);
  form.set("confirm", "true");
  form.set("metadata[order_id]", order.id);
  form.set("metadata[order_number]", order.number);
  form.set("metadata[acp_checkout_session_id]", input.sessionId);
  form.set("metadata[channel]", "acp");

  const key = `acp:${input.sessionId}:${(await sha256hex(input.token)).slice(0, 32)}`;
  const { status, body } = await stripeForm(env, "/v1/payment_intents", form, key, {
    "stripe-version": env.SHOP_ACP_STRIPE_VERSION || DEFAULT_STRIPE_VERSION,
  });
  return intentOutcome(status, body);
}
