// GET /api/shop/invoices/:number?k=… — the buyer's copy of their invoice.
//
// Served as HTML, from the snapshot frozen when it was issued. Nothing is
// recomputed: if the seller has since changed their address or a price, this
// document still says what it said on the day. The owner's copy is the same
// page behind the admin gate (admin/invoices/[number].ts).

import type { Env } from "../_env";
import { fail } from "../_lib";
import { findInvoice, invoiceKeyMatches, invoiceResponse } from "../_invoice-page";
import { getOrder } from "../_orders";
import { ensureSchema } from "../_schema";

export const onRequestGet: PagesFunction<Env> = async ({ request, env, params }) => {
  if (!env.SHOP_DB) return fail(request, "not_configured", "The shop database is not bound.", 503);
  await ensureSchema(env);

  const number = decodeURIComponent(String(params.number ?? ""));
  const key = new URL(request.url).searchParams.get("k") ?? "";

  const invoice = await findInvoice(env, number);
  if (!invoice) return fail(request, "not_found", "No such invoice.", 404);

  const order = await getOrder(env, invoice.order_id);
  // Knowing an invoice number is not authorisation: the order key that came
  // with the purchase, or the key mailed with the invoice, is.
  if (!order || !(await invoiceKeyMatches(invoice, order, key))) {
    return fail(request, "not_found", "No invoice matches that link.", 404);
  }

  return invoiceResponse(env, invoice);
};
