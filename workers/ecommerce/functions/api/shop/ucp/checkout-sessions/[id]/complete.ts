// POST /api/shop/ucp/checkout-sessions/:id/complete — pay with the Google Pay
// token (through Stripe) and turn the session into an order (_ucp-complete.ts).

import { completeSession } from "../../../_ucp-complete";
import { ucpRoute } from "../../../_ucp-route";

export const onRequestPost = ucpRoute(completeSession, { mutates: true });
