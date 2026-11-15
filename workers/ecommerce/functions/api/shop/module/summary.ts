// Cloudflare Pages Function: the module summary (GO-156, the GO-148 contract).
//   GET /api/shop/module/summary → {contract, generatedAt, metrics, attention,
//       problems}: paid orders, revenue, refunds and products on sale; orders
//       and messages waiting for the owner; configuration problems
//       (workers/module-summary.schema.json)
//
// Behind the same gate as the admin API (_gate.ts), with ecommerce:read. With
// no database bound, Access and `auth` still get an answer naming that
// problem; the password mode cannot check its lockout and gets the admin
// API's 503. Aggregates only (_summary.ts).

import type { Env } from "../_env";
import { adminGate } from "../_gate";
import { json } from "../_lib";
import { needFor } from "../_permissions";
import { isLocalHost, shopSummary } from "../_summary";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const url = new URL(request.url);
  const admin = await adminGate(request, env, needFor("GET", url.pathname), { withoutDatabase: true });
  if (admin instanceof Response) return admin;
  return json(await shopSummary(env, { live: !isLocalHost(url.hostname) }));
};
