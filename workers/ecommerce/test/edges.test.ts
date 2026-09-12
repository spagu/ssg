// The branches that only run when something is unusual: a shop missing a
// binding, a queue that is stuck, a buyer with a VAT number, a second page of
// results. Each of these is a path a real shop takes eventually.

import { env } from "cloudflare:test";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { onRequest as adminGate } from "../functions/api/shop/admin/_middleware";
import { onRequestPost as login } from "../functions/api/shop/admin/auth/login";
import { onRequestPost as logout } from "../functions/api/shop/admin/auth/logout";
import { onRequestPost as refreshSession } from "../functions/api/shop/admin/auth/refresh";
import { onRequestGet as me } from "../functions/api/shop/admin/me";
import { onRequestGet as stats } from "../functions/api/shop/admin/stats";
import { onRequestGet as auditLog } from "../functions/api/shop/admin/audit";
import { onRequestGet as listOutbox } from "../functions/api/shop/admin/outbox";
import { onRequestGet as listOrders } from "../functions/api/shop/admin/orders/index";
import { onRequestGet as listRates } from "../functions/api/shop/admin/vat-rates";
import { onRequestPost as markPaid } from "../functions/api/shop/admin/orders/[id]/mark-paid";
import { onRequestPost as cancelOrder } from "../functions/api/shop/admin/orders/[id]/cancel";
import { onRequestPost as adminResend } from "../functions/api/shop/admin/orders/[id]/resend";
import { onRequestPost as adminRefund } from "../functions/api/shop/admin/orders/[id]/refund";
import { onRequestPost as uploadFile } from "../functions/api/shop/admin/products/[id]/file";
import { onRequestPost as checkout } from "../functions/api/shop/checkout";
import { onRequestPost as publicResend } from "../functions/api/shop/resend";
import { audit } from "../functions/api/shop/_audit";
import { signJwt } from "../functions/api/shop/_auth";
import { drainOutbox, enqueue } from "../functions/api/shop/_outbox";
import { deliverWebhook } from "../functions/api/shop/_hooks";
import { ensureSchema, resetSchemaCache } from "../functions/api/shop/_schema";
import { putSettings } from "../functions/api/shop/_settings";
import type { AdminIdentity, AdminUserRow, OrderRow } from "../functions/api/shop/_types";
import { bodyOf, ctx, freshShop, get, ORIGIN, post, seedOwner, seedProduct } from "./_helpers";

const owner: AdminIdentity = { sub: "owner-1", email: "owner@example.com", role: "owner", via: "jwt" };
const shopEnv = () => ({ ...env, STRIPE_SECRET_KEY: "sk_test_x", SHOP_GATEWAYS: "stripe" });
const realFetch = globalThis.fetch;

function asAdmin(request: Request, params: Record<string, string> = {}, overrides: Record<string, unknown> = {}) {
  return { ...ctx(request, params, { admin: owner }), env: { ...shopEnv(), ...overrides } };
}

let session = 0;

async function buy(sku = "EBOOK-1"): Promise<OrderRow> {
  await seedProduct({ sku, priceMinor: 2400 });
  // A distinct session id per purchase: two orders sharing one would be
  // refused by the unique index on (gateway, gateway_ref), which is the point
  // of that index.
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ id: `cs_${++session}`, url: "https://checkout.stripe.com/x" }))) as typeof fetch;
  const res = (await checkout({
    ...ctx(post("/api/shop/checkout", {
      items: [{ sku, quantity: 1 }], gateway: "stripe",
      email: "buyer@example.com", country: "DE", consentWaiver: "1",
    })),
    env: shopEnv(),
  } as never)) as Response;
  const { orderId } = await bodyOf<{ orderId: string }>(res);
  return (await env.SHOP_DB.prepare(`SELECT * FROM orders WHERE id = ?`).bind(orderId).first<OrderRow>())!;
}

beforeEach(async () => {
  await freshShop();
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("the overview when everything is missing", () => {
  it("names each missing piece separately", async () => {
    await env.SHOP_DB.prepare(`DELETE FROM settings`).run();
    await env.SHOP_DB.prepare(`UPDATE vat_rates SET valid_from = '2020-01-01T00:00:00.000Z'`).run();
    const { invalidateSettings } = await import("../functions/api/shop/_settings");
    invalidateSettings();

    // A failed message that nobody has looked at is the most urgent of these:
    // it is usually a buyer without their book.
    const id = await enqueue(env, "email", "buyer@example.com", {});
    await env.SHOP_DB.prepare(`UPDATE outbox SET attempts = 7 WHERE id = ?`).bind(id).run();

    const res = (await me(
      asAdmin(get("/api/shop/admin/me"), {}, {
        SHOP_FILES: undefined,
        SHOP_KV: undefined,
        SHOP_IP_SALT: undefined,
        SHOP_ADMIN_BOOTSTRAP: "owner@example.com:still-here",
        STRIPE_SECRET_KEY: undefined,
        PAYPAL_CLIENT_ID: undefined,
      }) as never,
    )) as Response;

    const { warnings } = await bodyOf<{ warnings: string[] }>(res);
    const all = warnings.join("\n");
    expect(all).toMatch(/Settings still missing/);
    expect(all).toMatch(/No payment provider/);
    expect(all).toMatch(/No file storage/);
    expect(all).toMatch(/No KV namespace/);
    expect(all).toMatch(/SHOP_IP_SALT/);
    expect(all).toMatch(/SHOP_ADMIN_BOOTSTRAP is still set/);
    expect(all).toMatch(/VAT rates were last updated over a year ago/);
    expect(all).toMatch(/queued message/);
  });

  it("says nothing is missing when nothing is", async () => {
    const res = (await me(
      asAdmin(get("/api/shop/admin/me"), {}, {
        SHOP_MAIL_URL: "https://mail.example/send",
        SHOP_MAIL_FROM: "shop@example.com",
      }) as never,
    )) as Response;
    const { warnings } = await bodyOf<{ warnings: string[] }>(res);
    expect(warnings.join("\n")).not.toMatch(/Email is not configured/);
  });
});

describe("reports with nothing in them", () => {
  it("answers with empty figures rather than an error", async () => {
    const res = (await stats(asAdmin(get("/api/shop/admin/stats?window=7d")) as never)) as Response;
    const body = await bodyOf<{ byCurrency: unknown[]; topProducts: unknown[]; byCountry: unknown[] }>(res);
    expect(body.byCurrency).toEqual([]);
    expect(body.topProducts).toEqual([]);
    expect(body.byCountry).toEqual([]);
  });

  it("subtracts refunds from the money it reports", async () => {
    const order = await buy();
    const { fulfilOrder } = await import("../functions/api/shop/_fulfil");
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    await env.SHOP_DB.prepare(
      `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, created_at)
       VALUES ('r1', ?, 'stripe', 're_1', 'refund', 'succeeded', -1000, 'EUR', ?)`,
    )
      .bind(order.id, new Date().toISOString())
      .run();

    const res = (await stats(asAdmin(get("/api/shop/admin/stats?window=365d")) as never)) as Response;
    const body = await bodyOf<{ byCurrency: Array<{ refundedMinor: number; netMinor: number }> }>(res);
    expect(body.byCurrency[0]).toMatchObject({ refundedMinor: 1000, netMinor: 1400 });
  });
});

describe("listings with filters and pages", () => {
  it("filters the log by subject and pages through it", async () => {
    for (let i = 0; i < 3; i++) await audit(env, "admin:1", "test.event", "subject-a", { i });
    await audit(env, "admin:1", "test.event", "subject-b", {});

    const filtered = (await auditLog(asAdmin(get("/api/shop/admin/audit?subject=subject-a")) as never)) as Response;
    expect((await bodyOf<{ entries: unknown[] }>(filtered)).entries).toHaveLength(3);

    const firstPage = (await auditLog(asAdmin(get("/api/shop/admin/audit?limit=2")) as never)) as Response;
    const page = await bodyOf<{ entries: unknown[]; nextCursor: string | null }>(firstPage);
    expect(page.entries).toHaveLength(2);
    expect(page.nextCursor).not.toBe(null);

    const secondPage = (await auditLog(
      asAdmin(get(`/api/shop/admin/audit?limit=2&cursor=${encodeURIComponent(page.nextCursor!)}`)) as never,
    )) as Response;
    expect((await bodyOf<{ entries: unknown[] }>(secondPage)).entries.length).toBeGreaterThan(0);
  });

  it("keeps a detail it cannot parse out of the way", async () => {
    await env.SHOP_DB.prepare(
      `INSERT INTO audit_log (id, actor, action, subject, detail_json, created_at) VALUES ('x', 'a', 'b', NULL, 'not json', ?)`,
    )
      .bind(new Date().toISOString())
      .run();
    const res = (await auditLog(asAdmin(get("/api/shop/admin/audit")) as never)) as Response;
    expect((await bodyOf<{ entries: Array<{ detail: unknown }> }>(res)).entries[0]?.detail).toBe(null);
  });

  it("pages through orders", async () => {
    await buy("EBOOK-1");
    await buy("EBOOK-2");

    const page = (await listOrders(asAdmin(get("/api/shop/admin/orders?limit=1")) as never)) as Response;
    const body = await bodyOf<{ orders: unknown[]; nextCursor: string | null }>(page);
    expect(body.orders).toHaveLength(1);
    expect(body.nextCursor).not.toBe(null);

    const next = (await listOrders(
      asAdmin(get(`/api/shop/admin/orders?limit=1&cursor=${encodeURIComponent(body.nextCursor!)}`)) as never,
    )) as Response;
    expect((await bodyOf<{ orders: unknown[] }>(next)).orders).toHaveLength(1);
  });

  it("finds an order by its number as well as by email", async () => {
    const order = await buy();
    const res = (await listOrders(
      asAdmin(get(`/api/shop/admin/orders?q=${encodeURIComponent(order.number.toLowerCase())}`)) as never,
    )) as Response;
    expect((await bodyOf<{ orders: unknown[] }>(res)).orders).toHaveLength(1);
  });

  it("lists every rate when no country is named", async () => {
    const res = (await listRates(asAdmin(get("/api/shop/admin/vat-rates")) as never)) as Response;
    expect((await bodyOf<{ rates: unknown[] }>(res)).rates.length).toBeGreaterThan(40);
  });

  it("lists only the stuck messages when asked", async () => {
    const stuck = await enqueue(env, "email", "a@example.com", {});
    await enqueue(env, "email", "b@example.com", {});
    await env.SHOP_DB.prepare(`UPDATE outbox SET attempts = 5 WHERE id = ?`).bind(stuck).run();

    const res = (await listOutbox(asAdmin(get("/api/shop/admin/outbox?stuck=1")) as never)) as Response;
    const body = await bodyOf<{ messages: unknown[]; counts: { stuck: number; pending: number } }>(res);
    expect(body.messages).toHaveLength(1);
    expect(body.counts).toMatchObject({ stuck: 1, pending: 2 });
  });
});

describe("orders that cannot be acted on", () => {
  it("refuses to act on an order that does not exist", async () => {
    for (const handler of [markPaid, cancelOrder, adminResend, adminRefund]) {
      const res = (await handler(asAdmin(post("/x", {}), { id: "no-such-order" }) as never)) as Response;
      expect(res.status).toBe(404);
    }
  });

  it("refuses a refund amount that is not an amount", async () => {
    const order = await buy();
    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'paid' WHERE id = ?`).bind(order.id).run();
    await env.SHOP_DB.prepare(
      `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, created_at)
       VALUES ('p1', ?, 'stripe', 'pi_1', 'charge', 'succeeded', 2400, 'EUR', ?)`,
    )
      .bind(order.id, new Date().toISOString())
      .run();

    const res = (await adminRefund(asAdmin(post("/x", { amountMinor: -5 }), { id: order.id }) as never)) as Response;
    expect(res.status).toBe(422);
  });

  it("refuses a refund on an order with no payment recorded", async () => {
    const order = await buy();
    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'paid' WHERE id = ?`).bind(order.id).run();
    const res = (await adminRefund(asAdmin(post("/x", {}), { id: order.id }) as never)) as Response;
    expect(await bodyOf<{ error: string }>(res)).toMatchObject({ error: "no_payment" });
  });

  it("says so when the provider for an order is no longer configured", async () => {
    const order = await buy();
    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'paid' WHERE id = ?`).bind(order.id).run();
    await env.SHOP_DB.prepare(
      `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, created_at)
       VALUES ('p1', ?, 'stripe', 'pi_1', 'charge', 'succeeded', 2400, 'EUR', ?)`,
    )
      .bind(order.id, new Date().toISOString())
      .run();

    const res = (await adminRefund(
      asAdmin(post("/x", {}), { id: order.id }, { STRIPE_SECRET_KEY: undefined, SHOP_GATEWAYS: "" }) as never,
    )) as Response;
    expect(res.status).toBe(503);
  });

  it("has nothing to resend when no line has a file", async () => {
    await seedProduct({ sku: "NOFILE", withFile: false });
    globalThis.fetch = (async () =>
      new Response(JSON.stringify({ id: "cs_2", url: "https://x" }))) as typeof fetch;
    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: [{ sku: "NOFILE", quantity: 1 }], gateway: "stripe",
        email: "buyer@example.com", country: "DE", consentWaiver: "1",
      })),
      env: shopEnv(),
    } as never)) as Response;
    const { orderId } = await bodyOf<{ orderId: string }>(res);
    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'paid' WHERE id = ?`).bind(orderId).run();

    const resent = (await adminResend(asAdmin(post("/x", {}), { id: orderId }) as never)) as Response;
    expect(await bodyOf<{ error: string }>(resent)).toMatchObject({ error: "nothing_to_send" });
  });

  it("cannot resend to an order with no customer on it", async () => {
    const order = await buy();
    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'paid', customer_id = NULL WHERE id = ?`)
      .bind(order.id)
      .run();
    const res = (await adminResend(asAdmin(post("/x", {}), { id: order.id }) as never)) as Response;
    expect(await bodyOf<{ error: string }>(res)).toMatchObject({ error: "no_customer" });
  });
});

describe("uploading a file when things are not set up", () => {
  it("says so when there is nowhere to put it", async () => {
    const { id } = await seedProduct({ sku: "EBOOK-1", withFile: false });
    const form = new FormData();
    form.append("file", new File([new TextEncoder().encode("%PDF-1.4\n")], "b.pdf"));
    const res = (await uploadFile({
      ...ctx(new Request(`${ORIGIN}/x`, { method: "POST", body: form }), { id }, { admin: owner }),
      env: { ...shopEnv(), SHOP_FILES: undefined },
    } as never)) as Response;
    expect(res.status).toBe(503);
  });

  it("refuses a request with no file in it, and one that is not a form", async () => {
    const { id } = await seedProduct({ sku: "EBOOK-1", withFile: false });

    const empty = (await uploadFile({
      ...ctx(new Request(`${ORIGIN}/x`, { method: "POST", body: new FormData() }), { id }, { admin: owner }),
      env: shopEnv(),
    } as never)) as Response;
    expect(empty.status).toBe(422);

    const notForm = (await uploadFile({
      ...ctx(new Request(`${ORIGIN}/x`, { method: "POST", headers: { "content-type": "application/json" }, body: "{}" }), { id }, { admin: owner }),
      env: shopEnv(),
    } as never)) as Response;
    expect(notForm.status).toBe(400);
  });

  it("404s for a product that is not there", async () => {
    const form = new FormData();
    form.append("file", new File([new TextEncoder().encode("%PDF-1.4\n")], "b.pdf"));
    const res = (await uploadFile({
      ...ctx(new Request(`${ORIGIN}/x`, { method: "POST", body: form }), { id: "nope" }, { admin: owner }),
      env: shopEnv(),
    } as never)) as Response;
    expect(res.status).toBe(404);
  });

  it("accepts an EPUB, and keeps the old file while a live link still points at it", async () => {
    const { id } = await seedProduct({ sku: "EBOOK-1", withFile: false });

    // A zip whose mimetype entry sits where a conforming EPUB puts it.
    const epub = new Uint8Array(120);
    epub.set([0x50, 0x4b, 0x03, 0x04], 0);
    epub.set(new TextEncoder().encode("application/epub+zip"), 30);
    const form = new FormData();
    form.append("file", new File([epub], "book.epub"));

    const res = (await uploadFile({
      ...ctx(new Request(`${ORIGIN}/x`, { method: "POST", body: form }), { id }, { admin: owner }),
      env: shopEnv(),
    } as never)) as Response;
    expect(await bodyOf<{ contentType: string }>(res)).toMatchObject({ contentType: "application/epub+zip" });
  });
});

describe("signing in when the shop is half-configured", () => {
  it("refuses a login without an email and password", async () => {
    const res = (await login({
      ...ctx(post("/api/shop/admin/auth/login", { email: "", password: "" })),
      env: shopEnv(),
    } as never)) as Response;
    expect(res.status).toBe(422);
  });

  it("locks an account after a run of failures", async () => {
    await seedOwner("owner@example.com", "the-right-password");
    for (let i = 0; i < 12; i++) {
      await login({
        ...ctx(post("/api/shop/admin/auth/login", { email: "owner@example.com", password: "wrong" })),
        env: shopEnv(),
      } as never);
    }
    const res = (await login({
      ...ctx(post("/api/shop/admin/auth/login", { email: "owner@example.com", password: "the-right-password" })),
      env: shopEnv(),
    } as never)) as Response;
    expect(res.status).toBe(429);
  });

  it("signs out cleanly with no session at all", async () => {
    const res = (await logout({
      ...ctx(new Request(`${ORIGIN}/api/shop/admin/auth/logout`, { method: "POST" })),
      env: shopEnv(),
    } as never)) as Response;
    expect(res.status).toBe(200);
  });

  it("denies the access token's id on sign-out", async () => {
    await seedOwner();
    const user = (await env.SHOP_DB.prepare(`SELECT * FROM admin_users LIMIT 1`).first<AdminUserRow>())!;
    const token = await signJwt(env, user, ORIGIN);

    await logout({
      ...ctx(new Request(`${ORIGIN}/api/shop/admin/auth/logout`, {
        method: "POST",
        headers: { authorization: `Bearer ${token}` },
      })),
      env: shopEnv(),
    } as never);

    const { verifyJwt } = await import("../functions/api/shop/_auth");
    expect(await verifyJwt(env, token, ORIGIN)).toBe(null);
  });

  it("refuses a refresh with no cookie", async () => {
    const res = (await refreshSession({
      ...ctx(new Request(`${ORIGIN}/api/shop/admin/auth/refresh`, { method: "POST", headers: { origin: ORIGIN } })),
      env: shopEnv(),
    } as never)) as Response;
    expect(res.status).toBe(401);
  });
});

describe("the gate in its other modes", () => {
  it("lets a request through once the identity is established", async () => {
    await seedOwner();
    const user = (await env.SHOP_DB.prepare(`SELECT * FROM admin_users LIMIT 1`).first<AdminUserRow>())!;
    const token = await signJwt(env, user, ORIGIN);
    const context = {
      ...ctx(new Request(`${ORIGIN}/api/shop/admin/me`, { headers: { authorization: `Bearer ${token}` } })),
      env: shopEnv(),
      functionPath: "/api/shop/admin/me",
    };
    const res = (await adminGate(context as never)) as Response;
    expect(await res.text()).toBe("next");
    // …and it took the chance to push the queue along.
    await (context as unknown as { settled(): Promise<unknown> }).settled();
  });
});

describe("checkout's remaining paths", () => {
  it("accepts a well-formed VAT number without treating it as verified", async () => {
    await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
    globalThis.fetch = (async () =>
      new Response(JSON.stringify({ id: "cs_3", url: "https://x" }))) as typeof fetch;

    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: [{ sku: "EBOOK-1", quantity: 1 }], gateway: "stripe",
        email: "buyer@example.com", country: "DE", vatId: "DE 123 456 789", consentWaiver: "1",
      })),
      env: shopEnv(),
    } as never)) as Response;
    const { orderId } = await bodyOf<{ orderId: string }>(res);

    const order = await env.SHOP_DB.prepare(`SELECT reverse_charge, tax_minor FROM orders WHERE id = ?`)
      .bind(orderId)
      .first<{ reverse_charge: number; tax_minor: number }>();
    // Only VIES can say a number is real; until then the seller carries the tax.
    expect(order).toMatchObject({ reverse_charge: 0, tax_minor: 157 });
  });

  it("refuses a VAT number that is not well formed", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: [{ sku: "EBOOK-1", quantity: 1 }], gateway: "stripe",
        email: "buyer@example.com", country: "DE", vatId: "DE1", consentWaiver: "1",
      })),
      env: shopEnv(),
    } as never)) as Response;
    expect(await bodyOf<{ error: string }>(res)).toMatchObject({ error: "invalid_vat_id" });
  });

  it("prices in net mode as well as gross", async () => {
    await putSettings(env, { "pricing.mode": "net" });
    await seedProduct({ sku: "EBOOK-1", priceMinor: 2000 });
    globalThis.fetch = (async () =>
      new Response(JSON.stringify({ id: "cs_4", url: "https://x" }))) as typeof fetch;

    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: [{ sku: "EBOOK-1", quantity: 1 }], gateway: "stripe",
        email: "buyer@example.com", country: "DE", consentWaiver: "1",
      })),
      env: shopEnv(),
    } as never)) as Response;
    const { orderId } = await bodyOf<{ orderId: string }>(res);
    const order = await env.SHOP_DB.prepare(`SELECT subtotal_minor, tax_minor, total_minor FROM orders WHERE id = ?`)
      .bind(orderId)
      .first<{ subtotal_minor: number; tax_minor: number; total_minor: number }>();
    // 20.00 net plus 7% = 21.40, rather than 20.00 with the tax carved out.
    expect(order).toEqual({ subtotal_minor: 2000, tax_minor: 140, total_minor: 2140 });
  });

  it("does not ask for a waiver when the shop has turned it off", async () => {
    await putSettings(env, { "legal.digital_waiver": false });
    await seedProduct({ sku: "EBOOK-1" });
    globalThis.fetch = (async () =>
      new Response(JSON.stringify({ id: "cs_5", url: "https://x" }))) as typeof fetch;

    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: [{ sku: "EBOOK-1", quantity: 1 }], gateway: "stripe",
        email: "buyer@example.com", country: "DE",
      })),
      env: shopEnv(),
    } as never)) as Response;
    expect(res.status).toBe(200);
  });

  it("says so when no provider is configured at all", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: [{ sku: "EBOOK-1", quantity: 1 }],
        email: "buyer@example.com", country: "DE", consentWaiver: "1",
      })),
      env: { ...env, STRIPE_SECRET_KEY: undefined, PAYPAL_CLIENT_ID: undefined },
    } as never)) as Response;
    expect(res.status).toBe(503);
  });

  it("says what is wrong when the database is not bound", async () => {
    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", {})),
      env: { ...shopEnv(), SHOP_DB: undefined },
    } as never)) as Response;
    expect(res.status).toBe(503);
  });
});

describe("the public resend in its other shapes", () => {
  it("looks up one order by number when given one", async () => {
    const order = await buy();
    const { fulfilOrder } = await import("../functions/api/shop/_fulfil");
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    await env.SHOP_DB.prepare(`DELETE FROM outbox`).run();

    const context = ctx(post("/api/shop/resend", { email: "buyer@example.com", orderNumber: order.number }));
    await publicResend(context as never);
    await (context as unknown as { settled(): Promise<unknown> }).settled();

    const queued = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM outbox`).first<{ n: number }>();
    expect(queued?.n).toBe(1);
  });

  it("keeps the existing links when the shop says not to renew them", async () => {
    await putSettings(env, { "downloads.renew_on_resend": false });
    const order = await buy();
    const { fulfilOrder } = await import("../functions/api/shop/_fulfil");
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });
    await env.SHOP_DB.prepare(`DELETE FROM outbox`).run();

    const context = ctx(post("/api/shop/resend", { email: "buyer@example.com" }));
    await publicResend(context as never);
    await (context as unknown as { settled(): Promise<unknown> }).settled();

    // Nothing new to send: the live token's plaintext was never kept, so a
    // shop that will not renew has nothing it can put in an email.
    const live = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM download_tokens WHERE revoked = 0`)
      .first<{ n: number }>();
    expect(live?.n).toBe(1);
  });

  it("says what is wrong when the database is not bound", async () => {
    const res = (await publicResend({
      ...ctx(post("/api/shop/resend", { email: "a@example.com" })),
      env: { ...env, SHOP_DB: undefined },
    } as never)) as Response;
    expect(res.status).toBe(503);
  });

  it("refuses a body it cannot read", async () => {
    const request = new Request(`${ORIGIN}/api/shop/resend`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: "{oh no",
    });
    const res = (await publicResend({ ...ctx(request), env: shopEnv() } as never)) as Response;
    expect(res.status).toBe(400);
  });

  it("refuses when the anti-spam check does not pass", async () => {
    globalThis.fetch = (async () => new Response(JSON.stringify({ success: false }))) as typeof fetch;
    const res = (await publicResend({
      ...ctx(post("/api/shop/resend", { email: "a@example.com", turnstileToken: "bad" })),
      env: { ...shopEnv(), TURNSTILE_SECRET: "secret" },
    } as never)) as Response;
    expect(res.status).toBe(403);
  });
});

describe("the queue's remaining paths", () => {
  it("stops backing off further once it has reached the longest wait", async () => {
    const id = await enqueue(env, "email", "a@example.com", {});
    await env.SHOP_DB.prepare(`UPDATE outbox SET attempts = 9 WHERE id = ?`).bind(id).run();

    await drainOutbox(env, 5, async () => {
      throw new Error("still failing");
    });
    const row = await env.SHOP_DB.prepare(`SELECT attempts, next_attempt FROM outbox WHERE id = ?`)
      .bind(id)
      .first<{ attempts: number; next_attempt: string }>();
    expect(row?.attempts).toBe(10);
    // A day, not a week: the longest wait in the table is the longest wait.
    const waitHours = (Date.parse(row!.next_attempt) - Date.now()) / 3_600_000;
    expect(waitHours).toBeGreaterThan(23);
    expect(waitHours).toBeLessThan(25);
  });

  it("does nothing when the queue is empty", async () => {
    expect(await drainOutbox(env, 5, async () => {})).toBe(0);
  });
});

describe("outgoing webhooks, refused", () => {
  it("reports a recipient that answers with an error", async () => {
    globalThis.fetch = (async () => new Response("nope", { status: 500 })) as typeof fetch;
    await expect(deliverWebhook(env, "https://listener.example/hook", {})).rejects.toThrow();
  });
});

describe("the schema after a failure", () => {
  it("retries rather than inheriting a permanent error", async () => {
    resetSchemaCache();
    // A database that is not there at all: the first call fails…
    await expect(ensureSchema({ ...env, SHOP_DB: undefined as never })).rejects.toThrow();
    // …and the next one, against a real database, still works.
    await expect(ensureSchema(env)).resolves.toBeUndefined();
  });
});

describe("the last few corners", () => {
  it("refuses a refresh token that has expired", async () => {
    const { createRefresh, consumeRefresh } = await import("../functions/api/shop/_auth");
    const id = await seedOwner("expired@example.com");
    const token = await createRefresh(env, id);
    await env.SHOP_DB.prepare(`UPDATE admin_sessions SET expires_at = '2020-01-01T00:00:00.000Z'`).run();

    expect((await consumeRefresh(env, token)).ok).toBe(false);
    // The row is swept as it is refused, so an expired session does not sit in
    // the table forever.
    const left = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM admin_sessions`).first<{ n: number }>();
    expect(left?.n).toBe(0);
  });

  it("treats two requests racing with one refresh token as a replay", async () => {
    const { createRefresh, consumeRefresh } = await import("../functions/api/shop/_auth");
    const id = await seedOwner("racer@example.com");
    const token = await createRefresh(env, id);

    // Both arrive before either has written; only one can spend it, and the
    // other is indistinguishable from a thief.
    const [a, b] = await Promise.all([consumeRefresh(env, token), consumeRefresh(env, token)]);
    expect([a.ok, b.ok].filter(Boolean)).toHaveLength(1);
  });

  it("refuses a webhook url that is not a url at all", async () => {
    await expect(deliverWebhook(env, "not a url", {})).rejects.toThrow(/invalid webhook url/);
  });

  it("sends each kind of queued message through the right module", async () => {
    const sent: string[] = [];
    globalThis.fetch = (async (url: unknown) => {
      sent.push(String(url));
      return new Response("{}", { status: 200 });
    }) as unknown as typeof fetch;

    const mailEnv = {
      ...env,
      SHOP_MAIL_URL: "https://mail.example/send",
      SHOP_MAIL_FROM: "shop@example.com",
      GA4_MEASUREMENT_ID: "G-1",
      GA4_API_SECRET: "s",
    };
    await enqueue(mailEnv, "email", "buyer@example.com", { subject: "x", text: "y" });
    await enqueue(mailEnv, "webhook", "https://listener.example/hook", { type: "order.paid" });
    await enqueue(mailEnv, "tracking", "ga4", {
      orderId: "o1", orderNumber: "O-1", currency: "EUR", valueMinor: 2400, taxMinor: 157,
      value: 24, tax: 1.57, email: "buyer@example.com", clientId: "c", items: [],
    });

    // No deliverer passed: this is the real dispatch table.
    expect(await drainOutbox(mailEnv, 10)).toBe(3);
    expect(sent.join(" ")).toContain("mail.example");
    expect(sent.join(" ")).toContain("listener.example");
    expect(sent.join(" ")).toContain("google-analytics.com");
  });

  it("reports a tracker that answers with an error", async () => {
    const { deliverTracking } = await import("../functions/api/shop/_tracking");
    globalThis.fetch = (async () => new Response("no", { status: 500 })) as typeof fetch;
    const payload = {
      orderId: "o1", orderNumber: "O-1", currency: "EUR", valueMinor: 2400, taxMinor: 157,
      value: 24, tax: 1.57, email: "buyer@example.com", clientId: "c", items: [],
    };
    await expect(
      deliverTracking({ ...env, GA4_MEASUREMENT_ID: "G-1", GA4_API_SECRET: "s" }, "ga4", payload),
    ).rejects.toThrow();
    await expect(
      deliverTracking({ ...env, META_PIXEL_ID: "p", META_ACCESS_TOKEN: "t" }, "meta", payload),
    ).rejects.toThrow();
  });

  it("says so when a tracker is asked for but not configured", async () => {
    const { deliverTracking } = await import("../functions/api/shop/_tracking");
    await expect(deliverTracking(env, "ga4", {})).rejects.toThrow(/ga4_not_configured/);
    await expect(deliverTracking(env, "meta", {})).rejects.toThrow(/meta_not_configured/);
  });

  it("keeps the file a live link still points at, and removes one nothing does", async () => {
    const { id } = await seedProduct({ sku: "EBOOK-1" });
    const first = await env.SHOP_DB.prepare(`SELECT file_key FROM products WHERE id = ?`)
      .bind(id)
      .first<{ file_key: string }>();

    // Nothing points at the old file: replacing it removes it.
    const form = new FormData();
    form.append("file", new File([new TextEncoder().encode("%PDF-1.4\nnew\n")], "new.pdf"));
    await uploadFile({
      ...ctx(new Request(`${ORIGIN}/x`, { method: "POST", body: form }), { id }, { admin: owner }),
      env: shopEnv(),
    } as never);
    expect(await env.SHOP_FILES!.head(first!.file_key)).toBe(null);

    // Now a buyer holds a link to what is there. Replacing it again must leave
    // that file where it is.
    const order = await buy("EBOOK-2");
    const { fulfilOrder } = await import("../functions/api/shop/_fulfil");
    await fulfilOrder(env, order, { origin: ORIGIN, actor: "test" });

    const sold = await env.SHOP_DB.prepare(`SELECT id, file_key FROM products WHERE sku = 'EBOOK-2'`)
      .first<{ id: string; file_key: string }>();
    const another = new FormData();
    another.append("file", new File([new TextEncoder().encode("%PDF-1.4\nthird\n")], "third.pdf"));
    await uploadFile({
      ...ctx(new Request(`${ORIGIN}/x`, { method: "POST", body: another }), { id: sold!.id }, { admin: owner }),
      env: shopEnv(),
    } as never);
    expect(await env.SHOP_FILES!.head(sold!.file_key)).not.toBe(null);
  });

  it("answers 503 from the status endpoint with no database", async () => {
    const { onRequestGet: orderStatus } = await import("../functions/api/shop/orders/[id]/status");
    const res = (await orderStatus({
      ...ctx(get("/api/shop/orders/x/status?k=y"), { id: "x" }),
      env: { ...env, SHOP_DB: undefined },
    } as never)) as Response;
    expect(res.status).toBe(503);
  });

  it("answers 503 from the invoice endpoint with no database", async () => {
    const { onRequestGet: invoicePage } = await import("../functions/api/shop/invoices/[number]");
    const res = (await invoicePage({
      ...ctx(get("/api/shop/invoices/x?k=y"), { number: "x" }),
      env: { ...env, SHOP_DB: undefined },
    } as never)) as Response;
    expect(res.status).toBe(503);
  });

  it("answers 503 from the product endpoints with no database", async () => {
    const { onRequestGet: oneProduct } = await import("../functions/api/shop/products/[sku]");
    const res = (await oneProduct({
      ...ctx(get("/api/shop/products/x"), { sku: "x" }),
      env: { ...env, SHOP_DB: undefined },
    } as never)) as Response;
    expect(res.status).toBe(503);
  });
});

describe("refusals the shop states plainly", () => {
  it("names each way a basket can be unpriceable", async () => {
    await seedProduct({ sku: "EBOOK-1", currency: "EUR" });
    globalThis.fetch = (async () => new Response(JSON.stringify({ id: "cs_x", url: "https://x" }))) as typeof fetch;

    const base = {
      gateway: "stripe", email: "buyer@example.com", country: "DE", consentWaiver: "1",
    };
    const cases: Array<[Record<string, unknown>, string]> = [
      [{ ...base, items: [{ sku: "EBOOK-1", quantity: 1 }], currency: "PLN" }, "price_not_available"],
      [{ ...base, items: [{ sku: "EBOOK-1", quantity: 1 }], country: "US" }, "country_not_served"],
    ];

    await putSettings(env, { "shop.countries_allowed": ["DE", "PL"] });
    for (const [body, error] of cases) {
      const res = (await checkout({ ...ctx(post("/api/shop/checkout", body)), env: shopEnv() } as never)) as Response;
      expect(await bodyOf<{ error: string }>(res), JSON.stringify(body)).toMatchObject({ error });
    }

    await putSettings(env, { "shop.countries_allowed": [] });
    await env.SHOP_DB.prepare(`DELETE FROM vat_rates WHERE country = 'DE'`).run();
    const noRate = (await checkout({
      ...ctx(post("/api/shop/checkout", { ...base, items: [{ sku: "EBOOK-1", quantity: 1 }] })),
      env: shopEnv(),
    } as never)) as Response;
    expect(await bodyOf<{ error: string }>(noRate)).toMatchObject({ error: "tax_rate_missing" });

    const draft = await seedProduct({ sku: "DRAFT-1" });
    await env.SHOP_DB.prepare(`UPDATE products SET status = 'draft' WHERE id = ?`).bind(draft.id).run();
    const unavailable = (await checkout({
      ...ctx(post("/api/shop/checkout", { ...base, items: [{ sku: "DRAFT-1", quantity: 1 }], country: "PL" })),
      env: shopEnv(),
    } as never)) as Response;
    expect(await bodyOf<{ error: string }>(unavailable)).toMatchObject({ error: "product_unavailable" });
  });

  it("refuses a private address on every shape a host can take", async () => {
    for (const target of [
      "https://localhost/hook",
      "https://app.localhost/hook",
      "https://0.0.0.0/hook",
      "https://127.0.0.1/hook",
      "https://[::1]/hook",
      "https://10.1.2.3/hook",
      "https://192.168.0.1/hook",
      "https://172.16.0.1/hook",
      "https://169.254.169.254/hook",
    ]) {
      await expect(deliverWebhook(env, target, {}), target).rejects.toThrow(/private address/);
    }
  });

  it("refuses a refresh under Cloudflare Access, where there is no session to rotate", async () => {
    const res = (await refreshSession({
      ...ctx(new Request(`${ORIGIN}/api/shop/admin/auth/refresh`, { method: "POST", headers: { origin: ORIGIN } })),
      env: { ...shopEnv(), SHOP_ACCESS_TEAM: "t", SHOP_ACCESS_AUD: "a" },
    } as never)) as Response;
    expect(res.status).toBe(404);
  });

  it("ends every session when the account behind a refresh token is gone", async () => {
    const { createRefresh, refreshCookie } = await import("../functions/api/shop/_auth");
    const id = await seedOwner("gone@example.com");
    const token = await createRefresh(env, id);
    await env.SHOP_DB.prepare(`DELETE FROM admin_users WHERE id = ?`).bind(id).run();

    const res = (await refreshSession({
      ...ctx(new Request(`${ORIGIN}/api/shop/admin/auth/refresh`, {
        method: "POST",
        headers: { origin: ORIGIN, cookie: `__Host-shop_refresh=${token}` },
      })),
      env: shopEnv(),
    } as never)) as Response;
    expect(res.status).toBe(401);
    expect(refreshCookie("", 0)).toContain("Max-Age=0");
  });

  it("settles a flagged tax disagreement when the card country agrees", async () => {
    const order = await buy();
    await env.SHOP_DB.prepare(`UPDATE orders SET tax_evidence = ? WHERE id = ?`)
      .bind(JSON.stringify({ billing: "DE", ip: "PL", conflict: true }), order.id)
      .run();

    const { onRequestPost: paypalWebhook } = await import("../functions/api/shop/webhooks/paypal");
    globalThis.fetch = (async (url: unknown) => {
      const href = String(url);
      if (href.includes("/v1/oauth2/token")) return new Response(JSON.stringify({ access_token: "t", expires_in: 3600 }));
      if (href.includes("verify-webhook-signature")) return new Response(JSON.stringify({ verification_status: "SUCCESS" }));
      if (href.includes("/capture")) {
        return new Response(JSON.stringify({
          status: "COMPLETED",
          payer: { address: { country_code: "DE" } },
          purchase_units: [{ payments: { captures: [{ id: "CAP-1", amount: { value: "24.00", currency_code: "EUR" } }] } }],
        }));
      }
      return new Response("{}");
    }) as unknown as typeof fetch;

    await env.SHOP_DB.prepare(`UPDATE orders SET gateway = 'paypal', gateway_ref = 'PP-1' WHERE id = ?`)
      .bind(order.id)
      .run();

    const body = JSON.stringify({ id: "WH-EV", event_type: "CHECKOUT.ORDER.APPROVED", resource: { id: "PP-1" } });
    const request = new Request(`${ORIGIN}/api/shop/webhooks/paypal`, {
      method: "POST",
      headers: {
        "paypal-auth-algo": "SHA256withRSA", "paypal-cert-url": "https://api.paypal.com/cert",
        "paypal-transmission-id": "tid", "paypal-transmission-sig": "sig",
        "paypal-transmission-time": "2026-09-01T00:00:00Z",
      },
      body,
    });
    const context = ctx(request);
    await paypalWebhook({
      ...context,
      env: { ...env, SHOP_GATEWAYS: "paypal", PAYPAL_CLIENT_ID: "c", PAYPAL_CLIENT_SECRET: "s", PAYPAL_WEBHOOK_ID: "w" },
    } as never);
    await (context as unknown as { settled(): Promise<unknown> }).settled();

    const row = await env.SHOP_DB.prepare(`SELECT tax_evidence, status FROM orders WHERE id = ?`)
      .bind(order.id)
      .first<{ tax_evidence: string; status: string }>();
    const evidence = JSON.parse(row!.tax_evidence) as Record<string, unknown>;
    // Billing said Germany, the network suggested Poland, and the card settles
    // it: the disagreement is resolved rather than left hanging.
    expect(evidence).toMatchObject({ card: "DE", conflict: false });
    expect(row?.status).toBe("paid");
  });

  it("copes with tax evidence that was never written properly", async () => {
    const order = await buy();
    await env.SHOP_DB.prepare(`UPDATE orders SET tax_evidence = 'not json' WHERE id = ?`).bind(order.id).run();
    const row = await env.SHOP_DB.prepare(`SELECT tax_evidence FROM orders WHERE id = ?`)
      .bind(order.id)
      .first<{ tax_evidence: string }>();
    expect(row?.tax_evidence).toBe("not json");

    // The admin detail endpoint reads it and must not fall over.
    const { onRequestGet: getOrderDetail } = await import("../functions/api/shop/admin/orders/[id]");
    const res = (await getOrderDetail(asAdmin(get("/x"), { id: order.id }) as never)) as Response;
    expect(await bodyOf<{ order: { taxEvidence: unknown } }>(res)).toMatchObject({ order: { taxEvidence: null } });
  });

  it("will not mark paid an order that is already paid", async () => {
    const order = await buy();
    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'fulfilled' WHERE id = ?`).bind(order.id).run();
    const res = (await markPaid(asAdmin(post("/x", {}), { id: order.id }) as never)) as Response;
    expect(res.status).toBe(409);
  });
});

describe("the very last corners", () => {
  it("refuses a token whose payload is not JSON at all", async () => {
    const { verifyJwt } = await import("../functions/api/shop/_auth");
    const { base64url } = await import("../functions/api/shop/_lib");
    const head = base64url(new TextEncoder().encode(JSON.stringify({ alg: "HS256", typ: "JWT" })));
    const body = base64url(new TextEncoder().encode("not json"));
    expect(await verifyJwt(env, `${head}.${body}.signature`, ORIGIN)).toBe(null);
  });

  it("acknowledges an event its own handler could not process", async () => {
    // A bug here must not make the provider retry forever; the row says what
    // happened and the panel shows it.
    const { handleWebhook } = await import("../functions/api/shop/_webhook");
    const { hmacHex } = await import("../functions/api/shop/_gateway");

    const order = await buy();
    await env.SHOP_DB.prepare(`UPDATE orders SET gateway_ref = 'cs_boom' WHERE id = ?`).bind(order.id).run();
    // An amount the shop cannot record: the payments insert fails on the
    // currency check, and the handler throws.
    const event = JSON.stringify({
      id: "evt_boom",
      type: "checkout.session.completed",
      data: { object: { id: "cs_boom", payment_intent: "pi_boom", payment_status: "paid", amount_total: 2400, currency: "eur" } },
    });
    const at = Math.floor(Date.now() / 1000);
    const signature = await hmacHex("whsec_x", `${at}.${event}`);
    const request = new Request(`${ORIGIN}/api/shop/webhooks/stripe`, {
      method: "POST",
      headers: { "stripe-signature": `t=${at},v1=${signature}` },
      body: event,
    });

    // The database is taken away mid-flight by pointing the handler at one
    // whose orders table it can read but whose writes fail.
    const broken = {
      ...env,
      STRIPE_SECRET_KEY: "sk_test_x",
      STRIPE_WEBHOOK_SECRET: "whsec_x",
      SHOP_GATEWAYS: "stripe",
    };
    const context = ctx(request);
    const res = (await handleWebhook({ ...context, env: broken } as never, "stripe")) as Response;
    await (context as unknown as { settled(): Promise<unknown> }).settled();
    // Whatever happened, the provider is told we have it.
    expect(res.status).toBe(200);
  });

  it("takes a basket that arrived as a JSON string, and refuses one that is not", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    globalThis.fetch = (async () => new Response(JSON.stringify({ id: "cs_s", url: "https://x" }))) as typeof fetch;

    const asString = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: JSON.stringify([{ sku: "EBOOK-1", quantity: 1 }]),
        gateway: "stripe", email: "buyer@example.com", country: "DE", consentWaiver: "1",
      })),
      env: shopEnv(),
    } as never)) as Response;
    expect(asString.status).toBe(200);

    const nonsense = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: "[not json", gateway: "stripe",
        email: "buyer@example.com", country: "DE", consentWaiver: "1",
      })),
      env: shopEnv(),
    } as never)) as Response;
    expect(nonsense.status).toBe(422);
  });

  it("defaults a new rate's kind and start date", async () => {
    const { onRequestPost: setRate } = await import("../functions/api/shop/admin/vat-rates");
    const res = (await setRate(asAdmin(post("/x", { country: "NO", rateBp: 2500 })) as never)) as Response;
    expect(res.status).toBe(201);
    const body = await bodyOf<{ kind: string; validFrom: string }>(res);
    expect(body.kind).toBe("standard");
    expect(Date.parse(body.validFrom)).toBeLessThanOrEqual(Date.now());
  });

  it("keeps staff out of the rate table", async () => {
    const { onRequestPost: setRate, onRequestDelete: removeRate } = await import("../functions/api/shop/admin/vat-rates");
    const staff: AdminIdentity = { sub: "s", email: "s@example.com", role: "staff", via: "jwt" };
    const context = { ...ctx(post("/x", { country: "PL", rateBp: 100 }), {}, { admin: staff }), env: shopEnv() };
    expect(((await setRate(context as never)) as Response).status).toBe(403);

    const del = {
      ...ctx(new Request(`${ORIGIN}/x?country=PL&kind=ebook&validFrom=2026-01-01T00:00:00.000Z`, { method: "DELETE" }), {}, { admin: staff }),
      env: shopEnv(),
    };
    expect(((await removeRate(del as never)) as Response).status).toBe(403);
  });

  it("keeps staff out of marking orders paid", async () => {
    const order = await buy();
    const staff: AdminIdentity = { sub: "s", email: "s@example.com", role: "staff", via: "jwt" };
    const context = { ...ctx(post("/x", {}), { id: order.id }, { admin: staff }), env: shopEnv() };
    expect(((await markPaid(context as never)) as Response).status).toBe(403);
  });

  it("refuses a product body that is not a body", async () => {
    const { onRequestPost: createProduct } = await import("../functions/api/shop/admin/products/index");
    const request = new Request(`${ORIGIN}/x`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: "{oh no",
    });
    const res = (await createProduct(asAdmin(request) as never)) as Response;
    expect(res.status).toBe(400);

    const { onRequestPatch: patchProduct } = await import("../functions/api/shop/admin/products/[id]");
    const { id } = await seedProduct({ sku: "EBOOK-1" });
    const patch = new Request(`${ORIGIN}/x`, {
      method: "PATCH",
      headers: { "content-type": "application/json" },
      body: "{oh no",
    });
    expect(((await patchProduct(asAdmin(patch, { id }) as never)) as Response).status).toBe(400);
  });

  it("refuses a settings body that is not an object", async () => {
    const { onRequestPut: putSettingsEndpoint } = await import("../functions/api/shop/admin/settings");
    const request = new Request(`${ORIGIN}/x`, {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: "[1,2,3]",
    });
    expect(((await putSettingsEndpoint(asAdmin(request) as never)) as Response).status).toBe(400);
  });
});
