// Access tokens: compact JWS, ES256 (ECDSA P-256 / SHA-256) only. A leading
// underscore keeps this file out of the Pages route table.
//
// verifyToken is the reference for the portable verifier other workers will
// carry (GO-135): the algorithm comes from this code, never from the token's
// header; the key is chosen by `kid` among the keys we hold; every claim the
// contract defines is checked. It imports nothing but WebCrypto and the two
// base64url helpers, so it can be lifted into a self-contained file.

import { b64urlDecode, b64urlEncode } from "./_lib";
import type { SigningKey } from "./_keys";

// CONTRACT_VERSION is the `ver` claim: the token format other workers rely on.
// An incompatible change is a new number, and verifiers refuse numbers they do
// not know.
export const CONTRACT_VERSION = 1;
// LEEWAY_SECONDS tolerates small clock differences between edge locations.
export const LEEWAY_SECONDS = 30;

export interface AccessClaims {
  iss: string;
  aud: string;
  sub: string; // users.id
  sid: string; // sessions.id
  email: string;
  iat: number;
  exp: number;
  ver: number;
  perms: string[]; // the account's effective permissions (GO-133), sorted; wildcards as written
  // kind: absent = "user" (a person's session); "service" = a machine's token
  // from POST /api/auth/token (GO-141). Which channel may carry which kind is
  // the caller's rule (CONTRACT.md): accessClaims takes user tokens only.
  kind?: "user" | "service";
  // mfa_at: when the session last passed a second factor (GO-140), Unix
  // seconds; absent when it has not. Informative: verifiers ignore it.
  mfa_at?: number;
}

const enc = new TextEncoder();
const part = (v: unknown) => b64urlEncode(enc.encode(JSON.stringify(v)));

export async function signToken(key: SigningKey, claims: AccessClaims): Promise<string> {
  const unsigned = `${part({ alg: "ES256", typ: "JWT", kid: key.kid })}.${part(claims)}`;
  // WebCrypto signs in the IEEE P1363 form (r || s) that JWS expects.
  const sig = await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, key.privateKey, enc.encode(unsigned));
  return `${unsigned}.${b64urlEncode(new Uint8Array(sig))}`;
}

export interface VerifyOptions {
  keys: Map<string, CryptoKey>;
  issuer: string;
  audience: string;
  nowSeconds: number;
}

export type Verified = { ok: true; claims: AccessClaims } | { ok: false; reason: string };

function decodeJSON(s: string): Record<string, unknown> | null {
  const bytes = b64urlDecode(s);
  if (!bytes) return null;
  try {
    const v = JSON.parse(new TextDecoder("utf-8", { fatal: true, ignoreBOM: false }).decode(bytes));
    return v && typeof v === "object" && !Array.isArray(v) ? v : null;
  } catch {
    return null;
  }
}

const isNum = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v);

// verifyToken checks a token's shape, signature and claims, in that order, and
// says which check failed (for logs and tests; callers answer one generic 401).
export async function verifyToken(token: string, opt: VerifyOptions): Promise<Verified> {
  const parts = token.split(".");
  if (parts.length !== 3) return { ok: false, reason: "malformed" };
  const header = decodeJSON(parts[0]);
  const payload = decodeJSON(parts[1]);
  const sig = b64urlDecode(parts[2]);
  if (!header || !payload || !sig) return { ok: false, reason: "malformed" };
  // The only algorithm is ours: "none", HS256 (the public key used as an HMAC
  // secret) and everything else stop here, before any key is touched.
  if (header.alg !== "ES256" || "crit" in header) return { ok: false, reason: "algorithm" };
  const key = typeof header.kid === "string" ? opt.keys.get(header.kid) : undefined;
  if (!key) return { ok: false, reason: "unknown_key" };
  if (sig.length !== 64) return { ok: false, reason: "signature" };
  const valid = await crypto.subtle.verify({ name: "ECDSA", hash: "SHA-256" }, key, sig, enc.encode(`${parts[0]}.${parts[1]}`));
  if (!valid) return { ok: false, reason: "signature" };

  const p = payload;
  if (p.ver !== CONTRACT_VERSION) return { ok: false, reason: "version" };
  if (p.iss !== opt.issuer) return { ok: false, reason: "issuer" };
  if (p.aud !== opt.audience) return { ok: false, reason: "audience" };
  if (!isNum(p.exp) || !isNum(p.iat)) return { ok: false, reason: "claims" };
  if (p.exp <= opt.nowSeconds - LEEWAY_SECONDS) return { ok: false, reason: "expired" };
  if (p.iat > opt.nowSeconds + LEEWAY_SECONDS) return { ok: false, reason: "future" };
  if (typeof p.sub !== "string" || typeof p.sid !== "string" || typeof p.email !== "string") return { ok: false, reason: "claims" };
  if (!Array.isArray(p.perms) || !p.perms.every((x) => typeof x === "string")) return { ok: false, reason: "claims" };
  // An unknown kind is a token format we do not know, as the portable verifier says.
  const kind = p.kind ?? "user";
  if (kind !== "user" && kind !== "service") return { ok: false, reason: "version" };
  return { ok: true, claims: p as unknown as AccessClaims };
}
