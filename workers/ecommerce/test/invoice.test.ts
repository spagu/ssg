// Invoices. The two properties that matter are that a number is never reused
// and never skipped, and that an issued invoice never changes afterwards.

import { env } from "cloudflare:test";
import { beforeEach, describe, expect, it } from "vitest";
import {
  buildSnapshot,
  issueCreditNote,
  issueInvoice,
  nextInvoiceNumber,
  renderInvoiceHtml,
  type InvoiceSnapshot,
} from "../functions/api/shop/_invoice";
import { createOrder, priceBasket, upsertCustomer, type PricedBasket } from "../functions/api/shop/_orders";
import { taxContext } from "../functions/api/shop/_tax";
import { putSettings } from "../functions/api/shop/_settings";
import type { CustomerRow, InvoiceRow, OrderItemRow, OrderRow } from "../functions/api/shop/_types";
import { freshShop, seedProduct } from "./_helpers";

interface Sale {
  order: OrderRow;
  items: OrderItemRow[];
  customer: CustomerRow;
}

async function sale(opts: { priceMinor?: number; country?: string; sku?: string } = {}): Promise<Sale> {
  const sku = opts.sku ?? `EBOOK-${Math.random().toString(36).slice(2, 8)}`;
  await seedProduct({ sku, priceMinor: opts.priceMinor ?? 2400 });
  const ctx = await taxContext(env);
  const priced = (await priceBasket(env, ctx, {
    lines: [{ sku, quantity: 1 }],
    currency: "EUR",
    billingCountry: opts.country ?? "DE",
    ipCountry: null,
    vatId: "",
    vatIdValid: false,
  })) as PricedBasket;
  const customer = await upsertCustomer(env, {
    email: "buyer@example.com",
    name: "A Buyer",
    vatId: "",
    vatIdValid: null,
    country: opts.country ?? "DE",
  });
  const { order, items } = await createOrder(env, priced, customer, {
    gateway: "stripe",
    locale: "en",
    consentMarketing: false,
    consentWaiver: true,
    ipHash: null,
    userAgent: "test",
  });
  return { order, items, customer };
}

beforeEach(async () => {
  await freshShop();
});

describe("numbering", () => {
  it("counts up without gaps", async () => {
    const year = new Date().getUTCFullYear();
    expect(await nextInvoiceNumber(env, "FV")).toBe(`FV/${year}/000001`);
    expect(await nextInvoiceNumber(env, "FV")).toBe(`FV/${year}/000002`);
    expect(await nextInvoiceNumber(env, "FV")).toBe(`FV/${year}/000003`);
  });

  it("gives every series its own run", async () => {
    const year = new Date().getUTCFullYear();
    await nextInvoiceNumber(env, "FV");
    expect(await nextInvoiceNumber(env, "KOR")).toBe(`KOR/${year}/000001`);
  });

  it("gives a number to exactly one of two callers at once", async () => {
    // The whole reason it is one UPDATE … RETURNING: two webhooks arriving
    // together must not read the same value and issue two invoices numbered 1.
    const numbers = await Promise.all(Array.from({ length: 8 }, () => nextInvoiceNumber(env, "FV")));
    expect(new Set(numbers).size).toBe(8);
  });

  it("follows the format the owner configured", async () => {
    await putSettings(env, { "invoice.format": "{series}-{year}-{n:04}" });
    expect(await nextInvoiceNumber(env, "FV")).toBe(`FV-${new Date().getUTCFullYear()}-0001`);
  });
});

describe("the snapshot", () => {
  it("freezes both parties, the lines and the totals", async () => {
    const { order, items, customer } = await sale({ priceMinor: 2400 });
    const snapshot = await buildSnapshot(env, order, items, customer, "", 1);

    expect(snapshot.seller.name).toBe("Example Press");
    expect(snapshot.buyer.email).toBe("buyer@example.com");
    expect(snapshot.totals.totalMinor).toBe(2400);
    expect(snapshot.totals.byRate).toEqual([{ rateBp: 700, netMinor: 2243, taxMinor: 157, grossMinor: 2400 }]);
  });

  it("groups the totals by rate, which is what a VAT return wants", async () => {
    const { order, items, customer } = await sale();
    // A second line at a different rate, as a mixed basket would produce.
    const extra: OrderItemRow = {
      ...items[0]!,
      id: "second",
      sku: "OTHER",
      name: "Other",
      tax_rate_bp: 1900,
      tax_minor: 100,
      total_minor: 626,
      unit_minor: 526,
    };
    const snapshot = await buildSnapshot(env, order, [...items, extra], customer, "", 1);
    expect(snapshot.totals.byRate).toHaveLength(2);
  });

  it("multiplies by -1 for a credit note", async () => {
    const { order, items, customer } = await sale();
    const snapshot = await buildSnapshot(env, order, items, customer, "", -1);
    expect(snapshot.totals.totalMinor).toBeLessThan(0);
  });

  it("copes with an order that has no customer row", async () => {
    const { order, items } = await sale();
    const snapshot = await buildSnapshot(env, order, items, null, "", 1);
    expect(snapshot.buyer.email).toBe("");
  });
});

describe("issuing", () => {
  it("issues once, however many times it is asked", async () => {
    const { order, items, customer } = await sale();
    const first = await issueInvoice(env, order, items, customer, "");
    const second = await issueInvoice(env, order, items, customer, "");
    expect(second.id).toBe(first.id);
    expect(second.number).toBe(first.number);

    const count = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM invoices`).first<{ n: number }>();
    expect(count?.n).toBe(1);
  });

  it("answers a refund with a credit note, not an edit", async () => {
    const { order, items, customer } = await sale({ priceMinor: 2400 });
    const invoice = await issueInvoice(env, order, items, customer, "");
    const credit = await issueCreditNote(env, order, items, customer, invoice, 2400, "");

    expect(credit.kind).toBe("credit_note");
    expect(credit.corrects_id).toBe(invoice.id);
    expect(credit.total_minor).toBe(-2400);
    expect(credit.number).toContain("KOR");

    // The original is untouched: that is what makes it evidence.
    const stored = await env.SHOP_DB.prepare(`SELECT * FROM invoices WHERE id = ?`)
      .bind(invoice.id)
      .first<InvoiceRow>();
    expect(stored?.total_minor).toBe(2400);
    expect(stored?.snapshot_json).toBe(invoice.snapshot_json);
  });

  it("credits only what was refunded on a partial refund", async () => {
    const { order, items, customer } = await sale({ priceMinor: 2400 });
    const invoice = await issueInvoice(env, order, items, customer, "");
    const credit = await issueCreditNote(env, order, items, customer, invoice, 1000, "");
    expect(credit.total_minor).toBe(-1000);
  });
});

describe("the printable invoice", () => {
  it("carries both parties, the number and the totals", async () => {
    const { order, items, customer } = await sale({ priceMinor: 2400 });
    const invoice = await issueInvoice(env, order, items, customer, "");
    const html = renderInvoiceHtml(invoice);

    expect(html).toContain(invoice.number);
    expect(html).toContain("Example Press");
    expect(html).toContain("buyer@example.com");
    expect(html).toContain("24.00");
    // Self-contained: an invoice that needs the network to render is an invoice
    // that stops rendering.
    expect(html).not.toMatch(/<script|<link rel="stylesheet"/);
    expect(html).toContain("@media print");
  });

  it("escapes what the buyer typed", async () => {
    const { order, items, customer } = await sale();
    await env.SHOP_DB.prepare(`UPDATE customers SET name = ? WHERE id = ?`)
      .bind('<script>alert(1)</script>', customer.id)
      .run();
    const fresh = (await env.SHOP_DB.prepare(`SELECT * FROM customers WHERE id = ?`)
      .bind(customer.id)
      .first<CustomerRow>())!;
    const invoice = await issueInvoice(env, order, items, fresh, "");

    const html = renderInvoiceHtml(invoice);
    expect(html).not.toContain("<script>alert(1)</script>");
    expect(html).toContain("&lt;script&gt;");
  });

  it("names the invoice it corrects", async () => {
    const { order, items, customer } = await sale();
    const invoice = await issueInvoice(env, order, items, customer, "");
    const credit = await issueCreditNote(env, order, items, customer, invoice, 2400, "");
    expect(renderInvoiceHtml(credit, invoice.number)).toContain(invoice.number);
  });

  it("prints the tax note when there is one", async () => {
    const { order, items, customer } = await sale();
    const invoice = await issueInvoice(env, order, items, customer, "Reverse charge — VAT to be accounted for by the recipient.");
    expect(renderInvoiceHtml(invoice)).toContain("Reverse charge");
  });

  it("renders a Polish invoice in Polish", async () => {
    const { order, items, customer } = await sale();
    await env.SHOP_DB.prepare(`UPDATE orders SET locale = 'pl' WHERE id = ?`).bind(order.id).run();
    const plOrder = { ...order, locale: "pl" };
    const invoice = await issueInvoice(env, plOrder, items, customer, "");
    const snapshot = JSON.parse(invoice.snapshot_json) as InvoiceSnapshot;
    expect(snapshot.locale).toBe("pl");
    expect(renderInvoiceHtml(invoice)).toMatch(/Faktura|Sprzedawca/);
  });
});
