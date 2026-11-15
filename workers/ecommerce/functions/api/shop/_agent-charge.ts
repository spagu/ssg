// A Stripe PaymentIntent created and confirmed in one call by an agent
// checkout — what ACP's Shared Payment Token (_acp-payment.ts) and UCP's
// Google Pay token (_ucp-payment.ts) both end in — read as an outcome, and
// recorded the way the webhook path records a charge.

import type { Env } from "./_env";
import { redact } from "./_gateway";
import { newId, nowISO } from "./_lib";
import type { OrderRow } from "./_types";

export type ChargeOutcome =
  | { kind: "paid"; ref: string; amountMinor: number; currency: string; raw: unknown }
  | { kind: "declined"; reason: string };

/** Stripe's answer to `POST /v1/payment_intents … confirm=true`. A refusal
 *  (card declined, token spent, a 3-D Secure step these flows cannot perform)
 *  is an outcome; Stripe failing (5xx) throws, so the caller can say "nothing
 *  was charged, retry" instead of "declined". */
export function intentOutcome(status: number, body: Record<string, unknown>): ChargeOutcome {
  if (status >= 500) throw new Error(`stripe answered ${status}`);
  if (status >= 400) {
    const error = (body.error ?? {}) as { code?: string; decline_code?: string };
    return { kind: "declined", reason: String(error.decline_code ?? error.code ?? `http_${status}`) };
  }
  // requires_action (3-D Secure) and processing are not money in the bank,
  // and neither protocol's authentication step is implemented here.
  if (body.status !== "succeeded") return { kind: "declined", reason: `status_${String(body.status)}` };
  return {
    kind: "paid",
    ref: String(body.id),
    amountMinor: Number(body.amount_received),
    currency: String(body.currency).toUpperCase(),
    raw: redact(body),
  };
}

/** The charge, as the webhook path records one: a payments row (once — the
 *  unique index on gateway + ref + kind makes a repeat a no-op) and the
 *  PaymentIntent as the order's gateway reference, so a later refund event
 *  from Stripe finds the order. */
export async function recordCharge(env: Env, order: OrderRow, outcome: Extract<ChargeOutcome, { kind: "paid" }>): Promise<void> {
  await env.SHOP_DB.batch([
    env.SHOP_DB.prepare(
      `INSERT OR IGNORE INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, raw_json, created_at)
       VALUES (?, ?, 'stripe', ?, 'charge', 'succeeded', ?, ?, ?, ?)`,
    ).bind(newId(), order.id, outcome.ref, outcome.amountMinor, outcome.currency, JSON.stringify(outcome.raw).slice(0, 20000), nowISO()),
    env.SHOP_DB.prepare(`UPDATE orders SET gateway_ref = ?, updated_at = ? WHERE id = ? AND gateway_ref IS NULL`).bind(
      outcome.ref,
      nowISO(),
      order.id,
    ),
  ]);
}
