// POST /api/shop/acp/checkout_sessions/:id/cancel — abandon the session.

import { cancelSession } from "../../../_acp-checkout";
import { acpRoute } from "../../../_acp-route";

export const onRequestPost = acpRoute(cancelSession, { idempotent: true });
