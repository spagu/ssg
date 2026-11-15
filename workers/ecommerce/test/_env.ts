// The bindings every test file starts from: a working shop on fake storage.
//
// node --test runs each test file in its own process, so each file gets its own
// database, bucket and namespace — the isolation the Workers test pool used to
// give. A test that needs a different shape spreads this one: `{ ...env, X }`.
import type { Env } from "../functions/api/shop/_env";
import { fakeD1, fakeKV, fakeR2, type FakeD1, type FakeKV, type FakeR2 } from "./fakes";

export const env = {
  SHOP_DB: fakeD1(),
  SHOP_FILES: fakeR2(),
  SHOP_KV: fakeKV(),
  // Enough configuration to be a working shop.
  SHOP_TEST: "1",
  SHOP_ADMIN_PASSWORD: "test-admin-password",
  SHOP_IP_SALT: "test-salt",
  SHOP_GATEWAYS: "stripe,paypal",
  SHOP_PAYPAL_ENV: "sandbox",
} as unknown as Env & { SHOP_DB: FakeD1 & D1Database; SHOP_FILES: FakeR2 & R2Bucket; SHOP_KV: FakeKV & KVNamespace };
