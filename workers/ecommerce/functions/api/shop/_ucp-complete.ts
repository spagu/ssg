// POST /checkout-sessions/{id}/complete — the one UCP step that takes money.
//
//   claim the session (ready_for_complete → complete_in_progress, one CAS:
//     of two concurrent completes exactly one gets past this line)
//   price it again (the catalogue may have moved since the platform looked)
//   find or write the order — channel 'ucp', gateway 'stripe', status pending
//   charge the Google Pay token through Stripe (idempotent per session + token)
//   record the payment, then the SAME fulfilment as a storefront order
//     (_fulfil.ts: invoice number without gaps, download tokens, emails)
//   mark the session completed, queue the order event for the platform
//
// A decline, and Stripe failing, are answered 200 with the session back at
// ready_for_complete and a `payment_declined` / `payment_failed` message —
// Google's rule ("return 200 OK even when your payment gateway returns a 4xx
// or 5xx") so its sheet can ask for another card. A completion that died
// after charging is finished by the next one, without a new charge.

import type { AgentOpInput } from "./_agent-common";
import { recordCharge } from "./_agent-charge";
import { audit } from "./_audit";
import { fulfilOrder, markNeedsReview } from "./_fulfil";
import { userAgent } from "./_lib";
import { createOrder, getOrder, upsertCustomer, type PricedBasket } from "./_orders";
import { drainOutbox } from "./_outbox";
import { getSettingString } from "./_settings";
import type { OrderRow } from "./_types";
import { stateConflict, withBuyer } from "./_ucp-checkout";
import { errorMessage, notFound, ucpError, type UcpMessage, type UcpResult } from "./_ucp-http";
import { enqueueUcpEvent, ucpOrder } from "./_ucp-order";
import { chargeGooglePay, readInstrument, type InstrumentReading } from "./_ucp-payment";
import { priceUcp, type UcpPriced } from "./_ucp-pricing";
import { renderSession } from "./_ucp-render";
import { chargeTest } from "./_ucp-testing";
import { attachOrder, cancelPendingOrder, casUpdate, loadSession, parseJson, type UcpDetails, type UcpItem, type UcpSessionRow } from "./_ucp-session";

/** A completion still "in progress" this long after its last write died
 *  half-way and may be resumed. Long enough that a live one has finished. */
export const COMPLETE_STALE_MS = 5 * 60_000;

const PAYMENT_PATH = "$.payment.instruments[0]";
type Paying = Extract<InstrumentReading, { ok: true }>;
const EMPTY: UcpPriced = { ready: false, lineItems: [], totals: [], messages: [], totalMinor: 0 };

export async function completeSession(input: AgentOpInput): Promise<UcpResult> {
  const { env, body, id, origin } = input;
  const reading = readInstrument(env, body.payment);
  if (!reading.ok && reading.missing) {
    return ucpError(400, "invalid_request", "payment.instruments is required to complete.", { path: "$.payment.instruments" });
  }

  const row = await loadSession(env, id);
  if (!row) return notFound(origin);
  if (row.status === "completed") return show(input, row);
  const stale = new Date(Date.now() - COMPLETE_STALE_MS).toISOString();
  const resumable = row.status === "complete_in_progress" && row.updated_at < stale;
  if (row.status === "canceled" || (row.status === "complete_in_progress" && !resumable)) return stateConflict(row);
  if (!reading.ok) {
    return show(input, row, [errorMessage("payment_ineligible", "This payment method is not accepted here; choose another.", "recoverable", PAYMENT_PATH)]);
  }

  // The billing country is the last piece of tax evidence the platform can
  // give; it fills a session that had no country, never overrides one.
  const buyer = parseJson<Record<string, string> | null>(row.buyer_json, null);
  const details = parseJson<UcpDetails>(row.details_json, {});
  let { country, priced_json: pricedJson } = row;
  let ready = row.status === "ready_for_complete" || resumable;
  if (!country && reading.billingCountry) {
    const filled = withBuyer((await priceUcp(env, { items: items(row), currency: row.currency, country: reading.billingCountry, origin })).state, buyer);
    country = reading.billingCountry;
    pricedJson = JSON.stringify(filled.state);
    details.countrySource = "billing";
    ready = filled.status === "ready_for_complete";
  }
  if (!ready) return show(input, row);

  // The claim. Losing it means another request changed or claimed the session.
  details.instrument = reading.instrument;
  if (!(await casUpdate(env, row, { status: "complete_in_progress", country, priced_json: pricedJson, details_json: JSON.stringify(details) }))) {
    return stateConflict((await loadSession(env, id))!);
  }
  const held = (await loadSession(env, id))!;
  try {
    return await finish(input, held, buyer!, reading);
  } catch (e) {
    await casUpdate(env, held, { status: "ready_for_complete" });
    await audit(env, "ucp", "ucp.complete_error", held.id, { error: String(e).slice(0, 300) });
    return show(input, (await loadSession(env, id))!, [errorMessage("payment_failed", "The payment could not be processed right now. Nothing was charged; try again.", "recoverable", PAYMENT_PATH)]);
  }
}

const items = (row: UcpSessionRow): UcpItem[] => parseJson<UcpItem[]>(row.items_json, []);

async function show(input: AgentOpInput, row: UcpSessionRow, extra: UcpMessage[] = []): Promise<UcpResult> {
  const body = await renderSession(input.env, row, input.origin);
  return { status: 200, body: { ...body, messages: [...(body.messages as UcpMessage[]), ...extra] } };
}

async function finish(input: AgentOpInput, row: UcpSessionRow, buyer: Record<string, string>, pay: Paying): Promise<UcpResult> {
  const { env, origin, request } = input;
  const before = parseJson<UcpPriced>(row.priced_json, EMPTY);

  // The recovery point: an order of this session that already has a payment
  // was charged by a request that died before it finished. Charging again —
  // perhaps with a different token — would take the money twice.
  const prior = row.order_id ? await getOrder(env, row.order_id) : null;
  if (prior?.gateway_ref) return conclude(input, row, prior, row.order_key!, before);

  const priced = await priceUcp(env, { items: items(row), currency: row.currency, country: row.country, origin });
  const { state, status } = withBuyer(priced.state, buyer);
  if (!priced.basket || state.totalMinor !== before.totalMinor) {
    if (priced.basket) state.messages.push({ type: "warning", code: "totals_changed", path: "$.totals", content: "The total changed since the session was priced; review it and complete again." });
    return putBack(input, row, state, status);
  }

  const { order, orderKey } = await ensureOrder(input, row, prior, priced.basket, buyer, userAgent(request));
  const outcome = pay.test ? chargeTest(order, pay.test) : await chargeGooglePay(env, { sessionId: row.id, order, stripeToken: pay.stripeToken });
  if (outcome.kind === "declined") {
    await audit(env, "ucp", "ucp.payment_declined", order.id, { number: order.number, reason: outcome.reason.slice(0, 100) });
    state.messages.push(errorMessage("payment_declined", "The payment was declined. Choose another payment method.", "recoverable", PAYMENT_PATH));
    return putBack(input, row, state, "ready_for_complete");
  }
  if (outcome.amountMinor !== order.total_minor || outcome.currency !== order.currency.toUpperCase()) {
    // Stripe charged what it was asked; anything else is a human's decision,
    // parked before the payment is recorded so a resumed completion never
    // mistakes this order for one that only needs delivering.
    await markNeedsReview(env, order, `ucp paid ${outcome.amountMinor} ${outcome.currency}, expected ${order.total_minor} ${order.currency}`, { origin, actor: "ucp" });
  }
  await recordCharge(env, order, outcome);
  return conclude(input, row, order, orderKey, state);
}

/** Delivery and the paperwork, exactly as for a storefront order, then the
 *  session's last write and the platform's event. Safe to run twice. */
async function conclude(input: AgentOpInput, row: UcpSessionRow, order: OrderRow, orderKey: string, state: UcpPriced): Promise<UcpResult> {
  const { env, origin } = input;
  const current = (await getOrder(env, order.id))!;
  if (current.status !== "needs_review") await fulfilOrder(env, current, { origin, actor: "ucp", orderKey });
  await casUpdate(env, row, { status: "completed", priced_json: JSON.stringify(state) });
  const done = (await loadSession(env, row.id))!;
  await enqueueUcpEvent(env, (await ucpOrder(env, done, origin))!, order.id, origin);
  input.waitUntil(drainOutbox(env, 5).catch(() => 0));
  return show(input, done);
}

async function putBack(input: AgentOpInput, row: UcpSessionRow, state: UcpPriced, status: UcpSessionRow["status"]): Promise<UcpResult> {
  await casUpdate(input.env, row, { status, priced_json: JSON.stringify({ ...state, ready: status === "ready_for_complete" }) });
  return show(input, (await loadSession(input.env, row.id))!);
}

/** The session's order: the one an earlier attempt wrote, if it still
 *  matches the basket, or a new one. A stale one is cancelled, never reused
 *  at the wrong price. Invoice numbers are issued only at fulfilment, so a
 *  skipped order number never leaves a gap in them. */
async function ensureOrder(
  input: AgentOpInput,
  row: UcpSessionRow,
  existing: OrderRow | null,
  basket: PricedBasket,
  buyer: Record<string, string>,
  agent: string,
): Promise<{ order: OrderRow; orderKey: string }> {
  const { env } = input;
  if (existing?.status === "pending" && existing.total_minor === basket.totalMinor && existing.currency === basket.currency) {
    return { order: existing, orderKey: row.order_key! };
  }
  await cancelPendingOrder(env, row.order_id);

  const name = [buyer.first_name, buyer.last_name].filter(Boolean).join(" ");
  const customer = await upsertCustomer(env, { email: buyer.email!, name, vatId: "", vatIdValid: null, country: basket.taxCountry });
  const details = parseJson<UcpDetails>(row.details_json, {});
  const created = await createOrder(env, basket, customer, {
    gateway: "stripe",
    locale: details.language ?? row.locale ?? "en",
    consentMarketing: false,
    // The waiver went out as a disclosure the platform must not hide
    // (_ucp-pricing); completing is the buyer's agreement to it.
    consentWaiver: (await getSettingString(env, "legal.digital_waiver")) !== "false",
    ipHash: null,
    userAgent: agent,
    channel: "ucp",
  });
  await attachOrder(env, row, created.order.id, created.orderKey);
  return { order: created.order, orderKey: created.orderKey };
}
