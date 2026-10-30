// The subscriptions table. A leading underscore keeps this file out of the
// Pages route table — it is imported, never served.

import { b64urlDecode } from "./_lib";
import type { Env } from "./_lib";

export interface Subscription {
  endpoint: string;
  p256dh: string; // base64url, the browser's P-256 public key
  auth: string; // base64url, the 16-byte authentication secret
}

// saveSubscription inserts a subscription, or refreshes its keys when the
// browser subscribes again with the same endpoint.
export async function saveSubscription(env: Env, sub: Subscription, uaHash: string | null): Promise<void> {
  const now = new Date().toISOString();
  await env.PUSH_DB.prepare(
    `INSERT INTO push_subscriptions (endpoint, p256dh, auth, ua_hash, created_at, updated_at)
     VALUES (?, ?, ?, ?, ?, ?)
     ON CONFLICT(endpoint) DO UPDATE SET
       p256dh = excluded.p256dh, auth = excluded.auth,
       ua_hash = excluded.ua_hash, updated_at = excluded.updated_at`,
  )
    .bind(sub.endpoint, sub.p256dh, sub.auth, uaHash, now, now)
    .run();
}

export async function deleteSubscription(env: Env, endpoint: string): Promise<void> {
  await env.PUSH_DB.prepare(`DELETE FROM push_subscriptions WHERE endpoint = ?`).bind(endpoint).run();
}

export async function listSubscriptions(env: Env): Promise<Subscription[]> {
  const out = await env.PUSH_DB.prepare(`SELECT endpoint, p256dh, auth FROM push_subscriptions`).all<Subscription>();
  return out.results ?? [];
}

// The push services of the browsers people use. An endpoint is a URL the
// worker will POST to later, so accepting any URL would let anyone turn
// notify into a request to a host of their choosing.
const PUSH_HOSTS = ["fcm.googleapis.com", "push.services.mozilla.com", "push.apple.com", "notify.windows.com"];

function pushHosts(env: Env): string[] {
  const list = (env.PUSH_ENDPOINT_HOSTS || "")
    .split(",")
    .map((h) => h.trim().toLowerCase())
    .filter(Boolean);
  return list.length ? list : PUSH_HOSTS;
}

function allowedHost(host: string, hosts: string[]): boolean {
  return hosts.some((h) => host === h || host.endsWith("." + h));
}

// parseSubscription checks a browser's PushSubscription.toJSON(): an https
// endpoint at a known push service, a 65-byte uncompressed P-256 key and a
// 16-byte auth secret. Anything else is null.
export function parseSubscription(env: Env, body: Record<string, unknown> | null): Subscription | null {
  const endpoint = body?.endpoint;
  const keys = body?.keys as Record<string, unknown> | undefined;
  if (typeof endpoint !== "string" || endpoint.length > 2048 || !keys || typeof keys !== "object") return null;
  const { p256dh, auth } = keys;
  if (typeof p256dh !== "string" || typeof auth !== "string" || p256dh.length > 128 || auth.length > 64) return null;
  let url: URL;
  try {
    url = new URL(endpoint);
  } catch {
    return null;
  }
  if (url.protocol !== "https:" || url.username || url.password || !allowedHost(url.hostname, pushHosts(env))) return null;
  const pub = b64urlDecode(p256dh);
  const secret = b64urlDecode(auth);
  if (!pub || pub.length !== 65 || pub[0] !== 4 || !secret || secret.length !== 16) return null;
  return { endpoint, p256dh, auth };
}
