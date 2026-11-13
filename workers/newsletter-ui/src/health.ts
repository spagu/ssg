// GET {base}/api/health — what an admin (or a deploy check) needs to see that
// the worker is wired up: schema version, whether the `newsletter` template's
// table is there, and which optional bindings are bound. Behind admin auth like
// every other API route; it names no secrets, only whether they are set.

import type { RouteContext } from "./router";
import { json } from "./lib";
import { schemaVersion, subscribersReady } from "./schema";

export async function health({ env }: RouteContext): Promise<Response> {
  const subscribers = await subscribersReady(env);
  return json({
    ok: subscribers,
    schemaVersion: await schemaVersion(env),
    subscribersTable: subscribers,
    auth: env.NEWSLETTER_ACCESS_TEAM && env.NEWSLETTER_ACCESS_AUD ? "access" : "basic",
    bindings: {
      queue: !!env.NEWSLETTER_QUEUE,
      rateLimiter: !!env.RATE_LIMITER,
      assets: !!env.ASSETS,
    },
    ...(subscribers ? {} : { hint: MISSING_SUBSCRIBERS }),
  });
}

export const MISSING_SUBSCRIBERS =
  "the subscribers table does not exist yet: deploy the `newsletter` template (ssg new worker newsletter) " +
  "against the same D1 database and send one request to it, or apply workers/newsletter/schema.sql";
