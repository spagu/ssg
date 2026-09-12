// One interface, several payment providers.
//
// The core knows nothing about Stripe or PayPal: it hands an order to a gateway
// and gets back a URL to send the buyer to, and later it hands a webhook
// request to the same gateway and gets back a normalised event. Adding a third
// provider is a new file and a name in SHOP_GATEWAYS, not another branch in the
// checkout.

import type { Env } from "./_env";
import { enabledGateways } from "./_env";
import type { CustomerRow, OrderItemRow, OrderRow, PaymentRow } from "./_types";

export interface CheckoutInput {
  order: OrderRow;
  items: OrderItemRow[];
  customer: CustomerRow;
  /** Where the buyer lands afterwards. Neither URL may change order state (S-09). */
  successUrl: string;
  cancelUrl: string;
  locale: string;
  /** Gross pricing means the unit amounts already include tax. */
  pricingMode: "gross" | "net";
  /** Let the provider compute tax instead of us (tax.mode = "stripe"). */
  providerTax: boolean;
}

export interface CheckoutResult {
  redirectUrl: string;
  /** The provider's id for this checkout, stored as orders.gateway_ref. */
  providerRef: string;
}

/** Everything a webhook can mean to us, and nothing else. A provider event we
 *  do not act on becomes "ignored" rather than an error: providers add event
 *  types, and a 500 makes them retry forever. */
export type GatewayEvent =
  | {
      kind: "paid";
      providerRef: string;
      paymentRef: string;
      amountMinor: number;
      currency: string;
      /** The card or account country, when the provider tells us — the third
       *  piece of VAT evidence. */
      cardCountry?: string | null;
      raw: unknown;
    }
  | { kind: "failed"; providerRef: string; reason: string; raw: unknown }
  | {
      kind: "refunded";
      providerRef: string;
      paymentRef: string;
      amountMinor: number;
      currency: string;
      raw: unknown;
    }
  | { kind: "ignored"; eventType: string };

export interface VerifiedWebhook {
  eventId: string;
  eventType: string;
  event: GatewayEvent;
}

export interface PaymentGateway {
  readonly name: string;
  createCheckout(input: CheckoutInput, env: Env): Promise<CheckoutResult>;
  /** Returns null when the signature does not verify. The handler then answers
   *  400 and writes nothing — an unverified body must not reach the inbox. */
  verifyWebhook(request: Request, rawBody: string, env: Env): Promise<VerifiedWebhook | null>;
  refund(payment: PaymentRow, amountMinor: number, env: Env): Promise<{ refundRef: string }>;
}

type Factory = (env: Env) => PaymentGateway;

const registry = new Map<string, Factory>();

export function registerGateway(name: string, factory: Factory): void {
  registry.set(name, factory);
}

/** The gateways this deployment can actually use, in the order the owner listed
 *  them (the storefront shows the buttons in that order). */
export function gateways(env: Env): PaymentGateway[] {
  return enabledGateways(env)
    .map((name) => registry.get(name)?.(env))
    .filter((g): g is PaymentGateway => Boolean(g));
}

export function gateway(env: Env, name: string): PaymentGateway | null {
  if (!enabledGateways(env).includes(name)) return null;
  return registry.get(name)?.(env) ?? null;
}

/** Parses a Stripe-style signature header: `t=1234,v1=abc,v1=def`.
 *
 *  Shared because our own outgoing webhooks are signed the same way (a
 *  recipient who already integrates Stripe knows the format), and because the
 *  parsing is the part that is easy to get subtly wrong. */
export function parseSignatureHeader(header: string): { t: string; v1: string[] } {
  const parts = header.split(",").map((p) => p.trim().split("="));
  const t = parts.find(([k]) => k === "t")?.[1] ?? "";
  const v1 = parts.filter(([k]) => k === "v1").map(([, v]) => v ?? "");
  return { t, v1 };
}

export async function hmacHex(secret: string, message: string): Promise<string> {
  const key = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const sig = await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(message));
  return [...new Uint8Array(sig)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

/** How old a signed webhook may be. Stripe recommends five minutes; without
 *  this check a captured request can be replayed forever, which is the hole the
 *  existing stripe-checkout template still has (ECOM-000-a). */
export const SIGNATURE_TOLERANCE_S = 300;

export function withinTolerance(timestamp: string, nowS = Math.floor(Date.now() / 1000)): boolean {
  const t = Number.parseInt(timestamp, 10);
  if (!Number.isFinite(t)) return false;
  return Math.abs(nowS - t) <= SIGNATURE_TOLERANCE_S;
}

/** Strips the fields a provider payload carries that we must never store
 *  (S-03). We keep raw events for disputes, not for secrets. */
export function redact(value: unknown): unknown {
  const SECRETS = new Set(["client_secret", "access_token", "refresh_token", "secret", "password"]);
  const walk = (node: unknown, depth: number): unknown => {
    if (depth > 8 || node === null || typeof node !== "object") return node;
    if (Array.isArray(node)) return node.map((v) => walk(v, depth + 1));
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(node as Record<string, unknown>)) {
      out[k] = SECRETS.has(k) ? "[redacted]" : walk(v, depth + 1);
    }
    return out;
  };
  return walk(value, 0);
}
