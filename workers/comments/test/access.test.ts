// Cloudflare Access refusals the compatibility suite does not pin, and the
// identity accessIdentity hands back (GO-136): a team given as a hostname,
// Access unreachable, a key set with non-RSA keys, a payload that is not JSON,
// another algorithm, an unknown key, a single-string audience, the JWKS cache.
import { test } from "node:test";
import assert from "node:assert/strict";
import { accessIdentity, verifyAccess } from "../functions/api/comments/_access.ts";

const b64url = (s: string | Uint8Array) => Buffer.from(s).toString("base64").replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
const pair = (await crypto.subtle.generateKey(
  { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
  true,
  ["sign", "verify"],
)) as CryptoKeyPair;
const pub = await crypto.subtle.exportKey("jwk", pair.publicKey);

async function token(host: string, claims: Record<string, unknown> = {}, header: Record<string, unknown> = { alg: "RS256", kid: "k1" }, rawPayload?: string) {
  const head = b64url(JSON.stringify(header));
  const body = rawPayload ?? b64url(JSON.stringify({ aud: "aud-1", iss: `https://${host}`, exp: Math.floor(Date.now() / 1000) + 600, ...claims }));
  const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", pair.privateKey, new TextEncoder().encode(`${head}.${body}`));
  return `${head}.${body}.${b64url(new Uint8Array(sig))}`;
}

// serve answers the Access certs of one host per team; anything else is a 500.
async function serve(body: (fetched: string[]) => Promise<void>) {
  const real = globalThis.fetch;
  const fetched: string[] = [];
  globalThis.fetch = (async (url: string) => {
    fetched.push(String(url));
    if (String(url).includes("down")) return new Response("no", { status: 502 });
    return Response.json({ keys: [{ kid: "ec", kty: "EC" }, { kid: "k1", kty: "RSA", n: pub.n, e: pub.e }] });
  }) as typeof fetch;
  try {
    await body(fetched);
  } finally {
    globalThis.fetch = real;
  }
}

const env = (team: string) => ({ COMMENTS_ACCESS_TEAM: team, COMMENTS_ACCESS_AUD: "aud-1" }) as never;
const req = (t: string) => new Request("https://example.com/api/comments/admin", { headers: { "cf-access-jwt-assertion": t } });
const refusal = async (r: Response | null | { email: string | null }) => {
  assert.ok(r instanceof Response);
  return [r.status, await r.json()];
};

test("access: a full hostname team, a string audience, the email handed back, the keys cached", async () => {
  await serve(async (fetched) => {
    const host = "team-a.example.com";
    assert.deepEqual(await accessIdentity(req(await token(host, { email: "mod@example.com" })), env(host)), { email: "mod@example.com" });
    assert.deepEqual(await accessIdentity(req(await token(host, { email: "" })), env(host)), { email: null });
    assert.equal(await verifyAccess(req(await token(host)), env(host)), null);
    assert.deepEqual(fetched, ["https://team-a.example.com/cdn-cgi/access/certs"], "one fetch, then the cache");
  });
});

test("access: refusals — Access down, wrong algorithm, unknown key, a payload that is not JSON", async () => {
  await serve(async () => {
    assert.deepEqual(await refusal(await verifyAccess(req(await token("down.example.com")), env("down.example.com"))), [503, { error: "cannot reach Cloudflare Access to verify" }]);
    const host = "team-b.example.com";
    assert.deepEqual(await refusal(await verifyAccess(req(await token(host, {}, { alg: "HS256", kid: "k1" })), env(host))), [401, { error: "unexpected token algorithm" }]);
    assert.deepEqual(await refusal(await verifyAccess(req(await token(host, {}, { alg: "RS256", kid: "ec" })), env(host))), [401, { error: "unknown Access signing key" }]);
    assert.deepEqual(await refusal(await verifyAccess(req(await token(host, {}, undefined, b64url("not json"))), env(host))), [401, { error: "malformed Access token" }]);
  });
});
