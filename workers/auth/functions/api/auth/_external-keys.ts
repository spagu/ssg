// The key that verifies an external issuer's token (#435). A leading
// underscore keeps this file out of the Pages route table.
//
// The key comes from the configuration only — the JWKS URL, the static public
// key or the shared secret (_external-config.ts) — never from the token: its
// `jku`, `x5u`, `jwk` and `x5c` headers are never read. The token's `kid`
// only chooses among the configured keys; a key without a `kid` is used only
// when it is the only one. A key is used only for the family its `kty`/`crv`
// belong to, and only when its own `alg` and `use`, when present, agree with
// the token's algorithm. RSA keys shorter than 2048 bits are refused.
//
// The JWKS is fetched through providerFetch (https only, the configured host
// only, no redirect, a time and a size limit) and cached by _key-cache.ts:
// an hour, a refetch for an unknown `kid` at most once a minute, the last
// good set used for up to a day when the issuer cannot be reached.

import type { ExternalSetup } from "./_external-config";
import { cachedKeys } from "./_key-cache";
import type { KeyCache, KeyTimes } from "./_key-cache";
import { b64urlDecode } from "./_lib";
import { okJson, providerFetch } from "./_provider-fetch";

export const externalKeyTimes: KeyTimes = { ttlMs: 60 * 60_000, coolDownMs: 60_000, maxStaleMs: 24 * 60 * 60_000 };

// Exported for the tests (they clear it between cases).
export const externalKeyCache: KeyCache<Jwk[]> = new Map();

const MAX_KEYS = 100;

// A configured key: a JWK's members as given (unchecked until used), or the
// DER of a PEM public key.
export type Jwk = { [member: string]: unknown; spki?: Uint8Array };

// The WebCrypto parameter shapes, as plain objects (the runtimes' type names differ).
type Params = { name: string; [param: string]: unknown };

interface Family {
  kty: string;
  crv?: string;
  importAs: Params;
  verifyAs: string | Params;
  sigBytes?: number; // the exact JWS signature length, where the algorithm fixes it
}

const sha = (bits: string) => `SHA-${bits}`;

// family: the WebCrypto parameters of a JWS algorithm this worker accepts.
export function family(alg: string): Family {
  const bits = alg.slice(2);
  const hash = sha(bits);
  switch (alg.slice(0, 2)) {
    case "RS":
      return { kty: "RSA", importAs: { name: "RSASSA-PKCS1-v1_5", hash }, verifyAs: "RSASSA-PKCS1-v1_5" };
    case "PS":
      return { kty: "RSA", importAs: { name: "RSA-PSS", hash }, verifyAs: { name: "RSA-PSS", saltLength: Number(bits) / 8 } };
    case "ES": {
      const crv = bits === "512" ? "P-521" : `P-${bits}`;
      return { kty: "EC", crv, importAs: { name: "ECDSA", namedCurve: crv }, verifyAs: { name: "ECDSA", hash }, sigBytes: bits === "512" ? 132 : (Number(bits) / 8) * 2 };
    }
    case "HS":
      return { kty: "oct", importAs: { name: "HMAC", hash }, verifyAs: "HMAC" };
    default: // EdDSA — Ed25519 is the curve WebCrypto verifies
      return { kty: "OKP", crv: "Ed25519", importAs: { name: "Ed25519" }, verifyAs: { name: "Ed25519" }, sigBytes: 64 };
  }
}

// staticKeys reads AUTH_JWT_PUBLIC_KEY: a JWK, a JWK Set, or a PEM
// "PUBLIC KEY" (SPKI). [] when it is none of them.
export function staticKeys(raw: string): Jwk[] {
  const pem = /^-----BEGIN PUBLIC KEY-----([A-Za-z0-9+/=\s\\n]+)-----END PUBLIC KEY-----$/.exec(raw.trim());
  if (pem) {
    try {
      const bin = atob(pem[1].replace(/\\n|\s/g, ""));
      return [{ spki: Uint8Array.from(bin, (c) => c.charCodeAt(0)) }];
    } catch {
      return [];
    }
  }
  try {
    const v = JSON.parse(raw) as unknown;
    return keyList(v && typeof v === "object" && Array.isArray((v as { keys?: unknown }).keys) ? (v as { keys: unknown[] }).keys : [v]);
  } catch {
    return [];
  }
}

const keyList = (raw: unknown[]): Jwk[] => raw.filter((k): k is Jwk => Boolean(k) && typeof k === "object" && !Array.isArray(k)).slice(0, MAX_KEYS);

// fetchJwks asks the issuer; null (logged, without any token) when that fails.
async function fetchJwks(s: Extract<ExternalSetup["source"], { kind: "jwks" }>): Promise<Jwk[] | null> {
  const got = okJson(await providerFetch(s.url, [s.host]), "JWKS");
  const keys = got.ok && Array.isArray(got.data.keys) ? keyList(got.data.keys as unknown[]) : [];
  if (!keys.length) {
    console.warn(`auth: external issuer keys not fetched: ${got.ok ? "the JWK Set holds no key" : got.detail}`);
    return null;
  }
  return keys;
}

// pick: the configured key the token's `kid` names, or the only one.
function pick(keys: Jwk[], kid: unknown): Jwk | undefined {
  if (typeof kid === "string" && kid) {
    const named = keys.find((k) => k.kid === kid);
    if (named) return named;
  }
  return keys.length === 1 && keys[0].kid === undefined ? keys[0] : undefined;
}

async function importKey(k: Jwk, alg: string): Promise<CryptoKey | string> {
  const f = family(alg);
  try {
    if (k.spki) {
      const key = await crypto.subtle.importKey("spki", k.spki as BufferSource, f.importAs, false, ["verify"]);
      const short = f.kty === "RSA" && (key.algorithm as { modulusLength: number }).modulusLength < 2048;
      return short ? "RSA key shorter than 2048 bits" : key;
    }
    if (k.kty !== f.kty || (f.crv !== undefined && k.crv !== f.crv)) return "key type does not fit the algorithm";
    if ((k.alg !== undefined && k.alg !== alg) || (k.use !== undefined && k.use !== "sig")) return "key not meant for this algorithm";
    if (f.kty === "RSA" && (b64urlDecode(String(k.n ?? ""))?.length ?? 0) < 256) return "RSA key shorter than 2048 bits";
    const { kty, n, e, crv, x, y } = k;
    return await crypto.subtle.importKey("jwk", { kty, n, e, crv, x, y } as JsonWebKey, f.importAs, false, ["verify"]);
  } catch {
    return "key cannot be imported";
  }
}

export type KeyFound = { ok: true; key: CryptoKey } | { ok: false; reason: string; unavailable: boolean };

// externalKey: the key to verify a token of `alg` whose header names `kid`.
export async function externalKey(setup: ExternalSetup, alg: string, kid: unknown, now: number): Promise<KeyFound> {
  const src = setup.source;
  const no = (reason: string, unavailable = false): KeyFound => ({ ok: false, reason, unavailable });
  if (src.kind === "secret") {
    return { ok: true, key: await crypto.subtle.importKey("raw", src.secret as BufferSource, family(alg).importAs, false, ["verify"]) };
  }
  let keys: Jwk[];
  if (src.kind === "static") keys = src.keys;
  else {
    const has = (list: Jwk[]) => pick(list, kid) !== undefined;
    const got = await cachedKeys({ cache: externalKeyCache, id: src.url, times: externalKeyTimes, fetch: () => fetchJwks(src), has }, now);
    if (got === "unavailable") return no("keys unavailable", true);
    keys = got;
  }
  const k = pick(keys, kid);
  if (!k) return no("unknown key id");
  const key = await importKey(k, alg);
  return typeof key === "string" ? no(key) : { ok: true, key };
}
