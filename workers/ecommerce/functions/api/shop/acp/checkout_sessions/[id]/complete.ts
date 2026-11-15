// POST /api/shop/acp/checkout_sessions/:id/complete — pay with a Stripe
// Shared Payment Token and turn the session into an order (_acp-complete.ts).

import { completeSession } from "../../../_acp-complete";
import { acpRoute } from "../../../_acp-route";

export const onRequestPost = acpRoute(completeSession, { idempotent: true });
