// Test helper: access tokens as the `auth` worker signs them (contract v1,
// ES256), from a key pair made here — so this template's tests need nothing
// from workers/auth.
const enc = new TextEncoder();
const b64 = (b: Uint8Array) => Buffer.from(b).toString("base64url");
const part = (v: unknown) => b64(enc.encode(JSON.stringify(v)));

const pair = (await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"])) as CryptoKeyPair;
const { kty, crv, x, y } = (await crypto.subtle.exportKey("jwk", pair.publicKey)) as JsonWebKey;
const jwk = { crv, kty, x, y };
// PUBLIC_KEY: the inline public JWK a consumer is configured with.
export const PUBLIC_KEY = JSON.stringify(jwk);
const kid = b64(new Uint8Array(await crypto.subtle.digest("SHA-256", enc.encode(JSON.stringify(jwk)))));

export const ISSUER = "https://example.com/api/auth";

// mint signs a token; `kind` defaults to "service", lifetime 10 minutes from now.
export async function mint(perms: string[], over: Record<string, unknown> = {}): Promise<string> {
  const iat = Math.floor(Date.now() / 1000);
  const claims = { iss: ISSUER, aud: "example.com", sub: "svc-1", sid: "0123456789ab", email: "", iat, exp: iat + 600, ver: 1, perms, kind: "service", ...over };
  const unsigned = `${part({ alg: "ES256", typ: "JWT", kid })}.${part(claims)}`;
  const sig = await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, pair.privateKey, enc.encode(unsigned));
  return `${unsigned}.${b64(new Uint8Array(sig))}`;
}

// authVars: a complete <PREFIX>_AUTH_* set for this key.
export const authVars = (prefix: string): Record<string, string> => ({
  [`${prefix}_AUTH_ISSUER`]: ISSUER,
  [`${prefix}_AUTH_PUBLIC_KEY`]: PUBLIC_KEY,
  [`${prefix}_AUTH_LOGIN_URL`]: "/auth/login",
});
