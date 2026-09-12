// Download links, and the queue that carries everything the shop sends.

import { env } from "cloudflare:test";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  checkToken,
  inRangeWindow,
  issueTokens,
  openRangeWindow,
  revokeTokensForOrder,
  spendUse,
} from "../functions/api/shop/_downloads";
import { drainInBackground, drainOutbox, enqueue, retryOutbox } from "../functions/api/shop/_outbox";
import { audit } from "../functions/api/shop/_audit";
import { adminNoticeMail, deliverEmail, orderPaidMail, refundedMail, resendMail } from "../functions/api/shop/_mail";
import { deliverWebhook, endpointsFor } from "../functions/api/shop/_hooks";
import { deliverTracking, enabledTrackers } from "../functions/api/shop/_tracking";
import { createOrder, priceBasket, upsertCustomer, type PricedBasket } from "../functions/api/shop/_orders";
import { taxContext } from "../functions/api/shop/_tax";
import { putSettings } from "../functions/api/shop/_settings";
import { sha256hex } from "../functions/api/shop/_lib";
import type { OutboxRow } from "../functions/api/shop/_outbox";
import type { OrderItemRow, OrderRow, ProductRow } from "../functions/api/shop/_types";
import { freshShop, seedProduct } from "./_helpers";

async function paidOrder(): Promise<{ order: OrderRow; items: OrderItemRow[]; products: Map<string, ProductRow> }> {
  const { id } = await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
  const ctx = await taxContext(env);
  const priced = (await priceBasket(env, ctx, {
    lines: [{ sku: "EBOOK-1", quantity: 1 }],
    currency: "EUR",
    billingCountry: "DE",
    ipCountry: null,
    vatId: "",
    vatIdValid: false,
  })) as PricedBasket;
  const customer = await upsertCustomer(env, {
    email: "b@example.com", name: "B", vatId: "", vatIdValid: null, country: "DE",
  });
  const { order, items } = await createOrder(env, priced, customer, {
    gateway: "stripe", locale: "en", consentMarketing: false, consentWaiver: true, ipHash: null, userAgent: "t",
  });
  await env.SHOP_DB.prepare(`UPDATE orders SET status = 'paid' WHERE id = ?`).bind(order.id).run();
  const product = (await env.SHOP_DB.prepare(`SELECT * FROM products WHERE id = ?`).bind(id).first<ProductRow>())!;
  return { order: { ...order, status: "paid" }, items, products: new Map([[id, product]]) };
}

beforeEach(async () => {
  await freshShop();
});

describe("download tokens", () => {
  it("issues one per line and accepts it once per allowed use", async () => {
    const { items, products } = await paidOrder();
    const [token] = await issueTokens(env, items, products);
    expect(token).toBeDefined();

    const check = await checkToken(env, token!.token);
    expect(check.ok).toBe(true);

    const hash = await sha256hex(token!.token);
    expect(await spendUse(env, hash)).toBe(true);
    expect(await spendUse(env, hash)).toBe(true);
    expect(await spendUse(env, hash)).toBe(true);
    // The limit is in the statement, so the database decides who wins.
    expect(await spendUse(env, hash)).toBe(false);
    expect(await checkToken(env, token!.token)).toEqual({ ok: false, reason: "exhausted" });
  });

  it("says exactly why it refused", async () => {
    const { order, items, products } = await paidOrder();
    const [token] = await issueTokens(env, items, products);
    const hash = await sha256hex(token!.token);

    expect(await checkToken(env, "never-issued")).toEqual({ ok: false, reason: "not_found" });

    await env.SHOP_DB.prepare(`UPDATE download_tokens SET expires_at = '2000-01-01T00:00:00.000Z' WHERE token_hash = ?`)
      .bind(hash)
      .run();
    expect(await checkToken(env, token!.token)).toEqual({ ok: false, reason: "expired" });

    await env.SHOP_DB.prepare(`UPDATE download_tokens SET revoked = 1 WHERE token_hash = ?`).bind(hash).run();
    expect(await checkToken(env, token!.token)).toEqual({ ok: false, reason: "revoked" });

    // A token for an order that is no longer paid delivers nothing, whatever
    // the token itself says.
    await env.SHOP_DB.prepare(`UPDATE download_tokens SET revoked = 0, expires_at = ? WHERE token_hash = ?`)
      .bind(new Date(Date.now() + 86400000).toISOString(), hash)
      .run();
    await env.SHOP_DB.prepare(`UPDATE orders SET status = 'refunded' WHERE id = ?`).bind(order.id).run();
    expect(await checkToken(env, token!.token)).toEqual({ ok: false, reason: "not_paid" });
  });

  it("does not issue twice while a live token exists", async () => {
    const { items, products } = await paidOrder();
    expect(await issueTokens(env, items, products)).toHaveLength(1);
    expect(await issueTokens(env, items, products)).toHaveLength(0);
  });

  it("issues again once the old ones are revoked, which is what resend needs", async () => {
    const { order, items, products } = await paidOrder();
    await issueTokens(env, items, products);
    expect(await revokeTokensForOrder(env, order.id)).toBe(1);
    expect(await issueTokens(env, items, products)).toHaveLength(1);
  });

  it("issues nothing for a line with no file behind it", async () => {
    const { items } = await paidOrder();
    expect(await issueTokens(env, items, new Map())).toHaveLength(0);
    expect(await issueTokens(env, [], new Map())).toHaveLength(0);
  });

  it("lets a reader app finish a range download on one use", async () => {
    const hash = "hash";
    expect(await inRangeWindow(env, hash, "client")).toBe(false);
    await openRangeWindow(env, hash, "client");
    expect(await inRangeWindow(env, hash, "client")).toBe(true);
    expect(await inRangeWindow(env, hash, "another-client")).toBe(false);
  });

  it("counts every request when there is no KV to remember with", async () => {
    const noKv = { ...env, SHOP_KV: undefined };
    await openRangeWindow(noKv, "hash", "client");
    expect(await inRangeWindow(noKv, "hash", "client")).toBe(false);
  });
});

describe("the outbox", () => {
  it("delivers, marks done, and does not deliver again", async () => {
    await enqueue(env, "email", "a@example.com", { subject: "Hello" });
    const seen: OutboxRow[] = [];
    expect(await drainOutbox(env, 5, async (row) => void seen.push(row))).toBe(1);
    expect(seen).toHaveLength(1);
    expect(await drainOutbox(env, 5, async () => {})).toBe(0);
  });

  it("backs off further after each failure", async () => {
    const id = await enqueue(env, "email", "a@example.com", {});
    const fail = async () => {
      throw new Error("the mail server said no");
    };

    await drainOutbox(env, 5, fail);
    const first = (await env.SHOP_DB.prepare(`SELECT * FROM outbox WHERE id = ?`).bind(id).first<OutboxRow>())!;
    expect(first.attempts).toBe(1);
    expect(first.last_error).toContain("the mail server said no");
    // A minute away, so the next ordinary request does not retry it at once.
    expect(Date.parse(first.next_attempt)).toBeGreaterThan(Date.now() + 30_000);

    // …and it is not picked up until then.
    expect(await drainOutbox(env, 5, fail)).toBe(0);
  });

  it("lets a human push a stuck message to the front", async () => {
    const id = await enqueue(env, "email", "a@example.com", {});
    await drainOutbox(env, 5, async () => {
      throw new Error("no");
    });
    expect(await retryOutbox(env, id)).toBe(true);
    expect(await drainOutbox(env, 5, async () => {})).toBe(1);
    // A message already delivered cannot be retried.
    expect(await retryOutbox(env, id)).toBe(false);
    expect(await retryOutbox(env, "no-such-id")).toBe(false);
  });

  it("truncates a provider's essay before it reaches the panel", async () => {
    const id = await enqueue(env, "email", "a@example.com", {});
    await drainOutbox(env, 5, async () => {
      throw new Error("x".repeat(2000));
    });
    const row = (await env.SHOP_DB.prepare(`SELECT * FROM outbox WHERE id = ?`).bind(id).first<OutboxRow>())!;
    expect(row.last_error?.length).toBe(500);
  });

  it("refuses a kind nothing knows how to send", async () => {
    await env.SHOP_DB.prepare(
      `INSERT INTO outbox (id, kind, target, payload_json, attempts, next_attempt, created_at)
       VALUES ('x', 'carrier-pigeon', 'nowhere', '{}', 0, ?, ?)`,
    )
      .bind(new Date().toISOString(), new Date().toISOString())
      .run();
    expect(await drainOutbox(env, 5)).toBe(0);
    const row = (await env.SHOP_DB.prepare(`SELECT * FROM outbox WHERE id = 'x'`).first<OutboxRow>())!;
    expect(row.last_error).toContain("carrier-pigeon");
  });

  it("drains in the background without the caller waiting", async () => {
    await enqueue(env, "email", "a@example.com", {});
    const promises: Promise<unknown>[] = [];
    drainInBackground(env, { waitUntil: (p) => promises.push(p) }, 1);
    await Promise.all(promises);
    // It ran, and whether it succeeded is not the caller's problem.
    expect(promises).toHaveLength(1);
  });
});

describe("email", () => {
  it("refuses to pretend it sent something", async () => {
    // Silently succeeding would leave a buyer waiting for an email nobody ever
    // tried to send.
    await expect(deliverEmail({ ...env, SHOP_MAIL_URL: undefined }, "a@example.com", {})).rejects.toThrow(
      /mail_not_configured/,
    );
  });

  it("posts to the configured API", async () => {
    const calls: Array<{ url: string; init: RequestInit }> = [];
    const original = globalThis.fetch;
    globalThis.fetch = (async (url: string, init: RequestInit) => {
      calls.push({ url, init });
      return new Response("{}", { status: 200 });
    }) as unknown as typeof fetch;

    await deliverEmail(
      { ...env, SHOP_MAIL_URL: "https://mail.example/send", SHOP_MAIL_FROM: "shop@example.com", SHOP_MAIL_KEY: "k" },
      "buyer@example.com",
      { subject: "Your book", text: "Here it is." },
    );
    expect(calls[0]?.url).toBe("https://mail.example/send");
    expect(String(calls[0]?.init.body)).toContain("buyer@example.com");

    globalThis.fetch = original;
  });

  it("throws on a refusal, so the queue keeps the message", async () => {
    const original = globalThis.fetch;
    globalThis.fetch = (async () => new Response("no", { status: 500 })) as typeof fetch;
    await expect(
      deliverEmail(
        { ...env, SHOP_MAIL_URL: "https://mail.example/send", SHOP_MAIL_FROM: "shop@example.com" },
        "a@example.com",
        {},
      ),
    ).rejects.toThrow();
    globalThis.fetch = original;
  });

  it("writes the four messages the shop sends", () => {
    const input = {
      locale: "en",
      shopName: "Example Press",
      orderNumber: "O-2026-000001",
      totalMinor: 2400,
      currency: "EUR",
      downloads: [{ name: "A Book", url: "https://shop.example.com/x", expiresAt: "2026-12-01", maxUses: 3 }],
      resendUrl: "https://shop.example.com/shop/resend/",
    };
    expect(orderPaidMail(input).subject).toContain("O-2026-000001");
    expect(orderPaidMail(input).text).toContain("https://shop.example.com/x");
    expect(resendMail(input).subject.length).toBeGreaterThan(0);
    expect(refundedMail({ ...input, creditNoteUrl: "https://shop.example.com/kor" }).text).toContain("kor");
    expect(orderPaidMail({ ...input, locale: "pl" }).text).toMatch(/[ąćęłńóśźż]/);
  });

  it("tells the owner about a sale without telling them who bought it", () => {
    // The admin notice goes to an address that may be a shared inbox or a
    // phone; it says a sale happened, not who made it.
    const notice = adminNoticeMail("paid", {
      orderNumber: "O-2026-000001",
      totalMinor: 2400,
      currency: "EUR",
      country: "DE",
      gateway: "stripe",
      panelUrl: "https://shop.example.com/ecommerce-admin/",
    });
    expect(notice.text).toContain("O-2026-000001");
    expect(notice.text).not.toContain("@");
    expect(adminNoticeMail("needs_review", {
      orderNumber: "O-2026-000002",
      totalMinor: 1,
      currency: "EUR",
      country: "DE",
      gateway: "stripe",
      reason: "paid 1, expected 2400",
    }).subject).toMatch(/review/i);
  });
});

describe("outgoing webhooks", () => {
  it("reads its endpoints from settings and filters by event", async () => {
    await putSettings(env, {
      "webhooks.endpoints": [
        { url: "https://a.example/hook", events: ["order.paid"], secret: "s1" },
        { url: "https://b.example/hook", events: ["order.refunded"] },
        { url: "https://c.example/hook" },
      ],
    });
    const paid = await endpointsFor(env, "order.paid");
    // An endpoint that names no events receives none: a webhook you did not
    // ask for is worse than one that never arrives.
    expect(paid.map((e) => e.url)).toEqual(["https://a.example/hook"]);
    expect((await endpointsFor(env, "order.refunded")).map((e) => e.url)).toEqual(["https://b.example/hook"]);
  });

  it("signs the body the way Stripe signs its own", async () => {
    const calls: RequestInit[] = [];
    const original = globalThis.fetch;
    globalThis.fetch = (async (_url: string, init: RequestInit) => {
      calls.push(init);
      return new Response("ok", { status: 200 });
    }) as unknown as typeof fetch;

    await deliverWebhook(env, "https://a.example/hook", { event: "order.paid", secret: "s1" });
    const headers = calls[0]?.headers as Record<string, string>;
    expect(headers["x-ssg-shop-signature"]).toMatch(/^t=\d+,v1=[0-9a-f]{64}$/);
    // The secret signs the body; it must not be in the body.
    expect(String(calls[0]?.body)).not.toContain("s1");

    globalThis.fetch = original;
  });

  it("refuses a target inside the network it is running in", async () => {
    // A webhook URL is configuration, but configuration can be wrong, and an
    // endpoint that will fetch any URL is an SSRF pivot.
    await expect(deliverWebhook(env, "http://169.254.169.254/latest/meta-data/", {})).rejects.toThrow();
    await expect(deliverWebhook(env, "http://localhost:8080/", {})).rejects.toThrow();
    await expect(deliverWebhook(env, "https://10.0.0.1/", {})).rejects.toThrow();
    await expect(deliverWebhook(env, "ftp://example.com/", {})).rejects.toThrow();
  });
});

describe("tracking", () => {
  it("is off until it is configured", () => {
    expect(enabledTrackers(env)).toEqual([]);
    expect(enabledTrackers({ ...env, GA4_MEASUREMENT_ID: "G-1", GA4_API_SECRET: "s" })).toEqual(["ga4"]);
    expect(enabledTrackers({ ...env, META_PIXEL_ID: "p", META_ACCESS_TOKEN: "t" })).toEqual(["meta"]);
  });

  it("posts a purchase to whichever is configured", async () => {
    const urls: string[] = [];
    const original = globalThis.fetch;
    globalThis.fetch = (async (url: string) => {
      urls.push(String(url));
      return new Response("{}", { status: 200 });
    }) as unknown as typeof fetch;

    const configured = { ...env, GA4_MEASUREMENT_ID: "G-1", GA4_API_SECRET: "s", META_PIXEL_ID: "p", META_ACCESS_TOKEN: "t" };
    const payload = {
      orderId: "o1",
      orderNumber: "O-1",
      currency: "EUR",
      valueMinor: 2400,
      taxMinor: 157,
      value: 24,
      tax: 1.57,
      email: "buyer@example.com",
      items: [{ sku: "EBOOK-1", name: "A Book", quantity: 1, price: 24 }],
      clientId: "client",
    };
    await deliverTracking(configured, "ga4", payload);
    await deliverTracking(configured, "meta", payload);
    expect(urls[0]).toContain("google-analytics.com");
    expect(urls[1]).toContain("facebook.com");

    globalThis.fetch = original;
  });

  it("refuses a target nobody taught it", async () => {
    await expect(deliverTracking(env, "tiktok", {})).rejects.toThrow();
  });
});

describe("the audit log", () => {
  it("records who did what, and survives a detail it cannot serialise", async () => {
    await audit(env, "admin:1", "product.create", "p1", { sku: "EBOOK-1" });
    const row = await env.SHOP_DB.prepare(`SELECT * FROM audit_log`).first<{ actor: string; detail_json: string }>();
    expect(row?.actor).toBe("admin:1");
    expect(row?.detail_json).toContain("EBOOK-1");

    const circular: Record<string, unknown> = {};
    circular.self = circular;
    await expect(audit(env, "admin:1", "weird", null, circular)).resolves.toBeUndefined();
  });
});

describe("draining does not break the request it rides on", () => {
  it("swallows a failure entirely", async () => {
    await enqueue(env, "email", "a@example.com", {});
    const spy = vi.fn();
    drainInBackground({ ...env, SHOP_DB: undefined as never }, { waitUntil: spy }, 1);
    expect(spy).toHaveBeenCalled();
  });
});
