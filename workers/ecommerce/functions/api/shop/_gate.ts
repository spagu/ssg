// The gate in front of every admin route (GO-155) and the module routes
// (GO-156). A leading underscore keeps this file out of the Pages route table.
//
// admin/_middleware.ts calls it for everything under /api/shop/admin/;
// module/index.ts and module/summary.ts call it themselves, because Pages runs
// a middleware only for its own directory. One function, so the two answer a
// refusal byte for byte the same. In order:
//   1. the database: bound, and its schema current (the password mode's
//      lockout lives in it) — see `withoutDatabase` below;
//   2. the admin request budget, spent before the credential is checked, so a
//      stolen one cannot walk the order list at machine speed;
//   3. who is calling (_auth.ts: Access → auth → the shared password);
//   4. whether the route is one the tables know (_permissions.ts) — an unknown
//      one is a 404 in every mode, fail closed;
//   5. for an `auth` service token (GO-157): the route is listed for machines
//      and the token names the route's permission exactly — a wildcard
//      (`ecommerce:*`, `*`) is a person's convenience, never a machine's;
//   6. the permission itself.

import { authenticate, modeOf, permit, withLogin } from "./_auth";
import type { Admin } from "./_auth";
import type { Env } from "./_env";
import { fail, json } from "./_lib";
import { machineSafe } from "./_permissions";
import type { Need } from "./_permissions";
import { guard } from "./_ratelimit";
import { ensureSchema } from "./_schema";

export interface GateOptions {
  // withoutDatabase: let Access and `auth` through with no D1 bound — they
  // check a signature and read no table — so the summary can report the
  // missing database instead of a bare 503. The password mode still needs the
  // database for its lockout and is refused without it.
  withoutDatabase?: boolean;
}

// adminGate returns who passed, or the Response to send instead. `need` is
// what the route needs (needFor), null for a route no table knows.
export async function adminGate(request: Request, env: Env, need: Need | null, options: GateOptions = {}): Promise<Admin | Response> {
  if (env.SHOP_DB) {
    await ensureSchema(env);
  } else if (!options.withoutDatabase || !["access", "auth"].includes(modeOf(env))) {
    return fail(request, "not_configured", "The shop database is not bound.", 503);
  }

  const limited = await guard(env, request, "admin");
  if (limited) return limited;

  const admin = await authenticate(request, env);
  if (admin instanceof Response) return withLogin(request, env, admin);

  if (need === null) return fail(request, "not_found", "There is no such admin endpoint.", 404);
  if (admin.service) {
    const refused = machineRefusal(request, admin, need);
    if (refused) return refused;
  }
  return permit(admin, need) ?? admin;
}

// machineRefusal: the 403 for a service token outside what machines may do.
function machineRefusal(request: Request, admin: Admin, need: Need): Response | null {
  const url = new URL(request.url);
  if (!machineSafe(request.method, url.pathname)) {
    return fail(request, "service_token_not_accepted", "This endpoint is for people: a service token is accepted only by the export, the sales figures and the module summary.", 403);
  }
  if (admin.perms.includes(need)) return null;
  const message = `A service token needs the permission ${need} by its exact name; a wildcard does not count for machines.`;
  return json({ error: "forbidden", permission: need, message }, 403);
}
