// GET {base}/api/health — what an admin (or a deploy check) needs to see that
// the worker is wired up: schema version, whether the `newsletter` template's
// table is there, and which optional bindings are bound. Behind admin auth like
// every other API route; it names no secrets, only whether they are set.

import type { RouteContext } from "./router";
import { modeOf } from "./auth";
import { json } from "./lib";
import type { Env } from "./lib";
import { schemaVersion, subscribersReady } from "./schema";

export async function health({ env }: RouteContext): Promise<Response> {
  const subscribers = await subscribersReady(env);
  return json({
    ok: subscribers,
    schemaVersion: await schemaVersion(env),
    subscribersTable: subscribers,
    auth: authName(env),
    bindings: {
      queue: !!env.NEWSLETTER_QUEUE,
      rateLimiter: !!env.RATE_LIMITER,
      assets: !!env.ASSETS,
    },
    ...(subscribers ? {} : { hint: MISSING_SUBSCRIBERS }),
  });
}

// authName: "access", "auth" or "basic" — the password mode keeps the name
// health and settings have always reported.
export const authName = (env: Env): string => {
  const mode = modeOf(env);
  return mode === "password" ? "basic" : mode;
};

export const MISSING_SUBSCRIBERS =
  "the subscribers table does not exist yet: deploy the `newsletter` template (ssg new worker newsletter) " +
  "against the same D1 database and send one request to it, or apply workers/newsletter/schema.sql";
