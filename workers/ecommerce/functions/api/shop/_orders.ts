// Building an order: pricing a basket, writing it down, finding it again.
//
// The prices here are the only prices that matter. What the browser thinks a
// product costs is a display detail; the buyer sends SKUs and quantities, and
// this module asks the database (S-04).

import type { Env } from "./_env";
import { newId, nowISO, randomToken, sha256hex, timingSafeEqual } from "./_lib";
import { getSetting } from "./_settings";
import { decideTax, priceLine, resolveTaxCountry, type TaxContext, type TaxEvidence } from "./_tax";
import type { CustomerRow, OrderItemRow, OrderRow, ProductRow } from "./_types";

export interface BasketLine {
  sku: string;
  quantity: number;
}

export interface PricedBasket {
  currency: string;
  items: Array<Omit<OrderItemRow, "id" | "order_id">>;
  subtotalMinor: number;
  taxMinor: number;
  totalMinor: number;
  taxCountry: string | null;
  reverseCharge: boolean;
  evidence: TaxEvidence;
  products: ProductRow[];
}

export type PricingError =
  | { error: "empty_basket" }
  | { error: "unknown_sku"; sku: string }
  | { error: "product_unavailable"; sku: string }
  | { error: "price_not_available"; sku: string; currency: string }
  | { error: "country_required" }
  | { error: "tax_rate_missing"; country: string }
  | { error: "country_not_served"; country: string };

/** Prices a basket end to end: looks up each product, finds its price in the
 *  requested currency, decides the tax, and totals it.
 *
 *  Everything it needs from the buyer is a list of SKUs, a currency and a
 *  country. Everything else comes from the database. */
export async function priceBasket(
  env: Env,
  ctx: TaxContext,
  opts: {
    lines: BasketLine[];
    currency: string;
    billingCountry: string | null;
    ipCountry: string | null;
    vatId: string;
    vatIdValid: boolean;
  },
): Promise<PricedBasket | PricingError> {
  if (opts.lines.length === 0) return { error: "empty_basket" };

  const currency = opts.currency.toUpperCase();
  const { country, evidence } = resolveTaxCountry(opts.billingCountry, opts.ipCountry);

  // An empty allow-list means "anywhere the tax rules can price"; a non-empty
  // one is the seller deliberately narrowing where they sell.
  const allowedList = await allowedCountries(env);
  if (country && allowedList.length > 0 && !allowedList.includes(country)) {
    return { error: "country_not_served", country };
  }

  const at = nowISO();
  const items: Array<Omit<OrderItemRow, "id" | "order_id">> = [];
  const products: ProductRow[] = [];
  let subtotal = 0;
  let tax = 0;
  let total = 0;
  let reverseCharge = false;
  let taxCountry: string | null = country;

  for (const line of opts.lines) {
    const product = await env.SHOP_DB.prepare(`SELECT * FROM products WHERE sku = ?`)
      .bind(line.sku)
      .first<ProductRow>();
    if (!product) return { error: "unknown_sku", sku: line.sku };
    if (product.status !== "active") return { error: "product_unavailable", sku: line.sku };
    // Physical goods need shipping, addresses and a different place-of-supply
    // rule. The module that adds them is planned; until then the shop says so
    // rather than selling something it cannot deliver.
    if (product.kind !== "digital") return { error: "product_unavailable", sku: line.sku };

    const price = await env.SHOP_DB.prepare(
      `SELECT amount_minor FROM prices WHERE product_id = ? AND currency = ?`,
    )
      .bind(product.id, currency)
      .first<{ amount_minor: number }>();
    if (!price) return { error: "price_not_available", sku: line.sku, currency };

    const decision = await decideTax(env, ctx, {
      country,
      vatId: opts.vatId,
      vatIdValid: opts.vatIdValid,
      taxCategory: product.tax_category,
      atISO: at,
    });
    if ("error" in decision) {
      if (decision.error === "country_required") return { error: "country_required" };
      return { error: "tax_rate_missing", country: country ?? "" };
    }
    reverseCharge = reverseCharge || decision.reverseCharge;
    taxCountry = decision.country ?? taxCountry;

    const priced = priceLine(price.amount_minor, line.quantity, decision.rateBp, ctx.pricingMode);
    items.push({
      product_id: product.id,
      sku: product.sku,
      name: product.name,
      quantity: line.quantity,
      unit_minor: priced.unitMinor,
      tax_rate_bp: priced.taxRateBp,
      tax_minor: priced.taxMinor,
      total_minor: priced.totalMinor,
    });
    products.push(product);
    subtotal += priced.unitMinor * line.quantity;
    tax += priced.taxMinor;
    total += priced.totalMinor;
  }

  return {
    currency,
    items,
    subtotalMinor: subtotal,
    taxMinor: tax,
    totalMinor: total,
    taxCountry,
    reverseCharge,
    evidence,
    products,
  };
}

/** The countries the shop sells to, however the setting was written: a JSON
 *  array from the panel, or a comma-separated string typed by hand. */
export async function allowedCountries(env: Env): Promise<string[]> {
  const raw = await getSetting(env, "shop.countries_allowed");
  const list = Array.isArray(raw)
    ? raw.map(String)
    : typeof raw === "string" && raw
      ? raw.split(",")
      : [];
  return list.map((c) => c.trim().toUpperCase()).filter(Boolean);
}

/** Finds an existing customer by email or creates one. The email is the
 *  identity: there are no accounts, and a buyer who returns should not become a
 *  second person in the database. */
export async function upsertCustomer(
  env: Env,
  data: { email: string; name: string; vatId: string; vatIdValid: boolean | null; country: string | null },
): Promise<CustomerRow> {
  const emailLc = data.email.toLowerCase();
  const existing = await env.SHOP_DB.prepare(`SELECT * FROM customers WHERE email_lc = ?`)
    .bind(emailLc)
    .first<CustomerRow>();

  if (existing) {
    await env.SHOP_DB.prepare(
      `UPDATE customers SET name = COALESCE(NULLIF(?, ''), name),
                            vat_id = COALESCE(NULLIF(?, ''), vat_id),
                            vat_id_valid = COALESCE(?, vat_id_valid),
                            country = COALESCE(NULLIF(?, ''), country)
        WHERE id = ?`,
    )
      .bind(data.name, data.vatId, data.vatIdValid === null ? null : data.vatIdValid ? 1 : 0, data.country ?? "", existing.id)
      .run();
    return (await env.SHOP_DB.prepare(`SELECT * FROM customers WHERE id = ?`).bind(existing.id).first<CustomerRow>())!;
  }

  const row: CustomerRow = {
    id: newId(),
    email: data.email,
    email_lc: emailLc,
    name: data.name || null,
    vat_id: data.vatId || null,
    vat_id_valid: data.vatIdValid === null ? null : data.vatIdValid ? 1 : 0,
    country: data.country,
    created_at: nowISO(),
  };
  await env.SHOP_DB.prepare(
    `INSERT INTO customers (id, email, email_lc, name, vat_id, vat_id_valid, country, created_at)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
  )
    .bind(row.id, row.email, row.email_lc, row.name, row.vat_id, row.vat_id_valid, row.country, row.created_at)
    .run();
  return row;
}

/** Human-facing order numbers, separate from invoice numbers: an order number
 *  may be skipped or abandoned, an invoice number may not (see _invoice). */
export async function nextOrderNumber(env: Env): Promise<string> {
  const year = new Date().getUTCFullYear();
  const row = await env.SHOP_DB.prepare(
    `SELECT COUNT(*) AS n FROM orders WHERE number LIKE ?`,
  )
    .bind(`O-${year}-%`)
    .first<{ n: number }>();
  const n = (row?.n ?? 0) + 1;
  return `O-${year}-${String(n).padStart(6, "0")}`;
}

export interface CreatedOrder {
  order: OrderRow;
  items: OrderItemRow[];
  /** The plaintext key. Stored only as a hash; this copy goes to the buyer. */
  orderKey: string;
}

export async function createOrder(
  env: Env,
  priced: PricedBasket,
  customer: CustomerRow,
  meta: {
    gateway: string;
    locale: string;
    consentMarketing: boolean;
    consentWaiver: boolean;
    ipHash: string | null;
    userAgent: string;
  },
): Promise<CreatedOrder> {
  const orderKey = randomToken(24);
  const keyHash = await sha256hex(orderKey);
  const now = nowISO();
  const order: OrderRow = {
    id: newId(),
    number: await nextOrderNumber(env),
    key_hash: keyHash,
    status: "pending",
    customer_id: customer.id,
    currency: priced.currency,
    subtotal_minor: priced.subtotalMinor,
    tax_minor: priced.taxMinor,
    total_minor: priced.totalMinor,
    tax_country: priced.taxCountry,
    tax_evidence: JSON.stringify(priced.evidence),
    reverse_charge: priced.reverseCharge ? 1 : 0,
    gateway: meta.gateway,
    gateway_ref: null,
    consent_marketing: meta.consentMarketing ? 1 : 0,
    consent_waiver: meta.consentWaiver ? 1 : 0,
    ip_hash: meta.ipHash,
    user_agent: meta.userAgent,
    locale: meta.locale,
    notes: null,
    created_at: now,
    paid_at: null,
    updated_at: now,
  };

  const items: OrderItemRow[] = priced.items.map((i) => ({ ...i, id: newId(), order_id: order.id }));

  await env.SHOP_DB.batch([
    env.SHOP_DB.prepare(
      `INSERT INTO orders (id, number, key_hash, status, customer_id, currency, subtotal_minor, tax_minor,
                           total_minor, tax_country, tax_evidence, reverse_charge, gateway, gateway_ref,
                           consent_marketing, consent_waiver, ip_hash, user_agent, locale, notes,
                           created_at, paid_at, updated_at)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    ).bind(
      order.id, order.number, order.key_hash, order.status, order.customer_id, order.currency,
      order.subtotal_minor, order.tax_minor, order.total_minor, order.tax_country, order.tax_evidence,
      order.reverse_charge, order.gateway, order.gateway_ref, order.consent_marketing, order.consent_waiver,
      order.ip_hash, order.user_agent, order.locale, order.notes, order.created_at, order.paid_at, order.updated_at,
    ),
    ...items.map((i) =>
      env.SHOP_DB.prepare(
        `INSERT INTO order_items (id, order_id, product_id, sku, name, quantity, unit_minor, tax_rate_bp, tax_minor, total_minor)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      ).bind(i.id, i.order_id, i.product_id, i.sku, i.name, i.quantity, i.unit_minor, i.tax_rate_bp, i.tax_minor, i.total_minor),
    ),
  ]);

  return { order, items, orderKey };
}

export async function getOrder(env: Env, id: string): Promise<OrderRow | null> {
  return env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(id).first<OrderRow>();
}

export async function getOrderItems(env: Env, orderId: string): Promise<OrderItemRow[]> {
  const { results } = await env.SHOP_DB.prepare(
    `SELECT * FROM order_items WHERE order_id = ? ORDER BY rowid`,
  )
    .bind(orderId)
    .all<OrderItemRow>();
  return results ?? [];
}

export async function findOrderByGatewayRef(env: Env, gateway: string, ref: string): Promise<OrderRow | null> {
  return env.SHOP_DB.prepare(`SELECT * FROM orders WHERE gateway = ? AND gateway_ref = ?`)
    .bind(gateway, ref)
    .first<OrderRow>();
}

/** Checks the key a buyer presents for their own order. Knowing the UUID is not
 *  enough: status and invoice reads need the key too, so a guessed or leaked id
 *  reveals nothing. */
export async function orderKeyMatches(order: OrderRow, key: string): Promise<boolean> {
  if (!key) return false;
  return timingSafeEqual(await sha256hex(key), order.key_hash);
}
