// The payments screen's API.
//
// It reports what a seller cannot otherwise find out — is this wired up, test
// or live, has a webhook ever arrived — and it decides which providers a buyer
// is offered. What it deliberately cannot do is show or change a key: those are
// wrangler secrets, not rows, and the test below is the one that keeps it that
// way.

import { env } from "cloudflare:test";
import { beforeEach, describe, expect, it } from "vitest";
import { onRequestGet as readGateways, onRequestPost as writeOrder } from "../functions/api/shop/admin/gateways";
import { onRequestGet as catalogue } from "../functions/api/shop/products/index";
import { onRequestPost as checkout } from "../functions/api/shop/checkout";
import { offeredGateways } from "../functions/api/shop/_gateways";
import { nowISO } from "../functions/api/shop/_lib";
import type { AdminIdentity } from "../functions/api/shop/_types";
import { bodyOf, ctx, freshShop, get, ORIGIN, post, seedProduct } from "./_helpers";

const owner: AdminIdentity = { sub: "owner-1", email: "owner@example.com", role: "owner", via: "jwt" };
const staff: AdminIdentity = { sub: "staff-1", email: "staff@example.com", role: "staff", via: "jwt" };

const both = () => ({
  ...env,
  STRIPE_SECRET_KEY: "sk_test_x",
  STRIPE_WEBHOOK_SECRET: "whsec_x",
  PAYPAL_CLIENT_ID: "id",
  PAYPAL_CLIENT_SECRET: "secret",
  PAYPAL_WEBHOOK_ID: "wh-1",
  SHOP_GATEWAYS: "stripe,paypal",
});

type ShopEnv = ReturnType<typeof both>;

const asAdmin = (request: Request, envOverride: Partial<ShopEnv> = both(), who = owner) => ({
  ...ctx(request, {}, { admin: who }),
  env: envOverride,
});

const orderBody = (order: unknown) =>
  new Request(`${ORIGIN}/api/shop/admin/gateways`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ order }),
  });

interface GatewayOut {
  name: string;
  configured: boolean;
  enabled: boolean;
  missing: string[];
  mode: string;
  webhookUrl: string;
  events: string[];
  eventsSeen: number;
  lastEventType: string | null;
  paymentsTaken: number;
}
interface GatewayList {
  gateways: GatewayOut[];
  order: string[];
  testMode: boolean;
  note: string;
}

beforeEach(async () => {
  await freshShop();
});

describe("what the screen reports", () => {
  it("names what each provider is missing, rather than just saying no", async () => {
    const res = (await readGateways(
      asAdmin(get("/api/shop/admin/gateways"), { ...env, STRIPE_SECRET_KEY: "sk_test_x" }) as never,
    )) as Response;
    const body = await bodyOf<GatewayList>(res);
    const stripe = body.gateways.find((g) => g.name === "stripe")!;
    const paypal = body.gateways.find((g) => g.name === "paypal")!;

    // The key is there and the webhook secret is not, which is the state a shop
    // sits in for the twenty minutes between the two dashboard pages.
    expect(stripe.configured).toBe(false);
    expect(stripe.missing).toEqual(["STRIPE_WEBHOOK_SECRET"]);
    expect(paypal.missing).toEqual(["PAYPAL_CLIENT_ID", "PAYPAL_CLIENT_SECRET", "PAYPAL_WEBHOOK_ID"]);
  });

  it("says whether the keys in use are test keys", async () => {
    const test = await bodyOf<GatewayList>((await readGateways(asAdmin(get("/x")) as never)) as Response);
    expect(test.gateways.find((g) => g.name === "stripe")!.mode).toBe("test");

    const live = await bodyOf<GatewayList>(
      (await readGateways(asAdmin(get("/x"), { ...both(), STRIPE_SECRET_KEY: "sk_live_x" }) as never)) as Response,
    );
    expect(live.gateways.find((g) => g.name === "stripe")!.mode).toBe("live");
  });

  it("hands over the exact webhook URL and the events to send", async () => {
    const body = await bodyOf<GatewayList>(
      (await readGateways(asAdmin(get("/api/shop/admin/gateways")) as never)) as Response,
    );
    const stripe = body.gateways.find((g) => g.name === "stripe")!;

    // The half that goes wrong quietly: without it the shop never hears that a
    // payment succeeded, and a buyer pays without being sent their book.
    expect(stripe.webhookUrl).toBe(`${ORIGIN}/api/shop/webhooks/stripe`);
    expect(stripe.events).toContain("checkout.session.completed");
    expect(stripe.events).toContain("charge.refunded");
  });

  it("says whether a webhook has ever arrived, and when", async () => {
    const before = await bodyOf<GatewayList>((await readGateways(asAdmin(get("/x")) as never)) as Response);
    expect(before.gateways.find((g) => g.name === "stripe")!.eventsSeen).toBe(0);

    await env.SHOP_DB.prepare(
      `INSERT INTO webhook_inbox (gateway, event_id, event_type, received_at) VALUES ('stripe', 'evt_1', 'checkout.session.completed', ?)`,
    )
      .bind(nowISO())
      .run();

    const after = await bodyOf<GatewayList>((await readGateways(asAdmin(get("/x")) as never)) as Response);
    const stripe = after.gateways.find((g) => g.name === "stripe")!;
    expect(stripe.eventsSeen).toBe(1);
    expect(stripe.lastEventType).toBe("checkout.session.completed");
  });

  it("counts the payments each provider has actually taken", async () => {
    await env.SHOP_DB.prepare(
      `INSERT INTO payments (id, order_id, gateway, gateway_ref, kind, status, amount_minor, currency, created_at)
       VALUES ('p1', 'o1', 'stripe', 'pi_1', 'charge', 'succeeded', 2400, 'EUR', ?)`,
    )
      .bind(nowISO())
      .run();

    const body = await bodyOf<GatewayList>((await readGateways(asAdmin(get("/x")) as never)) as Response);
    expect(body.gateways.find((g) => g.name === "stripe")!.paymentsTaken).toBe(1);
  });

  it("shows no key, and says why there is none to show", async () => {
    const res = (await readGateways(asAdmin(get("/x")) as never)) as Response;
    const text = await res.text();

    // The whole point of keeping them in wrangler rather than in the database:
    // this endpoint could not leak one if it wanted to.
    expect(text).not.toContain("sk_test_x");
    expect(text).not.toContain("whsec_x");
    expect(text).not.toContain("secret\"");
    expect(JSON.parse(text).note).toMatch(/never stored in this shop's database/);
  });
});

describe("choosing what a buyer is offered", () => {
  it("sets the order, and the first one is the default at checkout", async () => {
    const res = (await writeOrder(asAdmin(orderBody(["paypal", "stripe"])) as never)) as Response;
    expect(res.status).toBe(200);
    expect(await offeredGateways(both())).toEqual(["paypal", "stripe"]);

    await seedProduct({ sku: "EBOOK-1" });
    const realFetch = globalThis.fetch;
    const called: string[] = [];
    globalThis.fetch = (async (url: unknown) => {
      called.push(String(url));
      if (String(url).includes("oauth2/token")) {
        return new Response(JSON.stringify({ access_token: "t", expires_in: 3600 }));
      }
      return new Response(JSON.stringify({ id: "PP-1", links: [{ rel: "approve", href: "https://paypal/x" }] }));
    }) as unknown as typeof fetch;

    // No gateway named in the request: it takes the first the seller chose.
    const bought = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: [{ sku: "EBOOK-1", quantity: 1 }],
        email: "buyer@example.com", country: "DE", consentWaiver: "1",
      })),
      env: both(),
    } as never)) as Response;
    globalThis.fetch = realFetch;

    expect(bought.status).toBe(200);
    expect(called.join(" ")).toContain("paypal");
  });

  it("tells the storefront the same order", async () => {
    await writeOrder(asAdmin(orderBody(["paypal", "stripe"])) as never);
    const res = (await catalogue({ ...ctx(get("/api/shop/products")), env: both() } as never)) as Response;
    expect(await bodyOf<{ shop: { gateways: string[] } }>(res)).toMatchObject({
      shop: { gateways: ["paypal", "stripe"] },
    });
  });

  it("stops offering one that was taken off the list", async () => {
    await writeOrder(asAdmin(orderBody(["stripe"])) as never);
    expect(await offeredGateways(both())).toEqual(["stripe"]);

    await seedProduct({ sku: "EBOOK-1" });
    const res = (await checkout({
      ...ctx(post("/api/shop/checkout", {
        items: [{ sku: "EBOOK-1", quantity: 1 }], gateway: "paypal",
        email: "buyer@example.com", country: "DE", consentWaiver: "1",
      })),
      env: both(),
    } as never)) as Response;

    // Configured is not the same as offered: a provider the seller has taken
    // off the storefront must not still be reachable by posting its name.
    expect(await bodyOf<{ error: string }>(res)).toMatchObject({ error: "unknown_gateway" });
  });

  it("refuses to offer one that has no credentials", async () => {
    const res = (await writeOrder(
      asAdmin(orderBody(["stripe", "paypal"]), { ...env, STRIPE_SECRET_KEY: "sk_test_x", STRIPE_WEBHOOK_SECRET: "w" }) as never,
    )) as Response;
    // The button would take a buyer to a 502 at the last step of a purchase.
    expect(res.status).toBe(422);
    expect(await bodyOf<{ error: string }>(res)).toMatchObject({ error: "gateway_unavailable" });
  });

  it("refuses a provider it has never heard of", async () => {
    const res = (await writeOrder(asAdmin(orderBody(["bitcoin"])) as never)) as Response;
    expect(res.status).toBe(422);
  });

  it("refuses a body that is not a list", async () => {
    const res = (await writeOrder(asAdmin(orderBody("stripe")) as never)) as Response;
    expect(res.status).toBe(400);
  });

  it("keeps staff out of it", async () => {
    const res = (await writeOrder(asAdmin(orderBody(["stripe"]), both(), staff) as never)) as Response;
    expect(res.status).toBe(403);
  });

  it("falls back to the environment when nobody has chosen", async () => {
    expect(await offeredGateways(both())).toEqual(["stripe", "paypal"]);
  });

  it("drops a chosen provider whose keys have since been removed", async () => {
    await writeOrder(asAdmin(orderBody(["stripe", "paypal"])) as never);
    // PayPal's credentials are taken away afterwards.
    const without = { ...both(), PAYPAL_CLIENT_ID: undefined, PAYPAL_CLIENT_SECRET: undefined };
    expect(await offeredGateways(without)).toEqual(["stripe"]);
  });
});
