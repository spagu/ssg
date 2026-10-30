// Cloudflare Pages Function: the application server's public key.
//   GET /api/push/key   → {"publicKey":"<base64url>"}
//
// push.js asks for it before subscribing, so the key lives in one place — the
// worker's environment — and a theme never has to be rebuilt to rotate it.

import { json } from "./_lib";
import type { Env } from "./_lib";

export const onRequestGet: PagesFunction<Env> = async ({ env }) => {
  if (!env.PUSH_VAPID_PUBLIC_KEY) return json({ error: "not_configured" }, 503);
  return json({ publicKey: env.PUSH_VAPID_PUBLIC_KEY }, 200, { "cache-control": "public, max-age=300" });
};
