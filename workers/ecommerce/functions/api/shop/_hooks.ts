// Outgoing webhooks: telling the seller's own systems that something happened.
//
// Signed the same way Stripe signs its own (t=…,v1=…), because a recipient who
// has already integrated one payment provider knows how to verify that, and a
// bespoke scheme would be one more thing to get wrong.

import type { Env } from "./_env";
import { hmacHex } from "./_gateway";
import { getSetting } from "./_settings";

export interface WebhookEndpoint {
  url: string;
  events: string[];
  secret: string;
}

export type ShopEvent = "order.paid" | "order.fulfilled" | "order.refunded" | "order.needs_review";

/** Hosts we refuse to call.
 *
 *  A Worker cannot see how a hostname resolves, so this cannot be a complete
 *  SSRF defence — but a literal private address or localhost is never a
 *  legitimate webhook target, and refusing those costs nothing. The rest is the
 *  seller pointing at their own infrastructure, which is their decision. */
function isForbiddenTarget(url: URL): boolean {
  const host = url.hostname.toLowerCase();
  if (host === "localhost" || host.endsWith(".localhost") || host === "0.0.0.0") return true;
  if (/^127\./.test(host) || host === "[::1]" || host === "::1") return true;
  if (/^10\./.test(host)) return true;
  if (/^192\.168\./.test(host)) return true;
  if (/^172\.(1[6-9]|2\d|3[01])\./.test(host)) return true;
  if (/^169\.254\./.test(host)) return true; // link-local, and the cloud metadata address
  return false;
}

export async function endpointsFor(env: Env, event: ShopEvent): Promise<WebhookEndpoint[]> {
  const raw = await getSetting(env, "webhooks.endpoints");
  if (!Array.isArray(raw)) return [];
  return (raw as WebhookEndpoint[]).filter(
    (e) => e && typeof e.url === "string" && Array.isArray(e.events) && e.events.includes(event),
  );
}

export interface WebhookPayload {
  id: string;
  type: ShopEvent;
  createdAt: string;
  data: unknown;
  /** Carried through to the signature and the header. */
  secret?: string;
}

/** Called by the outbox. `target` is the URL; the payload carries the secret so
 *  a rotated endpoint does not break a queued delivery halfway. */
export async function deliverWebhook(env: Env, target: string, payload: Record<string, unknown>): Promise<void> {
  let url: URL;
  try {
    url = new URL(target);
  } catch {
    throw new Error(`invalid webhook url: ${target}`);
  }
  if (url.protocol !== "https:") throw new Error("webhook url must be https");
  if (isForbiddenTarget(url)) throw new Error("webhook url points at a private address");

  const { secret, ...event } = payload as { secret?: string } & Record<string, unknown>;
  const body = JSON.stringify(event);
  const t = Math.floor(Date.now() / 1000);

  const headers: Record<string, string> = {
    "content-type": "application/json",
    "x-ssg-shop-event": String(event.type ?? ""),
    "idempotency-key": String(event.id ?? ""),
  };
  if (secret) {
    headers["x-ssg-shop-signature"] = `t=${t},v1=${await hmacHex(secret, `${t}.${body}`)}`;
  }

  const res = await fetch(url.toString(), {
    method: "POST",
    headers,
    body,
    signal: AbortSignal.timeout(10000),
  });
  if (!res.ok) throw new Error(`webhook ${res.status}`);
}
