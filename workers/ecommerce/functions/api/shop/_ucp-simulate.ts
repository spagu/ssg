// POST /testing/simulate-shipping/{id} — the conformance suite's hook for
// advancing an order (GO-187). Not part of the protocol; the suite calls it
// with a `Simulation-Secret` header and expects 403 without the right one and
// 200 with it, then an order event whose snapshot shows a `shipped`
// fulfillment event.
//
// This shop sells downloads, delivered the moment an order is paid, so
// nothing ships. In test mode the hook therefore changes nothing stored: it
// sends the order's current snapshot as an order event with one fulfillment
// event added, described as simulated. Outside test mode, or with no secret
// configured, the path does not exist (404); next to a live configuration
// test mode is refused like every UCP route (503).

import type { Env } from "./_env";
import { shopOrigin } from "./_fulfil";
import { json, sha256hex, timingSafeEqual } from "./_lib";
import { drainOutbox } from "./_outbox";
import { ensureSchema } from "./_schema";
import { notConfigured, ucpMissing } from "./_ucp-auth";
import { ucpEnabled } from "./_ucp-http";
import { enqueueUcpEvent, ucpOrder } from "./_ucp-order";
import { sessionForOrder } from "./_ucp-session";
import { ucpTestMode } from "./_ucp-testing";

type Snapshot = Record<string, unknown> & { line_items: Array<{ id: string; quantity: { total: number } }> };

export const onSimulateShipping: PagesFunction<Env> = async ({ request, env, params, waitUntil }) => {
  const secret = env.SHOP_UCP_SIMULATION_SECRET;
  if (!ucpEnabled(env) || !ucpTestMode(env) || !secret || !env.SHOP_DB) return new Response("Not Found", { status: 404 });
  if (ucpMissing(env).length > 0) {
    const refused = notConfigured();
    return json(refused.body, refused.status, refused.headers);
  }
  const given = request.headers.get("simulation-secret") ?? "";
  // Hashed first, as the API key is, so the compare is fixed-length.
  if (!given || !timingSafeEqual(await sha256hex(given), await sha256hex(secret))) {
    return json({ code: "forbidden", content: "A valid Simulation-Secret is required." }, 403);
  }

  await ensureSchema(env);
  const origin = await shopOrigin(env, request);
  const session = await sessionForOrder(env, String(params.id));
  const order = session ? ((await ucpOrder(env, session, origin)) as Snapshot | null) : null;
  if (!order) return json({ code: "not_found", content: "No completed UCP order has this id." }, 404);

  const now = new Date().toISOString();
  order.fulfillment = {
    ...(order.fulfillment as Record<string, unknown>),
    events: [
      {
        id: `evt_sim_${crypto.randomUUID().replaceAll("-", "")}`,
        occurred_at: now,
        type: "shipped",
        line_items: order.line_items.filter((li) => li.quantity.total > 0).map((li) => ({ id: li.id, quantity: li.quantity.total })),
        tracking_number: "SIMULATED",
        tracking_url: String(order.permalink_url),
        description: "Simulated by the conformance test mode; a download needs no shipping.",
      },
    ],
  };
  // A new Webhook-Id: this is a second event about the order, not a retry of
  // the first.
  await enqueueUcpEvent(env, order, `sim_${now}_${order.id}`, origin);
  waitUntil(drainOutbox(env, 5).catch(() => 0));
  return json({ status: "shipped" }, 200);
};
