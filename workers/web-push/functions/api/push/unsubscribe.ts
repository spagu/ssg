// Cloudflare Pages Function: forget a push subscription.
//   POST /api/push/unsubscribe   {"endpoint":"…"}
//
// The answer is the same whether the endpoint was stored or not, so the
// endpoint cannot be used to find out who is subscribed.

import { json, readJSON, sameOrigin } from "./_lib";
import type { Env } from "./_lib";
import { ensureSchema } from "./_schema";
import { deleteSubscription } from "./_store";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  if (!sameOrigin(request)) return json({ error: "forbidden" }, 403);
  if (!env.PUSH_DB) return json({ error: "not_configured" }, 503);
  const body = await readJSON(request);
  const endpoint = body?.endpoint;
  if (typeof endpoint !== "string" || !endpoint || endpoint.length > 2048) return json({ error: "invalid_body" }, 400);
  await ensureSchema(env);
  await deleteSubscription(env, endpoint);
  return json({ ok: true });
};
