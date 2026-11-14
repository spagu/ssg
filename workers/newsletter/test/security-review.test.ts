// Regression test for the security review (workers/auth/docs/SECURITY-REVIEW.md).
import { test } from "node:test";
import assert from "node:assert/strict";
import { requireAdmin } from "../functions/api/newsletter/_lib.ts";

const TEAM = "example-team";
const AUD = "aud-newsletter";
const b64url = (b: Uint8Array | string) => Buffer.from(b).toString("base64").replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");

async function issuer() {
  const pair = (await crypto.subtle.generateKey({ name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" }, true, [
    "sign",
    "verify",
  ])) as CryptoKeyPair;
  const jwk = await crypto.subtle.exportKey("jwk", pair.publicKey);
  const head = b64url(JSON.stringify({ alg: "RS256", kid: "k1" }));
  const body = b64url(JSON.stringify({ aud: [AUD], iss: `https://${TEAM}.cloudflareaccess.com`, exp: Math.floor(Date.now() / 1000) + 600, email: "m@example.com" }));
  const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", pair.privateKey, new TextEncoder().encode(`${head}.${body}`));
  const real = globalThis.fetch;
  globalThis.fetch = (async () => Response.json({ keys: [{ kid: "k1", kty: "RSA", n: jwk.n, e: jwk.e }] })) as typeof fetch;
  return { token: `${head}.${body}.${b64url(new Uint8Array(sig))}`, restore: () => void (globalThis.fetch = real) };
}

test("SR-10: Access mode refuses a cross-site write carried only by the CF_Authorization cookie", async () => {
  const { token, restore } = await issuer();
  try {
    const env = { NEWSLETTER_DB: null, NEWSLETTER_ACCESS_TEAM: TEAM, NEWSLETTER_ACCESS_AUD: AUD } as never;
    const post = (headers: Record<string, string>) => new Request("https://example.com/api/newsletter/admin", { method: "POST", headers: { cookie: `CF_Authorization=${token}`, ...headers }, body: "{}" });
    const forged = await requireAdmin(post({ origin: "https://attacker.example.net", "sec-fetch-site": "cross-site", "content-type": "text/plain" }), env, "newsletter:settings");
    assert.equal(forged?.status, 403);
    assert.equal(await requireAdmin(post({ origin: "https://example.com" }), env, "newsletter:settings"), null);
    const read = new Request("https://example.com/api/newsletter/admin", { headers: { cookie: `CF_Authorization=${token}` } });
    assert.equal(await requireAdmin(read, env, "newsletter:read"), null);
  } finally {
    restore();
  }
});
