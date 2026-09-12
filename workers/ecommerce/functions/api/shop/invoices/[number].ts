// GET /api/shop/invoices/:number?k=… — the buyer's copy of their invoice.
//
// Served as HTML, from the snapshot frozen when it was issued. Nothing is
// recomputed: if the seller has since changed their address or a price, this
// document still says what it said on the day.

import type { Env } from "../_env";
import { fail } from "../_lib";
import { renderInvoiceHtml } from "../_invoice";
import { getOrder, orderKeyMatches } from "../_orders";
import { ensureSchema } from "../_schema";
import type { InvoiceRow } from "../_types";

export const onRequestGet: PagesFunction<Env> = async ({ request, env, params }) => {
  if (!env.SHOP_DB) return fail(request, "not_configured", "The shop database is not bound.", 503);
  await ensureSchema(env);

  const number = decodeURIComponent(String(params.number ?? ""));
  const key = new URL(request.url).searchParams.get("k") ?? "";

  const invoice = await env.SHOP_DB.prepare(`SELECT * FROM invoices WHERE number = ?`)
    .bind(number)
    .first<InvoiceRow>();
  if (!invoice) return fail(request, "not_found", "No such invoice.", 404);

  const order = await getOrder(env, invoice.order_id);
  // Knowing an invoice number is not authorisation: the order key that came
  // with the purchase is.
  if (!order || !(await orderKeyMatches(order, key))) {
    return fail(request, "not_found", "No invoice matches that link.", 404);
  }

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
};
