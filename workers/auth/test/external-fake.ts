// A scripted external JWT issuer for the #435 tests: its JWKS endpoint
// (https://id.example.com/.well-known/jwks.json), answered from test keys,
// and tokens it signs — or forged. It replaces fetch while installed; any
// other URL is a test failure.
import { onRequestPost as exchange } from "../functions/api/auth/jwt.ts";
import { externalKeyCache } from "../functions/api/auth/_external-keys.ts";
import { b64urlEncode } from "../functions/api/auth/_lib.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { Browser, T0, call, makeEnv } from "./fakes.ts";
import type { ReqOptions } from "./fakes.ts";

export const ISS = "https://id.example.com";
export const AUD = "https://example.com";
export const JWKS = `${ISS}/.well-known/jwks.json`;
export const SECRET = "s".repeat(32);
export const BASE_ENV = { AUTH_JWT_ISSUER: ISS, AUTH_JWT_AUDIENCE: AUD, AUTH_JWT_JWKS_URL: JWKS };

export const jwtEnv = (over: Record<string, string> = {}) => makeEnv({ ...BASE_ENV, ...over } as Partial<Env>);

const enc = (v: unknown) => b64urlEncode(new TextEncoder().encode(JSON.stringify(v)));
const te = new TextEncoder();

export interface TestKey {
  alg: string;
  kid: string;
  privateKey: CryptoKey;
  jwk: JsonWebKey & { kid: string };
  spkiPem: string;
}

const GEN: Record<string, { gen: RsaHashedKeyGenParams | EcKeyGenParams | Algorithm; sign: AlgorithmIdentifier | RsaPssParams | EcdsaParams }> = {
  RS256: { gen: { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" } as RsaHashedKeyGenParams, sign: "RSASSA-PKCS1-v1_5" },
  PS256: { gen: { name: "RSA-PSS", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" } as RsaHashedKeyGenParams, sign: { name: "RSA-PSS", saltLength: 32 } },
  ES256: { gen: { name: "ECDSA", namedCurve: "P-256" }, sign: { name: "ECDSA", hash: "SHA-256" } },
  ES384: { gen: { name: "ECDSA", namedCurve: "P-384" }, sign: { name: "ECDSA", hash: "SHA-384" } },
  ES512: { gen: { name: "ECDSA", namedCurve: "P-521" }, sign: { name: "ECDSA", hash: "SHA-512" } },
  EdDSA: { gen: { name: "Ed25519" }, sign: { name: "Ed25519" } },
  RS1024: { gen: { name: "RSASSA-PKCS1-v1_5", modulusLength: 1024, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" } as RsaHashedKeyGenParams, sign: "RSASSA-PKCS1-v1_5" },
};

export async function testKey(alg: string, kid = `${alg}-key`): Promise<TestKey> {
  const g = GEN[alg];
  const pair = (await crypto.subtle.generateKey(g.gen, true, ["sign", "verify"])) as CryptoKeyPair;
  const pub = (await crypto.subtle.exportKey("jwk", pair.publicKey)) as JsonWebKey;
  const { kty, n, e, crv, x, y } = pub;
  const der = new Uint8Array(await crypto.subtle.exportKey("spki", pair.publicKey));
  const spkiPem = `-----BEGIN PUBLIC KEY-----\n${btoa(String.fromCharCode(...der))}\n-----END PUBLIC KEY-----`;
  const signAlg = alg === "RS1024" ? "RS256" : alg;
  return { alg: signAlg, kid, privateKey: pair.privateKey, jwk: JSON.parse(JSON.stringify({ kty, n, e, crv, x, y, kid, alg: signAlg, use: "sig" })), spkiPem };
}

export const RS = await testKey("RS256");
export const OTHER_RS = await testKey("RS256", "other-key");

export type Forge = "none" | "hs256-public" | "bad-signature" | "short-signature";

// sign: a compact JWS of header.claims with the key (or forged as asked).
export async function sign(claims: Record<string, unknown>, key: TestKey = RS, header: Record<string, unknown> = {}, forge?: Forge): Promise<string> {
  if (forge === "none") return `${enc({ alg: "none", ...header })}.${enc(claims)}.`;
  if (forge === "hs256-public") {
    // The algorithm-confusion attack: HMAC with the issuer's PUBLIC key as the secret.
    const input = `${enc({ alg: "HS256", kid: key.kid, ...header })}.${enc(claims)}`;
    const k = await crypto.subtle.importKey("raw", te.encode(JSON.stringify(key.jwk)), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
    return `${input}.${b64urlEncode(new Uint8Array(await crypto.subtle.sign("HMAC", k, te.encode(input))))}`;
  }
  const input = `${enc({ alg: key.alg, typ: "JWT", kid: key.kid, ...header })}.${enc(claims)}`;
  const sig = new Uint8Array(await crypto.subtle.sign(GEN[key.alg].sign, key.privateKey, te.encode(input)));
  if (forge === "bad-signature") sig[0] ^= 0xff;
  return `${input}.${b64urlEncode(forge === "short-signature" ? sig.slice(1) : sig)}`;
}

// hmac: an HS256/384/512 token with a shared secret.
export async function hmac(claims: Record<string, unknown>, secret = SECRET, alg = "HS256", header: Record<string, unknown> = {}): Promise<string> {
  const input = `${enc({ alg, typ: "JWT", ...header })}.${enc(claims)}`;
  const k = await crypto.subtle.importKey("raw", te.encode(secret), { name: "HMAC", hash: `SHA-${alg.slice(2)}` }, false, ["sign"]);
  return `${input}.${b64urlEncode(new Uint8Array(await crypto.subtle.sign("HMAC", k, te.encode(input))))}`;
}

let jtis = 0;
// claims: what the issuer puts in a fresh token for `sub`, issued at `now`.
export function claims(over: Record<string, unknown> = {}, now = T0): Record<string, unknown> {
  const iat = Math.floor(now / 1000);
  return { iss: ISS, aud: AUD, sub: "ext-1001", email: "ada@example.com", email_verified: true, iat, nbf: iat, exp: iat + 300, jti: `jti-${++jtis}`, ...over };
}

export const mint = (over: Record<string, unknown> = {}, key: TestKey = RS, now = T0): Promise<string> => sign(claims(over, now), key);

export class FakeIssuer {
  requests: string[] = [];
  keys: TestKey[] = [RS];
  mode: "ok" | "500" | "throw" | "empty" | "junk" = "ok";
  private real = globalThis.fetch;
  install(): this {
    externalKeyCache.clear();
    globalThis.fetch = (async (url: string) => {
      const u = String(url);
      this.requests.push(u);
      if (u !== JWKS) throw new Error(`unexpected fetch ${u}`);
      if (this.mode === "throw") throw new TypeError("fetch failed");
      if (this.mode === "500") return new Response("{}", { status: 500 });
      const keys = this.mode === "empty" ? [] : this.mode === "junk" ? [null, "x", [1]] : this.keys.map((k) => k.jwk);
      return new Response(JSON.stringify({ keys }), { headers: { "content-type": "application/json" } });
    }) as typeof fetch;
    return this;
  }
  restore(): void {
    globalThis.fetch = this.real;
    externalKeyCache.clear();
  }
}

// post: the exchange as the issuer's page would send it — a plain form by
// default (from this origin unless `origin` says otherwise), or the bearer
// header of a same-origin fetch.
export async function post(
  env: Env,
  token: string | null,
  o: { via?: "form" | "bearer" | "json"; next?: string; browser?: Browser; origin?: string; headers?: Record<string, string> } = {},
): Promise<Response> {
  const browser = o.browser ?? new Browser();
  const fields: Record<string, string> = {};
  if (o.next !== undefined) fields.next = o.next;
  const headers: Record<string, string> = { ...(o.origin ? { origin: o.origin } : {}), ...o.headers };
  const opts: ReqOptions = { browser, headers };
  if (o.via === "bearer") {
    if (token) headers.authorization = `Bearer ${token}`;
    Object.assign(opts, { json: fields });
  } else if (o.via === "json") {
    Object.assign(opts, { json: token ? { ...fields, token } : fields });
  } else {
    Object.assign(opts, { form: token ? { ...fields, token } : fields });
  }
  return call(exchange, env, "/api/auth/jwt", opts);
}

export const where = (res: Response): string => res.headers.get("location") ?? "";
export const errorCode = (res: Response): string | null => new URL(where(res) || "/", "https://example.com").searchParams.get("error");
export const sqlite = (env: Env) => (env.AUTH_DB as unknown as { sqlite: { prepare(s: string): { get(...a: unknown[]): unknown; all(...a: unknown[]): unknown[]; run(...a: unknown[]): unknown } } }).sqlite;
export { T0 };
