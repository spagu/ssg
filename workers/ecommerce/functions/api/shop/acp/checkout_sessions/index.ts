// POST /api/shop/acp/checkout_sessions — create an ACP checkout session
// (GO-163). The platform is given https://<site>/api/shop/acp as the base URL;
// the rest is the protocol's own path. Off (404) unless SHOP_ACP_ENABLED.

import { createSession } from "../../_acp-checkout";
import { acpRoute } from "../../_acp-route";

export const onRequestPost = acpRoute(createSession, { idempotent: true });
