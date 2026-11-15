// The demo shop the local dev server (dev-server.mjs) shows: six products,
// sixty orders over the last 45 days in every state and every channel (web,
// ACP, UCP), with real invoices,
// credit notes and download links made by the shop's own fulfilment code, a
// few refunds, failed payment notifications and an outbox where some messages
// keep failing. Every person is invented and every address is @example.com.
//
// Deterministic: a fixed pseudo-random sequence and dates counted back from
// today 00:00 UTC, so two runs on one day draw the same screens — which is
// what before/after screenshots compare.
import type { Env } from "../functions/api/shop/_env";
import { fulfilOrder } from "../functions/api/shop/_fulfil";
import { issueCreditNote } from "../functions/api/shop/_invoice";
import { netFromGross } from "../functions/api/shop/_money";
import { ensureSchema } from "../functions/api/shop/_schema";
import { putSettings } from "../functions/api/shop/_settings";
import type { InvoiceRow, OrderItemRow, OrderRow } from "../functions/api/shop/_types";

const DAY = 86_400_000;
const BOOKS: [string, string, number, string][] = [
  ["EBOOK-SHOP", "The One-Person Shop", 2400, "active"],
  ["EBOOK-10K", "Ten Thousand Words a Month", 1900, "active"],
  ["EBOOK-VAT", "VAT Without Tears", 1500, "active"],
  ["EBOOK-INV", "Invoices That Survive an Audit", 1200, "active"],
  ["EBOOK-PRICE", "Pricing a Book You Wrote", 900, "active"],
  ["EBOOK-DRAFT", "Selling Courses (draft)", 4900, "draft"],
];
const PEOPLE = ["Ada Nowak", "Bruno Keller", "Camille Martin", "Dara Byrne", "Elena Rossi", "Femke de Vries", "Gosia Lis", "Hugo Laurent", "Ines Garcia", "Jonas Weber", "Kasia Zielińska", "Liam Walsh", "Marta Kowalczyk", "Noah Fischer", "Olivia Brown", "Piotr Mazur"];
const COUNTRIES = ["PL", "DE", "FR", "IE", "IT", "NL", "PL", "DE", "FR", "ES", "PL", "IE", "PL", "DE", "GB", "PL"];
// Sixty orders, oldest first: mostly paid; the rest every other state.
const STATES = "pppppcpppfpprpppnppppcppppppfpppprppppppcpppppppnppppppppppp";

export async function seedDemoShop(env: Env): Promise<void> {
  await ensureSchema(env);
  await putSettings(env, {
    "seller.name": "Paperless Press sp. z o.o.", "seller.address": "ul. Przykładowa 1\n00-001 Warszawa", "seller.country": "PL",
    "seller.vat_id": "PL0000000000", "seller.email": "help@example.com", "shop.name": "Paperless Press", "shop.url": "https://example.com",
    "currency.base": "EUR", "pricing.mode": "gross", "tax.mode": "table",
  });
  const db = env.SHOP_DB;
  const run = (sql: string, ...v: unknown[]) => db.prepare(sql).bind(...v).run();
  const today = Math.floor(Date.now() / DAY) * DAY;
  const iso = (t: number) => new Date(t).toISOString();
  let seed = 7;
  const rand = (n: number) => ((seed = (seed * 48271) % 2147483647) % n);

  for (const [i, [sku, name, price, status]] of BOOKS.entries()) {
    const id = `prod-${i + 1}`;
    const key = `products/${id}/${sku.toLowerCase()}.pdf`;
    await run(
      `INSERT INTO products (id, sku, name, description, status, file_key, file_name, file_size, file_sha256, image_url, created_at, updated_at)
       VALUES (?, ?, ?, 'PDF and EPUB in one purchase.', ?, ?, ?, 11, ?, NULL, ?, ?)`,
      id, sku, name, status, status === "active" ? key : null, status === "active" ? `${sku.toLowerCase()}.pdf` : null, "0".repeat(64), iso(today - 60 * DAY), iso(today - 60 * DAY),
    );
    await run(`INSERT INTO prices (product_id, currency, amount_minor) VALUES (?, 'EUR', ?)`, id, price);
    if (status === "active") await env.SHOP_FILES!.put(key, new TextEncoder().encode("%PDF-1.4\ndemo\n"));
  }
  for (const [i, name] of PEOPLE.entries()) {
    const email = `${name.split(" ")[0]!.toLowerCase()}@example.com`;
    await run(`INSERT INTO customers (id, email, email_lc, name, country, created_at) VALUES (?, ?, ?, ?, ?, ?)`, `cust-${i}`, email, email, name, COUNTRIES[i], iso(today - 50 * DAY));
  }

  for (let n = 0; n < STATES.length; n++) {
    const at = today - Math.round(((STATES.length - n) * 45 * DAY) / STATES.length) + (n * 7 % 14 + 8) * 3_600_000;
    const who = rand(PEOPLE.length);
    const country = COUNTRIES[who]!;
    const rate = (await db.prepare(`SELECT rate_bp FROM vat_rates WHERE country = ? AND kind = 'ebook'`).bind(country).first<{ rate_bp: number }>())!.rate_bp;
    const lines = [rand(5), ...(rand(4) === 0 ? [rand(5)] : [])].filter((b, i, all) => all.indexOf(b) === i);
    const id = `ord-${String(n + 1).padStart(3, "0")}`;
    const gross = lines.reduce((sum, b) => sum + BOOKS[b]![2], 0);
    const tax = gross - netFromGross(gross, rate);
    // Every channel the panel can tell apart (GO-165): most orders from the
    // storefront, some through an agent platform (ACP), a few from Google's
    // agentic checkout (UCP). Agent orders pay through Stripe.
    const channel = n % 5 === 1 ? "acp" : n % 7 === 3 ? "ucp" : "web";
    const byPayPal = rand(3) === 0; // drawn for every order, so the sequence is as before
    const gateway = byPayPal && channel === "web" ? "paypal" : "stripe";
    await run(
      `INSERT INTO orders (id, number, key_hash, status, customer_id, currency, subtotal_minor, tax_minor, total_minor, tax_country, gateway, gateway_ref, locale, created_at, updated_at, channel)
       VALUES (?, ?, 'demo', 'pending', ?, 'EUR', ?, ?, ?, ?, ?, ?, 'en', ?, ?, ?)`,
      id, `O-2026-${String(n + 1).padStart(6, "0")}`, `cust-${who}`, gross - tax, tax, gross, country, gateway, `${gateway}-ref-${n + 1}`, iso(at), iso(at), channel,
    );
    for (const b of lines) {
      const [sku, name, price] = BOOKS[b]!;
      const lineTax = price - netFromGross(price, rate);
      await run(
        `INSERT INTO order_items (id, order_id, product_id, sku, name, quantity, unit_minor, tax_rate_bp, tax_minor, total_minor) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, ?)`,
        `${id}-${b}`, id, `prod-${b + 1}`, sku, name, price - lineTax, rate, lineTax, price,
      );
    }
    await settle(env, id, STATES[n]!, at, gross);
  }

  // The outbox: delivered, except three messages that keep failing. None is
  // due, so the panel's background drain leaves the demo as it is.
  await run(`UPDATE outbox SET done_at = created_at, attempts = 1`);
  await run(`UPDATE outbox SET done_at = NULL, attempts = 5, last_error = 'mail_not_configured', next_attempt = ? WHERE id IN (SELECT id FROM outbox ORDER BY created_at DESC LIMIT 3)`, iso(today + 30 * DAY));
  // Two payment notifications this week could not be applied.
  for (const [i, days] of [[1, 2], [2, 5]] as const) {
    await run(`INSERT INTO webhook_inbox (gateway, event_id, event_type, received_at, processed_at, result) VALUES ('stripe', ?, 'checkout.session.completed', ?, ?, 'error:no order matches this session')`, `evt_demo_${i}`, iso(today - days * DAY), iso(today - days * DAY));
  }
}

// settle moves one order to its demo state through the shop's own code, then
// dates everything it wrote to the order's day.
async function settle(env: Env, id: string, state: string, at: number, gross: number): Promise<void> {
  const db = env.SHOP_DB;
  const run = (sql: string, ...v: unknown[]) => db.prepare(sql).bind(...v).run();
  const iso = (t: number) => new Date(t).toISOString();
  const order = () => db.prepare(`SELECT * FROM orders WHERE id = ?`).bind(id).first<OrderRow>() as Promise<OrderRow>;
  if (state === "c") return void (await run(`UPDATE orders SET status = 'cancelled' WHERE id = ?`, id));
  if (state === "f") return void (await run(`UPDATE orders SET status = 'failed' WHERE id = ?`, id));
  if (state === "n") return void (await run(`UPDATE orders SET status = 'needs_review', notes = 'Paid amount differs from the order total.' WHERE id = ?`, id));
  if (at > Date.now() - 2 * DAY && state === "p" && id.endsWith("9")) return; // a recent one still pending
  await fulfilOrder(env, await order(), { origin: "https://example.com", actor: "demo" });
  const paidAt = iso(at + 90_000);
  await run(`UPDATE orders SET paid_at = ?, updated_at = ? WHERE id = ?`, paidAt, paidAt, id);
  await run(`UPDATE invoices SET issued_at = ?, created_at = ? WHERE order_id = ?`, paidAt, paidAt, id);
  await run(`UPDATE outbox SET created_at = ?, next_attempt = ? WHERE created_at > ?`, paidAt, paidAt, iso(Date.now() - 60_000));
  await run(`UPDATE audit_log SET created_at = ? WHERE subject = ?`, paidAt, id);
  const fresh = await order();
  await run(
    `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, created_at) VALUES (?, ?, ?, ?, 'charge', 'succeeded', ?, 'EUR', ?)`,
    `pay-${id}`, id, fresh.gateway, fresh.gateway_ref, gross, paidAt,
  );
  if (state !== "r") return;
  const refundedAt = iso(at + 2 * DAY);
  await run(
    `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, created_at) VALUES (?, ?, ?, ?, 'refund', 'succeeded', ?, 'EUR', ?)`,
    `ref-${id}`, id, fresh.gateway, `${fresh.gateway_ref}-refund`, -gross, refundedAt,
  );
  const items = (await db.prepare(`SELECT * FROM order_items WHERE order_id = ?`).bind(id).all<OrderItemRow>()).results;
  const original = (await db.prepare(`SELECT * FROM invoices WHERE order_id = ? AND kind = 'invoice'`).bind(id).first<InvoiceRow>())!;
  const customer = await db.prepare(`SELECT * FROM customers WHERE id = ?`).bind(fresh.customer_id).first();
  await issueCreditNote(env, fresh, items, customer as never, original, gross, "Refunded at the buyer's request.");
  await run(`UPDATE invoices SET issued_at = ?, created_at = ? WHERE corrects_id = ?`, refundedAt, refundedAt, original.id);
  await run(`UPDATE orders SET status = 'refunded', updated_at = ? WHERE id = ?`, refundedAt, id);
  await run(`UPDATE download_tokens SET revoked = 1 WHERE order_item_id IN (SELECT id FROM order_items WHERE order_id = ?)`, id);
}
