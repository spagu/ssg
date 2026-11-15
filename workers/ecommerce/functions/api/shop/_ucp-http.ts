// What every UCP answer has in common: the protocol version, the `ucp`
// envelope, protocol errors and the "no resource" error response.
//
// Implemented against the Universal Commerce Protocol release 2026-04-08 —
// the version Google's Merchant guides implement (developers.google.com/
// merchant/ucp, read 2026-11-14) — from the protocol's repository
// (github.com/Universal-Commerce-Protocol/ucp, tag v2026-04-08):
// docs/specification/checkout.md, checkout-rest.md, overview.md,
// signatures.md, order.md and source/schemas. UCP.md in this template lists
// where the documents disagree and what this code chose.

import type { Env } from "./_env";
import type { AgentResult } from "./_agent-common";

/** The protocol version spoken, in every `ucp` envelope. */
export const UCP_VERSION = "2026-04-08";

/** The capability every checkout response confirms. */
export const CHECKOUT_CAPABILITY = "dev.ucp.shopping.checkout";
export const ORDER_CAPABILITY = "dev.ucp.shopping.order";

export type UcpResult = AgentResult;

/** True only when the owner switched UCP on. Anything else — unset, "1",
 *  "yes" — is off, and off is a 404 on every UCP route. */
export function ucpEnabled(env: Env): boolean {
  return env.SHOP_UCP_ENABLED === "true";
}

/** A protocol error (overview.md, "Error Handling"): an HTTP status and a
 *  JSON body with `code` and `content`. Used for what is wrong with the call
 *  itself — authentication, a malformed body, idempotency, rate limits —
 *  never for what is wrong with a basket, which goes in `messages`. */
export function ucpError(status: number, code: string, content: string, extra: Record<string, unknown> = {}): UcpResult {
  return { status, body: { code, content, ...extra } };
}

/** One error message, in the shape message_error.json requires. */
export interface UcpMessage extends Record<string, unknown> {
  type: "error" | "warning" | "info";
  content: string;
}

export function errorMessage(
  code: string,
  content: string,
  severity: "recoverable" | "requires_buyer_input" | "requires_buyer_review" | "unrecoverable",
  path?: string,
): UcpMessage {
  return { type: "error", code, content, severity, ...(path ? { path } : {}) };
}

/** The response when no resource can be shown: a create the shop refuses
 *  outright, or a session that does not exist. HTTP 200, `ucp.status:
 *  "error"`, as checkout.md and the REST binding prescribe. */
export function errorResponse(messages: UcpMessage[], continueUrl: string): UcpResult {
  return {
    status: 200,
    body: {
      ucp: { version: UCP_VERSION, status: "error", capabilities: { [CHECKOUT_CAPABILITY]: [{ version: UCP_VERSION }] } },
      messages,
      continue_url: continueUrl,
    },
  };
}

export const notFound = (origin: string): UcpResult =>
  errorResponse([errorMessage("not_found", "No such checkout session.", "unrecoverable")], `${origin}/`);
