// Key material and tokens for the login-provider fakes (GO-151): RS256 key
// pairs like Google's, Microsoft's and Apple's, ID tokens signed with them (or
// tampered with), and the Apple developer key that signs client secrets.
import { b64urlDecode, b64urlEncode } from "../functions/api/auth/_lib.ts";

const enc = (v: unknown) => b64urlEncode(new TextEncoder().encode(JSON.stringify(v)));

export interface RsaKey {
  kid: string;
  privateKey: CryptoKey;
  jwk: JsonWebKey & { kid: string };
}

export async function rsaKey(kid: string, bits = 2048): Promise<RsaKey> {
  const pair = (await crypto.subtle.generateKey(
    { name: "RSASSA-PKCS1-v1_5", modulusLength: bits, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
    true,
    ["sign", "verify"],
  )) as CryptoKeyPair;
  const { kty, n, e } = (await crypto.subtle.exportKey("jwk", pair.publicKey)) as JsonWebKey;
  return { kid, privateKey: pair.privateKey, jwk: { kty, n, e, kid, alg: "RS256", use: "sig" } };
}

export type Forge = "rs256" | "none" | "hs256-public" | "bad-signature";

// signJwt signs header.claims with the key — or forges it as asked.
export async function signJwt(header: Record<string, unknown>, claims: Record<string, unknown>, key: RsaKey, forge: Forge = "rs256"): Promise<string> {
  if (forge === "none") return `${enc({ ...header, alg: "none" })}.${enc(claims)}.`;
  if (forge === "hs256-public") {
    // The classic confusion attack: HMAC with the provider's PUBLIC key as the secret.
    const input = `${enc({ ...header, alg: "HS256" })}.${enc(claims)}`;
    const secret = new TextEncoder().encode(JSON.stringify(key.jwk));
    const k = await crypto.subtle.importKey("raw", secret, { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
    return `${input}.${b64urlEncode(new Uint8Array(await crypto.subtle.sign("HMAC", k, new TextEncoder().encode(input))))}`;
  }
  const input = `${enc({ alg: "RS256", typ: "JWT", kid: key.kid, ...header })}.${enc(claims)}`;
  const sig = new Uint8Array(await crypto.subtle.sign("RSASSA-PKCS1-v1_5", key.privateKey, new TextEncoder().encode(input)));
  if (forge === "bad-signature") sig[0] ^= 0xff;
  return `${input}.${b64urlEncode(sig)}`;
}

// appleKey: a P-256 developer key as Apple hands it out (a .p8 PEM) and its
// public half, for checking the client secrets the worker signs.
export async function appleKey(): Promise<{ pem: string; publicKey: CryptoKey }> {
  const pair = (await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"])) as CryptoKeyPair;
  const der = new Uint8Array(await crypto.subtle.exportKey("pkcs8", pair.privateKey));
  const b64 = btoa(String.fromCharCode(...der));
  const lines = b64.match(/.{1,64}/g) as string[];
  return { pem: `-----BEGIN PRIVATE KEY-----\n${lines.join("\n")}\n-----END PRIVATE KEY-----`, publicKey: pair.publicKey };
}

// verifyEs256 checks a JWT the way Apple's token endpoint would; null if bad.
export async function verifyEs256(jwt: string, key: CryptoKey): Promise<{ header: Record<string, unknown>; claims: Record<string, unknown> } | null> {
  const [h, c, s] = jwt.split(".");
  const sig = b64urlDecode(s ?? "");
  if (!h || !c || !sig) return null;
  const ok = await crypto.subtle.verify({ name: "ECDSA", hash: "SHA-256" }, key, sig as BufferSource, new TextEncoder().encode(`${h}.${c}`));
  const dec = (p: string) => JSON.parse(new TextDecoder().decode(b64urlDecode(p) as Uint8Array)) as Record<string, unknown>;
  return ok ? { header: dec(h), claims: dec(c) } : null;
}

export const s256 = async (verifier: string): Promise<string> => b64urlEncode(new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier))));
