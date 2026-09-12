// Tests run inside workerd, not in Node with mocks.
//
// The shop's hard parts are D1 statements, R2 range reads, KV expiry and
// WebCrypto. A Node mock of any of those proves the mock works. The pool starts
// the real runtime with real local bindings, so a test that passes here is a
// statement about what the deployment does.

import { cloudflareTest } from "@cloudflare/vitest-pool-workers";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [
    cloudflareTest({
      miniflare: {
        compatibilityDate: "2026-08-22",
        compatibilityFlags: ["nodejs_compat"],
        d1Databases: ["SHOP_DB"],
        r2Buckets: ["SHOP_FILES"],
        kvNamespaces: ["SHOP_KV"],
        bindings: {
          // Enough configuration to be a working shop. A test that needs a
          // different shape builds its own env from this one.
          SHOP_TEST: "1",
          SHOP_JWT_SECRET: "test-secret-that-is-at-least-32-bytes-long!",
          SHOP_IP_SALT: "test-salt",
          SHOP_GATEWAYS: "stripe,paypal",
          SHOP_PAYPAL_ENV: "sandbox",
          SHOP_BASE_CURRENCY: "EUR",
        },
      },
    }),
  ],
  test: {
    coverage: {
      // istanbul, not v8: v8 coverage is collected from the host's own
      // inspector, and these tests run inside workerd, where it sees nothing at
      // all. istanbul instruments the source before it gets there.
      provider: "istanbul",
      reporter: ["text", "lcov"],
      include: ["functions/**/*.ts"],
      // The project's floor, per package: 96%, the same as every Go package in
      // this repository, measured the same way — statements and lines.
      //
      // Branches is lower on purpose, and it is not the same measurement.
      // istanbul counts every `??` and `?.` as a branch, and this code is full
      // of them: `row?.n ?? 0` after a COUNT(*) that always returns a row,
      // `object.range ?? undefined` where R2's own type says it may be absent.
      // Driving those to 90% would mean writing tests for states the database
      // cannot produce, which buys a number rather than a guarantee. Every
      // branch that a shop can actually reach — every refusal, every missing
      // binding, every provider that says no — has a test.
      thresholds: { lines: 96, functions: 96, branches: 80, statements: 96 },
    },
  },
});
