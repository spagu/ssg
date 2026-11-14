// Passkeys as a second factor (GO-140): checking what the browser's WebAuthn
// API returns. A leading underscore keeps this file out of the Pages route
// table. Pure functions over bytes and the expected values; the challenges
// and the stored keys are _webauthn-store.ts.
//
// Registration: clientDataJSON (type webauthn.create, our challenge, our
// origin, not cross-origin), then the attestation object's authenticator data
// (our rpId's hash, user present, attested credential data) and the COSE key.
// The attestation STATEMENT is not verified — the options ask for "none" and
// this worker trusts no vendor list; a passkey is "something you have" here,
// not proof of a particular make of authenticator.
//
// Assertion: the same client-data checks (type webauthn.get), the rpId hash,
// user presence (user verification is not required: this is a SECOND factor,
// after a password or a provider), the signature over authenticatorData ‖
// SHA-256(clientDataJSON) with the stored key, and the counter: it must grow,
// except that an authenticator reporting 0 every time (synced passkeys do)
// stays accepted at 0 — the WebAuthn spec's own exception.
//
// Algorithms, all verified by the Workers runtime's WebCrypto (checked with
// a local workerd before this was written): ES256 (-7, P-256 — what nearly
// every authenticator uses), RS256 (-257, Windows Hello), EdDSA (-8, Ed25519).

import { decode, decodeFirst } from "./_webauthn-cbor";
import type { Cbor } from "./_webauthn-cbor";
import { b64urlEncode, bytesEqual } from "./_lib";

export const ALGORITHMS = [-7, -257, -8];
const FLAG_UP = 0x01;
const FLAG_AT = 0x40;

export class WebAuthnError extends Error {}
const fail = (why: string): never => {
  throw new WebAuthnError(why);
};

export interface Expected {
  challenge: string; // base64url, as issued
  origin: string;
  rpId: string;
}

const sha256 = async (b: Uint8Array): Promise<Uint8Array> => new Uint8Array(await crypto.subtle.digest("SHA-256", b));

// checkClientData: the browser's own record of the ceremony.
export function checkClientData(raw: Uint8Array, type: string, e: Expected): void {
  let c: Record<string, unknown>;
  try {
    c = JSON.parse(new TextDecoder("utf-8", { fatal: true, ignoreBOM: false }).decode(raw));
  } catch {
    return fail("client data is not JSON");
  }
  if (!c || typeof c !== "object") fail("client data is not an object");
  if (c.type !== type) fail("wrong ceremony type");
  if (typeof c.challenge !== "string" || c.challenge !== e.challenge) fail("wrong challenge");
  if (c.origin !== e.origin) fail("wrong origin");
  if (c.crossOrigin === true) fail("cross-origin ceremony");
}

export interface AuthData {
  rpIdHash: Uint8Array;
  flags: number;
  signCount: number;
  credentialId?: Uint8Array;
  cose?: Cbor;
}

// parseAuthData reads authenticator data: rpIdHash (32), flags (1), counter
// (4), then — with the AT flag — AAGUID (16), id length (2), id, COSE key.
export function parseAuthData(b: Uint8Array): AuthData {
  if (b.length < 37) fail("authenticator data too short");
  const out: AuthData = { rpIdHash: b.slice(0, 32), flags: b[32], signCount: new DataView(b.buffer, b.byteOffset + 33, 4).getUint32(0) };
  if (out.flags & FLAG_AT) {
    if (b.length < 55) fail("attested credential data too short");
    const len = (b[53] << 8) | b[54];
    if (len === 0 || len > 1023 || b.length < 55 + len) fail("bad credential id length");
    out.credentialId = b.slice(55, 55 + len);
    out.cose = decodeFirst(b.slice(55 + len)).value;
  }
  return out;
}

const bytesOf = (m: Map<number | string, Cbor>, k: number, len?: number): Uint8Array => {
  const v = m.get(k);
  if (!(v instanceof Uint8Array) || (len !== undefined && v.length !== len)) return fail(`COSE key parameter ${k} missing or wrong`);
  return v;
};

// coseToJwk turns a COSE_Key into the algorithm and a JWK WebCrypto imports.
export function coseToJwk(cose: Cbor | undefined): { alg: number; jwk: JsonWebKey } {
  if (!(cose instanceof Map)) return fail("COSE key is not a map");
  const kty = cose.get(1);
  const alg = cose.get(3);
  if (alg === -7 && kty === 2 && cose.get(-1) === 1) {
    return { alg, jwk: { kty: "EC", crv: "P-256", x: b64urlEncode(bytesOf(cose, -2, 32)), y: b64urlEncode(bytesOf(cose, -3, 32)) } };
  }
  if (alg === -257 && kty === 3) {
    const n = bytesOf(cose, -1);
    if (n.length < 256) fail("RSA key under 2048 bits");
    return { alg, jwk: { kty: "RSA", n: b64urlEncode(n), e: b64urlEncode(bytesOf(cose, -2)) } };
  }
  if (alg === -8 && kty === 1 && cose.get(-1) === 6) return { alg, jwk: { kty: "OKP", crv: "Ed25519", x: b64urlEncode(bytesOf(cose, -2, 32)) } };
  return fail("unsupported key type or algorithm");
}

const b64d = (s: string): Uint8Array => Uint8Array.from(atob(s.replaceAll("-", "+").replaceAll("_", "/")), (c) => c.charCodeAt(0));

// importKey: Ed25519 goes in as raw bytes — the Workers runtime refuses an
// OKP JWK whose `alg` it does not expect, raw works everywhere.
export function importKey(alg: number, jwk: JsonWebKey): Promise<CryptoKey> {
  if (alg === -7) return crypto.subtle.importKey("jwk", jwk, { name: "ECDSA", namedCurve: "P-256" }, false, ["verify"]);
  if (alg === -257) return crypto.subtle.importKey("jwk", jwk, { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" }, false, ["verify"]);
  return crypto.subtle.importKey("raw", b64d(jwk.x as string), { name: "Ed25519" }, false, ["verify"]);
}

// derToP1363: WebAuthn's ECDSA signatures are ASN.1 DER; WebCrypto wants r‖s.
export function derToP1363(der: Uint8Array): Uint8Array | null {
  if (der.length < 8 || der[0] !== 0x30 || der[1] !== der.length - 2) return null;
  const out = new Uint8Array(64);
  let at = 2;
  for (const half of [0, 32]) {
    if (der[at] !== 0x02) return null;
    const len = der[at + 1];
    let int = der.slice(at + 2, at + 2 + len);
    if (len === 0 || int.length !== len) return null;
    while (int.length > 1 && int[0] === 0) int = int.slice(1);
    if (int.length > 32) return null;
    out.set(int, half + 32 - int.length);
    at += 2 + len;
  }
  return at === der.length ? out : null;
}

export interface Registered {
  credentialId: string;
  alg: number;
  jwk: JsonWebKey;
  signCount: number;
}

// verifyRegistration checks a navigator.credentials.create() answer.
export async function verifyRegistration(clientData: Uint8Array, attestation: Uint8Array, e: Expected): Promise<Registered> {
  checkClientData(clientData, "webauthn.create", e);
  const att = decode(attestation);
  if (!(att instanceof Map) || typeof att.get("fmt") !== "string") return fail("bad attestation object");
  const raw = att.get("authData");
  if (!(raw instanceof Uint8Array)) return fail("no authenticator data");
  const auth = parseAuthData(raw);
  if (!bytesEqual(auth.rpIdHash, await sha256(new TextEncoder().encode(e.rpId)))) fail("wrong relying party");
  if (!(auth.flags & FLAG_UP)) fail("user not present");
  if (!auth.credentialId) return fail("no credential in the answer");
  const { alg, jwk } = coseToJwk(auth.cose);
  await importKey(alg, jwk); // a key WebCrypto refuses (a point off the curve) is refused now, not at sign-in
  return { credentialId: b64urlEncode(auth.credentialId), alg, jwk, signCount: auth.signCount };
}

export interface Stored {
  alg: number;
  jwk: JsonWebKey;
  signCount: number;
}

// verifyAssertion checks a navigator.credentials.get() answer against the
// stored key; returns the new counter to store.
export async function verifyAssertion(clientData: Uint8Array, authData: Uint8Array, signature: Uint8Array, key: Stored, e: Expected): Promise<number> {
  checkClientData(clientData, "webauthn.get", e);
  const auth = parseAuthData(authData);
  if (!bytesEqual(auth.rpIdHash, await sha256(new TextEncoder().encode(e.rpId)))) fail("wrong relying party");
  if (!(auth.flags & FLAG_UP)) fail("user not present");
  const signed = new Uint8Array([...authData, ...(await sha256(clientData))]);
  const sig = key.alg === -7 ? derToP1363(signature) : signature;
  if (!sig) return fail("malformed signature");
  const params = key.alg === -7 ? { name: "ECDSA", hash: "SHA-256" } : key.alg === -257 ? { name: "RSASSA-PKCS1-v1_5" } : { name: "Ed25519" };
  if (!(await crypto.subtle.verify(params, await importKey(key.alg, key.jwk), sig, signed))) fail("bad signature");
  if ((auth.signCount !== 0 || key.signCount !== 0) && auth.signCount <= key.signCount) fail("signature counter did not grow");
  return auth.signCount;
}
