// Prints a fresh VAPID key pair for the web-push worker (Node 20 or later):
//
//   node workers/web-push/scripts/vapid-keys.mjs
//
// The public key goes in [vars] as PUSH_VAPID_PUBLIC_KEY; the private key is a
// secret: wrangler pages secret put PUSH_VAPID_PRIVATE_KEY. Changing the pair
// later invalidates every existing subscription.
const pair = await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"]);
const pub = new Uint8Array(await crypto.subtle.exportKey("raw", pair.publicKey));
const { d } = await crypto.subtle.exportKey("jwk", pair.privateKey);
const b64url = (bytes) => Buffer.from(bytes).toString("base64url");

console.log(`PUSH_VAPID_PUBLIC_KEY=${b64url(pub)}`);
console.log(`PUSH_VAPID_PRIVATE_KEY=${d}`);
