// The test mode the UCP conformance suite needs (GO-187), off by default.
//
// The suite (github.com/Universal-Commerce-Protocol/conformance, read at
// commit 0642adf, 2026-11-13) drives a server the way a platform would, from a
// local machine: its agent profile is an `http://localhost` URL, it pays with
// the sample server's test handler `mock_payment_handler` and the tokens
// `success_token` / `fail_token`, its order-event receiver is plain http, and
// it advances an order through `/testing/simulate-shipping/{id}`. None of that
// is in the protocol, and none of it may reach a shop that sells for real.
//
// So everything here needs SHOP_UCP_TEST_MODE = "true", and the mode is
// refused — every UCP route and the profile answer 503 — while the
// configuration could move real money: a live Stripe key or Google Pay in
// PRODUCTION (`ucpMissing` in _ucp-auth.ts). A test payment delivers the
// product without charging anything, which is the point of a test run and the
// reason the mode is off by default (docs/UCP-OPERATIONS.md).

import type { Env } from "./_env";
import type { ChargeOutcome } from "./_agent-charge";
import type { OrderRow } from "./_types";

/** The handler id and tokens of the suite's fixtures (payment_instruments.csv
 *  of its flower-shop sample). */
export const TEST_HANDLER_ID = "mock_payment_handler";
const TEST_TOKENS = { success_token: "success", fail_token: "fail" } as const;
export type TestPayment = (typeof TEST_TOKENS)[keyof typeof TEST_TOKENS];

/** True only when the owner switched the test mode on. */
export const ucpTestMode = (env: Env): boolean => env.SHOP_UCP_TEST_MODE === "true";

/** True when the configuration can take real money. */
export function liveConfig(env: Env): boolean {
  return (
    env.SHOP_UCP_GPAY_ENVIRONMENT === "PRODUCTION" ||
    // String(undefined) is "undefined", which matches neither.
    /^(sk|rk)_live_/.test(String(env.STRIPE_SECRET_KEY)) ||
    /^pk_live_/.test(String(env.SHOP_UCP_STRIPE_PUBLISHABLE_KEY))
  );
}

/** Test mode next to a live configuration: closed, never open. */
export const testModeRefused = (env: Env): boolean => ucpTestMode(env) && liveConfig(env);

/** The URL schemes allowed for the platform's profile and its order-event
 *  receiver: https, and http as well in test mode. */
export const allowedScheme = (env: Env, protocol: string): boolean =>
  protocol === "https:" || (protocol === "http:" && ucpTestMode(env));

const isObject = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === "object" && !Array.isArray(v);

/** The test payment an instrument asks for, or null when it is not one —
 *  outside test mode, another handler, or a token the suite does not use. */
export function testPayment(env: Env, instrument: Record<string, unknown>): TestPayment | null {
  if (!ucpTestMode(env) || instrument.handler_id !== TEST_HANDLER_ID) return null;
  const credential = instrument.credential;
  if (!isObject(credential) || credential.type !== "token" || typeof credential.token !== "string") return null;
  return Object.hasOwn(TEST_TOKENS, credential.token) ? TEST_TOKENS[credential.token as keyof typeof TEST_TOKENS] : null;
}

/** "Charges" a test payment: success is paid in full with a `test_` ref, so
 *  the payment row says what it was; failure throws, which the completion
 *  answers as `payment_failed` with nothing charged — the code the suite
 *  expects for `fail_token`. */
export function chargeTest(order: OrderRow, payment: TestPayment): ChargeOutcome {
  if (payment === "fail") throw new Error("test payment: fail_token");
  return {
    kind: "paid",
    ref: `test_${crypto.randomUUID().replaceAll("-", "")}`,
    amountMinor: order.total_minor,
    currency: order.currency.toUpperCase(),
    raw: { test: true },
  };
}
