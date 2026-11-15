// Shared setup for the ACP tests (GO-163): an ACP-enabled shop, requests the
// way an agent platform sends them, a Stripe that answers Shared Payment
// Token charges, and a platform webhook receiver.
//
// The fakes are built from the published documents, not from the live
// services: the Agentic Commerce Protocol release 2026-04-17 (headers,
// error shapes, Merchant-Signature) and Stripe's "Shared payment tokens —
// Sellers" page (POST /v1/payment_intents with
// payment_method_data[shared_payment_granted_token], confirm=true,
// a preview Stripe-Version, Idempotency-Key replay).

import { env } from "./_env";
import { ctx } from "./_helpers";
import { hmacHex } from "../functions/api/shop/_gateway";

export const ACP_KEY = "acp-test-bearer-6f1c2a9e";
export const ACP_WEBHOOK_SECRET = "acp-test-webhook-secret-41b7";
export const STRIPE_KEY = "sk_test_acpfake_8d2e";
export const RECEIVER = "https://agent.example.com/agentic_checkout/webhooks/order_events";

/** Every secret a test configures, for the scans that prove none leaks. */
export const SECRETS = [ACP_KEY, ACP_WEBHOOK_SECRET, STRIPE_KEY];

/** Turns ACP on with every secret it needs. */
export function enableAcp(overrides: Record<string, string | undefined> = {}): void {
  Object.assign(env, {
    SHOP_ACP_ENABLED: "true",
    SHOP_ACP_API_KEY: ACP_KEY,
    SHOP_ACP_MERCHANT_ID: "acct_test_example",
    SHOP_ACP_WEBHOOK_URL: RECEIVER,
    SHOP_ACP_WEBHOOK_SECRET: ACP_WEBHOOK_SECRET,
    SHOP_ACP_SIGNING_SECRET: undefined,
    STRIPE_SECRET_KEY: STRIPE_KEY,
    ...overrides,
  });
}

let counter = 0;

/** A request as the platform sends it: bearer, API-Version, a fresh
 *  Idempotency-Key on POST (pass `idempotency-key` to pin one, or `null` in
 *  `drop` to leave a header out). */
export function acpRequest(
  method: "GET" | "POST",
  path: string,
  body?: unknown,
  headers: Record<string, string> = {},
  drop: string[] = [],
): Request {
  const all: Record<string, string> = {
    authorization: `Bearer ${ACP_KEY}`,
    "api-version": "2026-04-17",
    "content-type": "application/json",
    "request-id": `req_${++counter}`,
    ...(method === "POST" ? { "idempotency-key": `idem-${counter}-${crypto.randomUUID()}` } : {}),
    ...headers,
  };
  for (const name of drop) delete all[name];
  return new Request(`https://shop.example.com/api/shop/acp${path}`, {
    method,
    headers: all,
    body: method === "POST" && body !== undefined ? (typeof body === "string" ? body : JSON.stringify(body)) : undefined,
  });
}

type Handler = (c: never) => Promise<Response>;

/** Calls a route handler as Pages would, background work included. */
export async function call(handler: Handler, request: Request, params: Record<string, string> = {}): Promise<Response> {
  const c = ctx(request, params);
  const res = await handler(c as never);
  await c.settled();
  return res;
}

export interface FakeStripe {
  fetch: typeof fetch;
  /** PaymentIntents actually created (a replay is not a new one). */
  created: Array<{ form: URLSearchParams; headers: Headers }>;
  calls: number;
}

/** Stripe for SPT charges. Tokens decide the answer:
 *  spt_ok… succeeds, spt_declined… is a 402 card_declined, spt_3ds… needs
 *  action, spt_500… is a Stripe outage, spt_short… charges one unit less.
 *  An Idempotency-Key seen before replays the first answer, as Stripe does. */
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
    const token = form.get("payment_method_data[shared_payment_granted_token]") ?? "";
    const amount = Number(form.get("amount"));
    const id = `pi_${state.created.length + 1}_${token}`;
    let reply: { status: number; body: unknown };
    if (token.startsWith("spt_500")) return Response.json({ error: { type: "api_error" } }, { status: 500 });
    if (token.startsWith("spt_declined")) {
      reply = { status: 402, body: { error: { type: "card_error", code: "card_declined", decline_code: "insufficient_funds", message: "Your card has insufficient funds." } } };
    } else if (token.startsWith("spt_bare")) {
      reply = { status: 400, body: {} };
    } else if (token.startsWith("spt_3ds")) {
      reply = { status: 200, body: { id, object: "payment_intent", status: "requires_action", amount, currency: form.get("currency") } };
    } else {
      const charged = token.startsWith("spt_short") ? amount - 1 : amount;
      reply = {
        status: 200,
        body: { id, object: "payment_intent", status: "succeeded", amount: charged, amount_received: charged, currency: form.get("currency"), client_secret: "pi_secret_must_not_be_stored" },
      };
    }
    state.created.push({ form, headers });
    replies.set(key, reply);
    return Response.json(reply.body, { status: reply.status });
  }) as typeof fetch;
  return state;
}

/** The platform's order-events receiver: verifies the Merchant-Signature the
 *  way the webhook spec tells the platform to, and keeps what it accepted. */
export function fakeReceiver(answer = 200) {
  const received: Array<{ body: Record<string, unknown>; signature: string; verified: boolean }> = [];
  const handle = async (request: Request): Promise<Response> => {
    const raw = await request.text();
    const signature = request.headers.get("merchant-signature") ?? "";
    const m = /^t=(\d+),v1=([a-f0-9]{64})$/.exec(signature);
    const fresh = m !== null && Math.abs(Date.now() / 1000 - Number(m[1])) <= 300;
    const verified = fresh && m![2] === (await hmacHex(ACP_WEBHOOK_SECRET, `${m![1]}.${raw}`));
    received.push({ body: JSON.parse(raw) as Record<string, unknown>, signature, verified });
    return Response.json({ received: true }, { status: verified ? answer : 401 });
  };
  return { received, handle };
}

/** A D1 whose writes matching `match` lose a race: run() does nothing and
 *  reports no row changed, as D1 does when a compare-and-swap misses. */
export function lossyD1<T extends { prepare: (sql: string) => any }>(db: T, match: RegExp): T {
  const wrap = (stmt: any): any => ({
    ...stmt,
    bind: (...a: unknown[]) => wrap(stmt.bind(...a)),
    run: async () => (match.test(stmt.sql) ? { success: true, results: [], meta: { changes: 0 } } : stmt.run()),
  });
  return { ...db, prepare: (sql: string) => wrap(db.prepare(sql)) };
}
