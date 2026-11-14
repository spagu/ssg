// `auth` sessions for the tests (GO-137), without importing workers/auth (the
// template is scaffolded alone): a P-256 key pair made here and tokens signed
// with WebCrypto exactly as the `auth` worker signs them (ES256 compact JWS,
// kid = RFC 7638 thumbprint, the contract's claims).
export const ISSUER = "https://example.com/api/auth";
export const LOGIN = "/auth/login";

const enc = new TextEncoder();
const b64u = (b: Uint8Array) => Buffer.from(b).toString("base64url");
const part = (v: unknown) => b64u(enc.encode(JSON.stringify(v)));

const pair = (await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"])) as CryptoKeyPair;
const jwk = await crypto.subtle.exportKey("jwk", pair.publicKey);
const canonical = { crv: "P-256", kty: "EC", x: jwk.x as string, y: jwk.y as string };
const KID = b64u(new Uint8Array(await crypto.subtle.digest("SHA-256", enc.encode(JSON.stringify(canonical)))));

// AUTH_VARS: a complete NEWSLETTER_AUTH_* set (audience defaults to the host).
export const AUTH_VARS = { NEWSLETTER_AUTH_ISSUER: ISSUER, NEWSLETTER_AUTH_PUBLIC_KEY: JSON.stringify(canonical), NEWSLETTER_AUTH_LOGIN_URL: LOGIN };

// token signs a session for `perms` (claims overridable).
export async function token(perms: string[], over: Record<string, unknown> = {}): Promise<string> {
  const now = Math.floor(Date.now() / 1000);
  const claims = { iss: ISSUER, aud: "example.com", sub: "u-ada", sid: "s-1", email: "ada@example.com", iat: now, exp: now + 600, ver: 1, perms, ...over };
  const head = `${part({ alg: "ES256", typ: "JWT", kid: KID })}.${part(claims)}`;
  const sig = new Uint8Array(await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, pair.privateKey, enc.encode(head)));
  return `${head}.${b64u(sig)}`;
}

// session: a request to `path` carrying a session for `perms` (null: none).
export async function session(path: string, perms: string[] | null, init: RequestInit = {}): Promise<Request> {
  const headers = new Headers(init.headers);
  if (perms) headers.set("cookie", `__Host-ssg-auth=${await token(perms)}`);
  return new Request(`https://example.com${path}`, { ...init, headers });
}
