// GET /api/shop/admin/export?type=orders|invoices|vat&from=&to= — a CSV for the
// accountant.
//
// Every cell goes through csvCell, which quotes it and defuses a leading = + -
// or @. A product name is buyer-supplied text in the seller's own spreadsheet;
// a CSV export is the classic way that becomes a formula (S-25).

import type { Env } from "../_env";
import { audit } from "../_audit";
import { csvCell, fail, str } from "../_lib";
import { minorToDecimalString } from "../_money";
import type { AdminData } from "./_middleware";

type Row = Record<string, unknown>;

function toCsv(headers: string[], rows: Row[]): string {
  const lines = [headers.map(csvCell).join(",")];
  for (const row of rows) lines.push(headers.map((h) => csvCell(row[h])).join(","));
  // A byte-order mark, written as an escape rather than an invisible character
  // in the source. Without it Excel reads the file as the local codepage and a
  // buyer called Müller arrives as MÃ¼ller.
  return `\uFEFF${lines.join("\r\n")}\r\n`;
}

const QUERIES: Record<string, { headers: string[]; sql: string }> = {
  orders: {
    headers: [
      "number", "status", "created_at", "paid_at", "email", "country",
      "tax_country", "vat_id", "reverse_charge", "currency",
      "subtotal", "tax", "total", "gateway",
    ],
    sql: `SELECT o.number, o.status, o.created_at, o.paid_at, c.email, c.country,
                 o.tax_country, c.vat_id, o.reverse_charge, o.currency,
                 o.subtotal_minor, o.tax_minor, o.total_minor, o.gateway
            FROM orders o
            LEFT JOIN customers c ON c.id = o.customer_id
           WHERE o.created_at >= ? AND o.created_at <= ?
           ORDER BY o.created_at`,
  },
  invoices: {
    headers: ["number", "kind", "issued_at", "order_number", "currency", "total"],
    sql: `SELECT v.number, v.kind, v.issued_at, o.number AS order_number, v.currency, v.total_minor
            FROM invoices v
            JOIN orders o ON o.id = v.order_id
           WHERE v.issued_at >= ? AND v.issued_at <= ?
           ORDER BY v.issued_at`,
  },
  // One row per country and rate: the shape an OSS return is filled in from.
  vat: {
    headers: ["tax_country", "rate_percent", "orders", "currency", "net", "tax"],
    sql: `SELECT o.tax_country, i.tax_rate_bp, COUNT(DISTINCT o.id) AS orders, o.currency,
                 SUM(i.total_minor - i.tax_minor) AS net, SUM(i.tax_minor) AS tax
            FROM orders o
            JOIN order_items i ON i.order_id = o.id
           WHERE o.status IN ('paid', 'fulfilled') AND o.paid_at >= ? AND o.paid_at <= ?
           GROUP BY o.tax_country, i.tax_rate_bp, o.currency
           ORDER BY o.tax_country`,
  },
};

/** Amounts leave as decimal strings in the row's own currency. A spreadsheet
 *  that reads 1999 as "nearly two thousand euro" is worse than no export. */
function humanise(type: string, row: Row): Row {
  const currency = String(row.currency ?? "EUR");
  const money = (key: string): void => {
    if (row[key] !== undefined && row[key] !== null) row[key] = minorToDecimalString(Number(row[key]), currency);
  };
  if (type === "orders") {
    row.subtotal = row.subtotal_minor;
    row.tax = row.tax_minor;
    row.total = row.total_minor;
    money("subtotal");
    money("tax");
    money("total");
    row.reverse_charge = row.reverse_charge === 1 ? "yes" : "no";
  } else if (type === "invoices") {
    row.total = row.total_minor;
    money("total");
  } else {
    row.rate_percent = (Number(row.tax_rate_bp ?? 0) / 100).toFixed(2);
    money("net");
    money("tax");
  }
  return row;
}

export const onRequestGet: PagesFunction<Env, string, AdminData> = async ({ request, env, data }) => {
  const q = new URL(request.url).searchParams;
  const type = str(q.get("type"), 20) || "orders";
  const spec = QUERIES[type];
  if (!spec) return fail(request, "invalid_type", "type must be orders, invoices or vat.", 422);

  const from = str(q.get("from"), 40) || new Date(Date.now() - 365 * 86400000).toISOString().slice(0, 10);
  const to = str(q.get("to"), 40) || new Date().toISOString().slice(0, 10);
  if (Number.isNaN(Date.parse(from)) || Number.isNaN(Date.parse(to))) {
    return fail(request, "invalid_range", "from and to must be dates (YYYY-MM-DD).", 422);
  }
  // An inclusive "to": a date alone means the end of that day, which is what
  // someone asking for "up to the 31st" means.
  const toEnd = to.length === 10 ? `${to}T23:59:59.999Z` : to;

  const { results } = await env.SHOP_DB.prepare(spec.sql).bind(from, toEnd).all<Row>();
  const body = toCsv(spec.headers, (results ?? []).map((r) => humanise(type, r)));

  await audit(env, `admin:${data.admin.sub}`, "export", type, { from, to, rows: results?.length ?? 0 });

  return new Response(body, {
    headers: {
      "content-type": "text/csv; charset=utf-8",
      "content-disposition": `attachment; filename="shop-${type}-${from}-${to}.csv"`,
      "cache-control": "no-store",
      "x-content-type-options": "nosniff",
    },
  });
};
