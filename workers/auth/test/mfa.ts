// Helpers for the GO-140 tests: a worker with the second factor on, codes
// computed as an authenticator app would, enrolment through the routes, and a
// software authenticator that answers WebAuthn ceremonies the way a browser
// and a security key do — with real keys (ES256, RS256, Ed25519), real CBOR
// and real signatures. Its answers are the "recorded" ones the tests replay
// and tamper with.
import { onRequestPost as login } from "../functions/api/auth/login.ts";
import { onRequestPost as totpStart } from "../functions/api/auth/mfa/totp/start.ts";
import { onRequestPost as totpConfirm } from "../functions/api/auth/mfa/totp/confirm.ts";
import { onRequestPost as verify } from "../functions/api/auth/mfa/verify.ts";
import { onRequestPost as stepUp } from "../functions/api/auth/mfa/step-up.ts";
import { base32Decode, hotp, stepAt } from "../functions/api/auth/_totp.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { b64urlEncode } from "../functions/api/auth/_lib.ts";
import { Browser, PASSWORD, call, makeEnv } from "./fakes.ts";

export const MFA_KEY = "mfa-test-key-0123456789-abcdefghijklmnop"; // 41 characters
export const mfaEnv = (over: Partial<Env> = {}) => makeEnv({ AUTH_MFA_KEY: MFA_KEY, ...over });

export { login, totpStart, totpConfirm, verify, stepUp };

// code: what the app shows for this secret at `now` (+ steps).
export const code = (secret: string, now: number, steps = 0): Promise<string> => hotp(base32Decode(secret) as Uint8Array, stepAt(now) + steps);

export async function signIn(env: Env, email: string, browser = new Browser()): Promise<{ browser: Browser; res: Response; body: Record<string, unknown> }> {
  const res = await call(login, env, "/api/auth/login", { json: { email, password: PASSWORD }, browser });
  return { browser, res, body: (await res.clone().json()) as Record<string, unknown> };
}

// enrolTotp: start + confirm through the routes, for a signed-in browser.
export async function enrolTotp(env: Env, browser: Browser, now: number, name = "Phone"): Promise<{ secret: string; factorId: string; codes?: string[] }> {
  const started = await call(totpStart, env, "/api/auth/mfa/totp/start", { json: {}, browser });
  if (started.status !== 201) throw new Error(`start ${started.status} ${await started.text()}`);
  const { secret, factorId } = (await started.json()) as { secret: string; factorId: string };
  const done = await call(totpConfirm, env, "/api/auth/mfa/totp/confirm", { json: { factorId, code: await code(secret, now), name }, browser });
  if (done.status !== 200) throw new Error(`confirm ${done.status} ${await done.text()}`);
  const body = (await done.json()) as { backupCodes?: string[] };
  return { secret, factorId, codes: body.backupCodes };
}

// ---- a minimal CBOR encoder (the decoder under test is the worker's) ----
type C = number | string | Uint8Array | C[] | Map<number | string, C> | boolean | null;
function head(major: number, n: number): number[] {
  if (n < 24) return [(major << 5) | n];
  if (n < 256) return [(major << 5) | 24, n];
  if (n < 65536) return [(major << 5) | 25, n >> 8, n & 255];
  return [(major << 5) | 26, (n >>> 24) & 255, (n >>> 16) & 255, (n >>> 8) & 255, n & 255];
}
export function cbor(v: C): Uint8Array {
  const out: number[] = [];
  const put = (x: C): void => {
    if (x === false) out.push(0xf4);
    else if (x === true) out.push(0xf5);
    else if (x === null) out.push(0xf6);
    else if (typeof x === "number") out.push(...(x >= 0 ? head(0, x) : head(1, -1 - x)));
    else if (typeof x === "string") {
      const b = new TextEncoder().encode(x);
      out.push(...head(3, b.length), ...b);
    } else if (x instanceof Uint8Array) out.push(...head(2, x.length), ...x);
    else if (Array.isArray(x)) {
      out.push(...head(4, x.length));
      x.forEach(put);
    } else {
      out.push(...head(5, x.size));
      for (const [k, val] of x) {
        put(k);
        put(val);
      }
    }
  };
  put(v);
  return Uint8Array.from(out);
}

const sha = async (b: Uint8Array) => new Uint8Array(await crypto.subtle.digest("SHA-256", b));
const enc = new TextEncoder();
const b64d = (s: string) => Uint8Array.from(atob(s.replaceAll("-", "+").replaceAll("_", "/")), (c) => c.charCodeAt(0));

// p1363ToDer: what an authenticator sends for ES256.
export function p1363ToDer(sig: Uint8Array): Uint8Array {
  const int = (b: Uint8Array) => {
    let i = 0;
    while (i < b.length - 1 && b[i] === 0) i++;
    const v = b.slice(i);
    return v[0] & 0x80 ? [0x02, v.length + 1, 0, ...v] : [0x02, v.length, ...v];
  };
  const body = [...int(sig.slice(0, 32)), ...int(sig.slice(32))];
  return Uint8Array.from([0x30, body.length, ...body]);
}

export type Alg = -7 | -257 | -8;

export class SoftAuthenticator {
  credentialId = crypto.getRandomValues(new Uint8Array(16));
  counter = 0;
  keys!: CryptoKeyPair;
  alg: Alg;
  counts: boolean; // false: always reports 0, like a synced passkey
  constructor(alg: Alg = -7, counts = true) {
    this.alg = alg;
    this.counts = counts;
  }
  async init(): Promise<this> {
    const params =
      this.alg === -7
        ? { name: "ECDSA", namedCurve: "P-256" }
        : this.alg === -257
          ? { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" }
          : { name: "Ed25519" };
    this.keys = (await crypto.subtle.generateKey(params as never, true, ["sign", "verify"])) as CryptoKeyPair;
    return this;
  }
  get id(): string {
    return b64urlEncode(this.credentialId);
  }
  async coseKey(): Promise<Map<number, C>> {
    const jwk = (await crypto.subtle.exportKey("jwk", this.keys.publicKey)) as JsonWebKey;
    if (this.alg === -7) return new Map<number, C>([[1, 2], [3, -7], [-1, 1], [-2, b64d(jwk.x as string)], [-3, b64d(jwk.y as string)]]);
    if (this.alg === -257) return new Map<number, C>([[1, 3], [3, -257], [-1, b64d(jwk.n as string)], [-2, b64d(jwk.e as string)]]);
    return new Map<number, C>([[1, 1], [3, -8], [-1, 6], [-2, b64d(jwk.x as string)]]);
  }
  async authData(rpId: string, flags: number, attested: boolean): Promise<Uint8Array> {
    if (this.counts) this.counter++;
    const count = [(this.counter >>> 24) & 255, (this.counter >>> 16) & 255, (this.counter >>> 8) & 255, this.counter & 255];
    const parts = [...(await sha(enc.encode(rpId))), flags | (attested ? 0x40 : 0), ...count];
    if (attested) parts.push(...new Uint8Array(16), 0, this.credentialId.length, ...this.credentialId, ...cbor(await this.coseKey()));
    return Uint8Array.from(parts);
  }
  // create answers navigator.credentials.create().
  async create(challenge: string, o: { origin?: string; rpId?: string; type?: string } = {}) {
    const clientDataJSON = enc.encode(JSON.stringify({ type: o.type ?? "webauthn.create", challenge, origin: o.origin ?? "https://example.com", crossOrigin: false }));
    const authData = await this.authData(o.rpId ?? "example.com", 0x01, true);
    const attestationObject = cbor(new Map<string, C>([["fmt", "none"], ["attStmt", new Map()], ["authData", authData]]));
    return { id: this.id, rawId: this.id, type: "public-key", response: { clientDataJSON: b64urlEncode(clientDataJSON), attestationObject: b64urlEncode(attestationObject), transports: ["usb"] } };
  }
  // get answers navigator.credentials.get().
  async get(challenge: string, o: { origin?: string; rpId?: string; type?: string; flags?: number } = {}) {
    const clientDataJSON = enc.encode(JSON.stringify({ type: o.type ?? "webauthn.get", challenge, origin: o.origin ?? "https://example.com" }));
    const authenticatorData = await this.authData(o.rpId ?? "example.com", o.flags ?? 0x05, false);
    const signed = Uint8Array.from([...authenticatorData, ...(await sha(clientDataJSON))]);
    const algo = this.alg === -7 ? { name: "ECDSA", hash: "SHA-256" } : this.alg === -257 ? { name: "RSASSA-PKCS1-v1_5" } : { name: "Ed25519" };
    let sig = new Uint8Array(await crypto.subtle.sign(algo, this.keys.privateKey, signed));
    if (this.alg === -7) sig = p1363ToDer(sig);
    return { id: this.id, rawId: this.id, type: "public-key", response: { clientDataJSON: b64urlEncode(clientDataJSON), authenticatorData: b64urlEncode(authenticatorData), signature: b64urlEncode(sig) } };
  }
}

// challengeIn: the challenge of an options answer.
export const challengeOf = (options: { publicKey: { challenge: string } }): string => options.publicKey.challenge;
