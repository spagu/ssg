// Invoices: a number with no gaps, and a snapshot that never changes.
//
// Two properties matter more than anything else here. The numbering must not
// skip (in most EU jurisdictions a gap has to be explained), and an issued
// invoice must not change when the seller later edits their address or a
// product's price — so everything it says is frozen into snapshot_json at the
// moment of issue.

import type { Env } from "./_env";
import { escapeHtml, newId, nowISO } from "./_lib";
import { formatMoney, minorToDecimalString } from "./_money";
import { getSettingString, sellerSnapshot, type SellerSnapshot } from "./_settings";
import type { CustomerRow, InvoiceRow, OrderItemRow, OrderRow } from "./_types";

export interface InvoiceSnapshot {
  seller: SellerSnapshot;
  buyer: {
    name: string;
    email: string;
    country: string;
    vatId: string;
  };
  order: {
    number: string;
    currency: string;
    paidAt: string | null;
    gateway: string | null;
    reverseCharge: boolean;
    taxCountry: string | null;
  };
  lines: Array<{
    name: string;
    sku: string;
    quantity: number;
    unitMinor: number;
    taxRateBp: number;
    taxMinor: number;
    totalMinor: number;
  }>;
  totals: {
    subtotalMinor: number;
    taxMinor: number;
    totalMinor: number;
    /** One row per rate, which is what a VAT return wants. */
    byRate: Array<{ rateBp: number; netMinor: number; taxMinor: number; grossMinor: number }>;
  };
  note: string;
  locale: string;
}

/** Takes the next number in a series atomically.
 *
 *  D1 runs statements against one database, so a single UPDATE ... RETURNING is
 *  enough: two webhooks arriving together cannot read the same value. Numbers
 *  are taken only when an invoice is really being issued, never when an order
 *  is created, so an abandoned checkout does not eat one. */
export async function nextInvoiceNumber(env: Env, series: string): Promise<string> {
  const year = new Date().getUTCFullYear();
  await env.SHOP_DB.prepare(`INSERT OR IGNORE INTO invoice_sequences (series, year, next) VALUES (?, ?, 1)`)
    .bind(series, year)
    .run();
  const row = await env.SHOP_DB.prepare(
    `UPDATE invoice_sequences SET next = next + 1 WHERE series = ? AND year = ? RETURNING next - 1 AS n`,
  )
    .bind(series, year)
    .first<{ n: number }>();
  const n = row?.n ?? 1;
  const format = (await getSettingString(env, "invoice.format")) || "{series}/{year}/{n:06}";
  return format
    .replaceAll("{series}", series)
    .replaceAll("{year}", String(year))
    .replace(/\{n:(\d+)\}/, (_m, width: string) => String(n).padStart(Number.parseInt(width, 10), "0"))
    .replaceAll("{n}", String(n));
}

function groupByRate(lines: InvoiceSnapshot["lines"]): InvoiceSnapshot["totals"]["byRate"] {
  const map = new Map<number, { rateBp: number; netMinor: number; taxMinor: number; grossMinor: number }>();
  for (const l of lines) {
    const entry = map.get(l.taxRateBp) ?? { rateBp: l.taxRateBp, netMinor: 0, taxMinor: 0, grossMinor: 0 };
    entry.netMinor += l.unitMinor * l.quantity;
    entry.taxMinor += l.taxMinor;
    entry.grossMinor += l.totalMinor;
    map.set(l.taxRateBp, entry);
  }
  return [...map.values()].sort((a, b) => a.rateBp - b.rateBp);
}

export async function buildSnapshot(
  env: Env,
  order: OrderRow,
  items: OrderItemRow[],
  customer: CustomerRow | null,
  note: string,
  sign: 1 | -1 = 1,
): Promise<InvoiceSnapshot> {
  const lines = items.map((i) => ({
    name: i.name,
    sku: i.sku,
    quantity: i.quantity,
    unitMinor: sign * i.unit_minor,
    taxRateBp: i.tax_rate_bp,
    taxMinor: sign * i.tax_minor,
    totalMinor: sign * i.total_minor,
  }));
  return {
    seller: await sellerSnapshot(env),
    buyer: {
      name: customer?.name ?? "",
      email: customer?.email ?? "",
      country: customer?.country ?? "",
      vatId: customer?.vat_id ?? "",
    },
    order: {
      number: order.number,
      currency: order.currency,
      paidAt: order.paid_at,
      gateway: order.gateway,
      reverseCharge: order.reverse_charge === 1,
      taxCountry: order.tax_country,
    },
    lines,
    totals: {
      subtotalMinor: sign * order.subtotal_minor,
      taxMinor: sign * order.tax_minor,
      totalMinor: sign * order.total_minor,
      byRate: groupByRate(lines),
    },
    note,
    locale: order.locale ?? "en",
  };
}

/** Issues an invoice for an order, once. A second call returns the one that
 *  already exists rather than taking another number. */
export async function issueInvoice(
  env: Env,
  order: OrderRow,
  items: OrderItemRow[],
  customer: CustomerRow | null,
  note: string,
): Promise<InvoiceRow> {
  const existing = await env.SHOP_DB.prepare(
    `SELECT * FROM invoices WHERE order_id = ? AND kind = 'invoice'`,
  )
    .bind(order.id)
    .first<InvoiceRow>();
  if (existing) return existing;

  const series = (await getSettingString(env, "invoice.series")) || "FV";
  const number = await nextInvoiceNumber(env, series);
  const snapshot = await buildSnapshot(env, order, items, customer, note, 1);
  const row: InvoiceRow = {
    id: newId(),
    number,
    kind: "invoice",
    order_id: order.id,
    corrects_id: null,
    issued_at: nowISO(),
    currency: order.currency,
    total_minor: order.total_minor,
    snapshot_json: JSON.stringify(snapshot),
    pdf_key: null,
    created_at: nowISO(),
  };
  await insertInvoice(env, row);
  return row;
}

/** A refund produces a credit note, never an edit: an issued invoice is not
 *  editable, which is the whole point of the snapshot. */
export async function issueCreditNote(
  env: Env,
  order: OrderRow,
  items: OrderItemRow[],
  customer: CustomerRow | null,
  original: InvoiceRow,
  amountMinor: number,
  note: string,
): Promise<InvoiceRow> {
  const series = (await getSettingString(env, "invoice.credit_series")) || "KOR";
  const number = await nextInvoiceNumber(env, series);
  const snapshot = await buildSnapshot(env, order, items, customer, note, -1);
  const row: InvoiceRow = {
    id: newId(),
    number,
    kind: "credit_note",
    order_id: order.id,
    corrects_id: original.id,
    issued_at: nowISO(),
    currency: order.currency,
    total_minor: -Math.abs(amountMinor),
    snapshot_json: JSON.stringify(snapshot),
    pdf_key: null,
    created_at: nowISO(),
  };
  await insertInvoice(env, row);
  return row;
}

async function insertInvoice(env: Env, row: InvoiceRow): Promise<void> {
  await env.SHOP_DB.prepare(
    `INSERT INTO invoices (id, number, kind, order_id, corrects_id, issued_at, currency, total_minor, snapshot_json, pdf_key, created_at)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
  )
    .bind(
      row.id, row.number, row.kind, row.order_id, row.corrects_id, row.issued_at,
      row.currency, row.total_minor, row.snapshot_json, row.pdf_key, row.created_at,
    )
    .run();
}

// ── Rendering ───────────────────────────────────────────────────────────────

const T = {
  en: {
    invoice: "Invoice", creditNote: "Credit note", seller: "Seller", buyer: "Buyer",
    number: "Number", issued: "Issue date", sold: "Date of sale", vatId: "VAT ID",
    item: "Item", qty: "Qty", unit: "Unit net", rate: "VAT", tax: "VAT amount", total: "Gross",
    subtotal: "Net total", taxTotal: "VAT total", grandTotal: "Total", paid: "Paid",
    method: "Payment method", corrects: "Corrects invoice", summaryByRate: "Summary by rate",
  },
  pl: {
    invoice: "Faktura", creditNote: "Faktura korygująca", seller: "Sprzedawca", buyer: "Nabywca",
    number: "Numer", issued: "Data wystawienia", sold: "Data sprzedaży", vatId: "NIP",
    item: "Pozycja", qty: "Ilość", unit: "Cena netto", rate: "VAT", tax: "Kwota VAT", total: "Brutto",
    subtotal: "Razem netto", taxTotal: "Razem VAT", grandTotal: "Do zapłaty", paid: "Zapłacono",
    method: "Metoda płatności", corrects: "Koryguje fakturę", summaryByRate: "Podsumowanie wg stawek",
  },
} as const;

/** The invoice as a self-contained HTML page.
 *
 *  No external stylesheet, no web font, no script: this document has to open in
 *  six years, from an archive, with no network. That is also why the print
 *  rules are inline. */
export function renderInvoiceHtml(invoice: InvoiceRow, correctsNumber?: string): string {
  const snap = JSON.parse(invoice.snapshot_json) as InvoiceSnapshot;
  const locale = snap.locale?.startsWith("pl") ? "pl" : "en";
  const t = T[locale];
  const money = (minor: number) => escapeHtml(formatMoney(minor, snap.order.currency, locale));
  const title = invoice.kind === "credit_note" ? t.creditNote : t.invoice;

  const rows = snap.lines
    .map(
      (l) => `<tr>
        <td>${escapeHtml(l.name)}<br><small>${escapeHtml(l.sku)}</small></td>
        <td class="n">${l.quantity}</td>
        <td class="n">${money(l.unitMinor)}</td>
        <td class="n">${(l.taxRateBp / 100).toFixed(l.taxRateBp % 100 === 0 ? 0 : 1)}%</td>
        <td class="n">${money(l.taxMinor)}</td>
        <td class="n">${money(l.totalMinor)}</td>
      </tr>`,
    )
    .join("");

  const byRate = snap.totals.byRate
    .map(
      (r) => `<tr>
        <td>${(r.rateBp / 100).toFixed(r.rateBp % 100 === 0 ? 0 : 1)}%</td>
        <td class="n">${money(r.netMinor)}</td>
        <td class="n">${money(r.taxMinor)}</td>
        <td class="n">${money(r.grossMinor)}</td>
      </tr>`,
    )
    .join("");

  return `<!doctype html>
<html lang="${locale}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow">
<title>${escapeHtml(title)} ${escapeHtml(invoice.number)}</title>
<style>
  :root { color-scheme: light; }
  body { font: 13px/1.5 system-ui, -apple-system, "Segoe UI", Roboto, sans-serif; color: #1f2328; max-width: 46rem; margin: 2rem auto; padding: 0 1.5rem; }
  header { display: flex; justify-content: space-between; gap: 2rem; align-items: flex-start; border-bottom: 2px solid #1967D2; padding-bottom: 1rem; }
  h1 { font-size: 1.4rem; margin: 0 0 .25rem; color: #1967D2; }
  .meta { text-align: right; font-size: .9rem; }
  .parties { display: flex; gap: 2rem; margin: 1.5rem 0; }
  .party { flex: 1; }
  .party h2 { font-size: .8rem; text-transform: uppercase; letter-spacing: .04em; color: #5f6368; margin: 0 0 .35rem; }
  table { width: 100%; border-collapse: collapse; margin: 1rem 0; }
  th, td { padding: .5rem .4rem; border-bottom: 1px solid #dadce0; text-align: left; vertical-align: top; }
  th { font-size: .75rem; text-transform: uppercase; letter-spacing: .03em; color: #5f6368; }
  td.n, th.n { text-align: right; white-space: nowrap; }
  small { color: #5f6368; }
  .totals { margin-left: auto; width: 18rem; }
  .totals td { border: 0; padding: .2rem 0; }
  .grand { font-weight: 700; font-size: 1.1rem; border-top: 2px solid #1f2328 !important; padding-top: .5rem !important; }
  .note { margin-top: 1.5rem; padding: .75rem 1rem; background: #f1f3f4; border-left: 3px solid #1967D2; }
  footer { margin-top: 2rem; font-size: .8rem; color: #5f6368; }
  @media print { body { margin: 0; max-width: none; padding: 1cm; font-size: 11px; } @page { size: A4; margin: 1.5cm; } }
</style>
</head>
<body>
<header>
  <div>
    <h1>${escapeHtml(title)}</h1>
    <div><strong>${escapeHtml(invoice.number)}</strong></div>
    ${correctsNumber ? `<div>${escapeHtml(t.corrects)}: ${escapeHtml(correctsNumber)}</div>` : ""}
  </div>
  <div class="meta">
    <div>${escapeHtml(t.issued)}: ${escapeHtml(invoice.issued_at.slice(0, 10))}</div>
    ${snap.order.paidAt ? `<div>${escapeHtml(t.sold)}: ${escapeHtml(snap.order.paidAt.slice(0, 10))}</div>` : ""}
    <div>${escapeHtml(t.method)}: ${escapeHtml(snap.order.gateway ?? "—")}</div>
  </div>
</header>

<div class="parties">
  <div class="party">
    <h2>${escapeHtml(t.seller)}</h2>
    <div><strong>${escapeHtml(snap.seller.name)}</strong></div>
    <div>${escapeHtml(snap.seller.address).replaceAll("\n", "<br>")}</div>
    <div>${escapeHtml(snap.seller.country)}</div>
    ${snap.seller.vatId ? `<div>${escapeHtml(t.vatId)}: ${escapeHtml(snap.seller.vatId)}</div>` : ""}
    ${snap.seller.registry ? `<div>${escapeHtml(snap.seller.registry)}</div>` : ""}
  </div>
  <div class="party">
    <h2>${escapeHtml(t.buyer)}</h2>
    <div><strong>${escapeHtml(snap.buyer.name || snap.buyer.email)}</strong></div>
    <div>${escapeHtml(snap.buyer.email)}</div>
    <div>${escapeHtml(snap.buyer.country)}</div>
    ${snap.buyer.vatId ? `<div>${escapeHtml(t.vatId)}: ${escapeHtml(snap.buyer.vatId)}</div>` : ""}
  </div>
</div>

<table>
  <thead><tr>
    <th>${escapeHtml(t.item)}</th><th class="n">${escapeHtml(t.qty)}</th><th class="n">${escapeHtml(t.unit)}</th>
    <th class="n">${escapeHtml(t.rate)}</th><th class="n">${escapeHtml(t.tax)}</th><th class="n">${escapeHtml(t.total)}</th>
  </tr></thead>
  <tbody>${rows}</tbody>
</table>

${
  snap.totals.byRate.length > 1
    ? `<h2 style="font-size:.8rem;text-transform:uppercase;color:#5f6368">${escapeHtml(t.summaryByRate)}</h2>
       <table><thead><tr><th>${escapeHtml(t.rate)}</th><th class="n">${escapeHtml(t.subtotal)}</th><th class="n">${escapeHtml(t.taxTotal)}</th><th class="n">${escapeHtml(t.total)}</th></tr></thead><tbody>${byRate}</tbody></table>`
    : ""
}

<table class="totals">
  <tr><td>${escapeHtml(t.subtotal)}</td><td class="n">${money(snap.totals.subtotalMinor)}</td></tr>
  <tr><td>${escapeHtml(t.taxTotal)}</td><td class="n">${money(snap.totals.taxMinor)}</td></tr>
  <tr><td class="grand">${escapeHtml(t.grandTotal)}</td><td class="n grand">${money(snap.totals.totalMinor)}</td></tr>
</table>

${snap.note ? `<div class="note">${escapeHtml(snap.note)}</div>` : ""}
${snap.order.paidAt ? `<p><strong>${escapeHtml(t.paid)}</strong></p>` : ""}

<footer>${escapeHtml(snap.order.number)} · ${escapeHtml(minorToDecimalString(Math.abs(snap.totals.totalMinor), snap.order.currency))} ${escapeHtml(snap.order.currency)}</footer>
</body>
</html>`;
}
