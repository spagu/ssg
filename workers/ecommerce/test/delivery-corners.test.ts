// The less travelled paths of delivery: an order with no customer or locale on
// it, a shop whose settings were cleared, an invoice in every shape, a file
// the bucket describes partially, and a database answer that comes back
// without the metadata the code normally reads.

import { env } from "./_env";
import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { audit } from "../functions/api/shop/_audit";
import { issueTokens, revokeTokensForOrder, spendUse } from "../functions/api/shop/_downloads";
import { fulfilOrder, markNeedsReview } from "../functions/api/shop/_fulfil";
import { endpointsFor } from "../functions/api/shop/_hooks";
import { issueCreditNote, issueInvoice, nextInvoiceNumber, renderInvoiceHtml } from "../functions/api/shop/_invoice";
import { readBody, sha256hex } from "../functions/api/shop/_lib";
import { adminNoticeMail, deliverEmail, orderPaidMail, refundedMail, resendMail } from "../functions/api/shop/_mail";
import { createOrder, getOrderItems, priceBasket, upsertCustomer, type PricedBasket } from "../functions/api/shop/_orders";
import { drainOutbox, enqueue, retryOutbox } from "../functions/api/shop/_outbox";
import { putSettings } from "../functions/api/shop/_settings";
import { taxContext } from "../functions/api/shop/_tax";
import { deliverTracking } from "../functions/api/shop/_tracking";
import type { InvoiceRow, OrderRow, ProductRow } from "../functions/api/shop/_types";
import { onRequestGet as download } from "../functions/api/shop/download/[token]";
import { onRequestGet as invoicePage } from "../functions/api/shop/invoices/[number]";
import { onRequestPost as resend } from "../functions/api/shop/resend";
import { ctx, freshShop, get, ORIGIN, post, seedProduct } from "./_helpers";
import { sparseD1 } from "./fakes";

const realFetch = globalThis.fetch;

/** An order for one book, as checkout would leave it, with its plaintext key. */
async function placeOrder(): Promise<{ order: OrderRow; orderKey: string; productId: string }> {
  const { id } = await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
  const tax = await taxContext(env);
  const priced = (await priceBasket(env, tax, {
    lines: [{ sku: "EBOOK-1", quantity: 1 }],
    currency: "EUR",
    billingCountry: "DE",
    ipCountry: null,
    vatId: "",
    vatIdValid: false,
  })) as PricedBasket;
  const customer = await upsertCustomer(env, {
    email: "buyer@example.com", name: "A Buyer", vatId: "", vatIdValid: null, country: "DE",
  });
  const { order, orderKey } = await createOrder(env, priced, customer, {
    gateway: "stripe", locale: "en", consentMarketing: false, consentWaiver: true, ipHash: null, userAgent: "t",
  });
  return { order, orderKey, productId: id };
}

async function reload(id: string): Promise<OrderRow> {
  return (await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(id).first<OrderRow>())!;
}

/** A paid order with its download token in hand. */
async function paidWithToken(): Promise<{ order: OrderRow; token: string; productId: string }> {
  const { order, productId } = await placeOrder();
  await env.SHOP_DB.prepare(`UPDATE orders SET status = 'paid' WHERE id = ?`).bind(order.id).run();
  const items = await getOrderItems(env, order.id);
  const product = (await env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`).bind(productId).first<ProductRow>())!;
  const [issued] = await issueTokens(env, items as never, new Map([[productId, product]]));
  return { order, token: issued!.token, productId };
}

/** The same database, answering statements that match `match` the way a D1
 *  response without `meta`/`results` (or, with `nullFirst`, without a row)
 *  looks. The code reads both through `??`, and these tests hold it to the
 *  safe reading: nothing changed, nothing found. */
function sparse(match: RegExp, opts: { nullFirst?: boolean } = {}) {
  return { ...env, SHOP_DB: sparseD1(env.SHOP_DB, match, { first: opts.nullFirst ? match : null }) as typeof env.SHOP_DB };
}

/** A minimal invoice row around a hand-built snapshot. */
function invoiceRow(snapshot: Record<string, unknown>, kind: "invoice" | "credit_note" = "invoice"): InvoiceRow {
  return {
    id: "inv-1", number: "FV/2026/000001", kind, order_id: "o-1", corrects_id: null,
    issued_at: "2026-11-14T10:00:00.000Z", currency: "EUR", total_minor: 2400,
    snapshot_json: JSON.stringify(snapshot), pdf_key: null, created_at: "2026-11-14T10:00:00.000Z",
  } as InvoiceRow;
}

function snapshot(over: Record<string, unknown> = {}) {
  return {
    seller: { name: "Example Press", address: "1 Example Street", country: "PL", vatId: "", email: "", registry: "KRS 0000000000" },
    buyer: { name: "", email: "buyer@example.com", country: "DE", vatId: "DE123456789" },
    order: { number: "O-2026-000001", currency: "EUR", paidAt: null, gateway: null, reverseCharge: false, taxCountry: "DE" },
    lines: [
      { name: "A Book", sku: "EBOOK-1", quantity: 1, unitMinor: 1000, taxRateBp: 550, taxMinor: 55, totalMinor: 1055 },
      { name: "A Course", sku: "COURSE-1", quantity: 1, unitMinor: 1000, taxRateBp: 2300, taxMinor: 230, totalMinor: 1230 },
    ],
    totals: {
      subtotalMinor: 2000, taxMinor: 285, totalMinor: 2285,
      byRate: [
        { rateBp: 550, netMinor: 1000, taxMinor: 55, grossMinor: 1055 },
        { rateBp: 2300, netMinor: 1000, taxMinor: 230, grossMinor: 1230 },
      ],
    },
    note: "",
    ...over,
  };
}

beforeEach(async () => {
  await freshShop();
  globalThis.fetch = (async () => {
    throw new Error("these tests must not reach the network unless they say so");
  }) as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("fulfilling an order the shop knows little about", () => {
  it("delivers an order with no customer, locale or settings to speak of", async () => {
    const { order } = await placeOrder();
    await env.SHOP_DB.prepare(`UPDATE orders SET customer_id = NULL, locale = NULL, reverse_charge = 1 WHERE id = ?`)
      .bind(order.id)
      .run();
    assert.deepEqual(await putSettings(env, { "shop.name": null, "seller.email": null }), []);

    const result = await fulfilOrder(env, await reload(order.id), { origin: ORIGIN, actor: "test" });
    assert.equal(result.changed, true);
    // Nobody to email, so the buyer's mail is not queued; the invoice still
    // exists, carrying the reverse-charge note in the default language.
    const mails = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM outbox WHERE kind = 'email'`).first<{ n: number }>();
    assert.equal(mails?.n, 0);
    const inv = await env.SHOP_DB.prepare(`SELECT snapshot_json FROM invoices WHERE order_id = ?`)
      .bind(order.id)
      .first<{ snapshot_json: string }>();
    const snap = JSON.parse(inv!.snapshot_json) as { locale: string; buyer: { email: string }; note: string };
    assert.equal(snap.locale, "en");
    assert.equal(snap.buyer.email, "");
    assert.match(snap.note, /reverse/i);
  });

  it("names the shop generically and sets no reply-to when those settings are cleared", async () => {
    const { order } = await placeOrder();
    await env.SHOP_DB.prepare(`UPDATE orders SET locale = NULL WHERE id = ?`).bind(order.id).run();
    await putSettings(env, { "shop.name": null, "seller.email": null });

    await fulfilOrder(env, await reload(order.id), { origin: ORIGIN, actor: "test" });
    const row = await env.SHOP_DB.prepare(`SELECT payload_json FROM outbox WHERE kind = 'email'`)
      .first<{ payload_json: string }>();
    const mail = JSON.parse(row!.payload_json) as { subject: string; replyTo?: string };
    assert.match(mail.subject, /— Shop \(/);
    assert.equal(mail.replyTo, undefined);
  });

  it("queues nothing when the database cannot confirm this call moved the order", async () => {
    const { order } = await placeOrder();
    const quiet = sparse(/UPDATE orders SET status = 'paid'/);
    const result = await fulfilOrder(quiet, order, { origin: ORIGIN, actor: "test" });
    // Without a change count this call is not the one that moved it, so it
    // answers truthfully about the order but sends nobody anything: a second
    // email is worse than relying on the caller that did move it.
    assert.equal(result.changed, false);
    assert.equal(result.queued, 0);
    const queued = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM outbox`).first<{ n: number }>();
    assert.equal(queued?.n, 0);
  });

  it("calls no webhooks for a review while that module is off", async () => {
    const { order } = await placeOrder();
    await putSettings(env, {
      "modules.webhooks": false,
      "webhooks.endpoints": [{ url: "https://hooks.example.com/x", events: ["order.needs_review"], secret: "s" }],
    });
    await markNeedsReview(env, order, "amount mismatch", { origin: ORIGIN, actor: "test" });
    const hooks = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM outbox WHERE kind = 'webhook'`).first<{ n: number }>();
    assert.equal(hooks?.n, 0);
    assert.equal((await reload(order.id)).status, "needs_review");
  });
});

describe("webhook endpoints that were stored badly", () => {
  it("treats anything but a list as no endpoints at all", async () => {
    await putSettings(env, { "webhooks.endpoints": null });
    assert.deepEqual(await endpointsFor(env, "order.paid"), []);
  });
});

describe("the audit log", () => {
  it("records an entry that carries no detail", async () => {
    await audit(env, "admin:1", "settings.viewed", null);
    const row = await env.SHOP_DB.prepare(`SELECT detail_json FROM audit_log WHERE action = 'settings.viewed'`)
      .first<{ detail_json: string | null }>();
    assert.equal(row?.detail_json, null);
  });
});

describe("a database answer without metadata", () => {
  it("counts a spend, a revocation and a retry as not having happened", async () => {
    const { order, token } = await paidWithToken();
    const quiet = sparse(/^UPDATE (download_tokens|outbox)/);
    assert.equal(await spendUse(quiet, await sha256hex(token)), false);
    assert.equal(await revokeTokensForOrder(quiet, order.id), 0);
    const id = await enqueue(env, "email", "a@example.com", {});
    assert.equal(await retryOutbox(quiet, id), false);
  });

  it("issues tokens as though none existed when the lookup returns no rows", async () => {
    const { order, productId } = await placeOrder();
    const items = await getOrderItems(env, order.id);
    const product = (await env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`).bind(productId).first<ProductRow>())!;
    const quiet = sparse(/SELECT order_item_id FROM download_tokens/);
    const issued = await issueTokens(quiet, items as never, new Map([[productId, product]]));
    assert.equal(issued.length, 1);
  });

  it("drains nothing when the queue query returns no rows", async () => {
    await enqueue(env, "email", "a@example.com", {});
    const quiet = sparse(/SELECT \* FROM outbox/);
    assert.equal(await drainOutbox(quiet, 5, async () => {}), 0);
  });

  it("falls back to number one when the sequence returns no row", async () => {
    const quiet = sparse(/RETURNING next - 1/, { nullFirst: true });
    assert.match(await nextInvoiceNumber(quiet, "FV"), /^FV\/\d{4}\/000001$/);
  });
});

describe("invoice numbering when the owner cleared the formats", () => {
  it("uses the default series, format and credit series", async () => {
    await putSettings(env, { "invoice.format": "", "invoice.series": "", "invoice.credit_series": "" });
    const { order } = await placeOrder();
    const items = await getOrderItems(env, order.id);
    const invoice = await issueInvoice(env, order, items as never, null, "");
    assert.match(invoice.number, /^FV\/\d{4}\/\d{6}$/);
    const credit = await issueCreditNote(env, order, items as never, null, invoice, 2400, "");
    assert.match(credit.number, /^KOR\/\d{4}\/\d{6}$/);
    assert.equal(credit.corrects_id, invoice.id);
  });

  it("follows a format with an unpadded counter", async () => {
    await putSettings(env, { "invoice.format": "{series}-{year}-{n}" });
    assert.match(await nextInvoiceNumber(env, "INV"), /^INV-\d{4}-1$/);
  });
});

describe("an invoice in every shape", () => {
  it("shows a summary by rate, fractional rates, the registry and the buyer's VAT id", () => {
    const html = renderInvoiceHtml(invoiceRow(snapshot()));
    assert.ok(html.includes("5.5%"));
    assert.ok(html.includes("23%"));
    assert.ok(html.includes("KRS 0000000000"));
    assert.ok(html.includes("DE123456789"));
    // No gateway recorded: a dash, not "null".
    assert.ok(html.includes(": —"));
    assert.ok(!html.includes("null"));
  });

  it("names the invoice it corrects, in Polish, with the seller's VAT id and no registry", () => {
    const snap = snapshot({
      locale: "pl-PL",
      seller: { name: "Example Press", address: "1 Example Street", country: "PL", vatId: "PL1234567890", email: "", registry: "" },
      buyer: { name: "A Buyer", email: "buyer@example.com", country: "DE", vatId: "" },
      order: { number: "O-1", currency: "EUR", paidAt: "2026-11-14T10:00:00.000Z", gateway: "stripe", reverseCharge: false, taxCountry: "DE" },
      lines: [{ name: "A Book", sku: "EBOOK-1", quantity: 1, unitMinor: -2000, taxRateBp: 2300, taxMinor: -460, totalMinor: -2460 }],
      totals: { subtotalMinor: -2000, taxMinor: -460, totalMinor: -2460, byRate: [{ rateBp: 2300, netMinor: -2000, taxMinor: -460, grossMinor: -2460 }] },
    });
    const html = renderInvoiceHtml(invoiceRow(snap, "credit_note"), "FV/2026/000001");
    assert.ok(html.includes("Koryguje fakturę"));
    assert.ok(html.includes("PL1234567890"));
    assert.ok(!html.includes("Podsumowanie wg stawek"));
  });
});

describe("the buyer's invoice link", () => {
  it("serves a credit note with the number of the invoice it corrects", async () => {
    const { order, orderKey } = await placeOrder();
    const { invoiceNumber } = await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    const original = (await env.SHOP_DB.prepare(`SELECT * FROM invoices WHERE number = ?`).bind(invoiceNumber!).first<InvoiceRow>())!;
    const items = await getOrderItems(env, order.id);
    const credit = await issueCreditNote(env, await reload(order.id), items as never, null, original, 2400, "");

    const res = (await invoicePage(
      ctx(get(`/api/shop/invoices/${encodeURIComponent(credit.number)}?k=${orderKey}`), { number: credit.number }) as never,
    )) as Response;
    assert.equal(res.status, 200);
    assert.ok((await res.text()).includes(invoiceNumber!));
  });

  it("refuses a link with no number and no key", async () => {
    const res = (await invoicePage(ctx(get("/api/shop/invoices/")) as never)) as Response;
    assert.equal(res.status, 404);
  });

  it("refuses a known number without the order key", async () => {
    const { order } = await placeOrder();
    const { invoiceNumber } = await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    const res = (await invoicePage(
      ctx(get(`/api/shop/invoices/${encodeURIComponent(invoiceNumber!)}`), { number: invoiceNumber! }) as never,
    )) as Response;
    assert.equal(res.status, 404);
  });

  it("says what is wrong when the database is not bound", async () => {
    const res = (await invoicePage({ ...ctx(get("/api/shop/invoices/x"), { number: "x" }), env: { ...env, SHOP_DB: undefined } } as never)) as Response;
    assert.equal(res.status, 503);
  });
});

describe("downloading in the less common cases", () => {
  it("says what is wrong when the database is not bound", async () => {
    const res = (await download({ ...ctx(get("/api/shop/download/x"), { token: "x" }), env: { ...env, SHOP_DB: undefined } } as never)) as Response;
    assert.equal(res.status, 503);
  });

  it("refuses a request that names no token", async () => {
    const res = (await download(ctx(get("/api/shop/download/")) as never)) as Response;
    assert.equal(res.status, 400);
  });

  it("404s when the product's file has been detached", async () => {
    const { token, productId } = await paidWithToken();
    await env.SHOP_DB.prepare(`UPDATE products SET file_key = NULL WHERE id = ?`).bind(productId).run();
    const res = (await download(ctx(get(`/api/shop/download/${token}`), { token }) as never)) as Response;
    assert.equal(res.status, 404);
    assert.equal((await res.json() as { error: string }).error, "file_missing");
  });

  it("404s when the bucket no longer has the file", async () => {
    const { token, productId } = await paidWithToken();
    const product = (await env.SHOP_DB.prepare(`SELECT file_key FROM products WHERE id = ?`).bind(productId).first<{ file_key: string }>())!;
    await env.SHOP_FILES.delete(product.file_key);
    const res = (await download(ctx(get(`/api/shop/download/${token}`), { token }) as never)) as Response;
    assert.equal(res.status, 404);
  });

  it("refuses when the use cannot be recorded", async () => {
    const { token } = await paidWithToken();
    const res = (await download({
      ...ctx(get(`/api/shop/download/${token}`), { token }),
      env: sparse(/UPDATE download_tokens SET uses/),
    } as never)) as Response;
    assert.equal(res.status, 410);
  });

  it("names the file after the sku and sends bytes when the file has no name or type", async () => {
    const { token, productId } = await paidWithToken();
    const product = (await env.SHOP_DB.prepare(`SELECT file_key FROM products WHERE id = ?`).bind(productId).first<{ file_key: string }>())!;
    await env.SHOP_FILES.put(product.file_key, "%PDF-1.4\nx\n");
    await env.SHOP_DB.prepare(`UPDATE products SET file_name = NULL WHERE id = ?`).bind(productId).run();

    const context = ctx(get(`/api/shop/download/${token}`, { "cf-connecting-ip": "198.51.100.9" }), { token });
    const res = (await download(context as never)) as Response;
    assert.equal(res.status, 200);
    assert.ok(res.headers.get("content-disposition")!.includes('filename="EBOOK-1.bin"'));
    assert.equal(res.headers.get("content-type"), "application/octet-stream");
    await context.settled();
    // The log keeps the hashed address, never the address itself.
    const log = await env.SHOP_DB.prepare(`SELECT ip_hash FROM downloads`).first<{ ip_hash: string | null }>();
    assert.ok(log?.ip_hash && !log.ip_hash.includes("198.51.100.9"));
  });

  it("answers an open-ended range the bucket reports without a length or offset", async () => {
    const shapes: Array<{ offset?: number; length?: number }> = [{ offset: 2 }, { offset: undefined, length: 3 }];
    for (const range of shapes) {
      await freshShop();
      const { token } = await paidWithToken();
      const bucket = env.SHOP_FILES;
      const reporting = {
        ...bucket,
        get: async (key: string, opts?: unknown) => {
          const object = await bucket.get(key, opts as never);
          return object && { ...object, range };
        },
      };
      const res = (await download({
        ...ctx(get(`/api/shop/download/${token}`, { range: "bytes=2-" }), { token }),
        env: { ...env, SHOP_FILES: reporting },
      } as never)) as Response;
      assert.equal(res.status, 206);
      const size = 11;
      const offset = range.offset ?? 0;
      const length = range.length ?? size - offset;
      assert.equal(res.headers.get("content-range"), `bytes ${offset}-${offset + length - 1}/${size}`);
    }
  });
});

describe("resending links in the less common cases", () => {
  async function settle(c: ReturnType<typeof ctx>): Promise<void> {
    await c.settled();
  }

  it("does nothing for a buyer whose orders were never paid", async () => {
    await placeOrder();
    const context = ctx(post("/api/shop/resend", { email: "buyer@example.com" }));
    const res = (await resend(context as never)) as Response;
    await settle(context);
    assert.equal(res.status, 200);
    const queued = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM outbox`).first<{ n: number }>();
    assert.equal(queued?.n, 0);
  });

  it("writes in the default language and names the shop generically when those are unset", async () => {
    const { order } = await placeOrder();
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    await env.SHOP_DB.prepare(`DELETE FROM outbox`).run();
    await env.SHOP_DB.prepare(`UPDATE orders SET locale = NULL WHERE id = ?`).bind(order.id).run();
    await putSettings(env, { "shop.name": null });

    const context = ctx(post("/api/shop/resend", { email: "buyer@example.com" }));
    await resend(context as never);
    await settle(context);
    const row = await env.SHOP_DB.prepare(`SELECT payload_json FROM outbox WHERE kind = 'email'`).first<{ payload_json: string }>();
    assert.match(JSON.parse(row!.payload_json).subject, /— Shop \(/);
  });

  it("sends nothing when the order's lines cannot be read", async () => {
    const { order } = await placeOrder();
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    await env.SHOP_DB.prepare(`DELETE FROM outbox`).run();

    const context = { ...ctx(post("/api/shop/resend", { email: "buyer@example.com" })), env: sparse(/SELECT \* FROM order_items WHERE order_id/) };
    await resend(context as never);
    await context.settled();
    const queued = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM outbox`).first<{ n: number }>();
    assert.equal(queued?.n, 0);
  });

  it("finds nothing when the order query returns no rows", async () => {
    const { order } = await placeOrder();
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    await env.SHOP_DB.prepare(`DELETE FROM outbox`).run();

    const context = { ...ctx(post("/api/shop/resend", { email: "buyer@example.com" })), env: sparse(/SELECT \* FROM orders WHERE customer_id/) };
    await resend(context as never);
    await context.settled();
    const queued = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM outbox`).first<{ n: number }>();
    assert.equal(queued?.n, 0);
  });
});

describe("mail in every shape", () => {
  const base = {
    locale: "en", shopName: "Example Press", orderNumber: "O-1", totalMinor: 2400, currency: "EUR",
    downloads: [{ name: "A Book", url: "https://example.com/d/1", expiresAt: "2026-12-14T00:00:00.000Z", maxUses: 3 }],
  };

  it("sends the HTML and the reply-to address when the message has them, and leaves them out otherwise", async () => {
    const bodies: Record<string, unknown>[] = [];
    globalThis.fetch = (async (_url: unknown, init?: RequestInit) => {
      bodies.push(JSON.parse(String(init?.body)));
      return new Response("ok");
    }) as typeof fetch;
    const mailEnv = { ...env, SHOP_MAIL_URL: "https://mail.example.com/send", SHOP_MAIL_FROM: "shop@example.com" };
    await deliverEmail(mailEnv, "a@example.com", { subject: "s", text: "t", html: "<p>t</p>", replyTo: "help@example.com" });
    await deliverEmail(mailEnv, "a@example.com", { subject: "s", text: "t" });
    assert.equal(bodies[0]!.html, "<p>t</p>");
    assert.equal(bodies[0]!.reply_to, "help@example.com");
    assert.ok(!("html" in bodies[1]!));
    assert.ok(!("reply_to" in bodies[1]!));
  });

  it("leaves out the lost-link line when there is no resend page", () => {
    const mail = orderPaidMail(base);
    assert.ok(!mail.html!.includes("/shop/resend/"));
    assert.ok(!mail.text.includes("/shop/resend/"));
  });

  it("writes the resend and refund mails in Polish", () => {
    const pl = { ...base, locale: "pl" };
    assert.notEqual(resendMail(pl).subject, resendMail(base).subject);
    assert.notEqual(refundedMail(pl).subject, refundedMail(base).subject);
  });

  it("links the credit note in the refund mail only when there is one", () => {
    const withNote = refundedMail({ ...base, creditNoteUrl: "https://example.com/inv/KOR-1" });
    assert.ok(withNote.text.includes("https://example.com/inv/KOR-1"));
    assert.ok(!refundedMail(base).text.includes("https://"));
  });

  it("puts a dash where the owner's notice has no country or gateway", () => {
    const blank = adminNoticeMail("paid", { orderNumber: "O-1", totalMinor: 2400, currency: "EUR", country: null, gateway: null });
    assert.match(blank.text, /Country: {2}—/);
    assert.match(blank.text, /Gateway: {2}—/);
    const known = adminNoticeMail("paid", { orderNumber: "O-1", totalMinor: 2400, currency: "EUR", country: "DE", gateway: "stripe" });
    assert.match(known.text, /Country: {2}DE/);
    assert.match(known.text, /Gateway: {2}stripe/);
  });
});

describe("tracking a purchase with no address", () => {
  it("hashes an empty address rather than failing", async () => {
    let sent: { data: Array<{ user_data: { em: string[] } }> } | null = null;
    globalThis.fetch = (async (_url: unknown, init?: RequestInit) => {
      sent = JSON.parse(String(init?.body));
      return new Response("{}");
    }) as typeof fetch;
    await deliverTracking({ ...env, META_PIXEL_ID: "p", META_ACCESS_TOKEN: "t" }, "meta", {
      orderId: "o", orderNumber: "O-1", currency: "EUR", value: 24, tax: 0, items: [],
    });
    assert.equal(sent!.data[0]!.user_data.em[0], await sha256hex(""));
  });
});

describe("reading a request body", () => {
  it("reads a body that declares no type as JSON", async () => {
    const request = new Request(`${ORIGIN}/x`, { method: "POST", body: new Blob(['{"a":1}']) });
    assert.equal(request.headers.get("content-type"), null);
    assert.deepEqual(await readBody(request), { a: 1 });
  });
});
