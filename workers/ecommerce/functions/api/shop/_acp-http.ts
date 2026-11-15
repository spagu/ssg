// What every ACP response has in common: the protocol's flat error shape, the
// versions this shop speaks, and a canonical form of a JSON body.
//
// Implemented against the Agentic Commerce Protocol as published in its
// repository, release 2026-04-17 (spec/2026-04-17/openapi/
// openapi.agentic_checkout.yaml and rfcs/rfc.agentic_checkout.md), read on
// 2026-11-14. The README section "Selling through AI agents (ACP)" lists the
// places that release contradicts itself and what this code chose.

import type { Env } from "./_env";
import type { AgentResult } from "./_agent-common";

/** The release the shapes here follow, sent back in every session. */
export const ACP_PROTOCOL_VERSION = "2026-04-17";

/** API-Version values accepted. 2026-01-16 is listed because the RFC text of
 *  the 2026-04-17 release still tells clients to send it; 2026-01-30 is the
 *  release in between. The shapes answered are always 2026-04-17's. */
export const ACP_SUPPORTED_VERSIONS = ["2026-04-17", "2026-01-30", "2026-01-16"];

export type AcpErrorType = "invalid_request" | "processing_error" | "service_unavailable";

/** What a handler returns; the route wrapper turns it into a Response and
 *  stores it against the idempotency key. */
export type AcpResult = AgentResult;

/** True only when the owner switched ACP on. Anything else — unset, "1",
 *  "yes" — is off, and off is a 404 on every ACP route. */
export function acpEnabled(env: Env): boolean {
  return env.SHOP_ACP_ENABLED === "true";
}

/** The protocol's Error object: {type, code, message, param?}. */
export function acpError(
  status: number,
  type: AcpErrorType,
  code: string,
  message: string,
  param?: string,
  extra: Record<string, unknown> = {},
): AcpResult {
  const body: Record<string, unknown> = { type, code, message, ...extra };
  if (param) body.param = param;
  return { status, body };
}

// The Response, the canonical JSON and the body parser are the same for ACP
// and UCP, so they live in _agent-common.ts and are re-exported here under
// the names the ACP modules already use.
export { canonicalJson, parseBody, toResponse } from "./_agent-common";
