// POST /api/shop/ucp/checkout-sessions/:id/cancel — abandon the session.

import { cancelSession } from "../../../_ucp-checkout";
import { ucpRoute } from "../../../_ucp-route";

export const onRequestPost = ucpRoute(cancelSession, { mutates: true });
