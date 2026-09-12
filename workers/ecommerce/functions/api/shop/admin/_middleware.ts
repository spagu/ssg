// Everything under /api/shop/admin/ goes through here.
//
// One gate rather than a check at the top of twenty handlers: a handler added
// later is protected by existing, not by remembering.

import type { Env } from "../_env";
import { requireAdmin } from "../_auth";
import { drainInBackground } from "../_outbox";
import { ensureSchema } from "../_schema";
import type { AdminIdentity } from "../_types";

/** The identity is passed to handlers through `data`, which Pages threads from
 *  middleware to the function it wraps. */
export interface AdminData extends Record<string, unknown> {
  admin: AdminIdentity;
}

export const onRequest: PagesFunction<Env, string, AdminData> = async (context) => {
  const { request, env, next, data, waitUntil } = context;
  const url = new URL(request.url);

  if (!env.SHOP_DB) {
    return new Response(JSON.stringify({ error: "not_configured", message: "The shop database is not bound." }), {
      status: 503,
      headers: { "content-type": "application/json", "cache-control": "no-store" },
    });
  }
  await ensureSchema(env);

  // The auth endpoints are the way in, so they cannot require being in.
  if (url.pathname.startsWith("/api/shop/admin/auth/")) return next();

  const identity = await requireAdmin(request, env);
  if (identity instanceof Response) return identity;
  data.admin = identity;

  // An open panel is the most reliable clock this shop has: every admin request
  // pushes a few rows of the outbox along.
  drainInBackground(env, { waitUntil }, 5);

  return next();
};
