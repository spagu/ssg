// Everything under /api/shop/admin/ goes through here.
//
// One gate rather than a check at the top of twenty handlers: a handler added
// later is protected by existing, not by remembering. The gate itself — the
// database, the request budget, who is calling, the route table, service
// tokens, the permission — is _gate.ts, shared with the module routes.

import type { Env } from "../_env";
import type { Admin } from "../_auth";
import { adminGate } from "../_gate";
import { drainInBackground } from "../_outbox";
import { needFor } from "../_permissions";

/** Who passed the gate reaches the handlers through `data`, which Pages threads
 *  from middleware to the function it wraps. `admin.actor` is what the shop's
 *  audit log records as having done something. */
export interface AdminData extends Record<string, unknown> {
  admin: Admin;
}

export const onRequest: PagesFunction<Env, string, AdminData> = async (context) => {
  const { request, env, next, data, waitUntil } = context;

  const admin = await adminGate(request, env, needFor(request.method, new URL(request.url).pathname));
  if (admin instanceof Response) return admin;

  data.admin = admin;

  // An open panel is the most reliable clock this shop has: every admin request
  // pushes a few rows of the outbox along.
  drainInBackground(env, { waitUntil }, 5);

  return next();
};
