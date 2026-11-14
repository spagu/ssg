// Encryption of the TOTP secrets at rest (GO-140). A leading underscore keeps
// this file out of the Pages route table.
//
// The secret an authenticator app shares with this worker must be readable
// here (to compute the expected code), so it cannot be hashed like a password:
// it is encrypted with AES-256-GCM under a key derived (HKDF-SHA-256) from the
// AUTH_MFA_KEY secret, which lives in the Workers secret store and never in
// D1. A copy of the database alone does not give anyone a single code.
//
// Stored form: "v1.<key id>.<iv>.<ciphertext>" (base64url). The key id is a
// short fingerprint of the derived key, so a rotation can tell which key a row
// was written with. The additional authenticated data binds a ciphertext to
// its own factor row and account: moved to another row it no longer opens.
//
// Rotation: put the old value in AUTH_MFA_KEY_PREVIOUS and a new one in
// AUTH_MFA_KEY. Rows still under the old key keep opening; each is rewritten
// under the new key the next time its code is accepted (_mfa-verify.ts).
// docs/SECOND-FACTOR.md says when the old value can go.

import { b64urlDecode, b64urlEncode, sha256hex } from "./_lib";
import type { Env } from "./_lib";

// MIN_KEY_LENGTH: a shorter AUTH_MFA_KEY is a mistake (a word, a placeholder),
// not a key. `openssl rand -base64 32` gives 44 characters.
export const MIN_KEY_LENGTH = 32;

export type MfaState = "off" | "ready" | "misconfigured";

// mfaState: the second factor is off without AUTH_MFA_KEY; a value that is too
// short keeps it ON (the policy still applies — failing closed) but nothing
// can be encrypted or decrypted until it is fixed.
export function mfaState(env: Env): MfaState {
  const k = (env.AUTH_MFA_KEY ?? "").trim();
  if (!k) return "off";
  return k.length >= MIN_KEY_LENGTH ? "ready" : "misconfigured";
}

export const mfaEnabled = (env: Env): boolean => mfaState(env) !== "off";

interface Key {
  id: string;
  key: CryptoKey;
}

const enc = new TextEncoder();

async function derive(secret: string): Promise<Key> {
  const base = await crypto.subtle.importKey("raw", enc.encode(secret), "HKDF", false, ["deriveKey"]);
  const key = await crypto.subtle.deriveKey(
    { name: "HKDF", hash: "SHA-256", salt: enc.encode("ssg-auth-mfa"), info: enc.encode("totp-secret v1") },
    base,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"],
  );
  return { id: (await sha256hex("ssg-auth-mfa-key-id\0" + secret)).slice(0, 12), key };
}

// keys: the current key first, then the previous one; [] when unusable.
async function keys(env: Env): Promise<Key[]> {
  if (mfaState(env) !== "ready") return [];
  const out = [await derive((env.AUTH_MFA_KEY as string).trim())];
  const prev = (env.AUTH_MFA_KEY_PREVIOUS ?? "").trim();
  if (prev.length >= MIN_KEY_LENGTH) out.push(await derive(prev));
  return out;
}

const aad = (factorId: string, userId: string): Uint8Array => enc.encode(`ssg-auth-totp\0${factorId}\0${userId}`);

// seal encrypts under the current key; null when the key is unusable.
export async function seal(env: Env, plain: Uint8Array, factorId: string, userId: string): Promise<string | null> {
  const [k] = await keys(env);
  if (!k) return null;
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const ct = new Uint8Array(await crypto.subtle.encrypt({ name: "AES-GCM", iv, additionalData: aad(factorId, userId) }, k.key, plain));
  return `v1.${k.id}.${b64urlEncode(iv)}.${b64urlEncode(ct)}`;
}

export interface Opened {
  plain: Uint8Array;
  stale: boolean; // written under the previous key: rewrite it
}

// open decrypts; null for a key that is gone, a tampered or moved row, or
// anything that is not our format.
export async function open(env: Env, sealed: string, factorId: string, userId: string): Promise<Opened | null> {
  const [v, id, ivRaw, ctRaw] = sealed.split(".");
  const iv = b64urlDecode(ivRaw ?? "");
  const ct = b64urlDecode(ctRaw ?? "");
  if (v !== "v1" || !iv || !ct) return null;
  const all = await keys(env);
  const k = all.find((x) => x.id === id);
  if (!k) return null;
  try {
    const plain = new Uint8Array(await crypto.subtle.decrypt({ name: "AES-GCM", iv, additionalData: aad(factorId, userId) }, k.key, ct));
    return { plain, stale: k !== all[0] };
  } catch {
    return null;
  }
}
