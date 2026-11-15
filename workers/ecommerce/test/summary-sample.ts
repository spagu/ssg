// The summary sample workers/modules.test.ts reads (GO-156): drives GET
// /api/shop/module/summary through the shared gate on a seeded database —
// every metric, every kind of attention item and a few problems show — and
// prints the body. Run from the repository root:
//   node --import ./workers/ecommerce/test/resolve.mjs workers/ecommerce/test/summary-sample.ts
import { onRequestGet } from "../functions/api/shop/module/summary.ts";
import { PASSWORD, ask, shopEnv } from "./admin-fixtures.ts";
import { seedSummaryShop } from "./summary-fixtures.ts";

// A public host with Stripe's test key and PayPal's sandbox: both warn. No
// R2, KV or mail: three problems.
const env = shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD, STRIPE_SECRET_KEY: "sk_test_x", PAYPAL_CLIENT_ID: "id", PAYPAL_CLIENT_SECRET: "secret", SHOP_GATEWAYS: "stripe,paypal" });
await seedSummaryShop(env);
const res = await onRequestGet({ request: await ask("/api/shop/module/summary", { password: PASSWORD }), env } as never);
const body = await res.text();
if (res.status !== 200) throw new Error(`summary answered ${res.status}: ${body}`);
process.stdout.write(body);
