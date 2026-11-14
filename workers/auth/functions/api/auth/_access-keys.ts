// The public keys of a Cloudflare Access team (GO-139), fetched from
// https://<team>.cloudflareaccess.com/cdn-cgi/access/certs and cached per
// isolate. A leading underscore keeps this file out of the Pages route table.
//
// The host is the configured team's only (_access-config.ts) and the request
// goes through providerFetch: https only, that host only, no redirect, a time
// and a size limit. Only RSA keys for RS256 are kept.
//
// Freshness without hammering the team, and without failing open:
//   - keys younger than TTL are used as they are;
//   - an unknown key id (Access rotated its keys) or an older cache asks again,
//     but at most once per COOL_DOWN — a stream of tokens with made-up key ids
//     cannot turn into a stream of requests to Cloudflare;
//   - when asking fails, the keys already held keep working for up to
//     MAX_STALE (a short outage of the certs endpoint does not lock everyone
//     out); with none usable the answer is "unavailable" and the sign-in is
//     refused — never accepted unchecked.
// Concurrent requests of one isolate share a single fetch.

import { okJson, providerFetch } from "./_provider-fetch";

export const accessKeyTimes = { ttlMs: 60 * 60_000, coolDownMs: 60_000, maxStaleMs: 24 * 60 * 60_000 };

interface Entry {
  keys: Map<string, CryptoKey>;
  fetchedAt: number; // 0 = never fetched successfully
  triedAt: number;
  pending: Promise<void> | null;
}

// Exported for the tests (they clear it between cases).
export const accessKeyCache = new Map<string, Entry>();

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

function entryOf(host: string): Entry {
  let e = accessKeyCache.get(host);
  if (!e) {
    e = { keys: new Map(), fetchedAt: 0, triedAt: Number.NEGATIVE_INFINITY, pending: null };
    accessKeyCache.set(host, e);
  }
  return e;
}

// accessKey: the key `kid` of the team at `host`; null when the team does not
// have it; "unavailable" when no usable keys could be had.
export async function accessKey(host: string, kid: string, now: number): Promise<CryptoKey | null | "unavailable"> {
  const e = entryOf(host);
  if (now - e.fetchedAt < accessKeyTimes.ttlMs && e.keys.has(kid)) return e.keys.get(kid) as CryptoKey;
  if (!e.pending && now - e.triedAt >= accessKeyTimes.coolDownMs) {
    e.triedAt = now;
    e.pending = fetchKeys(host).then((fresh) => {
      if (fresh) {
        e.keys = fresh;
        e.fetchedAt = now;
      }
      e.pending = null;
    });
  }
  if (e.pending) await e.pending;
  if (!e.keys.size || now - e.fetchedAt >= accessKeyTimes.maxStaleMs) return "unavailable";
  return e.keys.get(kid) ?? null;
}
