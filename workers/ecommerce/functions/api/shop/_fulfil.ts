// The step that turns a payment into a delivered order.
//
// Called by both webhooks and by an owner marking an order paid by hand, so it
// has to be safe to call twice, and safe to resume after being interrupted
// halfway. Every step is guarded by its own "have I already done this":
//
//   1. move the order to paid — the UPDATE names the states it may move FROM,
//      so the second caller changes nothing and stops here
//   2. issue the invoice — returns the existing one if there is one
//   3. issue download tokens — skips lines that already have one
//   4. queue the email, the admin notice, the tracking and any webhooks
//
// D1 has no transaction spanning these, which is exactly why each of them is
// idempotent rather than relying on a rollback that does not exist.

import type { Env } from "./_env";
import { audit } from "./_audit";
import { issueTokens } from "./_downloads";
import { endpointsFor } from "./_hooks";
import { issueInvoice } from "./_invoice";
import { newId, nowISO } from "./_lib";
import { adminNoticeMail, orderPaidMail, type DownloadLink } from "./_mail";
import { currencyDecimals } from "./_money";
import { getOrderItems } from "./_orders";
import { enqueue } from "./_outbox";
import { allSettings, getSettingString } from "./_settings";
import { enabledTrackers } from "./_tracking";
import type { CustomerRow, OrderRow, ProductRow } from "./_types";

export interface FulfilContext {
  /** Absolute site origin, e.g. https://example.com — used to build the links
   *  that go in the email. Taken from the request, so a preview deployment
   *  mails preview links. */
  origin: string;
  actor: string;
  /** The plaintext order key, when the caller has it (the webhook does not).
   *  Without it the email carries download links but no invoice link, because
   *  the invoice needs the key. */
  orderKey?: string;
}

export interface FulfilResult {
  changed: boolean;
  invoiceNumber?: string;
  tokensIssued: number;
  queued: number;
}

/** Moves an order to paid and does everything that follows. */
export async function fulfilOrder(env: Env, order: OrderRow, ctx: FulfilContext): Promise<FulfilResult> {
  // Step 1. The set of states we may move from is in the statement itself, so
  // two webhooks racing produce exactly one winner.
  const moved = await env.SHOP_DB.prepare(
    `UPDATE orders SET status = 'paid', paid_at = COALESCE(paid_at, ?), updated_at = ?
      WHERE id = ? AND status IN ('pending', 'needs_review')`,
  )
    .bind(nowISO(), nowISO(), order.id)
    .run();
  const changed = (moved.meta?.changes ?? 0) > 0;

  // A second caller still gets a truthful answer about the order, but does no
  // work and queues no second email.
  if (!changed) {
    const current = await env.SHOP_DB.prepare(`SELECT status FROM orders WHERE id = ?`)
      .bind(order.id)
      .first<{ status: string }>();
    if (current && !["paid", "fulfilled"].includes(current.status)) {
      return { changed: false, tokensIssued: 0, queued: 0 };
    }
  }

  const fresh = (await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(order.id).first<OrderRow>())!;
  const items = await getOrderItems(env, order.id);
  const customer = fresh.customer_id
    ? await env.SHOP_DB.prepare(`SELECT * FROM customers WHERE id = ?`).bind(fresh.customer_id).first<CustomerRow>()
    : null;

  // Step 2. The invoice, with the note explaining the tax treatment frozen in.
  const { taxNote } = await import("./_tax");
  const note = taxNote(
    { note: fresh.reverse_charge === 1 ? "reverse_charge" : "standard" },
    fresh.locale ?? "en",
  );
  const invoice = await issueInvoice(env, fresh, items, customer, note);

  // Step 3. Download tokens for every line that has a file behind it.
  const products = new Map<string, ProductRow>();
  for (const item of items) {
    const p = await env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`)
      .bind(item.product_id)
      .first<ProductRow>();
    if (p) products.set(item.product_id, p);
  }
  const tokens = await issueTokens(env, items, products);

  // Step 4. Everything that leaves the shop, queued rather than sent, so a slow
  // provider cannot turn a successful payment into a failed webhook response.
  let queued = 0;
  if (changed) {
    queued = await queueOutgoing(env, fresh, customer, invoice.number, tokens.map((t) => ({
      name: t.productName,
      url: `${ctx.origin}/api/shop/download/${t.token}`,
      expiresAt: t.expiresAt,
      maxUses: t.maxUses,
    })), items, ctx);
    await audit(env, ctx.actor, "order.paid", fresh.id, {
      number: fresh.number,
      total: fresh.total_minor,
      currency: fresh.currency,
      invoice: invoice.number,
    });
  }

  return { changed, invoiceNumber: invoice.number, tokensIssued: tokens.length, queued };
}

async function queueOutgoing(
  env: Env,
  order: OrderRow,
  customer: CustomerRow | null,
  invoiceNumber: string,
  downloads: DownloadLink[],
  items: Array<{ sku: string; name: string; quantity: number; total_minor: number }>,
  ctx: FulfilContext,
): Promise<number> {
  const settings = await allSettings(env);
  const shopName = String(settings["shop.name"] ?? "Shop");
  const sellerEmail = String(settings["seller.email"] ?? "");
  let queued = 0;

  if (customer?.email) {
    const mail = orderPaidMail({
      locale: order.locale ?? "en",
      shopName,
      orderNumber: order.number,
      totalMinor: order.total_minor,
      currency: order.currency,
      downloads,
      invoiceUrl: ctx.orderKey
        ? `${ctx.origin}/api/shop/invoices/${invoiceNumberToPath(invoiceNumber)}?k=${encodeURIComponent(ctx.orderKey)}`
        : undefined,
      resendUrl: `${ctx.origin}/shop/resend/`,
      replyTo: sellerEmail || undefined,
    });
    await enqueue(env, "email", customer.email, mail as unknown as Record<string, unknown>);
    queued++;
  }

  if (env.SHOP_MAIL_ADMIN) {
    const notice = adminNoticeMail("paid", {
      orderNumber: order.number,
      totalMinor: order.total_minor,
      currency: order.currency,
      country: order.tax_country,
      gateway: order.gateway,
      panelUrl: `${ctx.origin}/ecommerce-admin/`,
    });
    await enqueue(env, "email", env.SHOP_MAIL_ADMIN, notice as unknown as Record<string, unknown>);
    queued++;
  }

  // Tracking runs only with the buyer's marketing consent, recorded on the
  // order at checkout (S-16 in the plan, GDPR in practice).
  if (order.consent_marketing === 1 && customer?.email) {
    const factor = 10 ** currencyDecimals(order.currency);
    const payload = {
      orderId: order.id,
      orderNumber: order.number,
      currency: order.currency,
      valueMinor: order.total_minor,
      taxMinor: order.tax_minor,
      value: order.total_minor / factor,
      tax: order.tax_minor / factor,
      email: customer.email,
      clientId: order.id,
      items: items.map((i) => ({
        sku: i.sku,
        name: i.name,
        quantity: i.quantity,
        price: i.total_minor / i.quantity / factor,
      })),
    };
    for (const target of enabledTrackers(env)) {
      await enqueue(env, "tracking", target, payload);
      queued++;
    }
  }

  for (const endpoint of await endpointsFor(env, "order.paid")) {
    await enqueue(env, "webhook", endpoint.url, {
      id: newId(),
      type: "order.paid",
      createdAt: nowISO(),
      secret: endpoint.secret,
      data: {
        order: {
          id: order.id,
          number: order.number,
          currency: order.currency,
          totalMinor: order.total_minor,
          taxMinor: order.tax_minor,
          country: order.tax_country,
        },
        items: items.map((i) => ({ sku: i.sku, name: i.name, quantity: i.quantity })),
        customer: customer ? { email: customer.email, country: customer.country } : null,
      },
    });
    queued++;
  }

  return queued;
}

/** Invoice numbers contain slashes by default (FV/2026/000042), which cannot go
 *  in a path segment as they are. */
export function invoiceNumberToPath(number: string): string {
  return encodeURIComponent(number);
}

/** When the money arrived but something about it does not add up, the order
 *  stops here and a human decides. Never silently paid, never silently lost. */
export async function markNeedsReview(
  env: Env,
  order: OrderRow,
  reason: string,
  ctx: { origin: string; actor: string },
): Promise<void> {
  await env.SHOP_DB.prepare(
    `UPDATE orders SET status = 'needs_review', updated_at = ? WHERE id = ? AND status = 'pending'`,
  )
    .bind(nowISO(), order.id)
    .run();
  await audit(env, ctx.actor, "order.needs_review", order.id, { reason, number: order.number });

  if (env.SHOP_MAIL_ADMIN) {
    const notice = adminNoticeMail("needs_review", {
      orderNumber: order.number,
      totalMinor: order.total_minor,
      currency: order.currency,
      country: order.tax_country,
      gateway: order.gateway,
      reason,
      panelUrl: `${ctx.origin}/ecommerce-admin/`,
    });
    await enqueue(env, "email", env.SHOP_MAIL_ADMIN, notice as unknown as Record<string, unknown>);
  }

  for (const endpoint of await endpointsFor(env, "order.needs_review")) {
    await enqueue(env, "webhook", endpoint.url, {
      id: newId(),
      type: "order.needs_review",
      createdAt: nowISO(),
      secret: endpoint.secret,
      data: { order: { id: order.id, number: order.number }, reason },
    });
  }
}

/** The shop's public origin, preferring the configured one so emails sent from
 *  a preview deployment can still point at the real site when the owner wants
 *  that. */
export async function shopOrigin(env: Env, request: Request): Promise<string> {
  const configured = await getSettingString(env, "shop.url");
  if (configured) return configured.replace(/\/+$/, "");
  return new URL(request.url).origin;
}
