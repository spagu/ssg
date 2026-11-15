// GET  /api/shop/acp/checkout_sessions/:id — the session's current state
// POST /api/shop/acp/checkout_sessions/:id — update items, buyer, address

import { retrieveSession, updateSession } from "../../_acp-checkout";
import { acpRoute } from "../../_acp-route";

export const onRequestGet = acpRoute(retrieveSession, { idempotent: false });
export const onRequestPost = acpRoute(updateSession, { idempotent: true });
