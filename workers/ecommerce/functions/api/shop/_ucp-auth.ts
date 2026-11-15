// Who may call the UCP checkout.
//
// The protocol leaves authentication to the business ("API Keys — via
// X-API-Key header", OAuth 2.0, mTLS or HTTP Message Signatures;
// checkout-rest.md, "Security Considerations"), and Google's "Authentication"
// guide (read 2026-11-14) lists the same: an API key in X-API-Key, OAuth 2.0
// (recommended there) or mTLS. This shop implements the API key: the owner
// sets SHOP_UCP_API_KEY and gives the same value to Google. It is compared in
// constant time, and a UCP switched on without it — or without what a sale
// needs — is a 503: closed, never open.
//
// Every request must also carry `UCP-Agent: profile="https://…"` (an RFC 8941
// Dictionary). It is checked for shape; the platform's profile is not
// fetched — the API key already says who is calling (UCP.md, ambiguity 2).
// An `http://` profile is accepted only in the conformance test mode
// (_ucp-testing.ts), whose own agent runs on localhost.
//
// OAuth 2.0 and mTLS are not implemented: the protocol lists them as
// options ("Businesses MAY require authentication", checkout-rest.md),
// not requirements. mTLS would be terminated by Cloudflare before the
// Worker runs; UCP.md, ambiguity 2, says what that would take.

import type { Env } from "./_env";
import { sha256hex, timingSafeEqual } from "./_lib";
import { gpayEnvironment } from "./_ucp-payment";
import { signingJwk } from "./_ucp-sign";
import { ucpError, type UcpResult } from "./_ucp-http";
import { allowedScheme, testModeRefused } from "./_ucp-testing";

/** What is missing for UCP to sell, by setting name; empty when nothing is. */
export function ucpMissing(env: Env): string[] {
  const missing: string[] = [];
  if (!env.SHOP_UCP_API_KEY) missing.push("SHOP_UCP_API_KEY");
  // Stripe is the only processor behind the Google Pay handler, so UCP
  // without it could take orders it can never charge.
  if (!env.STRIPE_SECRET_KEY) missing.push("STRIPE_SECRET_KEY");
  if (!/^pk_(test|live)_/.test(env.SHOP_UCP_STRIPE_PUBLISHABLE_KEY ?? "")) missing.push("SHOP_UCP_STRIPE_PUBLISHABLE_KEY");
  // Order webhooks MUST be signed, and the profile must publish the key.
  if (!signingJwk(env)) missing.push("SHOP_UCP_SIGNING_KEY");
  if (gpayEnvironment(env) === "PRODUCTION" && !env.SHOP_UCP_GPAY_MERCHANT_ID) missing.push("SHOP_UCP_GPAY_MERCHANT_ID");
  // The test mode hands out products without payment: never next to a
  // configuration that takes real money (_ucp-testing.ts).
  if (testModeRefused(env)) missing.push("SHOP_UCP_TEST_MODE");
  return missing;
}

export const notConfigured = (): UcpResult => ({
  ...ucpError(503, "not_configured", "Universal Commerce Protocol checkout is switched on but not fully configured."),
  headers: { "retry-after": "300" },
});

/** The profile URL from a UCP-Agent header, or null when the header is
 *  missing, not a Dictionary with a quoted `profile`, or not https (http as
 *  well in test mode). */
export function agentProfile(header: string | null, env: Env): string | null {
  const match = /(?:^|[,;\s])profile="([^"\\]{1,2048})"/.exec(header ?? "");
  if (!match) return null;
  try {
    return allowedScheme(env, new URL(match[1]!).protocol) ? match[1]! : null;
  } catch {
    return null;
  }
}

/** Null when the request may go on; otherwise the error to answer with. */
export async function ucpGate(request: Request, env: Env): Promise<UcpResult | null> {
  if (ucpMissing(env).length > 0) return notConfigured();

  const given = request.headers.get("x-api-key") ?? "";
  // Hashing both sides first makes the compare fixed-length, so neither the
  // key's length nor its prefix leaks through timing.
  if (!given || !timingSafeEqual(await sha256hex(given), await sha256hex(env.SHOP_UCP_API_KEY!))) {
    return ucpError(401, "unauthorized", "A valid X-API-Key is required.");
  }
  if (!agentProfile(request.headers.get("ucp-agent"), env)) {
    return ucpError(400, "invalid_profile_url", 'The UCP-Agent header must be profile="https://…".');
  }
  return null;
}
