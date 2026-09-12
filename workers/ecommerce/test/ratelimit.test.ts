// The request budget. What is tested is that a caller who asks too often is
// refused, that two callers do not share a bucket, and that the shop fails in
// the direction its comment claims when the backend itself is broken.

import { env } from "cloudflare:test";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  BUDGETS,
  guard,
  rateLimitingAvailable,
  tooMany,
  withinBudget,
} from "../functions/api/shop/_ratelimit";
import { onRequestPost as checkout } from "../functions/api/shop/checkout";
import { onRequestPost as publicResend } from "../functions/api/shop/resend";
import { onRequestGet as download } from "../functions/api/shop/download/[token]";
import { onRequest as adminGate } from "../functions/api/shop/admin/_middleware";
import { bodyOf, ctx, freshShop, get, ORIGIN, post, seedProduct } from "./_helpers";

const realFetch = globalThis.fetch;
const shopEnv = () => ({ ...env, STRIPE_SECRET_KEY: "sk_test_x", SHOP_GATEWAYS: "stripe" });

/** A request that looks like it came from one address. */
function from(ip: string, path = "/api/shop/checkout"): Request {
  return new Request(`${ORIGIN}${path}`, { headers: { "cf-connecting-ip": ip } });
}

beforeEach(async () => {
  await freshShop();
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe("the budgets themselves", () => {
  it("fails closed where money moves and open where it does not", () => {
    // The two that write or spend must not be opened by a broken backend; the
    // two a buyer waits on must not be closed by one.
    expect(BUDGETS.checkout.onError).toBe("closed");
    expect(BUDGETS.resend.onError).toBe("closed");
    expect(BUDGETS.download.onError).toBe("open");
    expect(BUDGETS.status.onError).toBe("open");
  });

  it("gives the thank-you page room to poll and the mailer none", () => {
    // The page polls while it waits for a webhook, so its budget has to be
    // generous; resend sends email to an address the caller chose.
    expect(BUDGETS.status.max).toBeGreaterThan(BUDGETS.checkout.max);
    expect(BUDGETS.resend.max).toBeLessThan(BUDGETS.checkout.max);
    expect(BUDGETS.resend.windowS).toBeGreaterThan(BUDGETS.checkout.windowS);
  });
});

describe("spending a budget", () => {
  it("allows up to the cap and refuses after it", async () => {
    const request = from("203.0.113.1");
    for (let i = 0; i < BUDGETS.checkout.max; i++) {
      expect((await withinBudget(env, request, "checkout")).ok, `request ${i + 1}`).toBe(true);
    }
    const over = await withinBudget(env, request, "checkout");
    expect(over.ok).toBe(false);
    expect(over.retryAfter).toBe(BUDGETS.checkout.windowS);
  });

  it("does not let one caller spend another's", async () => {
    const noisy = from("203.0.113.1");
    for (let i = 0; i < BUDGETS.checkout.max + 5; i++) await withinBudget(env, noisy, "checkout");
    expect((await withinBudget(env, noisy, "checkout")).ok).toBe(false);

    // Somebody else, arriving now, is unaffected.
    expect((await withinBudget(env, from("203.0.113.2"), "checkout")).ok).toBe(true);
  });

  it("keeps the buckets apart", async () => {
    const request = from("203.0.113.3");
    for (let i = 0; i < BUDGETS.resend.max; i++) await withinBudget(env, request, "resend");
    expect((await withinBudget(env, request, "resend")).ok).toBe(false);
    // Exhausting the mailer must not stop the same person buying something.
    expect((await withinBudget(env, request, "checkout")).ok).toBe(true);
  });

  it("does not count a request it refused", async () => {
    // A bot that keeps knocking must not burn quota belonging to a real
    // visitor behind the same address, so a refusal writes nothing.
    const request = from("203.0.113.4");
    for (let i = 0; i < BUDGETS.resend.max; i++) await withinBudget(env, request, "resend");
    const before = (await env.SHOP_KV!.list({ prefix: "rl:resend:" })).keys.length;
    for (let i = 0; i < 20; i++) await withinBudget(env, request, "resend");
    expect((await env.SHOP_KV!.list({ prefix: "rl:resend:" })).keys.length).toBe(before);
  });

  it("uses the binding when the deployment has one", async () => {
    const asked: string[] = [];
    const bound = {
      ...env,
      RATE_LIMIT_CHECKOUT: {
        limit: async ({ key }: { key: string }) => {
          asked.push(key);
          return { success: asked.length <= 2 };
        },
      },
    };
    expect((await withinBudget(bound, from("203.0.113.5"), "checkout")).ok).toBe(true);
    expect((await withinBudget(bound, from("203.0.113.5"), "checkout")).ok).toBe(true);
    expect((await withinBudget(bound, from("203.0.113.5"), "checkout")).ok).toBe(false);
    // One key per bucket per caller, and it is not the raw address.
    expect(new Set(asked).size).toBe(1);
    expect(asked[0]).not.toContain("203.0.113.5");
  });

  it("falls back to the generic binding for a bucket with none of its own", async () => {
    let calls = 0;
    const bound = {
      ...env,
      RATE_LIMITER: {
        limit: async () => {
          calls += 1;
          return { success: true };
        },
      },
    };
    await withinBudget(bound, from("203.0.113.6"), "download");
    expect(calls).toBe(1);
  });

  it("enforces nothing when there is nothing to enforce with", async () => {
    const bare = { ...env, SHOP_KV: undefined, RATE_LIMITER: undefined };
    for (let i = 0; i < 50; i++) {
      expect((await withinBudget(bare, from("203.0.113.7"), "resend")).ok).toBe(true);
    }
    expect(rateLimitingAvailable(bare)).toBe(false);
    expect(rateLimitingAvailable(env)).toBe(true);
  });

  it("fails the way the bucket says when the backend throws", async () => {
    const broken = {
      ...env,
      RATE_LIMITER: {
        limit: async () => {
          throw new Error("the limiter is down");
        },
      },
    };
    expect((await withinBudget(broken, from("203.0.113.8"), "checkout")).ok).toBe(false);
    expect((await withinBudget(broken, from("203.0.113.8"), "download")).ok).toBe(true);
  });

  it("caps a caller Cloudflare gives no address for, rather than exempting them", async () => {
    const anonymous = new Request(`${ORIGIN}/api/shop/checkout`);
    for (let i = 0; i < BUDGETS.checkout.max; i++) await withinBudget(env, anonymous, "checkout");
    expect((await withinBudget(env, anonymous, "checkout")).ok).toBe(false);
  });

  it("keys on a hash rather than on the address itself", async () => {
    await withinBudget(env, from("203.0.113.9"), "download");
    const { keys } = await env.SHOP_KV!.list({ prefix: "rl:download:" });
    expect(keys).toHaveLength(1);
    expect(keys[0]!.name).not.toContain("203.0.113.9");
  });

  it("still caps a shop that keeps no IP salt at all", async () => {
    const unsalted = { ...env, SHOP_IP_SALT: undefined };
    for (let i = 0; i < BUDGETS.resend.max; i++) await withinBudget(unsalted, from("203.0.113.10"), "resend");
    expect((await withinBudget(unsalted, from("203.0.113.10"), "resend")).ok).toBe(false);
  });
});

describe("the refusal", () => {
  it("looks like every other error, and tells the caller when to come back", async () => {
    const response = tooMany(new Request(ORIGIN, { headers: { "cf-ray": "ray-1" } }), 60);
    expect(response.status).toBe(429);
    expect(response.headers.get("retry-after")).toBe("60");
    expect(response.headers.get("cache-control")).toBe("no-store");
    expect(await response.json()).toEqual({
      error: "rate_limited",
      message: "Too many requests. Wait a moment and try again.",
      requestId: "ray-1",
    });
  });

  it("never waits zero seconds", () => {
    expect(tooMany(new Request(ORIGIN), 0).headers.get("retry-after")).toBe("1");
  });

  it("hands back null while there is budget left", async () => {
    expect(await guard(env, from("203.0.113.11"), "checkout")).toBe(null);
  });
});

describe("the endpoints behind it", () => {
  it("stops a checkout run before it writes an order", async () => {
    await seedProduct({ sku: "EBOOK-1" });
    globalThis.fetch = (async () =>
      new Response(JSON.stringify({ id: "cs_1", url: "https://x" }))) as typeof fetch;

    const body = {
      items: [{ sku: "EBOOK-1", quantity: 1 }], gateway: "stripe",
      email: "buyer@example.com", country: "DE", consentWaiver: "1",
    };
    const request = () =>
      new Request(`${ORIGIN}/api/shop/checkout`, {
        method: "POST",
        headers: { "content-type": "application/json", "cf-connecting-ip": "198.51.100.1" },
        body: JSON.stringify(body),
      });

    let refused: Response | null = null;
    for (let i = 0; i < BUDGETS.checkout.max + 1; i++) {
      const res = (await checkout({ ...ctx(request()), env: shopEnv() } as never)) as Response;
      if (res.status === 429) refused = res;
    }
    expect(refused).not.toBe(null);
    expect(await bodyOf<{ error: string }>(refused!)).toMatchObject({ error: "rate_limited" });

    // Exactly the budget's worth of orders exist, and not one more.
    const orders = await env.SHOP_DB.prepare(`SELECT COUNT(*) AS n FROM orders`).first<{ n: number }>();
    expect(orders?.n).toBe(BUDGETS.checkout.max);
  });

  it("stops a resend run before it queues email", async () => {
    const request = () =>
      new Request(`${ORIGIN}/api/shop/resend`, {
        method: "POST",
        headers: { "content-type": "application/json", "cf-connecting-ip": "198.51.100.2" },
        body: JSON.stringify({ email: "buyer@example.com" }),
      });

    for (let i = 0; i < BUDGETS.resend.max; i++) {
      const res = (await publicResend({ ...ctx(request()), env: shopEnv() } as never)) as Response;
      expect(res.status).toBe(200);
    }
    const over = (await publicResend({ ...ctx(request()), env: shopEnv() } as never)) as Response;
    expect(over.status).toBe(429);
  });

  it("stops someone guessing download links", async () => {
    const request = () =>
      new Request(`${ORIGIN}/api/shop/download/${"x".repeat(40)}`, {
        headers: { "cf-connecting-ip": "198.51.100.3" },
      });

    let refused = false;
    for (let i = 0; i < BUDGETS.download.max + 1; i++) {
      const res = (await download(ctx(request(), { token: "x".repeat(40) }) as never)) as Response;
      if (res.status === 429) refused = true;
    }
    expect(refused).toBe(true);
  });

  it("caps the admin surface, and leaves signing in to its own throttle", async () => {
    const admin = (path: string) => ({
      ...ctx(new Request(`${ORIGIN}${path}`, { headers: { "cf-connecting-ip": "198.51.100.4" } })),
      env: shopEnv(),
      functionPath: path,
    });

    // The login endpoint is never refused by this budget: it has a tighter one
    // of its own, per address and per account.
    for (let i = 0; i < BUDGETS.admin.max + 2; i++) {
      const res = (await adminGate(admin("/api/shop/admin/auth/login") as never)) as Response;
      expect(res.status).not.toBe(429);
    }

    let refused = false;
    for (let i = 0; i < BUDGETS.admin.max + 1; i++) {
      const res = (await adminGate(admin("/api/shop/admin/orders") as never)) as Response;
      if (res.status === 429) refused = true;
    }
    expect(refused).toBe(true);
  });

  it("does not cap the provider webhooks", async () => {
    // Stripe and PayPal legitimately burst, they retry on anything but a 200,
    // and their requests are signature-verified before anything is written. A
    // budget here would drop real payments.
    const source = await import("../functions/api/shop/_webhook");
    expect(Object.keys(source)).toContain("handleWebhook");
    const text = BUDGETS as Record<string, unknown>;
    expect(text).not.toHaveProperty("webhook");
  });
});

describe("the panel is told", () => {
  it("warns when nothing can enforce a limit", async () => {
    const { onRequestGet: me } = await import("../functions/api/shop/admin/me");
    const res = (await me({
      ...ctx(get("/api/shop/admin/me"), {}, {
        admin: { sub: "1", email: "o@example.com", role: "owner", via: "jwt" },
      }),
      env: { ...shopEnv(), SHOP_KV: undefined, RATE_LIMITER: undefined },
    } as never)) as Response;
    const { warnings } = await bodyOf<{ warnings: string[] }>(res);
    expect(warnings.join("\n")).toMatch(/rate limit|budget/i);
  });
});
