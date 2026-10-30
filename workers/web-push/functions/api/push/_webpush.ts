// Web Push without a library: the payload encryption of RFC 8291 (aes128gcm,
// RFC 8188) and the VAPID authorisation of RFC 8292, on WebCrypto alone. A
// leading underscore keeps this file out of the Pages route table.
//
// The salt and the sender's key pair are random per message. Both can be
// passed in, which is how the tests reproduce the RFC's worked example byte
// for byte; nothing else should ever pass them.

import { b64urlDecode, b64urlEncode } from "./_lib";
import type { Subscription } from "./_store";

const enc = new TextEncoder();
const RECORD_SIZE = 4096;

function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let at = 0;
  for (const p of parts) {
    out.set(p, at);
    at += p.length;
  }
  return out;
}

async function hmac(key: Uint8Array, data: Uint8Array): Promise<Uint8Array> {
  const k = await crypto.subtle.importKey("raw", key, { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  return new Uint8Array(await crypto.subtle.sign("HMAC", k, data));
}

// hkdf is HKDF-SHA-256 for outputs of at most one block, which is all RFC 8291
// needs (32, 16 and 12 bytes).
async function hkdf(salt: Uint8Array, ikm: Uint8Array, info: Uint8Array, length: number): Promise<Uint8Array> {
  const prk = await hmac(salt, ikm);
  return (await hmac(prk, concat(info, new Uint8Array([1])))).slice(0, length);
}

// importPrivate turns a raw private scalar and its public point into a key
// WebCrypto can use; it has no raw import for EC private keys.
export function importPrivate(d: Uint8Array, pub: Uint8Array, usage: "ecdh" | "ecdsa"): Promise<CryptoKey> {
  const jwk = {
    kty: "EC",
    crv: "P-256",
    d: b64urlEncode(d),
    x: b64urlEncode(pub.slice(1, 33)),
    y: b64urlEncode(pub.slice(33, 65)),
  };
  return usage === "ecdh"
    ? crypto.subtle.importKey("jwk", jwk, { name: "ECDH", namedCurve: "P-256" }, false, ["deriveBits"])
    : crypto.subtle.importKey("jwk", jwk, { name: "ECDSA", namedCurve: "P-256" }, false, ["sign"]);
}

export interface SenderKeys {
  privateKey: CryptoKey; // ECDH
  publicKey: Uint8Array; // 65 bytes, uncompressed
}

async function freshSenderKeys(): Promise<SenderKeys> {
  const pair = (await crypto.subtle.generateKey({ name: "ECDH", namedCurve: "P-256" }, true, ["deriveBits"])) as CryptoKeyPair;
  const publicKey = new Uint8Array((await crypto.subtle.exportKey("raw", pair.publicKey)) as ArrayBuffer);
  return { privateKey: pair.privateKey, publicKey };
}

// encryptPayload returns the aes128gcm body for one subscription: the RFC 8188
// header (salt, record size, the sender's public key) and a single record.
export async function encryptPayload(
  plaintext: Uint8Array,
  sub: Subscription,
  fixed: { salt?: Uint8Array; sender?: SenderKeys } = {},
): Promise<Uint8Array> {
  const uaPublic = b64urlDecode(sub.p256dh);
  const authSecret = b64urlDecode(sub.auth);
  if (!uaPublic || uaPublic.length !== 65 || !authSecret || authSecret.length !== 16) {
    throw new Error("invalid subscription keys");
  }
  const salt = fixed.salt ?? crypto.getRandomValues(new Uint8Array(16));
  const sender = fixed.sender ?? (await freshSenderKeys());

  const uaKey = await crypto.subtle.importKey("raw", uaPublic, { name: "ECDH", namedCurve: "P-256" }, false, []);
  const ecdhSecret = new Uint8Array(await crypto.subtle.deriveBits({ name: "ECDH", public: uaKey }, sender.privateKey, 256));

  const keyInfo = concat(enc.encode("WebPush: info\0"), uaPublic, sender.publicKey);
  const ikm = await hkdf(authSecret, ecdhSecret, keyInfo, 32);
  const cek = await hkdf(salt, ikm, enc.encode("Content-Encoding: aes128gcm\0"), 16);
  const nonce = await hkdf(salt, ikm, enc.encode("Content-Encoding: nonce\0"), 12);

  const aes = await crypto.subtle.importKey("raw", cek, "AES-GCM", false, ["encrypt"]);
  // 0x02 marks the last (and only) record; no further padding.
  const record = new Uint8Array(
    await crypto.subtle.encrypt({ name: "AES-GCM", iv: nonce }, aes, concat(plaintext, new Uint8Array([2]))),
  );

  const header = new Uint8Array(21);
  header.set(salt, 0);
  new DataView(header.buffer).setUint32(16, RECORD_SIZE);
  header[20] = sender.publicKey.length;
  return concat(header, sender.publicKey, record);
}

// vapidAuthorization is the RFC 8292 header value for one push service: a
// JWT signed with the application server's key, valid for 12 hours, for the
// endpoint's origin only.
export async function vapidAuthorization(
  endpoint: string,
  subject: string,
  publicKey: string,
  privateKey: string,
  now = Date.now(),
): Promise<string> {
  const pub = b64urlDecode(publicKey);
  const d = b64urlDecode(privateKey);
  if (!pub || pub.length !== 65 || !d || d.length !== 32) throw new Error("invalid VAPID keys");
  const key = await importPrivate(d, pub, "ecdsa");
  const part = (v: unknown) => b64urlEncode(enc.encode(JSON.stringify(v)));
  const claims = { aud: new URL(endpoint).origin, exp: Math.floor(now / 1000) + 12 * 3600, sub: subject };
  const unsigned = `${part({ typ: "JWT", alg: "ES256" })}.${part(claims)}`;
  // WebCrypto signs in the IEEE P1363 form (r || s) that JWS expects.
  const sig = new Uint8Array(await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, key, enc.encode(unsigned)));
  return `vapid t=${unsigned}.${b64urlEncode(sig)}, k=${publicKey}`;
}

export interface VapidConfig {
  subject: string;
  publicKey: string;
  privateKey: string;
  ttl: number;
}

// sendPush delivers one message and returns the push service's status.
export async function sendPush(sub: Subscription, payload: unknown, vapid: VapidConfig): Promise<number> {
  const body = await encryptPayload(enc.encode(JSON.stringify(payload)), sub);
  const res = await fetch(sub.endpoint, {
    method: "POST",
    headers: {
      authorization: await vapidAuthorization(sub.endpoint, vapid.subject, vapid.publicKey, vapid.privateKey),
      "content-encoding": "aes128gcm",
      "content-type": "application/octet-stream",
      ttl: String(vapid.ttl),
      urgency: "normal",
    },
    body,
  });
  return res.status;
}
