// A shop in which every number and every attention item of the module summary
// (GO-156) shows, with recognisable personal data in every table a summary
// could leak from. Used by summary.test.ts and summary-sample.ts.
//
// What it holds, relative to `now`:
//   orders   o1 paid 3 days ago, EUR 24.00, its download link issued
//            o2 paid 2 days ago, EUR 19.00, no download link (undelivered)
//            o3 paid 10 days ago, EUR 24.00, refunded in full 5 days ago
//            o4 needs review · o5 pending · o6 paid 40 days ago (outside)
//            o7 paid 1 day ago, USD 10.00 (another currency), link issued
//   payments the EUR 24.00 refund and a USD 5.00 one
//   products two on sale (one with a file), one draft
//   outbox   one message tried 5 times, one delivered
//   webhooks one failed 2 days ago, one failed 10 days ago, one applied
//   VAT      Germany's ebook rate ends in 10 days with nothing after it
import type { Env } from "../functions/api/shop/_env";
import { ensureSchema } from "../functions/api/shop/_schema";
import { putSettings } from "../functions/api/shop/_settings";

// PERSONAL: values that must never appear in a summary.
export const PERSONAL = ["Zofia Wiśniewska", "zofia.wisniewska@example.com", "ul. Lipowa 7, 00-950 Warszawa", "PL5250001009", "203.0.113.77", "Mozilla/5.0 (Zofia)"];

const DAY = 86_400_000;

export async function seedSummaryShop(env: Env, now = Date.now(), settings: Record<string, unknown> = {}): Promise<void> {
  await ensureSchema(env);
  const at = (days: number) => new Date(now - days * DAY).toISOString();
  const db = env.SHOP_DB;
  const run = (sql: string, ...values: unknown[]) => db.prepare(sql).bind(...values).run();
  const [name, email, address, vatId, ip, agent] = PERSONAL;

  await putSettings(env, { "seller.name": "Example Press", "seller.address": address, "seller.country": "PL", ...settings });
  await run(`INSERT INTO customers (id, email, email_lc, name, vat_id, vat_id_valid, country, created_at) VALUES ('c1', ?, ?, ?, ?, 0, 'PL', ?)`, email, email, name, vatId, at(40));

  const product = `INSERT INTO products (id, sku, name, status, file_key, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`;
  await run(product, "p1", "BOOK-1", "A Book", "active", "products/p1/book.pdf", at(60), at(60));
  await run(product, "p2", "BOOK-2", "Notes", "active", null, at(60), at(60));
  await run(product, "p3", "BOOK-3", "Draft", "draft", null, at(60), at(60));

  const order = `INSERT INTO orders (id, number, key_hash, status, customer_id, currency, subtotal_minor, tax_minor, total_minor,
                   tax_country, ip_hash, user_agent, notes, created_at, paid_at, updated_at)
                 VALUES (?, ?, 'k', ?, 'c1', ?, ?, 0, ?, 'PL', ?, ?, ?, ?, ?, ?)`;
  const orders: [string, string, string, number, number | null][] = [
    ["o1", "paid", "EUR", 2400, 3],
    ["o2", "paid", "EUR", 1900, 2],
    ["o3", "refunded", "EUR", 2400, 10],
    ["o4", "needs_review", "EUR", 2400, null],
    ["o5", "pending", "EUR", 2400, null],
    ["o6", "paid", "EUR", 2400, 40],
    ["o7", "paid", "USD", 1000, 1],
  ];
  for (const [id, status, currency, total, paidDaysAgo] of orders) {
    const paid = paidDaysAgo === null ? null : at(paidDaysAgo);
    await run(order, id, `O-${id}`, status, currency, total, total, ip, agent, `Call ${name} at ${email}`, paid ?? at(1), paid, paid ?? at(1));
    await run(
      `INSERT INTO order_items (id, order_id, product_id, sku, name, quantity, unit_minor, tax_rate_bp, tax_minor, total_minor) VALUES (?, ?, 'p1', 'BOOK-1', 'A Book', 1, ?, 500, 0, ?)`,
      `i-${id}`, id, total, total,
    );
  }
  const token = `INSERT INTO download_tokens (token_hash, order_item_id, expires_at, max_uses, created_at) VALUES (?, ?, ?, 5, ?)`;
  for (const id of ["o1", "o3", "o6", "o7"]) await run(token, `t-${id}`, `i-${id}`, at(-30), at(1));

  const payment = `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, raw_json, created_at) VALUES (?, ?, 'stripe', ?, ?, 'succeeded', ?, ?, ?, ?)`;
  await run(payment, "pay-1", "o3", "ch_1", "charge", 2400, "EUR", JSON.stringify({ email }), at(10));
  await run(payment, "pay-2", "o3", "re_1", "refund", -2400, "EUR", JSON.stringify({ email }), at(5));
  await run(payment, "pay-3", "o7", "re_2", "refund", -500, "USD", null, at(1));

  const outbox = `INSERT INTO outbox (id, kind, target, payload_json, attempts, next_attempt, last_error, done_at, created_at) VALUES (?, 'email', ?, ?, ?, ?, ?, ?, ?)`;
  await run(outbox, "m1", email, JSON.stringify({ to: email, name }), 5, at(-1), `550 mailbox ${email} unavailable`, null, at(2));
  await run(outbox, "m2", email, JSON.stringify({ to: email }), 1, at(3), null, at(3), at(3));

  const hook = `INSERT INTO webhook_inbox (gateway, event_id, event_type, received_at, processed_at, result) VALUES ('stripe', ?, 'checkout.session.completed', ?, ?, ?)`;
  await run(hook, "evt_1", at(2), at(2), `error:no order for ${email}`);
  await run(hook, "evt_2", at(10), at(10), "error:old");
  await run(hook, "evt_3", at(1), at(1), "fulfilled");

  await run(`UPDATE vat_rates SET valid_to = ? WHERE country = 'DE' AND kind = 'ebook'`, at(-10));
}
