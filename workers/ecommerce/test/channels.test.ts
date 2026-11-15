// GO-165: where an order came from — the storefront (`web`), an agent platform
// through ACP (`acp`) or Google's agentic checkout (`ucp`) — in the order list,
// its filter and the stats; and the stats' refund bug: a fully refunded order
// was left out of gross *and* had its refund subtracted, so its money was
// taken off twice (_summary.ts always counted it once).

import { beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { env } from "./_env";
import { onRequestGet as listOrders } from "../functions/api/shop/admin/orders/index";
import { onRequestGet as stats } from "../functions/api/shop/admin/stats";
import { CHANNELS, isChannel } from "../functions/api/shop/_channels";
import { shopSummary } from "../functions/api/shop/_summary";
import { newId, nowISO } from "../functions/api/shop/_lib";
import { bodyOf, ctx, freshShop, get } from "./_helpers";
import { sparseD1 } from "./fakes";

const admin = { mode: "password", user: null, perms: ["ecommerce:*"], actor: "password" };
type Handler = (c: never) => Response | Promise<Response>;
const call = (handler: Handler, path: string, shop: typeof env = env) =>
  handler({ ...ctx(get(path), {}, { admin }), env: shop } as never) as Promise<Response>;

interface Seed {
  channel?: string;
  status?: string;
  total?: number;
  tax?: number;
  currency?: string;
  createdAt?: string;
  paidAt?: string | null;
}

/** An order written straight to the database. */
async function order(o: Seed = {}): Promise<string> {
  const id = newId();
  const at = o.createdAt ?? nowISO();
  await env.SHOP_DB.prepare(
    `INSERT INTO orders (id, number, key_hash, status, currency, subtotal_minor, tax_minor, total_minor,
                         gateway, gateway_ref, created_at, paid_at, updated_at, channel)
     VALUES (?, ?, 'h', ?, ?, ?, ?, ?, 'stripe', ?, ?, ?, ?, ?)`,
  )
    .bind(id, `ORD-${id.slice(0, 8)}`, o.status ?? "paid", o.currency ?? "EUR", o.total ?? 2400, o.tax ?? 0,
      o.total ?? 2400, `pi_${id}`, at, o.paidAt === undefined ? at : o.paidAt, at, o.channel ?? "web")
    .run();
  return id;
}

/** A succeeded refund of `amount` on `orderId`, made `at` (now by default). */
async function refund(orderId: string, amount: number, currency = "EUR", at = nowISO()): Promise<void> {
  await env.SHOP_DB.prepare(
    `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, created_at)
     VALUES (?, ?, 'stripe', ?, 'refund', 'succeeded', ?, ?, ?)`,
  )
    .bind(newId(), orderId, `re_${newId()}`, -amount, currency, at)
    .run();
}

interface CurrencyRow { currency: string; orders: number; grossMinor: number; taxMinor: number; refundedMinor: number; netMinor: number }
interface ChannelRow { channel: string; currency: string; orders: number; grossMinor: number; refunds: number; refundedMinor: number; netMinor: number }
interface Stats { byCurrency: CurrencyRow[]; byChannel: ChannelRow[] }

beforeEach(async () => {
  await freshShop();
});

describe("the channels an order can come from", () => {
  it("are web, ACP and UCP, and nothing else", () => {
    assert.deepEqual([...CHANNELS], ["web", "acp", "ucp"]);
    assert.deepEqual(["web", "acp", "ucp", "WEB", "", "shop"].map(isChannel), [true, true, true, false, false, false]);
  });
});

describe("the order list by channel", () => {
  it("says which channel each order came from", async () => {
    await order({ channel: "acp" });
    await order();
    const body = await bodyOf<{ orders: { channel: string }[] }>(await call(listOrders as Handler, "/x"));
    assert.deepEqual(body.orders.map((o) => o.channel).sort(), ["acp", "web"]);
  });

  it("filters by channel, counting the whole selection", async () => {
    await order({ channel: "acp" });
    await order({ channel: "ucp" });
    await order({ channel: "ucp", status: "pending" });
    await order();
    const body = await bodyOf<{ orders: { channel: string; status: string }[]; total: number }>(
      await call(listOrders as Handler, "/x?channel=ucp"),
    );
    assert.equal(body.total, 2);
    assert.deepEqual(body.orders.map((o) => o.channel), ["ucp", "ucp"]);
  });

  it("combines the channel with the status filter", async () => {
    await order({ channel: "ucp" });
    await order({ channel: "ucp", status: "pending" });
    const body = await bodyOf<{ orders: { status: string }[]; total: number }>(
      await call(listOrders as Handler, "/x?channel=ucp&status=pending"),
    );
    assert.deepEqual([body.total, body.orders[0]!.status], [1, "pending"]);
  });

  it("pages and sorts within one channel without losing or repeating a row", async () => {
    const base = Date.parse("2026-11-01T10:00:00.000Z");
    for (let i = 0; i < 5; i++) {
      await order({ channel: "acp", total: 1000 + i, createdAt: new Date(base + i * 60_000).toISOString() });
      await order({ channel: "web", total: 1000 + i, createdAt: new Date(base + i * 60_000).toISOString() });
    }
    const seen: number[] = [];
    let cursor: string | null = "";
    while (cursor !== null) {
      const path = `/x?channel=acp&sort=total&dir=asc&limit=2${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`;
      const page: { orders: { totalMinor: number; channel: string }[]; nextCursor: string | null; total: number } =
        await bodyOf(await call(listOrders as Handler, path));
      assert.equal(page.total, 5);
      assert.ok(page.orders.every((o) => o.channel === "acp"));
      seen.push(...page.orders.map((o) => o.totalMinor));
      cursor = page.nextCursor;
    }
    assert.deepEqual(seen, [1000, 1001, 1002, 1003, 1004]);
  });

  it("refuses a channel it does not know, rather than showing everything", async () => {
    for (const bad of ["shop", "ACP", "web'--"]) {
      const res = await call(listOrders as Handler, `/x?channel=${encodeURIComponent(bad)}`);
      assert.equal(res.status, 422, bad);
      assert.equal((await bodyOf<{ error: string }>(res)).error, "invalid_channel");
    }
  });

  it("treats an empty channel as no filter", async () => {
    await order({ channel: "acp" });
    await order();
    const body = await bodyOf<{ total: number }>(await call(listOrders as Handler, "/x?channel="));
    assert.equal(body.total, 2);
  });

  it("calls a row read without a channel a web order (rows from before migration 5)", async () => {
    await order();
    const shop = { ...env, SHOP_DB: sparseD1(env.SHOP_DB, /LIMIT \?/, { rows: [{ id: "o1", number: "N1", status: "paid" }] }) } as typeof env;
    const body = await bodyOf<{ orders: { channel: string }[] }>(await call(listOrders as Handler, "/x", shop));
    assert.equal(body.orders[0]!.channel, "web");
  });
});

describe("the stats", () => {
  it("count a fully refunded order in gross and take its refund off once", async () => {
    const kept = await order({ total: 3000, tax: 500 });
    const returned = await order({ total: 2000, tax: 300, status: "refunded" });
    await refund(returned, 2000);
    void kept;

    const body = await bodyOf<Stats>(await call(stats as Handler, "/x?window=30d"));
    assert.deepEqual(body.byCurrency, [
      // Gross is everything paid; the refund comes off once. VAT stays what
      // was kept: a full refund's credit note gave its VAT back.
      { currency: "EUR", orders: 2, grossMinor: 5000, taxMinor: 500, refundedMinor: 2000, netMinor: 3000 },
    ]);
  });

  it("agree with the module summary on revenue", async () => {
    await order({ total: 3000 });
    const partly = await order({ total: 2000 });
    await refund(partly, 500);
    const returned = await order({ total: 1500, status: "refunded" });
    await refund(returned, 1500);

    const body = await bodyOf<Stats>(await call(stats as Handler, "/x?window=30d"));
    const summary = await shopSummary(env, { live: false });
    const revenue = (summary as unknown as { metrics: { key: string; value: number }[] }).metrics.find((m) => m.key === "revenue")!;
    assert.equal(body.byCurrency[0]!.netMinor, 4500);
    assert.equal(revenue.value * 100, body.byCurrency[0]!.netMinor);
  });

  it("keep a refund made in the window for an order paid before it", async () => {
    const old = new Date(Date.now() - 60 * 86_400_000).toISOString();
    const earlier = await order({ total: 2000, status: "refunded", createdAt: old, currency: "USD" });
    await refund(earlier, 2000, "USD");
    const body = await bodyOf<Stats>(await call(stats as Handler, "/x?window=30d"));
    assert.deepEqual(body.byCurrency, [
      { currency: "USD", orders: 0, grossMinor: 0, taxMinor: 0, refundedMinor: 2000, netMinor: -2000 },
    ]);
    assert.deepEqual(body.byChannel, [
      { channel: "web", currency: "USD", orders: 0, grossMinor: 0, refunds: 1, refundedMinor: 2000, netMinor: -2000 },
    ]);
  });

  it("split orders, gross and refunds by channel, web first", async () => {
    await order({ channel: "ucp", total: 1000 });
    await order({ channel: "acp", total: 2000 });
    const acpBack = await order({ channel: "acp", total: 500, status: "refunded" });
    await refund(acpBack, 500);
    await order({ total: 4000 });
    await order({ total: 9000, status: "pending", paidAt: null });
    await order({ channel: "acp", total: 700, currency: "USD", status: "fulfilled" });

    const body = await bodyOf<Stats>(await call(stats as Handler, "/x?window=7d"));
    assert.deepEqual(body.byChannel, [
      { channel: "web", currency: "EUR", orders: 1, grossMinor: 4000, refunds: 0, refundedMinor: 0, netMinor: 4000 },
      { channel: "acp", currency: "EUR", orders: 2, grossMinor: 2500, refunds: 1, refundedMinor: 500, netMinor: 2000 },
      { channel: "acp", currency: "USD", orders: 1, grossMinor: 700, refunds: 0, refundedMinor: 0, netMinor: 700 },
      { channel: "ucp", currency: "EUR", orders: 1, grossMinor: 1000, refunds: 0, refundedMinor: 0, netMinor: 1000 },
    ]);
    // The channels add up to the currency totals.
    const eur = body.byCurrency.find((r) => r.currency === "EUR")!;
    const sum = body.byChannel.filter((r) => r.currency === "EUR").reduce((n, r) => n + r.netMinor, 0);
    assert.equal(sum, eur.netMinor);
  });

  it("list a channel it does not know after the ones it does", async () => {
    await order({ channel: "zz-later" });
    await order({ channel: "aa-later" });
    await order({ channel: "ucp" });
    const body = await bodyOf<Stats>(await call(stats as Handler, "/x"));
    assert.deepEqual(body.byChannel.map((r) => r.channel), ["ucp", "aa-later", "zz-later"]);
  });

  it("read a channel row without sums as zero", async () => {
    const shop = { ...env, SHOP_DB: sparseD1(env.SHOP_DB, /GROUP BY (o\.)?channel/, { rows: [{ channel: "acp", currency: "EUR", orders: 1, gross: null, refunds: null, refunded: null }] }) } as typeof env;
    const body = await bodyOf<Stats>(await call(stats as Handler, "/x", shop));
    assert.deepEqual(body.byChannel, [
      { channel: "acp", currency: "EUR", orders: 1, grossMinor: 0, refunds: 0, refundedMinor: 0, netMinor: 0 },
    ]);
  });

  it("put currencies of one channel in order whatever order the database used", async () => {
    const rows = [
      { channel: "acp", currency: "USD", orders: 1, gross: 100 },
      { channel: "acp", currency: "EUR", orders: 1, gross: 200 },
      { channel: "acp", currency: "CHF", orders: 1, gross: 300 },
    ];
    const shop = { ...env, SHOP_DB: sparseD1(env.SHOP_DB, /GROUP BY channel, currency/, { rows }) } as typeof env;
    const body = await bodyOf<Stats>(await call(stats as Handler, "/x", shop));
    assert.deepEqual(body.byChannel.map((r) => r.currency), ["CHF", "EUR", "USD"]);
  });

  it("answer with no channel rows when nothing sold", async () => {
    const body = await bodyOf<Stats>(await call(stats as Handler, "/x"));
    assert.deepEqual(body.byChannel, []);
  });
});
