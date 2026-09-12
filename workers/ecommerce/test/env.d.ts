// The bindings the test runtime provides.
//
// The pool types `env` from "cloudflare:test" as Cloudflare.Env — the global
// interface wrangler normally generates from a project's bindings. This shop
// declares its own Env by hand (functions/api/shop/_env.ts), so the two are
// joined here: what the tests reach for is exactly what vitest.config.ts
// configures.

import type { Env as ShopEnv } from "../functions/api/shop/_env";

declare global {
  namespace Cloudflare {
    interface Env extends ShopEnv {}
  }
}

export {};
