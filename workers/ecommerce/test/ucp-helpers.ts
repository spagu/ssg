// Shared setup for the UCP tests (GO-167): a UCP-enabled shop, requests the
// way Google's platform sends them, a Stripe that charges Google Pay card
// tokens, and an order-webhook receiver that verifies RFC 9421 signatures.
//
// The fakes are built from the published documents, not from live services:
// UCP release 2026-04-08 (checkout-rest.md, signatures.md, order.md), Google's
// Merchant UCP guides and Google Pay handler page (read 2026-11-14), Google
// Pay's processor table for Stripe and Stripe's PaymentMethod `card[token]`.

import { env } from "./_env";

export const UCP_KEY = "ucp-test-api-key-3b9f0c2d";
export const WEBHOOK_API_KEY = "ucp-test-goog-key-77aa";
export const STRIPE_KEY = "sk_test_ucpfake_5c1e";
export const PUBLISHABLE = "pk_test_ucpfake_publishable";
export const WEBHOOK_URL = "https://platform.example.com/v1/webhooks/partners/p-1/events/order";
export const PLATFORM_PROFILE = "https://platform.example.com/.well-known/ucp";
export const HANDLER = "gpay_test_handler";

/** A fresh ES256 key per test process, as the owner makes one. */
const pair = (await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"])) as CryptoKeyPair;
const privateJwk = { ...(await crypto.subtle.exportKey("jwk", pair.privateKey)), kid: "shop-2026-11" };
export const SIGNING_KEY = JSON.stringify(privateJwk);
export const PUBLIC_KEY = pair.publicKey;

/** Every secret a test configures, for the scans that prove none leaks. */
export const SECRETS = [UCP_KEY, WEBHOOK_API_KEY, STRIPE_KEY, String(privateJwk.d)];

/** Turns UCP on with everything it needs. */
export function enableUcp(overrides: Record<string, string | undefined> = {}): void {
  Object.assign(env, {
    SHOP_UCP_ENABLED: "true",
    SHOP_UCP_API_KEY: UCP_KEY,
    SHOP_UCP_SIGNING_KEY: SIGNING_KEY,
    SHOP_UCP_STRIPE_PUBLISHABLE_KEY: PUBLISHABLE,
    SHOP_UCP_GPAY_ENVIRONMENT: undefined,
    SHOP_UCP_GPAY_MERCHANT_ID: "BCR2DN4TEXAMPLE",
    SHOP_UCP_HANDLER_ID: HANDLER,
    SHOP_UCP_WEBHOOK_URL: WEBHOOK_URL,
    SHOP_UCP_WEBHOOK_API_KEY: WEBHOOK_API_KEY,
    STRIPE_SECRET_KEY: STRIPE_KEY,
    ...overrides,
  });
}

let counter = 0;

/** A request as the platform sends it: X-API-Key, UCP-Agent, Request-Id and a
 *  fresh Idempotency-Key on POST/PUT (pin one in `headers`, or name a header
 *  in `drop` to leave it out). */
export function ucpRequest(
  method: "GET" | "POST" | "PUT",
  path: string,
  body?: unknown,
  headers: Record<string, string> = {},
  drop: string[] = [],
): Request {
  const all: Record<string, string> = {
    "x-api-key": UCP_KEY,
    "ucp-agent": `profile="${PLATFORM_PROFILE}"`,
    "content-type": "application/json",
    "request-id": `req-${++counter}`,
    ...(method === "GET" ? {} : { "idempotency-key": crypto.randomUUID() }),
    ...headers,
  };
  for (const name of drop) delete all[name];
  return new Request(`https://shop.example.com/api/shop/ucp${path}`, {
    method,
    headers: all,
    body: method !== "GET" && body !== undefined ? (typeof body === "string" ? body : JSON.stringify(body)) : undefined,
  });
}

/** Google Pay's tokenization data for gateway "stripe": a Stripe token object
 *  as a JSON string. */
export const gpayToken = (id: string): string =>
  JSON.stringify({ id, object: "token", card: { brand: "Visa", last4: "4242", country: "DE" }, livemode: false });

/** The complete body Google sends (Google Pay handler page, "Example Google
 *  Pay payment instrument"). */
export function payWith(tokenId: string, extra: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    payment: {
      instruments: [
        {
          id: "pm_1",
          handler_id: HANDLER,
          type: "card",
          selected: true,
          display: { brand: "VISA", description: "Visa •••• 4242", last_digits: "4242" },
          billing_address: { address_country: "DE", postal_code: "10115" },
          credential: { type: "PAYMENT_GATEWAY", token: gpayToken(tokenId) },
          ...extra,
        },
      ],
    },
    signals: { "dev.ucp.buyer_ip": "203.0.113.42" },
  };
}

export interface FakeStripe {
  fetch: typeof fetch;
  /** PaymentIntents actually created (a replay is not a new one). */
  created: Array<{ form: URLSearchParams; headers: Headers }>;
  calls: number;
}

/** Stripe for Google Pay card tokens. Token ids decide the answer: tok_ok…
 *  succeeds, tok_declined… is a 402, tok_3ds… needs action, tok_500… is an
 *  outage, tok_short… charges one unit less. A seen Idempotency-Key replays
 *  the first answer, as Stripe does. */
export function fakeStripe(): FakeStripe {
  const replies = new Map<string, { status: number; body: unknown }>();
  const state: FakeStripe = { created: [], calls: 0, fetch: undefined as never };
  state.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url !== "https://api.stripe.com/v1/payment_intents") throw new Error(`unexpected fetch ${url}`);
    state.calls++;
    const headers = new Headers(init?.headers);
    const key = headers.get("idempotency-key") ?? "";
    const known = replies.get(key);
    if (known) return Response.json(known.body, { status: known.status });
    const form = new URLSearchParams(String(init?.body));
    const token = form.get("payment_method_data[card][token]") ?? "";
    const amount = Number(form.get("amount"));
    const id = `pi_${state.created.length + 1}_${token}`;
    if (token.startsWith("tok_500")) return Response.json({ error: { type: "api_error" } }, { status: 500 });
    let reply: { status: number; body: unknown };
    if (token.startsWith("tok_declined")) {
      reply = { status: 402, body: { error: { type: "card_error", code: "card_declined", decline_code: "insufficient_funds" } } };
    } else if (token.startsWith("tok_3ds")) {
      reply = { status: 200, body: { id, status: "requires_action", amount, currency: form.get("currency") } };
    } else {
      const charged = token.startsWith("tok_short") ? amount - 1 : amount;
      reply = { status: 200, body: { id, status: "succeeded", amount: charged, amount_received: charged, currency: form.get("currency"), client_secret: "pi_secret_never_stored" } };
    }
    state.created.push({ form, headers });
    replies.set(key, reply);
    return Response.json(reply.body, { status: reply.status });
  }) as typeof fetch;
  return state;
}

const fromB64 = (s: string): Uint8Array => Uint8Array.from(atob(s), (c) => c.charCodeAt(0));

/** Verifies a webhook the way signatures.md tells the platform to: parse
 *  Signature-Input, check Content-Digest against the raw body, rebuild the
 *  signature base and verify ES256 (raw r||s) with the profile's key. */
export async function verifyWebhook(request: Request, raw: string, key: CryptoKey = PUBLIC_KEY): Promise<boolean> {
  const input = request.headers.get("signature-input") ?? "";
  const m = /^sig1=(\((?:"[^"]+"\s?)+\);created=\d+;keyid="([^"]+)")$/.exec(input);
  const sig = /^sig1=:([A-Za-z0-9+/=]+):$/.exec(request.headers.get("signature") ?? "");
  if (!m || !sig || m[2] !== "shop-2026-11") return false;
  const digest = `sha-256=:${btoa(String.fromCharCode(...new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(raw)))))}:`;
  if (request.headers.get("content-digest") !== digest) return false;
  const url = new URL(request.url);
  const value = (name: string): string =>
    name === "@method" ? request.method : name === "@authority" ? url.host : name === "@path" ? url.pathname : name === "@query" ? url.search : request.headers.get(name) ?? "";
  const names = [...m[1]!.slice(0, m[1]!.indexOf(")")).matchAll(/"([^"]+)"/g)].map((x) => x[1]!);
  const base = [...names.map((n) => `"${n}": ${value(n)}`), `"@signature-params": ${m[1]}`].join("\n");
  return crypto.subtle.verify({ name: "ECDSA", hash: "SHA-256" }, key, fromB64(sig[1]!), new TextEncoder().encode(base));
}

/** The platform's order-webhook receiver. */
export function fakeReceiver(answer = 200) {
  const received: Array<{ body: Record<string, any>; headers: Headers; verified: boolean }> = [];
  const handle = async (request: Request): Promise<Response> => {
    const raw = await request.text();
    const verified = await verifyWebhook(request, raw);
    received.push({ body: JSON.parse(raw) as Record<string, any>, headers: request.headers, verified });
    return Response.json({ ucp: { version: "2026-04-08" } }, { status: verified ? answer : 401 });
  };
  return { received, handle };
}
