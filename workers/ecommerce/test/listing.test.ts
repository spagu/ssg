// Filtering, sorting and paging, on every list the panel shows.
//
// The scale tests are the point of the file. D1 caps how many parameters one
// statement may bind, and the catalogue used to look prices up with one bind
// per product — which worked for a demo shop and stopped working for a real
// one, at a size nobody would think to test by hand.

import { env } from "cloudflare:test";
import { beforeEach, describe, expect, it } from "vitest";
import { onRequestGet as adminProducts } from "../functions/api/shop/admin/products/index";
import { onRequestGet as publicProducts } from "../functions/api/shop/products/index";
import { onRequestGet as adminOrders } from "../functions/api/shop/admin/orders/index";
import { onRequestGet as adminOutbox } from "../functions/api/shop/admin/outbox";
import { enqueue } from "../functions/api/shop/_outbox";
import { createOrder, priceBasket, upsertCustomer, type PricedBasket } from "../functions/api/shop/_orders";
import { taxContext } from "../functions/api/shop/_tax";
import { newId, nowISO } from "../functions/api/shop/_lib";
import type { AdminIdentity, OrderRow } from "../functions/api/shop/_types";
import { bodyOf, ctx, freshShop, get, seedProduct } from "./_helpers";

const owner: AdminIdentity = { sub: "owner-1", email: "owner@example.com", role: "owner", via: "jwt" };
const asAdmin = (path: string) => ctx(get(path), {}, { admin: owner });

interface ProductPage {
  products: Array<{ sku: string; name: string; status: string; prices: Record<string, number> }>;
  total: number;
  nextCursor: string | null;
  sort?: string;
  dir?: string;
}
interface OrderPage {
  orders: Array<{ number: string; totalMinor: number; status: string; gateway: string | null; createdAt: string }>;
  total: number;
  nextCursor: string | null;
}

/** Many products, cheaply: one batch rather than a round trip each. */
async function seedMany(count: number, prefix = "BULK"): Promise<void> {
  const now = nowISO();
  const statements = [];
  for (let i = 0; i < count; i++) {
    const id = newId();
    statements.push(
      env.SHOP_DB.prepare(
        `INSERT INTO products (id, sku, name, description, kind, status, tax_category,
                               download_limit, download_days, created_at, updated_at)
         VALUES (?, ?, ?, NULL, 'digital', 'active', 'ebook', 5, 30, ?, ?)`,
      ).bind(id, `${prefix}-${String(i).padStart(5, "0")}`, `Book ${String(i).padStart(5, "0")}`, now, now),
      env.SHOP_DB.prepare(`INSERT INTO prices (product_id, currency, amount_minor) VALUES (?, 'EUR', ?)`)
        .bind(id, 1000 + i),
    );
  }
  // In chunks, because a batch is itself a request with a size.
  for (let i = 0; i < statements.length; i += 200) {
    await env.SHOP_DB.batch(statements.slice(i, i + 200));
  }
}

async function seedOrders(count: number): Promise<void> {
  await seedProduct({ sku: "EBOOK-1", priceMinor: 2400 });
  const taxCtx = await taxContext(env);
  const priced = (await priceBasket(env, taxCtx, {
    lines: [{ sku: "EBOOK-1", quantity: 1 }],
    currency: "EUR",
    billingCountry: "DE",
    ipCountry: null,
    vatId: "",
    vatIdValid: false,
  })) as PricedBasket;

  for (let i = 0; i < count; i++) {
    const customer = await upsertCustomer(env, {
      email: `buyer${i}@example.com`,
      name: `Buyer ${i}`,
      vatId: "",
      vatIdValid: null,
      country: i % 2 === 0 ? "DE" : "PL",
    });
    const { order } = await createOrder(env, priced, customer, {
      gateway: i % 2 === 0 ? "stripe" : "paypal",
      locale: "en",
      consentMarketing: false,
      consentWaiver: true,
      ipHash: null,
      userAgent: "t",
    });
    // Spread them out, and vary the amounts, so sorting has something to do.
    await env.SHOP_DB.prepare(
      `UPDATE orders SET created_at = ?, total_minor = ?, status = ? WHERE id = ?`,
    )
      .bind(
        new Date(Date.UTC(2026, 0, i + 1)).toISOString(),
        1000 + i * 37,
        i % 3 === 0 ? "paid" : "pending",
        order.id,
      )
      .run();
  }
}

beforeEach(async () => {
  await freshShop();
});

describe("a catalogue bigger than one answer", () => {
  it("lists a page of products, not all of them", async () => {
    // Well past D1's bound-parameter cap, which the old one-bind-per-product
    // price lookup ran into: the screen stopped loading rather than slowing
    // down, and only on a shop big enough that nobody was testing by hand.
    await seedMany(250);

    const res = (await adminProducts(asAdmin("/api/shop/admin/products?limit=25") as never)) as Response;
    expect(res.status).toBe(200);

    const body = await bodyOf<ProductPage>(res);
    expect(body.products).toHaveLength(25);
    expect(body.total).toBe(250);
    expect(body.nextCursor).not.toBe(null);
    // The prices came with them: the lookup is per page, so it still works.
    expect(Object.keys(body.products[0]!.prices)).toContain("EUR");
  });

  it("walks the whole catalogue without repeating or losing one", async () => {
    await seedMany(120);

    const seen: string[] = [];
    let cursor: string | null = null;
    for (let guard = 0; guard < 20; guard++) {
      const url: string = `/api/shop/admin/products?limit=25&sort=sku&dir=asc${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`;
      const res = (await adminProducts(asAdmin(url) as never)) as Response;
      const body: ProductPage = await bodyOf<ProductPage>(res);
      seen.push(...body.products.map((p) => p.sku));
      cursor = body.nextCursor;
      if (!cursor) break;
    }

    expect(seen).toHaveLength(120);
    expect(new Set(seen).size).toBe(120); // nothing twice
    expect([...seen].sort()).toEqual(seen); // and in the order asked for
  });

  it("answers the public catalogue by product code, for a page that knows what it shows", async () => {
    await seedMany(300);

    const res = (await publicProducts(
      ctx(get("/api/shop/products?sku=BULK-00007,BULK-00042")) as never,
    )) as Response;
    const body = await bodyOf<ProductPage>(res);

    expect(body.products.map((p) => p.sku).sort()).toEqual(["BULK-00007", "BULK-00042"]);
    // Asking by code is not paging: there is no more to fetch.
    expect(body.nextCursor).toBe(null);
  });

  it("pages the public catalogue rather than answering with the whole shop", async () => {
    await seedMany(300);
    const res = (await publicProducts(ctx(get("/api/shop/products")) as never)) as Response;
    const body = await bodyOf<ProductPage>(res);

    expect(body.products.length).toBeLessThanOrEqual(50);
    expect(body.total).toBe(300);
    expect(body.nextCursor).not.toBe(null);
  });

  it("caps how much one answer can be asked for", async () => {
    await seedMany(300);
    const res = (await publicProducts(ctx(get("/api/shop/products?limit=100000")) as never)) as Response;
    expect((await bodyOf<ProductPage>(res)).products.length).toBeLessThanOrEqual(200);
  });

  it("ignores a product code that is not one", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    const res = (await publicProducts(
      ctx(get("/api/shop/products?sku=EBOOK-1,../../etc/passwd,%20")) as never,
    )) as Response;
    expect((await bodyOf<ProductPage>(res)).products.map((p) => p.sku)).toEqual(["EBOOK-1"]);
  });
});

describe("finding one product among many", () => {
  beforeEach(async () => {
    await seedMany(60);
    await seedProduct({ sku: "FINDME", name: "The Needle" });
  });

  it("searches the name and the code", async () => {
    for (const query of ["needle", "FINDME", "find"]) {
      const res = (await adminProducts(asAdmin(`/api/shop/admin/products?q=${query}`) as never)) as Response;
      const body = await bodyOf<ProductPage>(res);
      expect(body.products.map((p) => p.sku), query).toEqual(["FINDME"]);
      // The count describes the search, not the shop.
      expect(body.total, query).toBe(1);
    }
  });

  it("filters by status", async () => {
    await env.SHOP_DB.prepare(`UPDATE products SET status = 'draft' WHERE sku = 'FINDME'`).run();
    const res = (await adminProducts(asAdmin("/api/shop/admin/products?status=draft") as never)) as Response;
    expect((await bodyOf<ProductPage>(res)).products.map((p) => p.sku)).toEqual(["FINDME"]);
  });

  it("refuses a status and a sort it does not have", async () => {
    for (const query of ["status=elsewhere", "sort=whatever"]) {
      const res = (await adminProducts(asAdmin(`/api/shop/admin/products?${query}`) as never)) as Response;
      expect(res.status, query).toBe(422);
    }
  });
});

describe("sorting the orders", () => {
  beforeEach(async () => {
    await seedOrders(12);
  });

  it("puts the newest first by default, and the oldest first when turned around", async () => {
    const newest = await bodyOf<OrderPage>(
      (await adminOrders(asAdmin("/api/shop/admin/orders?limit=5") as never)) as Response,
    );
    const oldest = await bodyOf<OrderPage>(
      (await adminOrders(asAdmin("/api/shop/admin/orders?limit=5&sort=date&dir=asc") as never)) as Response,
    );

    expect(newest.orders[0]!.createdAt > newest.orders[4]!.createdAt).toBe(true);
    expect(oldest.orders[0]!.createdAt < oldest.orders[4]!.createdAt).toBe(true);
    expect(newest.total).toBe(12);
  });

  it("sorts by amount as a number, not as text", async () => {
    // The trap: as text, "9" sorts after "10". These amounts cross that
    // boundary on purpose.
    const res = (await adminOrders(asAdmin("/api/shop/admin/orders?sort=total&dir=asc&limit=100") as never)) as Response;
    const amounts = (await bodyOf<OrderPage>(res)).orders.map((o) => o.totalMinor);
    expect(amounts).toEqual([...amounts].sort((a, b) => a - b));
  });

  it("pages a sort without repeating or losing a row", async () => {
    const seen: string[] = [];
    let cursor: string | null = null;
    for (let guard = 0; guard < 10; guard++) {
      const url: string = `/api/shop/admin/orders?limit=5&sort=total&dir=desc${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`;
      const body: OrderPage = await bodyOf<OrderPage>((await adminOrders(asAdmin(url) as never)) as Response);
      seen.push(...body.orders.map((o) => o.number));
      cursor = body.nextCursor;
      if (!cursor) break;
    }
    expect(seen).toHaveLength(12);
    expect(new Set(seen).size).toBe(12);
  });

  it("keeps unpaid orders in the list when sorting by when they were paid", async () => {
    // A NULL satisfies no comparison, so a cursor over paid_at would drop every
    // unpaid order the moment the reader turned a page.
    const res = (await adminOrders(asAdmin("/api/shop/admin/orders?sort=paid&limit=100") as never)) as Response;
    expect((await bodyOf<OrderPage>(res)).orders).toHaveLength(12);
  });

  it("refuses a sort it does not have", async () => {
    const res = (await adminOrders(asAdmin("/api/shop/admin/orders?sort=customer") as never)) as Response;
    expect(res.status).toBe(422);
  });
});

describe("filtering the orders", () => {
  beforeEach(async () => {
    await seedOrders(12);
  });

  it("filters by provider", async () => {
    const res = (await adminOrders(asAdmin("/api/shop/admin/orders?gateway=paypal&limit=100") as never)) as Response;
    const body = await bodyOf<OrderPage>(res);
    expect(body.orders.every((o) => o.gateway === "paypal")).toBe(true);
    expect(body.total).toBe(body.orders.length);
  });

  it("filters by a date range, with the closing day included", async () => {
    const res = (await adminOrders(
      asAdmin("/api/shop/admin/orders?from=2026-01-03&to=2026-01-05&limit=100") as never,
    )) as Response;
    const body = await bodyOf<OrderPage>(res);
    // The 3rd, 4th and 5th: "up to the 5th" means the whole of it.
    expect(body.orders).toHaveLength(3);
  });

  it("refuses a range that is not one", async () => {
    const res = (await adminOrders(asAdmin("/api/shop/admin/orders?from=last+tuesday") as never)) as Response;
    expect(res.status).toBe(422);
  });

  it("counts the filtered selection, not the whole table", async () => {
    const res = (await adminOrders(asAdmin("/api/shop/admin/orders?status=paid&limit=2") as never)) as Response;
    const body = await bodyOf<OrderPage>(res);
    expect(body.orders).toHaveLength(2);
    expect(body.total).toBe(4); // every third of twelve
  });
});

describe("paging the queue", () => {
  it("hands out one page and says where the next begins", async () => {
    for (let i = 0; i < 12; i++) await enqueue(env, "email", `buyer${i}@example.com`, {});

    const first = (await adminOutbox(asAdmin("/api/shop/admin/outbox?limit=5") as never)) as Response;
    const body = await bodyOf<{ messages: unknown[]; total: number; nextCursor: string | null }>(first);
    expect(body.messages).toHaveLength(5);
    expect(body.total).toBe(12);
    expect(body.nextCursor).not.toBe(null);

    const second = (await adminOutbox(
      asAdmin(`/api/shop/admin/outbox?limit=5&cursor=${encodeURIComponent(body.nextCursor!)}`) as never,
    )) as Response;
    expect((await bodyOf<{ messages: unknown[] }>(second)).messages).toHaveLength(5);
  });
});
