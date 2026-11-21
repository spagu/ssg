// Verifying an external issuer's JWT (#435), RFC 7519 with the RFC 8725
// rules. A leading underscore keeps this file out of the Pages route table.
//
// Every check refuses; nothing falls back to "let them in":
//   - a compact JWS of at most 16 KiB, JSON header and payload;
//   - `alg` on the configured list (`none` can never be on it), no `crit`;
//     the key from the configuration only (_external-keys.ts) — `jku`,
//     `x5u`, `jwk`, `x5c` are ignored;
//   - the signature;
//   - `iss` exactly the configured issuer; `aud` (a string, or an array
//     holding it) the configured audience;
//   - `exp` REQUIRED and not passed; `nbf`, when present, passed; `iat`
//     REQUIRED, not in the future, and no older than AUTH_JWT_MAX_AGE_SECONDS
//     — 60 s of clock skew on each;
//   - `jti` REQUIRED (single use is the route's, _external-replay.ts);
//   - the identity claim: `sub`, or a verified address (_external-identity.ts).
// No token or claim value ever reaches a log: reasons only.

import { SKEW_SECONDS } from "./_external-config";
import type { ExternalSetup } from "./_external-config";
import { externalKey, family } from "./_external-keys";
import { b64urlDecode } from "./_lib";

export const MAX_TOKEN = 16_384;

export type Claims = Record<string, unknown>;

export type JwtVerified = { ok: true; claims: Claims; jti: string; exp: number; iat: number } | { ok: false; reason: string; unavailable: boolean };

function jsonPart(part: string): Claims | null {
  const bytes = b64urlDecode(part);
  if (!bytes) return null;
  try {
    const v: unknown = JSON.parse(new TextDecoder("utf-8", { fatal: true, ignoreBOM: false }).decode(bytes));
    return v && typeof v === "object" && !Array.isArray(v) ? (v as Claims) : null;
  } catch {
    return null;
  }
}

const isTime = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v);

// claimProblem: why the (signature-checked) claims are not acceptable, or null.
export function claimProblem(c: Claims, s: ExternalSetup, now: number): string | null {
  if (c.iss !== s.issuer) return "issuer";
  const auds = Array.isArray(c.aud) ? c.aud : [c.aud];
  if (!auds.includes(s.audience)) return "audience";
  if (!isTime(c.exp)) return "no expiry";
  if (now >= c.exp + SKEW_SECONDS) return "expired";
  if (c.nbf !== undefined && (!isTime(c.nbf) || now + SKEW_SECONDS < c.nbf)) return "not yet valid";
  if (!isTime(c.iat)) return "no issued-at";
  if (now + SKEW_SECONDS < c.iat) return "issued in the future";
  if (now - c.iat > s.maxAgeSeconds + SKEW_SECONDS) return "too old";
  if (typeof c.jti !== "string" || !c.jti || c.jti.length > 255) return "no token id";
  return null;
}

export async function verifyExternalToken(token: string, s: ExternalSetup, nowMs: number): Promise<JwtVerified> {
  const fail = (reason: string, unavailable = false): JwtVerified => ({ ok: false, reason, unavailable });
  if (!token) return fail("no token");
  const parts = token.split(".");
  if (token.length > MAX_TOKEN || parts.length !== 3) return fail("malformed");
  const header = jsonPart(parts[0]);
  const claims = jsonPart(parts[1]);
  const sig = b64urlDecode(parts[2]);
  if (!header || !claims || !sig?.length) return fail("malformed");
  const alg = header.alg;
  if (typeof alg !== "string" || !s.algorithms.includes(alg) || "crit" in header) return fail("algorithm");
  const f = family(alg);
  if (f.sigBytes !== undefined && sig.length !== f.sigBytes) return fail("signature");
  const key = await externalKey(s, alg, header.kid, nowMs);
  if (!key.ok) return fail(key.reason, key.unavailable);
  const signed = new TextEncoder().encode(`${parts[0]}.${parts[1]}`);
  if (!(await crypto.subtle.verify(f.verifyAs, key.key, sig as BufferSource, signed))) return fail("signature");
  const problem = claimProblem(claims, s, Math.floor(nowMs / 1000));
  if (problem) return fail(problem);
  return { ok: true, claims, jti: claims.jti as string, exp: claims.exp as number, iat: claims.iat as number };
}
