// Who may open an invoice, and the one way it is served.
//
// Two doors lead to the same document: the buyer's link (GET
// /api/shop/invoices/:number?k=…) and the owner's panel (GET
// /api/shop/admin/invoices/:number, behind the admin gate). Both serve exactly
// what invoiceResponse builds, so the copy the owner prints is the copy the
// buyer was sent.
//
// The buyer's `k` is either the order key (the thank-you page has it, and so do
// the ACP/UCP paths) or the invoice's own link key. The second exists because a
// web order is paid by a webhook, and the webhook never holds the order key —
// only its hash is stored. So fulfilment issues a key for the invoice, mails it
// once, and keeps only its hash in invoices.link_hash, the way download tokens
// are kept: a database dump cannot open anyone's invoice.

import type { Env } from "./_env";
import { randomToken, sha256hex, timingSafeEqual } from "./_lib";
import { renderInvoiceHtml } from "./_invoice";
import { orderKeyMatches } from "./_orders";
import type { InvoiceRow, OrderRow } from "./_types";

/** Issues a fresh link key for an invoice and stores its hash. The plaintext is
 *  returned once, for the email, and never stored. A second call replaces the
 *  first key: only the newest mailed link opens the invoice. */
export async function issueInvoiceLinkKey(env: Env, invoiceNumber: string): Promise<string> {
  const key = randomToken(24);
  await env.SHOP_DB.prepare(`UPDATE invoices SET link_hash = ? WHERE number = ?`)
    .bind(await sha256hex(key), invoiceNumber)
    .run();
  return key;
}

/** The buyer's key opens their invoice when it is the order's key or the key
 *  mailed with this invoice. An empty key opens nothing. */
export async function invoiceKeyMatches(invoice: InvoiceRow, order: OrderRow, key: string): Promise<boolean> {
  if (!key) return false;
  if (await orderKeyMatches(order, key)) return true;
  return Boolean(invoice.link_hash) && timingSafeEqual(await sha256hex(key), invoice.link_hash!);
}

/** The path of the buyer's invoice link, relative to the shop. */
export function invoicePath(invoiceNumber: string, key: string): string {
  // Invoice numbers contain slashes by default (FV/2026/000042), which cannot
  // go in a path segment as they are.
  return `/api/shop/invoices/${encodeURIComponent(invoiceNumber)}?k=${encodeURIComponent(key)}`;
}

/** The invoice as a page: the frozen snapshot, never recomputed, with headers
 *  that keep it out of caches and search engines and let it run no script. */
export async function invoiceResponse(env: Env, invoice: InvoiceRow): Promise<Response> {
  const corrects = invoice.corrects_id
    ? await env.SHOP_DB.prepare(`SELECT number FROM invoices WHERE id = ?`)
        .bind(invoice.corrects_id)
        .first<{ number: string }>()
    : null;

  return new Response(renderInvoiceHtml(invoice, corrects?.number), {
    headers: {
      "content-type": "text/html; charset=utf-8",
      "cache-control": "private, no-store",
      "x-robots-tag": "noindex, nofollow",
      "content-security-policy": "default-src 'none'; style-src 'unsafe-inline'; img-src data:",
    },
  });
}

/** The invoice with this number, or null. */
export async function findInvoice(env: Env, invoiceNumber: string): Promise<InvoiceRow | null> {
  return env.SHOP_DB.prepare(`SELECT * FROM invoices WHERE number = ?`).bind(invoiceNumber).first<InvoiceRow>();
}
