// Invoices. The two properties that matter are that a number is never reused
// and never skipped, and that an issued invoice never changes afterwards.

import { env } from "./_env";
import { beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
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
    assert.equal(await nextInvoiceNumber(env, "FV"), `FV/${year}/000001`);
    assert.equal(await nextInvoiceNumber(env, "FV"), `FV/${year}/000002`);
    assert.equal(await nextInvoiceNumber(env, "FV"), `FV/${year}/000003`);
  });

  it("gives every series its own run", async () => {
    const year = new Date().getUTCFullYear();
    await nextInvoiceNumber(env, "FV");
    assert.equal(await nextInvoiceNumber(env, "KOR"), `KOR/${year}/000001`);
  });

  it("gives a number to exactly one of two callers at once", async () => {
    // The whole reason it is one UPDATE … RETURNING: two webhooks arriving
    // together must not read the same value and issue two invoices numbered 1.
    const numbers = await Promise.all(Array.from({ length: 8 }, () => nextInvoiceNumber(env, "FV")));
    assert.equal(new Set(numbers).size, 8);
  });

  it("follows the format the owner configured", async () => {
    await putSettings(env, { "invoice.format": "{series}-{year}-{n:04}" });
    assert.equal(await nextInvoiceNumber(env, "FV"), `FV-${new Date().getUTCFullYear()}-0001`);
  });
});

describe("the snapshot", () => {
  it("freezes both parties, the lines and the totals", async () => {
    const { order, items, customer } = await sale({ priceMinor: 2400 });
    const snapshot = await buildSnapshot(env, order, items, customer, "", 1);

    assert.equal(snapshot.seller.name, "Example Press");
    assert.equal(snapshot.buyer.email, "buyer@example.com");
    assert.equal(snapshot.totals.totalMinor, 2400);
    assert.deepEqual(snapshot.totals.byRate, [{ rateBp: 700, netMinor: 2243, taxMinor: 157, grossMinor: 2400 }]);
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
    assert.equal((snapshot.totals.byRate).length, 2);
  });

  it("multiplies by -1 for a credit note", async () => {
    const { order, items, customer } = await sale();
    const snapshot = await buildSnapshot(env, order, items, customer, "", -1);
    assert.ok((snapshot.totals.totalMinor) < 0);
  });

  it("copes with an order that has no customer row", async () => {
    const { order, items } = await sale();
    const snapshot = await buildSnapshot(env, order, items, null, "", 1);
    assert.equal(snapshot.buyer.email, "");
  });
});

describe("issuing", () => {
  it("issues once, however many times it is asked", async () => {
    const { order, items, customer } = await sale();
    const first = await issueInvoice(env, order, items, customer, "");
    const second = await issueInvoice(env, order, items, customer, "");
    assert.equal(second.id, first.id);
    assert.equal(second.number, first.number);

    const count = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM invoices`).first<{ n: number }>();
    assert.equal(count?.n, 1);
  });

  it("answers a refund with a credit note, not an edit", async () => {
    const { order, items, customer } = await sale({ priceMinor: 2400 });
    const invoice = await issueInvoice(env, order, items, customer, "");
    const credit = await issueCreditNote(env, order, items, customer, invoice, 2400, "");

    assert.equal(credit.kind, "credit_note");
    assert.equal(credit.corrects_id, invoice.id);
    assert.equal(credit.total_minor, -2400);
    assert.ok((credit.number).includes("KOR"));

    // The original is untouched: that is what makes it evidence.
    const stored = await env.SHOP_DB.prepare(`SELECT * FROM invoices WHERE id = ?`)
      .bind(invoice.id)
      .first<InvoiceRow>();
    assert.equal(stored?.total_minor, 2400);
    assert.equal(stored?.snapshot_json, invoice.snapshot_json);
  });

  it("credits only what was refunded on a partial refund", async () => {
    const { order, items, customer } = await sale({ priceMinor: 2400 });
    const invoice = await issueInvoice(env, order, items, customer, "");
    const credit = await issueCreditNote(env, order, items, customer, invoice, 1000, "");
    assert.equal(credit.total_minor, -1000);
  });
});

describe("the printable invoice", () => {
  it("carries both parties, the number and the totals", async () => {
    const { order, items, customer } = await sale({ priceMinor: 2400 });
    const invoice = await issueInvoice(env, order, items, customer, "");
    const html = renderInvoiceHtml(invoice);

    assert.ok((html).includes(invoice.number));
    assert.ok((html).includes("Example Press"));
    assert.ok((html).includes("buyer@example.com"));
    assert.ok((html).includes("24.00"));
    // Self-contained: an invoice that needs the network to render is an invoice
    // that stops rendering.
    assert.doesNotMatch(html, /<script|<link rel="stylesheet"/);
    assert.ok((html).includes("@media print"));
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
    assert.ok(!(html).includes("<script>alert(1)</script>"));
    assert.ok((html).includes("&lt;script&gt;"));
  });

  it("names the invoice it corrects", async () => {
    const { order, items, customer } = await sale();
    const invoice = await issueInvoice(env, order, items, customer, "");
    const credit = await issueCreditNote(env, order, items, customer, invoice, 2400, "");
    assert.ok((renderInvoiceHtml(credit, invoice.number)).includes(invoice.number));
  });

  it("prints the tax note when there is one", async () => {
    const { order, items, customer } = await sale();
    const invoice = await issueInvoice(env, order, items, customer, "Reverse charge — VAT to be accounted for by the recipient.");
    assert.ok((renderInvoiceHtml(invoice)).includes("Reverse charge"));
  });

  it("renders a Polish invoice in Polish", async () => {
    const { order, items, customer } = await sale();
    await env.SHOP_DB.prepare(`UPDATE orders SET locale = 'pl' WHERE id = ?`).bind(order.id).run();
    const plOrder = { ...order, locale: "pl" };
    const invoice = await issueInvoice(env, plOrder, items, customer, "");
    const snapshot = JSON.parse(invoice.snapshot_json) as InvoiceSnapshot;
    assert.equal(snapshot.locale, "pl");
    assert.match(renderInvoiceHtml(invoice), /Faktura|Sprzedawca/);
  });
});
