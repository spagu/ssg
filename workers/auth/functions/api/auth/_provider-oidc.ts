// OpenID Connect ID tokens (GO-151): verified HERE, locally, against the
// provider's published keys — never by asking the provider whether a token is
// good. A leading underscore keeps this file out of the Pages route table.
//
// Checked, in order, each a refusal:
//   - three base64url parts, a JSON header and payload;
//   - `alg` on the provider's allowlist (RS256 for all three providers):
//     "none", HS256 (which would let the PUBLIC key act as an HMAC secret) and
//     everything else are refused before any key is looked at;
//   - `kid` names a key in the provider's JWKS — an RSA key of at least 2048
//     bits whose own `alg`/`use`, when present, agree;
//   - the RSASSA-PKCS1-v1_5 SHA-256 signature;
//   - `iss` (a provider-specific check: Microsoft's issuer is per tenant),
//     `aud` (our client id; with several audiences `azp` must be ours),
//     `exp` (60 s leeway), `iat` not in the future, `nonce` equal to the one
//     this sign-in sent, `sub` present.
//
// The JWKS is cached per isolate for JWKS_TTL_MS. A `kid` not in the cache
// refetches the set (keys rotate) — but at most once per JWKS_COOLDOWN_MS, so
// tokens with invented key ids cannot make this worker hammer the provider.

import { b64urlDecode, secretEqual } from "./_lib";
import { providerFetch } from "./_provider-fetch";

export const JWKS_TTL_MS = 3600_000;
export const JWKS_COOLDOWN_MS = 60_000;
const LEEWAY_S = 60;

interface Cached {
  keys: JsonWebKey[];
  fetchedAt: number;
}
export const jwksCache = new Map<string, Cached>();

export interface OidcCheck {
  jwksUrl: string;
  hosts: readonly string[];
  algs: readonly string[];
  audience: string;
  nonce: string;
  now: number;
  issuer: (iss: unknown, claims: Record<string, unknown>) => boolean;
}

export type Verified = { ok: true; claims: Record<string, unknown> } | { ok: false; detail: string };

const decodeJson = (part: string): Record<string, unknown> | null => {
  const bytes = b64urlDecode(part);
  if (!bytes) return null;
  try {
    const v = JSON.parse(new TextDecoder().decode(bytes)) as unknown;
    return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : null;
  } catch {
    return null;
  }
};

async function loadKeys(c: OidcCheck, force: boolean): Promise<JsonWebKey[] | string> {
  const hit = jwksCache.get(c.jwksUrl);
  const age = hit ? c.now - hit.fetchedAt : Infinity;
  // Fresh and not asked to look again; or asked, but fetched too recently.
  if (hit && ((age < JWKS_TTL_MS && !force) || age < JWKS_COOLDOWN_MS)) return hit.keys;
  const got = await providerFetch(c.jwksUrl, c.hosts);
  const keys = got.ok && got.status === 200 ? (got.data as { keys?: unknown }).keys : undefined;
  if (!Array.isArray(keys)) return got.ok ? `JWKS: status ${got.status}, no key list` : got.detail;
  jwksCache.set(c.jwksUrl, { keys: keys as JsonWebKey[], fetchedAt: c.now });
  return keys as JsonWebKey[];
}

async function keyFor(c: OidcCheck, kid: string, alg: string): Promise<CryptoKey | string> {
  let keys = await loadKeys(c, false);
  let k = typeof keys === "string" ? undefined : keys.find((j) => (j as { kid?: unknown }).kid === kid);
  if (!k && typeof keys !== "string") {
    keys = await loadKeys(c, true);
    k = typeof keys === "string" ? undefined : keys.find((j) => (j as { kid?: unknown }).kid === kid);
  }
  if (typeof keys === "string") return keys;
  if (!k) return "unknown key id";
  if (k.kty !== "RSA" || (k.alg !== undefined && k.alg !== alg) || (k.use !== undefined && k.use !== "sig")) return "key unusable for this algorithm";
  if ((b64urlDecode(String(k.n ?? ""))?.length ?? 0) < 256) return "RSA key shorter than 2048 bits";
  try {
    return await crypto.subtle.importKey("jwk", { kty: "RSA", n: k.n, e: k.e }, { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" }, false, ["verify"]);
  } catch {
    return "key cannot be imported";
  }
}

function audienceOk(claims: Record<string, unknown>, audience: string): boolean {
  const aud = claims.aud;
  if (typeof aud === "string") return aud === audience;
  if (!Array.isArray(aud) || !aud.includes(audience)) return false;
  return aud.length === 1 || claims.azp === audience;
}

// verifyIdToken applies every check above.
export async function verifyIdToken(token: string, c: OidcCheck): Promise<Verified> {
  const parts = token.split(".");
  if (parts.length !== 3) return { ok: false, detail: "ID token is not a JWS" };
  const header = decodeJson(parts[0]);
  const claims = decodeJson(parts[1]);
  const sig = b64urlDecode(parts[2]);
  if (!header || !claims || !sig) return { ok: false, detail: "ID token is malformed" };
  const alg = header.alg;
  if (typeof alg !== "string" || !c.algs.includes(alg)) return { ok: false, detail: `ID token algorithm ${String(alg).slice(0, 16)} not allowed` };
  if (typeof header.kid !== "string") return { ok: false, detail: "ID token has no key id" };
  const key = await keyFor(c, header.kid, alg);
  if (typeof key === "string") return { ok: false, detail: `ID token: ${key}` };
  const signed = new TextEncoder().encode(`${parts[0]}.${parts[1]}`);
  if (!(await crypto.subtle.verify("RSASSA-PKCS1-v1_5", key, sig, signed))) return { ok: false, detail: "ID token signature is wrong" };
  const nowS = Math.floor(c.now / 1000);
  if (!c.issuer(claims.iss, claims)) return { ok: false, detail: "ID token issuer is wrong" };
  if (!audienceOk(claims, c.audience)) return { ok: false, detail: "ID token audience is wrong" };
  if (typeof claims.exp !== "number" || claims.exp + LEEWAY_S <= nowS) return { ok: false, detail: "ID token expired" };
  if (claims.iat !== undefined && (typeof claims.iat !== "number" || claims.iat - LEEWAY_S > nowS)) return { ok: false, detail: "ID token issued in the future" };
  if (typeof claims.nonce !== "string" || !(await secretEqual(claims.nonce, c.nonce))) return { ok: false, detail: "ID token nonce is wrong" };
  if (typeof claims.sub !== "string" || !claims.sub || claims.sub.length > 255) return { ok: false, detail: "ID token has no subject" };
  return { ok: true, claims };
}

// truthy reads a provider's boolean claim: Apple sends "true" as a string.
export const truthy = (v: unknown): boolean => v === true || v === "true";

// idTokenOf: the id_token of a token-endpoint answer.
export const idTokenOf = (data: Record<string, unknown>): string | null => (typeof data.id_token === "string" ? data.id_token : null);
