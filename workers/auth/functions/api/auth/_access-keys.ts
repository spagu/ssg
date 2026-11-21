// The public keys of a Cloudflare Access team (GO-139), fetched from
// https://<team>.cloudflareaccess.com/cdn-cgi/access/certs and cached per
// isolate. A leading underscore keeps this file out of the Pages route table.
//
// The host is the configured team's only (_access-config.ts) and the request
// goes through providerFetch: https only, that host only, no redirect, a time
// and a size limit. Only RSA keys for RS256 are kept.
//
// Freshness, the cool-down and the stale limit are _key-cache.ts's, shared
// with the external issuer's JWKS (#435).

import { cachedKeys } from "./_key-cache";
import type { KeyCache, KeyTimes } from "./_key-cache";
import { okJson, providerFetch } from "./_provider-fetch";

export const accessKeyTimes: KeyTimes = { ttlMs: 60 * 60_000, coolDownMs: 60_000, maxStaleMs: 24 * 60 * 60_000 };

// Exported for the tests (they clear it between cases).
export const accessKeyCache: KeyCache<Map<string, CryptoKey>> = new Map();

export const certsUrl = (host: string): string => `https://${host}/cdn-cgi/access/certs`;

type Jwk = { kty?: unknown; kid?: unknown; n?: unknown; e?: unknown; alg?: unknown };

async function importRsa(k: Jwk): Promise<CryptoKey | null> {
  if (k.kty !== "RSA" || typeof k.n !== "string" || typeof k.e !== "string" || (k.alg !== undefined && k.alg !== "RS256")) return null;
  try {
    return await crypto.subtle.importKey("jwk", { kty: "RSA", n: k.n, e: k.e, alg: "RS256", ext: true }, { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" }, false, ["verify"]);
  } catch {
    return null;
  }
}

// fetchKeys asks the team; null (logged, without any token) when that fails.
async function fetchKeys(host: string): Promise<Map<string, CryptoKey> | null> {
  const got = okJson(await providerFetch(certsUrl(host), [host]), "Access certs");
  if (!got.ok) {
    console.warn(`auth: Cloudflare Access keys not fetched: ${got.detail}`);
    return null;
  }
  const list = Array.isArray(got.data.keys) ? (got.data.keys as unknown[]) : [];
  const out = new Map<string, CryptoKey>();
  for (const k of list) {
    if (!k || typeof k !== "object") continue;
    const kid = (k as Jwk).kid;
    const key = typeof kid === "string" && kid ? await importRsa(k as Jwk) : null;
    if (key) out.set(kid as string, key);
  }
  if (!out.size) {
    console.warn("auth: Cloudflare Access answered with no usable RS256 key");
    return null;
  }
  return out;
}

// accessKey: the key `kid` of the team at `host`; null when the team does not
// have it; "unavailable" when no usable keys could be had.
export async function accessKey(host: string, kid: string, now: number): Promise<CryptoKey | null | "unavailable"> {
  const keys = await cachedKeys({ cache: accessKeyCache, id: host, times: accessKeyTimes, fetch: () => fetchKeys(host), has: (k) => k.has(kid) }, now);
  return keys === "unavailable" ? keys : (keys.get(kid) ?? null);
}
