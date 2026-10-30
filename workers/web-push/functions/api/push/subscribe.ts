// Cloudflare Pages Function: store a browser's push subscription.
//   POST /api/push/subscribe   the JSON of PushSubscription.toJSON()
//
// The same endpoint subscribing again refreshes its keys. Nothing about the
// visitor is kept but the subscription itself and, with PUSH_HASH_SALT, a
// salted hash of the User-Agent; no IP address at all.

import { json, readJSON, saltedHash, sameOrigin } from "./_lib";
import type { Env } from "./_lib";
import { ensureSchema } from "./_schema";
import { parseSubscription, saveSubscription } from "./_store";

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  if (!sameOrigin(request)) return json({ error: "forbidden" }, 403);
  if (!env.PUSH_DB) return json({ error: "not_configured" }, 503);
  const sub = parseSubscription(env, await readJSON(request));
  if (!sub) return json({ error: "invalid_subscription" }, 422);
  await ensureSchema(env);
  await saveSubscription(env, sub, await saltedHash(env, request.headers.get("user-agent")));
  return json({ ok: true }, 201);
};
