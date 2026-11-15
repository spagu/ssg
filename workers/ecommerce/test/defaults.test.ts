// GO-158: the owner's defaults — gross prices, EUR, Stripe and PayPal both
// offered, HTML invoices — said the same way by the code, the wrangler
// snippet, the example shop and the README.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { DEFAULT_GATEWAYS, enabledGateways } from "../functions/api/shop/_env";
import type { Env } from "../functions/api/shop/_env";
import { SETTING_DEFAULTS } from "../functions/api/shop/_settings";

const file = (path: string) => readFileSync(new URL(path, import.meta.url), "utf8");
const both = { STRIPE_SECRET_KEY: "sk_live_x", PAYPAL_CLIENT_ID: "id", PAYPAL_CLIENT_SECRET: "secret" } as Env;

test("code: gross prices in EUR; both providers offered, Stripe first, each once configured", () => {
  assert.equal(SETTING_DEFAULTS["pricing.mode"], "gross");
  assert.equal(SETTING_DEFAULTS["currency.base"], "EUR");
  assert.equal(DEFAULT_GATEWAYS, "stripe,paypal");
  assert.deepEqual(enabledGateways(both), ["stripe", "paypal"]);
  assert.deepEqual(enabledGateways({ PAYPAL_CLIENT_ID: "id", PAYPAL_CLIENT_SECRET: "secret" } as Env), ["paypal"]);
  assert.deepEqual(enabledGateways({ ...both, SHOP_GATEWAYS: "paypal" }), ["paypal"]);
});

test("the snippet, the example and the README say the same", () => {
  const snippet = file("../wrangler.snippet.toml");
  assert.match(snippet, /# SHOP_GATEWAYS = "stripe,paypal"\s+# the default/);
  assert.match(snippet, /prices are gross \(VAT\n# included\), the base currency is EUR, Stripe and PayPal are both offered and\n# invoices are HTML/);
  const example = file("../../../examples/ebook-shop/wrangler.toml");
  assert.match(example, /SHOP_GATEWAYS = "stripe,paypal"/);
  const seed = file("../../../examples/ebook-shop/scripts/seed.sh");
  assert.match(seed, /"currency.base": "EUR"/);
  assert.match(seed, /"pricing.mode": "gross"/);
  const readme = file("../README.md");
  assert.match(readme, /\| Prices \| gross/);
  assert.match(readme, /\| Currency \| EUR/);
  assert.match(readme, /\| Payment providers \| Stripe and PayPal/);
  assert.match(readme, /\| Invoices \| HTML/);
});

test("no variable the code never reads is documented as a setting", () => {
  for (const text of [file("../wrangler.snippet.toml"), file("../../../examples/ebook-shop/wrangler.toml"), file("../README.md")]) {
    assert.doesNotMatch(text, /SHOP_BASE_CURRENCY/);
  }
});
