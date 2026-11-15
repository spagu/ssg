// GET /api/shop/admin/invoices/:number — the owner's copy of an invoice or a
// credit note, to open or print from the order screen.
//
// The admin gate (admin/_middleware.ts) has already decided who may read it:
// ecommerce:read in `auth` mode, anyone signed in with Access or the shared
// password. The page is the buyer's page byte for byte (_invoice-page.ts).

import type { Env } from "../../_env";
import { fail } from "../../_lib";
import { findInvoice, invoiceResponse } from "../../_invoice-page";
import type { AdminData } from "../_middleware";

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env, params }) => {
  const invoice = await findInvoice(env, decodeURIComponent(String(params.number)));
  if (!invoice) return fail(request, "not_found", "No such invoice.", 404);
  return invoiceResponse(env, invoice);
};
