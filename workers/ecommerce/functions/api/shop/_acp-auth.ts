// Who may call the ACP checkout, and in which protocol version.
//
// The specification requires `Authorization: Bearer <token>` and
// `API-Version` on every call, and recommends a `Signature` + `Timestamp` pair
// "over canonical JSON" whose algorithm the seller publishes out of band. So:
//
//   - the bearer token is always required (SHOP_ACP_API_KEY, a secret);
//   - the signature is required only when the owner sets
//     SHOP_ACP_SIGNING_SECRET, and is checked as HMAC-SHA256 over
//     `<Timestamp>.<body>` — the scheme the protocol itself uses for the
//     webhooks the seller sends — base64url (or base64) encoded. The body may
//     be the raw bytes or their canonical JSON (sorted keys); both are
//     accepted because the release does not say which, and both need the key.
//
// Every comparison is constant time, and a missing secret is a 503: an ACP
// endpoint switched on without its key is closed, never open.

import type { Env } from "./_env";
import { ACP_SUPPORTED_VERSIONS, acpError, canonicalJson, parseBody, type AcpResult } from "./_acp-http";
import { base64url, sha256hex, timingSafeEqual } from "./_lib";

/** How far a request's Timestamp may be from our clock: the five minutes the
 *  protocol recommends for its own webhooks. */
export const ACP_CLOCK_SKEW_MS = 300_000;

/** Null when the request may go on; otherwise the error to answer with. */
export async function acpGate(request: Request, env: Env, raw: string): Promise<AcpResult | null> {
  // Stripe is the only payment handler offered, so ACP without it could take
  // orders it can never charge.
  if (!env.SHOP_ACP_API_KEY || !env.STRIPE_SECRET_KEY || !env.SHOP_ACP_MERCHANT_ID) {
    return acpError(503, "service_unavailable", "not_configured", "Agentic checkout is switched on but not fully configured.");
  }

  const header = request.headers.get("authorization") ?? "";
  const token = /^Bearer\s+(.+)$/i.exec(header)?.[1]?.trim() ?? "";
  // Hashing both sides first makes the compare fixed-length, so neither the
  // key's length nor its prefix leaks through timing.
  if (!token || !timingSafeEqual(await sha256hex(token), await sha256hex(env.SHOP_ACP_API_KEY))) {
    return {
      ...acpError(401, "invalid_request", "unauthorized", "A valid bearer token is required."),
      headers: { "www-authenticate": "Bearer" },
    };
  }

  const version = (request.headers.get("api-version") ?? "").trim();
  if (!ACP_SUPPORTED_VERSIONS.includes(version)) {
    return acpError(
      400,
      "invalid_request",
      version ? "unsupported_api_version" : "missing_api_version",
      version ? `API-Version ${version.slice(0, 32)} is not supported.` : "The API-Version header is required.",
      undefined,
      { supported_versions: ACP_SUPPORTED_VERSIONS },
    );
  }

  if (env.SHOP_ACP_SIGNING_SECRET && !(await signatureValid(request, env.SHOP_ACP_SIGNING_SECRET, raw))) {
    return acpError(401, "invalid_request", "invalid_signature", "The request signature is missing, stale or wrong.");
  }
  return null;
}

async function signatureValid(request: Request, secret: string, raw: string): Promise<boolean> {
  const signature = request.headers.get("signature") ?? "";
  const timestamp = request.headers.get("timestamp") ?? "";
  if (!signature || !timestamp) return false;
  // An old signature is a replay, however good its HMAC.
  const at = Date.parse(timestamp);
  if (!Number.isFinite(at) || Math.abs(Date.now() - at) > ACP_CLOCK_SKEW_MS) return false;

  const given = signature.trim().replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
  const parsed = parseBody(raw);
  const bodies = parsed.ok && raw.trim() !== "" ? [raw, canonicalJson(parsed.value)] : [raw];
  let ok = false;
  for (const body of bodies) {
    // No early return: both candidates are always computed and compared.
    ok = timingSafeEqual(given, await hmacBase64url(secret, `${timestamp}.${body}`)) || ok;
  }
  return ok;
}

/** HMAC-SHA256, base64url without padding. Exported for the tests, which
 *  sign requests the way a platform would. */
export async function hmacBase64url(secret: string, message: string): Promise<string> {
  const key = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  return base64url(new Uint8Array(await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(message))));
}
