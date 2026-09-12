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
export type { CheckoutInput, CheckoutResult, GatewayEvent, PaymentGateway, VerifiedWebhook } from "./_gateway";
