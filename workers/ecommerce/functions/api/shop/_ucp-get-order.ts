// GET /orders/{id} — the protocol's Get Order (order.md, "Operations"; REST
// binding order-rest.md, release 2026-04-08), part of the order capability
// the profile advertises (GO-187).
//
// "The business MUST authenticate requests to order data": this runs behind
// the same X-API-Key gate as checkout (_ucp-route.ts), and those platform
// credentials reach "orders originated by the platform" only — an order that
// came through a UCP checkout session. A storefront or ACP order is "not
// found" here, never shown. The body is the same full-order snapshot the
// order events carry (_ucp-order.ts); a missing order is HTTP 200 with
// `ucp.status: "error"` and a `not_found` message, as order.md shows it.

import type { AgentOpInput } from "./_agent-common";
import { ORDER_CAPABILITY, UCP_VERSION, errorMessage, type UcpResult } from "./_ucp-http";
import { ucpOrder } from "./_ucp-order";
import { sessionForOrder } from "./_ucp-session";

export async function getUcpOrder({ env, id, origin }: AgentOpInput): Promise<UcpResult> {
  const session = await sessionForOrder(env, id);
  const order = session ? await ucpOrder(env, session, origin) : null;
  if (order) return { status: 200, body: order };
  return {
    status: 200,
    body: {
      ucp: { version: UCP_VERSION, status: "error", capabilities: { [ORDER_CAPABILITY]: [{ version: UCP_VERSION }] } },
      messages: [errorMessage("not_found", "Order not found.", "unrecoverable")],
    },
  };
}
