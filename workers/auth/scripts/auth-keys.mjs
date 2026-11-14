// Prints a fresh ES256 (P-256) signing key pair for the auth worker
// (Node 20 or later):
//
//   node workers/auth/scripts/auth-keys.mjs
//
// AUTH_SIGNING_KEY is the private key as a JWK — a secret:
//   wrangler pages secret put AUTH_SIGNING_KEY      (paste the line's value)
// The public JWK is what /api/auth/jwks will publish; keep it for the next
// rotation, when it becomes AUTH_PREVIOUS_PUBLIC_KEY (a plain var). The key id
// printed is the RFC 7638 thumbprint the worker computes itself.
//
// Nothing is written to disk: copy the values, then clear your terminal.
import { createHash } from "node:crypto";

const pair = await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"]);
const { kty, crv, x, y, d } = await crypto.subtle.exportKey("jwk", pair.privateKey);
const kid = createHash("sha256").update(JSON.stringify({ crv, kty, x, y })).digest("base64url");

console.log(`# key id ${kid}`);
console.log(`AUTH_SIGNING_KEY=${JSON.stringify({ kty, crv, x, y, d })}`);
console.log(`# public key (for AUTH_PREVIOUS_PUBLIC_KEY at the next rotation):`);
console.log(`AUTH_PUBLIC_KEY=${JSON.stringify({ kty, crv, x, y })}`);
