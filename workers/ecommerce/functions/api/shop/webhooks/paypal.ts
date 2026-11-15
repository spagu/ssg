// POST /api/shop/webhooks/paypal

import type { Env } from "../_env";
import { handleWebhook } from "../_webhook";

export const onRequestPost: PagesFunction<Env> = (context) => handleWebhook(context, "paypal");
