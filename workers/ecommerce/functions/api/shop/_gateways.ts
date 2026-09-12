// The registry, with the providers actually in it.
//
// _gateway.ts defines what a payment provider is and keeps the registry;
// _stripe.ts and _paypal.ts register themselves when they are loaded. A module
// that is never imported is never loaded, so importing only _gateway.ts gives
// you an empty registry and a checkout that answers "unknown provider" for a
// provider that is plainly configured.
//
// This file is the one place that imports the implementations. Everything that
// needs to reach a provider imports it from here, and adding a third provider
// is one line here rather than a line in every caller.

import "./_stripe";
import "./_paypal";

export { gateway, gateways } from "./_gateway";

import { enabledGateways, type Env } from "./_env";
import { getSetting } from "./_settings";

/** The providers to offer a buyer, in the order their buttons appear.
 *
 *  The panel's order when one has been set, the environment's otherwise. A
 *  provider named in the setting whose secrets have since been removed is
 *  dropped rather than offered: a button that ends in a 502 at the last step of
 *  a purchase is worse than one button. */
export async function offeredGateways(env: Env): Promise<string[]> {
  const configured = enabledGateways(env);
  const chosen = await getSetting(env, "gateways.order");
  if (!Array.isArray(chosen) || chosen.length === 0) return configured;
  return chosen.map(String).filter((name) => configured.includes(name));
}
export type { CheckoutInput, CheckoutResult, GatewayEvent, PaymentGateway, VerifiedWebhook } from "./_gateway";
