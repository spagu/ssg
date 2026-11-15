// GO-156: the shop as a module — its manifest and summary routes behind the
// admin gate, the summary's numbers and attention items on a seeded shop, the
// configuration problems, and no personal data in the answer.
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { readSummary } from "../../module-manifest.ts";
import { MODULE } from "../functions/api/shop/_module";
import { MODULE_ROUTES } from "../functions/api/shop/_permissions";
import { configProblems, isLocalHost, shopSummary } from "../functions/api/shop/_summary";
import { counted, summaryOf } from "../functions/api/shop/_summary-kit";
import { invalidateSettings } from "../functions/api/shop/_settings";
import { onRequestGet as manifest } from "../functions/api/shop/module/index";
import { onRequestGet as summaryRoute } from "../functions/api/shop/module/summary";
import { AUTH_VARS, PASSWORD, ask, shopEnv } from "./admin-fixtures";
import { fakeKV, fakeR2 } from "./fakes";
import { PERSONAL, seedSummaryShop } from "./summary-fixtures";

const schema = JSON.parse(readFileSync(new URL("../../module-summary.schema.json", import.meta.url), "utf8"));
const NOW = Date.parse("2026-11-14T12:00:00.000Z");
const DAY = 86_400_000;
const complete = () => ({ SHOP_FILES: fakeR2(), SHOP_KV: fakeKV(), SHOP_MAIL_URL: "https://mail.example.com/send", SHOP_MAIL_FROM: "shop@example.com", STRIPE_SECRET_KEY: "sk_live_x", SHOP_GATEWAYS: "stripe,paypal" });
const valid = (s: unknown) => {
  const r = readSummary(s, schema);
  assert.ok(r.ok, r.ok ? "" : r.errors.join("\n"));
};
const call = async (route: typeof manifest, path: string, env: ReturnType<typeof shopEnv>, a: Parameters<typeof ask>[1]) => route({ request: await ask(path, a), env } as never);

describe("the summary of a seeded shop", () => {
  it("counts paid orders, revenue after refunds in the base currency, refunds and products on sale", async () => {
    const env = shopEnv({ ...complete(), STRIPE_SECRET_KEY: "sk_test_x", PAYPAL_CLIENT_ID: "id", PAYPAL_CLIENT_SECRET: "s" });
    await seedSummaryShop(env, NOW);
    const s = await shopSummary(env, { live: true }, new Date(NOW));
    valid(s);
    assert.equal(s.generatedAt, "2026-11-14T12:00:00.000Z");
    assert.deepEqual(s.metrics.map((m) => [m.key, m.value]), [["paidOrders", 4], ["revenue", 43], ["refunds", 2], ["productsOnSale", 2]]);
    assert.equal(s.metrics[1]!.label.en, "Revenue after refunds, last 30 days (EUR)");
    assert.deepEqual(
      s.attention.map((a) => [a.severity, a.href, a.text.en]),
      [
        ["action", "/ecommerce-admin/#orders", "1 order needs your review before the buyer gets the files."],
        ["action", "/ecommerce-admin/#orders", "1 paid order has no download link yet: the buyer is waiting for the file."],
        ["warning", "/ecommerce-admin/#payments", "1 payment notification from Stripe or PayPal could not be applied in the last 7 days."],
        ["warning", "/ecommerce-admin/#log", "1 outgoing message (an email or a webhook) keeps failing and needs attention."],
        ["warning", "/ecommerce-admin/#payments", "Stripe uses a test key (sk_test_…) on a public site: buyers cannot really pay."],
        ["warning", "/ecommerce-admin/#payments", 'PayPal runs in its sandbox on a public site (SHOP_PAYPAL_ENV is not "live"): buyers cannot really pay.'],
        ["warning", "/ecommerce-admin/#vat", "1 VAT rate ends within 30 days and has no rate to follow it."],
      ],
    );
    assert.deepEqual(s.problems, []);
  });

  it("holds no personal data: no name, address, email, VAT number, IP or browser", async () => {
    const env = shopEnv(complete());
    await seedSummaryShop(env, NOW);
    const text = JSON.stringify(await shopSummary(env, { live: true }, new Date(NOW)));
    for (const value of PERSONAL) assert.ok(!text.includes(value), value);
    assert.ok(!text.includes("@"), "no address of any kind");
  });

  it("plural forms follow each language", async () => {
    const env = shopEnv(complete());
    await seedSummaryShop(env, NOW);
    await env.SHOP_DB.prepare("UPDATE orders SET status = 'needs_review' WHERE id IN ('o5', 'o6')").run();
    const s = await shopSummary(env, { live: false }, new Date(NOW));
    assert.equal(s.attention[0]!.text.pl, "3 zamówienia czekają na Twoje sprawdzenie, zanim kupujący dostaną pliki.");
    assert.equal(s.attention[0]!.text.de, "3 Bestellungen warten auf Ihre Prüfung, bevor die Käufer ihre Dateien erhalten.");
  });

  it("an empty shop: zeros, nothing to do", async () => {
    const env = shopEnv(complete());
    await seedSummaryShop(env, NOW);
    for (const table of ["orders", "payments", "products", "outbox", "webhook_inbox", "vat_rates"]) await env.SHOP_DB.prepare(`DELETE FROM ${table}`).run();
    const s = await shopSummary(env, { live: true }, new Date(NOW));
    assert.deepEqual(s.metrics.map((m) => m.value), [0, 0, 0, 0]);
    assert.deepEqual(s.attention, []);
  });

  it("refunds larger than the window's sales count as nothing earned, not as a negative number", async () => {
    const env = shopEnv(complete());
    await seedSummaryShop(env, NOW);
    await env.SHOP_DB.prepare("UPDATE payments SET amount_minor = -99900 WHERE id = 'pay-2'").run();
    assert.equal((await shopSummary(env, { live: true }, new Date(NOW))).metrics[1]!.value, 0);
  });

  it("revenue is in the currency's main unit, whatever its decimals", async () => {
    const env = shopEnv(complete());
    await seedSummaryShop(env, NOW, { "currency.base": "JPY" });
    await env.SHOP_DB.prepare("UPDATE orders SET currency = 'JPY'").run();
    const s = await shopSummary(env, { live: true }, new Date(NOW));
    assert.deepEqual([s.metrics[1]!.value, s.metrics[1]!.label.en], [7700, "Revenue after refunds, last 30 days (JPY)"]);
  });

  it("a base currency cleared by hand reads as EUR", async () => {
    const env = shopEnv(complete());
    await seedSummaryShop(env, NOW);
    await env.SHOP_DB.prepare(`INSERT OR REPLACE INTO settings (key, value, updated_at) VALUES ('currency.base', '""', '2026-11-14')`).run();
    invalidateSettings();
    assert.equal((await shopSummary(env, { live: true }, new Date(NOW))).metrics[1]!.label.en, "Revenue after refunds, last 30 days (EUR)");
  });

  it("an old VAT table is worth a look; a live PayPal and a local host warn of nothing", async () => {
    const env = shopEnv({ ...complete(), STRIPE_SECRET_KEY: "sk_test_x", PAYPAL_CLIENT_ID: "id", PAYPAL_CLIENT_SECRET: "s", SHOP_PAYPAL_ENV: "live" });
    await seedSummaryShop(env, NOW);
    await env.SHOP_DB.prepare("UPDATE vat_rates SET valid_from = ?").bind(new Date(NOW - 400 * DAY).toISOString()).run();
    const live = await shopSummary(env, { live: true }, new Date(NOW));
    assert.deepEqual(live.attention.filter((a) => a.href.endsWith("#payments")).map((a) => a.text.en.split(" ")[0]), ["1", "Stripe"]);
    assert.deepEqual(live.attention.at(-1), { severity: "info", text: live.attention.at(-1)!.text, href: "/ecommerce-admin/#vat" });
    assert.match(live.attention.at(-1)!.text.en, /over a year ago/);
    const local = await shopSummary(env, { live: false }, new Date(NOW));
    assert.ok(!local.attention.some((a) => /test key|sandbox/.test(a.text.en)));
  });
});

describe("the summary kit's copy", () => {
  it("keeps a series to its cap, falls back to the plural form `other`, and defaults the lists", () => {
    const text = { en: "t", pl: "t", hi: "t", de: "t", fr: "t" };
    const points = Array.from({ length: 95 }, (_, i) => ({ day: new Date(Date.UTC(2026, 0, 1 + i)).toISOString().slice(0, 10), value: i }));
    const s = summaryOf({ metrics: [], series: { label: text, points } }, new Date(NOW));
    assert.equal(s.series!.points.length, 90);
    assert.deepEqual([s.attention, s.problems], [[], []]);
    const only = { other: "{count} x" };
    assert.equal(counted({ en: only, pl: only, hi: only, de: only, fr: only }, 1).en, "1 x");
  });
});

describe("configuration problems", () => {
  it("names each missing binding, provider, mail and seller detail — never a value", () => {
    const problems = configProblems(shopEnv({ STRIPE_SECRET_KEY: "" }), { "modules.invoices": true, "seller.name": "x", "seller.address": "", "seller.country": "PL" });
    assert.deepEqual(problems.map((p) => p.code), ["no_files", "no_kv", "no_gateway", "no_mail", "no_seller_details"]);
    for (const p of problems) for (const lang of ["en", "pl", "hi", "de", "fr"] as const) assert.ok(p.text[lang].length > 10);
    const off = configProblems(shopEnv(complete()), { "modules.invoices": false });
    assert.deepEqual(off, [], "no invoices of ours: the seller block is not needed");
  });

  it("without a database: only the problems", async () => {
    const env = { ...shopEnv(complete()), SHOP_DB: undefined } as never;
    const s = await shopSummary(env, { live: true }, new Date(NOW));
    valid(s);
    assert.deepEqual([s.metrics, s.attention, s.problems.map((p) => p.code)], [[], [], ["no_database"]]);
  });

  it("isLocalHost: development hosts only", () => {
    for (const h of ["localhost", "shop.localhost", "127.0.0.1", "[::1]", "LOCALHOST"]) assert.equal(isLocalHost(h), true, h);
    for (const h of ["example.com", "127.example.com", "localhost.example.com", "shop.pages.dev"]) assert.equal(isLocalHost(h), false, h);
  });
});

describe("the module routes", () => {
  it("are exactly the files under module/, each in the permission table", () => {
    const dir = new URL("../functions/api/shop/module/", import.meta.url);
    const onDisk = readdirSync(dir).map((f) => `GET /api/shop/module${f === "index.ts" ? "" : `/${f.replace(/\.ts$/, "")}`}`);
    assert.deepEqual(onDisk.sort(), Object.keys(MODULE_ROUTES).sort());
    assert.deepEqual(MODULE.endpoints, { manifest: "/api/shop/module", summary: "/api/shop/module/summary" });
  });

  it("serve the manifest and the summary behind the admin gate", async () => {
    const env = shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD });
    const res = await call(manifest, "/api/shop/module", env, { password: PASSWORD });
    assert.equal(res.status, 200);
    assert.deepEqual(await res.json(), JSON.parse(readFileSync(new URL("../module.json", import.meta.url), "utf8")));
    assert.equal(res.headers.get("cache-control"), "no-store");
    assert.equal((await call(summaryRoute, "/api/shop/module/summary", env, { password: PASSWORD })).status, 200);
    for (const route of [manifest, summaryRoute]) assert.equal((await call(route, "/api/shop/module", env, {})).status, 401);
  });

  it("in auth mode need ecommerce:read", async () => {
    const env = shopEnv(AUTH_VARS);
    for (const [route, path] of [[manifest, "/api/shop/module"], [summaryRoute, "/api/shop/module/summary"]] as const) {
      const other = await call(route, path, env, { perms: ["ecommerce:orders"] });
      assert.equal(other.status, 403);
      assert.equal(((await other.json()) as { permission: string }).permission, "ecommerce:read");
      assert.equal((await call(route, path, env, { perms: ["ecommerce:read"] })).status, 200);
    }
  });

  it("without a database: the summary still answers in auth mode; the password mode and the manifest get the admin API's 503", async () => {
    const auth = { ...shopEnv(AUTH_VARS), SHOP_DB: undefined } as never;
    const s = await call(summaryRoute, "/api/shop/module/summary", auth, { perms: ["ecommerce:read"] });
    assert.equal(s.status, 200);
    assert.deepEqual(((await s.json()) as { problems: { code: string }[] }).problems.map((p) => p.code).slice(0, 1), ["no_database"]);
    const manifestRes = await call(manifest, "/api/shop/module", auth, { perms: ["ecommerce:read"] });
    assert.equal(manifestRes.status, 503);
    const password = { ...shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD }), SHOP_DB: undefined } as never;
    const p = await call(summaryRoute, "/api/shop/module/summary", password, { password: PASSWORD });
    assert.equal(p.status, 503);
    assert.equal(((await p.json()) as { error: string }).error, "not_configured");
  });

  it("the summary route reads the host: a public one is live", async () => {
    const env = shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD, STRIPE_SECRET_KEY: "sk_test_x" });
    const res = await call(summaryRoute, "/api/shop/module/summary", env, { password: PASSWORD });
    const body = (await res.json()) as { attention: { text: { en: string } }[] };
    assert.ok(body.attention.some((a) => a.text.en.startsWith("Stripe uses a test key")));
  });
});
