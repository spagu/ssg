// POST /checkout_sessions/{id}/complete — the one step that takes money.
//
//   claim the session (ready_for_payment → complete_in_progress, one CAS:
//     of two concurrent completes exactly one gets past this line)
//   price it again (the catalogue may have moved since the agent looked)
//   find or write the order — channel 'acp', status pending
//   charge the Shared Payment Token (Stripe, idempotent per session + token)
//   record the payment, then the SAME fulfilment as a storefront order
//     (_fulfil.ts: invoice number without gaps, download tokens, emails)
//   mark the session completed, queue order_create for the platform
//
// A decline puts the session back to ready_for_payment with a MessageError,
// so the agent can complete again with another token, as the protocol's
// messages model intends. A failure on our side (Stripe unreachable, a bug)
// does the same and answers 5xx, which the idempotency layer never stores, so
// the agent's retry runs fresh — and meets Stripe's stored answer for the
// same token instead of a second charge.

import { audit } from "./_audit";
import { acpError, type AcpResult } from "./_acp-http";
import { isResult, readBuyer } from "./_acp-input";
import { chargeToken, readToken, recordCharge } from "./_acp-payment";
import { priceSession, type Message, type PricedState } from "./_acp-pricing";
import { renderSession } from "./_acp-render";
import { attachOrder, cancelPendingOrder, casUpdate, FINAL, loadSession, parseJson, type AcpItem, type SessionRow } from "./_acp-session";
import { acpOrder, enqueueOrderEvent } from "./_acp-webhook";
import { notFound, stateConflict, type OpInput } from "./_acp-checkout";
import { fulfilOrder, markNeedsReview } from "./_fulfil";
import { userAgent } from "./_lib";
import { createOrder, getOrder, upsertCustomer, type PricedBasket } from "./_orders";
import { drainOutbox } from "./_outbox";
import { getSettingString } from "./_settings";
import type { OrderRow } from "./_types";

/** A completion still "in progress" this long after its last write died
 *  half-way and may be resumed. Long enough that a live one has finished. */
export const COMPLETE_STALE_MS = 5 * 60_000;

export async function completeSession(input: OpInput): Promise<AcpResult> {
  const { env, body, id } = input;
  const token = readToken(body.payment_data);
  if (isResult(token)) return token;
  const buyerPatch = body.buyer === undefined ? undefined : readBuyer(body.buyer);
  if (isResult(buyerPatch)) return buyerPatch;

  const row = await loadSession(env, id);
  if (!row) return notFound();
  const stale = new Date(Date.now() - COMPLETE_STALE_MS).toISOString();
  const resumable = row.status === "complete_in_progress" && row.updated_at < stale;
  if (FINAL.includes(row.status) || (row.status === "complete_in_progress" && !resumable)) return stateConflict(row);
  if (row.status === "not_ready_for_payment") {
    return acpError(400, "invalid_request", "not_ready_for_payment", "The session has unresolved messages; update it first.");
  }
  const buyer = buyerPatch ?? parseJson<Record<string, string> | null>(row.buyer_json, null);
  if (!buyer) return acpError(400, "invalid_request", "missing", "The buyer's email is required to deliver the order.", "$.buyer.email");

  // The claim. Losing it means another request changed or claimed the session.
  if (!(await casUpdate(env, row, { status: "complete_in_progress", buyer_json: JSON.stringify(buyer) }))) {
    return stateConflict((await loadSession(env, id))!);
  }
  const held = (await loadSession(env, id))!;
  try {
    return await finish(input, held, buyer, token);
  } catch (e) {
    await casUpdate(env, held, { status: "ready_for_payment" });
    await audit(env, "acp", "acp.complete_error", held.id, { error: String(e).slice(0, 300) });
    return acpError(502, "processing_error", "payment_unavailable", "The payment could not be processed right now. Nothing was charged; retry.");
  }
}

async function finish(input: OpInput, row: SessionRow, buyer: Record<string, string>, token: string): Promise<AcpResult> {
  const { env, origin, request } = input;
  const before = parseJson<PricedState>(row.priced_json, { ready: false, lineItems: [], totals: [], messages: [], totalMinor: 0 });

  // The recovery point: an order of this session that already has a payment
  // was charged by a request that died before it finished. Charging again —
  // perhaps with a different token — would take the money twice, so the rest
  // of that request's work is done instead.
  const prior = row.order_id ? await getOrder(env, row.order_id) : null;
  if (prior?.gateway_ref) return conclude(input, row, prior, row.order_key!, before);

  const { state, basket } = await priceSession(env, {
    items: parseJson<AcpItem[]>(row.items_json, []),
    currency: row.currency,
    country: row.country,
    origin,
  });
  if (!basket || state.totalMinor !== before.totalMinor) {
    if (basket) state.messages.push(message("warning", "price_change", "$.totals", "The total changed since the session was priced; review it and complete again."));
    return putBack(input, row, state, basket ? "ready_for_payment" : "not_ready_for_payment");
  }

  const { order, orderKey } = await ensureOrder(env, row, prior, basket, buyer, userAgent(request));
  const outcome = await chargeToken(env, { sessionId: row.id, order, token });
  if (outcome.kind === "declined") {
    await audit(env, "acp", "acp.payment_declined", order.id, { number: order.number, reason: outcome.reason.slice(0, 100) });
    state.messages.push(message("error", "payment_declined", "$.payment_data", "The payment was declined. Ask the buyer for another payment method."));
    return putBack(input, row, state, "ready_for_payment");
  }
  if (outcome.amountMinor !== order.total_minor || outcome.currency !== order.currency.toUpperCase()) {
    // Stripe charged what it was asked; anything else is a human's decision.
    // Parked before the payment is recorded, so a resumed completion never
    // mistakes this order for one that only needs delivering.
    await markNeedsReview(env, order, `acp paid ${outcome.amountMinor} ${outcome.currency}, expected ${order.total_minor} ${order.currency}`, { origin, actor: "acp" });
  }
  await recordCharge(env, order, outcome);
  return conclude(input, row, order, orderKey, state);
}

/** Delivery and the paperwork, exactly as for a storefront order, then the
 *  session's last write and the platform's event. Safe to run twice. */
async function conclude(input: OpInput, row: SessionRow, order: OrderRow, orderKey: string, state: PricedState): Promise<AcpResult> {
  const { env, origin } = input;
  const current = (await getOrder(env, order.id))!;
  if (current.status !== "needs_review") await fulfilOrder(env, current, { origin, actor: "acp", orderKey });
  await casUpdate(env, row, { status: "completed", priced_json: JSON.stringify(state) });
  const done = (await loadSession(env, row.id))!;
  await enqueueOrderEvent(env, "order_create", (await acpOrder(env, done, origin))!);
  input.waitUntil(drainOutbox(env, 5).catch(() => 0));
  return { status: 200, body: await renderSession(env, done, origin) };
}

function message(type: "warning" | "error", code: string, param: string, content: string): Message {
  return { type, code, severity: "high", resolution: "requires_buyer_input", param, content_type: "plain", content };
}

async function putBack(input: OpInput, row: SessionRow, state: PricedState, status: "ready_for_payment" | "not_ready_for_payment"): Promise<AcpResult> {
  await casUpdate(input.env, row, { status, priced_json: JSON.stringify({ ...state, ready: status === "ready_for_payment" }) });
  return { status: 200, body: await renderSession(input.env, (await loadSession(input.env, row.id))!, input.origin) };
}

/** The session's order: the one an earlier attempt wrote, if it still matches
 *  the basket, or a new one. A stale one is cancelled, never reused at the
 *  wrong price. Order numbers may skip; invoice numbers are only issued at
 *  fulfilment, so they never do. */
async function ensureOrder(
  env: OpInput["env"],
  row: SessionRow,
  existing: OrderRow | null,
  basket: PricedBasket,
  buyer: Record<string, string>,
  agent: string,
): Promise<{ order: OrderRow; orderKey: string }> {
  if (existing?.status === "pending" && existing.total_minor === basket.totalMinor && existing.currency === basket.currency) {
    return { order: existing, orderKey: row.order_key! };
  }
  await cancelPendingOrder(env, row.order_id);

  const name = buyer.full_name ?? [buyer.first_name, buyer.last_name].filter(Boolean).join(" ");
  const customer = await upsertCustomer(env, { email: buyer.email!, name, vatId: "", vatIdValid: null, country: basket.taxCountry });
  const created = await createOrder(env, basket, customer, {
    gateway: "stripe",
    locale: row.locale ?? "en",
    consentMarketing: false,
    // The waiver text went out with every line and as a buyer-review message
    // (_acp-pricing); completing is the buyer's agreement to it.
    consentWaiver: (await getSettingString(env, "legal.digital_waiver")) !== "false",
    ipHash: null,
    userAgent: agent,
    channel: "acp",
  });
  // Held by this request (status complete_in_progress), so a plain write; the
  // version is left alone so the final CAS still matches.
  await attachOrder(env, row, created.order.id, created.orderKey);
  return { order: created.order, orderKey: created.orderKey };
}
