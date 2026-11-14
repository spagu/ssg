// The token signing key and the published public keys. A leading underscore
// keeps this file out of the Pages route table.
//
// AUTH_SIGNING_KEY (a wrangler secret) is the P-256 private key as a JWK —
// {"kty":"EC","crv":"P-256","x":…,"y":…,"d":…} — as printed by
// scripts/auth-keys.mjs. JWK rather than PKCS#8 because it carries the public
// half too, so the worker can publish it without a second setting.
// AUTH_PREVIOUS_PUBLIC_KEY (a plain var) is the retired key's PUBLIC JWK,
// listed in the JWKS during a rotation so tokens it signed keep verifying until
// they expire. There is never a default key and never one generated at
// runtime: without a valid AUTH_SIGNING_KEY the worker answers 503.
//
// The key id is the RFC 7638 thumbprint of the public key, computed here — a
// `kid` written into the JWK is ignored — so the same key always has the same
// id and two keys never share one.

import { b64urlEncode, sha256 } from "./_lib";
import type { Env } from "./_lib";

export interface PublicJWK {
  kty: "EC";
  crv: "P-256";
  x: string;
  y: string;
  kid: string;
  alg: "ES256";
  use: "sig";
}

export interface SigningKey {
  kid: string;
  privateKey: CryptoKey;
  publicJwk: PublicJWK;
}

export interface KeySet {
  signing: SigningKey;
  verify: Map<string, CryptoKey>; // kid → public key, current first
  jwks: PublicJWK[];
}

const EC = { name: "ECDSA", namedCurve: "P-256" } as const;
const SIG = { name: "ECDSA", hash: "SHA-256" } as const;

function parseJwk(raw: string | undefined): Record<string, unknown> | null {
  if (!raw) return null;
  try {
    const v = JSON.parse(raw);
    return v && typeof v === "object" && v.kty === "EC" && v.crv === "P-256" && typeof v.x === "string" && typeof v.y === "string" ? v : null;
  } catch {
    return null;
  }
}

// thumbprint is the RFC 7638 JWK thumbprint: SHA-256 over the required members
// in lexicographic order, no whitespace.
export async function thumbprint(x: string, y: string): Promise<string> {
  return b64urlEncode(await sha256(JSON.stringify({ crv: "P-256", kty: "EC", x, y })));
}

async function publicOf(jwk: Record<string, unknown>): Promise<{ key: CryptoKey; jwk: PublicJWK }> {
  const x = jwk.x as string;
  const y = jwk.y as string;
  const key = await crypto.subtle.importKey("jwk", { kty: "EC", crv: "P-256", x, y }, EC, false, ["verify"]);
  return { key, jwk: { kty: "EC", crv: "P-256", x, y, kid: await thumbprint(x, y), alg: "ES256", use: "sig" } };
}

// build imports both keys and proves the private key matches its public half
// by signing and verifying one message: a pasted key with a wrong x/y would
// otherwise sign tokens nobody can verify.
async function build(env: Env): Promise<KeySet | null> {
  const priv = parseJwk(env.AUTH_SIGNING_KEY);
  if (!priv || typeof priv.d !== "string") return null;
  try {
    const privateKey = await crypto.subtle.importKey("jwk", { kty: "EC", crv: "P-256", x: priv.x as string, y: priv.y as string, d: priv.d }, EC, false, ["sign"]);
    const current = await publicOf(priv);
    const probe = new TextEncoder().encode("ssg-auth key check");
    if (!(await crypto.subtle.verify(SIG, current.key, await crypto.subtle.sign(SIG, privateKey, probe), probe))) return null;
    const verify = new Map([[current.jwk.kid, current.key]]);
    const jwks = [current.jwk];
    const prevJwk = parseJwk(env.AUTH_PREVIOUS_PUBLIC_KEY);
    if (prevJwk) {
      const prev = await publicOf(prevJwk);
      if (!verify.has(prev.jwk.kid)) {
        verify.set(prev.jwk.kid, prev.key);
        jwks.push(prev.jwk);
      }
    }
    return { signing: { kid: current.jwk.kid, privateKey, publicJwk: current.jwk }, verify, jwks };
  } catch {
    return null;
  }
}

// Per-isolate cache, keyed by the exact configuration, so a redeploy with a
// new key (a new isolate anyway) or a test with another key never reuses one.
let cache: { id: string; keys: Promise<KeySet | null> } | null = null;

// loadKeys returns the key set, or null when AUTH_SIGNING_KEY is missing or
// unusable (the caller answers 503). A malformed AUTH_PREVIOUS_PUBLIC_KEY is
// ignored — it only widens what verifies — while a broken one fails closed.
export function loadKeys(env: Env): Promise<KeySet | null> {
  const id = `${env.AUTH_SIGNING_KEY ?? ""}\n${env.AUTH_PREVIOUS_PUBLIC_KEY ?? ""}`;
  if (!cache || cache.id !== id) cache = { id, keys: build(env) };
  return cache.keys;
}
