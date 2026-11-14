// Sessions of the `auth` worker for this template's tests (GO-136). The
// template is scaffolded alone and cannot import workers/auth, so this mints
// what auth's signToken mints (workers/auth/docs/CONTRACT.md, "The access
// token"): an ES256 compact JWS over the contract-v1 claims, signed with a key
// generated per test run. The verifier the worker carries checks it as it
// checks auth's own tokens.
const enc = new TextEncoder();
const b64url = (b: Uint8Array | string) =>
  Buffer.from(typeof b === "string" ? enc.encode(b) : b).toString("base64").replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");

export const ISSUER = "https://example.com/api/auth";
export const LOGIN_URL = "/auth/login";
export const COOKIE = "__Host-ssg-auth";

// keyPair: a P-256 signing key and its public JWK as the consumer's env holds it.
export async function keyPair() {
  const pair = (await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"])) as CryptoKeyPair;
  const { kty, crv, x, y } = await crypto.subtle.exportKey("jwk", pair.publicKey);
  const jwk = { crv, kty, x, y };
  const kid = b64url(new Uint8Array(await crypto.subtle.digest("SHA-256", enc.encode(JSON.stringify(jwk)))));
  return { privateKey: pair.privateKey, publicJwk: JSON.stringify({ ...jwk, alg: "ES256", use: "sig" }), kid };
}
export type KeyPair = Awaited<ReturnType<typeof keyPair>>;
export const KEYS = await keyPair();

// mint signs contract-v1 claims; `over` replaces any of them (or the kid).
export async function mint(perms: string[], over: Record<string, unknown> = {}, keys: KeyPair = KEYS): Promise<string> {
  const now = Math.floor(Date.now() / 1000);
  const claims = { iss: ISSUER, aud: "example.com", sub: "user-1", sid: "session-1", email: "ada@example.com", iat: now, exp: now + 600, ver: 1, perms, ...over };
  const head = b64url(JSON.stringify({ alg: "ES256", typ: "JWT", kid: keys.kid }));
  const body = b64url(JSON.stringify(claims));
  const sig = await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, keys.privateKey, enc.encode(`${head}.${body}`));
  return `${head}.${body}.${b64url(new Uint8Array(sig))}`;
}

// authEnv: the COMMENTS_AUTH_* set that turns the mode on, plus `extra`.
export const authEnv = (extra: Record<string, unknown> = {}, keys: KeyPair = KEYS) => ({
  COMMENTS_AUTH_ISSUER: ISSUER,
  COMMENTS_AUTH_LOGIN_URL: LOGIN_URL,
  COMMENTS_AUTH_PUBLIC_KEY: keys.publicJwk,
  ...extra,
});

// session: the headers of a browser holding `token`; state-changing requests
// also carry what the panel's fetch wrapper sends (same Origin, x-ssg-auth: 1).
export const session = (token: string, write = false): Record<string, string> => ({
  cookie: `theme=dark; ${COOKIE}=${token}`,
  ...(write ? { origin: "https://example.com", "sec-fetch-site": "same-origin", "x-ssg-auth": "1" } : {}),
});
