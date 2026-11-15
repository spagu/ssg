// The owner's copy of an invoice (GET /api/shop/admin/invoices/:number): the
// same page the buyer's link serves, behind the admin gate in every mode —
// the shared password, Cloudflare Access and `auth` with ecommerce:read — and
// never the hash that opens the buyer's copy in the order's JSON.
import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as adminInvoice } from "../functions/api/shop/admin/invoices/[number]";
import { onRequestGet as adminOrder } from "../functions/api/shop/admin/orders/[id]";
import { onRequestGet as buyerInvoice } from "../functions/api/shop/invoices/[number]";
import type { InvoiceSnapshot } from "../functions/api/shop/_invoice";
import { ensureSchema } from "../functions/api/shop/_schema";
import { ACCESS_VARS, AUTH_VARS, ORIGIN, PASSWORD, ask, serveAccessKeys, shopEnv, throughGate, type Ask } from "./admin-fixtures";

const NUMBER = "FV/2026/000007";
const PATH = `/api/shop/admin/invoices/${encodeURIComponent(NUMBER)}`;
let restore: () => void = () => {};
afterEach(() => restore());

const snapshot: InvoiceSnapshot = {
  seller: { name: "Books Ltd", address: "1 Example Street", country: "DE", vatId: "DE123", email: "shop@example.com", registry: "" },
  buyer: { name: "A <Buyer>", email: "buyer@example.com", country: "DE", vatId: "" },
  order: { number: "ORD-7", currency: "EUR", paidAt: "2026-11-01T10:00:00.000Z", gateway: "stripe", reverseCharge: false, taxCountry: "DE" },
  lines: [{ name: "An ebook", sku: "EBOOK-1", quantity: 1, unitMinor: 2243, taxRateBp: 700, taxMinor: 157, totalMinor: 2400 }],
  totals: { subtotalMinor: 2243, taxMinor: 157, totalMinor: 2400, byRate: [{ rateBp: 700, netMinor: 2243, taxMinor: 157, grossMinor: 2400 }] },
  note: "",
  locale: "en",
};

/** A shop in the given mode holding one order with one invoice and one credit
 *  note correcting it. */
async function shopWithInvoice(vars: Record<string, unknown>) {
  const env = shopEnv(vars);
  await ensureSchema(env);
  const now = "2026-11-01T10:00:00.000Z";
  await env.SHOP_DB.prepare(
    `INSERT INTO orders (id, number, key_hash, status, currency, subtotal_minor, tax_minor, total_minor, created_at, updated_at)
     VALUES ('o-7', 'ORD-7', 'not-a-real-hash', 'paid', 'EUR', 2243, 157, 2400, ?, ?)`,
  ).bind(now, now).run();
  const insert = `INSERT INTO invoices (id, number, kind, order_id, corrects_id, issued_at, currency, total_minor, snapshot_json, created_at, link_hash)
                  VALUES (?, ?, ?, 'o-7', ?, ?, 'EUR', ?, ?, ?, ?)`;
  await env.SHOP_DB.prepare(insert).bind("i-7", NUMBER, "invoice", null, now, 2400, JSON.stringify(snapshot), now, "f".repeat(64)).run();
  await env.SHOP_DB.prepare(insert).bind("c-7", "KOR/2026/000001", "credit_note", "i-7", now, -2400, JSON.stringify(snapshot), now, null).run();
  return env;
}

async function open(env: ReturnType<typeof shopEnv>, a: Ask, path = PATH): Promise<Response> {
  const number = path.split("/").pop()!;
  const { res } = await throughGate(await ask(path, a), env, (ctx) => adminInvoice(ctx as never) as Promise<Response>, { number });
  return res;
}

async function assertInvoicePage(res: Response): Promise<string> {
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("content-type"), "text/html; charset=utf-8");
  assert.equal(res.headers.get("cache-control"), "private, no-store");
  assert.match(res.headers.get("content-security-policy") ?? "", /default-src 'none'/);
  const html = await res.text();
  assert.ok(html.includes(NUMBER));
  assert.ok(html.includes("A &lt;Buyer&gt;"), "buyer text stays escaped");
  return html;
}

describe("the owner's copy of an invoice", () => {
  it("the shared password opens it; no password or a wrong one does not", async () => {
    const env = await shopWithInvoice({ SHOP_ADMIN_PASSWORD: PASSWORD });
    await assertInvoicePage(await open(env, { password: PASSWORD }));
    assert.equal((await open(env, {})).status, 401);
    assert.equal((await open(env, { password: "wrong" })).status, 401);
  });

  it("Cloudflare Access opens it", async () => {
    restore = serveAccessKeys();
    const env = await shopWithInvoice(ACCESS_VARS);
    await assertInvoicePage(await open(env, { access: true }));
    assert.equal((await open(env, {})).status, 401);
  });

  it("auth: ecommerce:read opens it, another permission is a 403 naming it, no session goes to the login", async () => {
    const env = await shopWithInvoice(AUTH_VARS);
    await assertInvoicePage(await open(env, { perms: ["ecommerce:read"] }));
    await assertInvoicePage(await open(env, { perms: ["ecommerce:*"] }));
    const denied = await open(env, { perms: ["ecommerce:orders"] });
    assert.equal(denied.status, 403);
    assert.equal(((await denied.json()) as { permission: string }).permission, "ecommerce:read");
    const page = await open(env, { headers: { accept: "text/html" } });
    assert.equal(page.status, 302);
    assert.ok(page.headers.get("location")?.startsWith(`${ORIGIN}/auth/login?next=`));
  });

  it("serves a credit note with the invoice it corrects, and a 404 for a number that does not exist", async () => {
    const env = await shopWithInvoice({ SHOP_ADMIN_PASSWORD: PASSWORD });
    const credit = await open(env, { password: PASSWORD }, `/api/shop/admin/invoices/${encodeURIComponent("KOR/2026/000001")}`);
    assert.equal(credit.status, 200);
    assert.ok((await credit.text()).includes(NUMBER));
    const missing = await open(env, { password: PASSWORD }, "/api/shop/admin/invoices/FV%2F1999%2F000001");
    assert.equal(missing.status, 404);
    assert.equal(((await missing.json()) as { error: string }).error, "not_found");
  });

  it("is the buyer's page byte for byte", async () => {
    const env = await shopWithInvoice({ SHOP_ADMIN_PASSWORD: PASSWORD });
    const owner = await (await open(env, { password: PASSWORD })).text();
    // The order key is not known here, so the buyer's door is opened by
    // matching the stored order hash; the page itself is what is compared.
    await env.SHOP_DB.prepare(`UPDATE orders SET key_hash = ? WHERE id = 'o-7'`)
      .bind("5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5") // sha256("12345")
      .run();
    const buyer = (await buyerInvoice({
      request: new Request(`${ORIGIN}/api/shop/invoices/${encodeURIComponent(NUMBER)}?k=12345`),
      env,
      params: { number: encodeURIComponent(NUMBER) },
    } as never)) as Response;
    assert.equal(await buyer.text(), owner);
  });

  it("the order's JSON never carries the invoice's link hash", async () => {
    const env = await shopWithInvoice({ SHOP_ADMIN_PASSWORD: PASSWORD });
    const { res } = await throughGate(
      await ask("/api/shop/admin/orders/o-7", { password: PASSWORD }),
      env,
      (ctx) => adminOrder(ctx as never) as Promise<Response>,
      { id: "o-7" },
    );
    const text = await res.text();
    assert.equal(res.status, 200);
    assert.ok(text.includes(NUMBER));
    assert.ok(!text.includes("link_hash") && !text.includes("f".repeat(64)));
  });
});
