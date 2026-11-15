// GET /api/shop/ucp/checkout-sessions/:id — the session as it stands
// PUT /api/shop/ucp/checkout-sessions/:id — replace items, buyer, destination

import { getSession, updateSession } from "../../_ucp-checkout";
import { ucpRoute } from "../../_ucp-route";

export const onRequestGet = ucpRoute(getSession, { mutates: false });
export const onRequestPut = ucpRoute(updateSession, { mutates: true });
