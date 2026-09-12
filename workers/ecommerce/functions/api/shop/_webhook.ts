// The webhook handler both providers share.
//
// Three rules decide everything here:
//
//   1. An unverified body is a 400 and is not written anywhere. Signature
//      first, always (S-05).
//   2. An event is processed once. The inbox insert is the lock: if the row
//      already exists this delivery is a duplicate and we say so cheerfully,
//      because providers retry and a 500 makes them retry harder (S-06).
//   3. The amount we were paid is compared with the amount we asked for. A
//      mismatch parks the order for a human instead of fulfilling it (S-07).

import type { Env } from "./_env";
import { audit } from "./_audit";
import { revokeTokensForOrder } from "./_downloads";
import { fulfilOrder, markNeedsReview, shopOrigin } from "./_fulfil";
import { gateway } from "./_gateways";
import type { GatewayEvent } from "./_gateway";
import { newId, nowISO } from "./_lib";
import { findOrderByGatewayRef, getOrderItems } from "./_orders";
import { drainOutbox } from "./_outbox";
import { ensureSchema } from "./_schema";
import type { CustomerRow, InvoiceRow, OrderRow } from "./_types";

type Ctx = Parameters<PagesFunction<Env>>[0];

export async function handleWebhook(context: Ctx, name: string): Promise<Response> {
  const { request, env, waitUntil } = context;
  if (!env.SHOP_DB) return new Response("shop database not bound", { status: 503 });
  await ensureSchema(env);

  const provider = gateway(env, name);
  if (!provider) return new Response("gateway not configured", { status: 503 });

  // The raw body is needed byte for byte: a re-serialised JSON object will not
  // match the signature.
  const raw = await request.text();
  const verified = await provider.verifyWebhook(request, raw, env);
  if (!verified) return new Response("signature verification failed", { status: 400 });

  // The insert is the idempotency lock. A duplicate delivery changes no rows
  // and returns here without touching an order.
  const claim = await env.SHOP_DB.prepare(
    `INSERT OR IGNORE INTO webhook_inbox (gateway, event_id, event_type, received_at) VALUES (?, ?, ?, ?)`,
  )
    .bind(name, verified.eventId, verified.eventType, nowISO())
    .run();
  if ((claim.meta?.changes ?? 0) === 0) {
    return json200({ received: true, action: "duplicate" });
  }

  let action = "ignored";
  try {
    action = await applyEvent(env, request, name, verified.event);
  } catch (e) {
    // The event is recorded as failed but still acknowledged: asking the
    // provider to redeliver forever will not fix a bug in our own handler, and
    // the row is now visible in the panel.
    const message = String(e instanceof Error ? e.message : e).slice(0, 300);
    await env.SHOP_DB.prepare(
      `UPDATE webhook_inbox SET processed_at = ?, result = ? WHERE gateway = ? AND event_id = ?`,
    )
      .bind(nowISO(), `error:${message}`, name, verified.eventId)
      .run();
    return json200({ received: true, action: "error" });
  }

  await env.SHOP_DB.prepare(
    `UPDATE webhook_inbox SET processed_at = ?, result = ? WHERE gateway = ? AND event_id = ?`,
  )
    .bind(nowISO(), action, name, verified.eventId)
    .run();

  // The provider gets its 200 now; the email goes out behind it.
  waitUntil(drainOutbox(env, 5).catch(() => 0));
  return json200({ received: true, action });
}

const json200 = (body: unknown): Response =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json", "cache-control": "no-store" },
  });

async function applyEvent(env: Env, request: Request, name: string, event: GatewayEvent): Promise<string> {
  if (event.kind === "ignored") return "ignored";

  const order = await findOrder(env, name, event.providerRef);
  if (!order) return "unknown-order";

  const origin = await shopOrigin(env, request);
  const actor = `webhook:${name}`;

  switch (event.kind) {
    case "paid":
      return applyPaid(env, order, event, origin, actor);
    case "failed":
      return applyFailed(env, order, event.reason, actor);
    case "refunded":
      return applyRefunded(env, order, event, origin, actor);
    default:
      return "ignored";
  }
}

/** Finds the order an event is about.
 *
 *  Providers do not use one id throughout. Stripe's checkout events name the
 *  session (which is what we stored as gateway_ref), but charge.refunded names
 *  the payment intent — the id we recorded on the payment row when the charge
 *  succeeded. Looking only at orders means a refund silently matches nothing,
 *  and the links keep working for a buyer who has their money back. */
async function findOrder(env: Env, name: string, providerRef: string): Promise<OrderRow | null> {
  const direct = await findOrderByGatewayRef(env, name, providerRef);
  if (direct) return direct;

  const payment = await env.SHOP_DB.prepare(
    `SELECT order_id FROM payments WHERE gateway = ? AND gateway_ref = ? ORDER BY created_at LIMIT 1`,
  )
    .bind(name, providerRef)
    .first<{ order_id: string }>();
  if (!payment) return null;

  return env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(payment.order_id).first<OrderRow>();
}

async function applyPaid(
  env: Env,
  order: OrderRow,
  event: Extract<GatewayEvent, { kind: "paid" }>,
  origin: string,
  actor: string,
): Promise<string> {
  await recordPayment(env, order, event.paymentRef, "charge", "succeeded", event.amountMinor, event.currency, event.raw);

  // The third piece of VAT evidence arrives here: the country of the card or
  // account. It can settle a disagreement the checkout flagged.
  if (event.cardCountry) {
    const evidence = safeParse(order.tax_evidence) ?? {};
    evidence.card = event.cardCountry;
    if (evidence.billing && evidence.card === evidence.billing) evidence.conflict = false;
    await env.SHOP_DB.prepare(`UPDATE orders SET tax_evidence = ? WHERE id = ?`)
      .bind(JSON.stringify(evidence), order.id)
      .run();
  }

  // What was asked for and what was paid must match. Anything else is a human's
  // decision, not a robot's.
  if (event.currency && event.currency.toUpperCase() !== order.currency.toUpperCase()) {
    await markNeedsReview(env, order, `paid in ${event.currency}, expected ${order.currency}`, { origin, actor });
    return "needs-review-currency";
  }
  if (event.amountMinor !== order.total_minor) {
    await markNeedsReview(
      env,
      order,
      `paid ${event.amountMinor} ${order.currency} minor, expected ${order.total_minor}`,
      { origin, actor },
    );
    return "needs-review-amount";
  }

  const result = await fulfilOrder(env, order, { origin, actor });
  return result.changed ? "fulfilled" : "already-paid";
}

async function applyFailed(env: Env, order: OrderRow, reason: string, actor: string): Promise<string> {
  // "expired" is an abandoned basket, not a failure worth alarming anyone
  // about; both leave the order unfulfilled and out of the way.
  const status = reason === "expired" ? "cancelled" : "failed";
  const res = await env.SHOP_DB.prepare(
    `UPDATE orders SET status = ?, updated_at = ? WHERE id = ? AND status = 'pending'`,
  )
    .bind(status, nowISO(), order.id)
    .run();
  if ((res.meta?.changes ?? 0) === 0) return "no-change";
  await audit(env, actor, `order.${status}`, order.id, { reason, number: order.number });
  return status;
}

async function applyRefunded(
  env: Env,
  order: OrderRow,
  event: Extract<GatewayEvent, { kind: "refunded" }>,
  origin: string,
  actor: string,
): Promise<string> {
  await recordPayment(env, order, event.paymentRef, "refund", "succeeded", event.amountMinor, event.currency, event.raw);

  const full = Math.abs(event.amountMinor) >= order.total_minor;
  if (full) {
    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'refunded', updated_at = ? WHERE id = ?`)
      .bind(nowISO(), order.id)
      .run();
    // The buyer has their money back, so the links stop working.
    await revokeTokensForOrder(env, order.id);
  }

  await issueCreditNoteFor(env, order, Math.abs(event.amountMinor));
  await audit(env, actor, "order.refunded", order.id, {
    number: order.number,
    amount: event.amountMinor,
    full,
  });
  return full ? "refunded" : "refunded-partial";
}

async function issueCreditNoteFor(env: Env, order: OrderRow, amountMinor: number): Promise<void> {
  const original = await env.SHOP_DB.prepare(
    `SELECT * FROM invoices WHERE order_id = ? AND kind = 'invoice'`,
  )
    .bind(order.id)
    .first<InvoiceRow>();
  if (!original) return; // nothing to correct — the order never got an invoice

  const existing = await env.SHOP_DB.prepare(
    `SELECT id FROM invoices WHERE corrects_id = ? AND total_minor = ?`,
  )
    .bind(original.id, -Math.abs(amountMinor))
    .first<{ id: string }>();
  if (existing) return; // this refund already has its credit note

  const items = await getOrderItems(env, order.id);
  const customer = order.customer_id
    ? await env.SHOP_DB.prepare(`SELECT * FROM customers WHERE id = ?`).bind(order.customer_id).first<CustomerRow>()
    : null;
  const { issueCreditNote } = await import("./_invoice");
  await issueCreditNote(env, order, items, customer, original, amountMinor, "");
}

async function recordPayment(
  env: Env,
  order: OrderRow,
  ref: string,
  kind: "charge" | "refund",
  status: "succeeded" | "failed" | "pending",
  amountMinor: number,
  currency: string,
  raw: unknown,
): Promise<void> {
  await env.SHOP_DB.prepare(
    `INSERT OR IGNORE INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, raw_json, created_at)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
  )
    .bind(
      newId(),
      order.id,
      order.gateway ?? "",
      ref,
      kind,
      status,
      Math.abs(amountMinor),
      (currency || order.currency).toUpperCase(),
      JSON.stringify(raw).slice(0, 20000),
      nowISO(),
    )
    .run();
}

function safeParse(value: string | null): Record<string, unknown> | null {
  if (!value) return null;
  try {
    return JSON.parse(value) as Record<string, unknown>;
  } catch {
    return null;
  }
}
