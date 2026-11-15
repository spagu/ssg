// POST /api/shop/ucp/checkout-sessions — create a UCP checkout session
// (GO-167). The platform finds https://<site>/api/shop/ucp as the `endpoint`
// in /.well-known/ucp; the rest is the protocol's own path. Off (404) unless
// SHOP_UCP_ENABLED is "true".

import { createSession } from "../../_ucp-checkout";
import { ucpRoute } from "../../_ucp-route";

export const onRequestPost = ucpRoute(createSession, { mutates: true });
