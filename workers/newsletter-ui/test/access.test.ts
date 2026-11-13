// Cloudflare Access: a real RS256 key pair, the JWKS endpoint stubbed.
import { test, before, after } from "node:test";
import assert from "node:assert/strict";
import { requireAdmin } from "../src/auth.ts";
import { verifyAccess } from "../src/access.ts";
import { req } from "./fakes.ts";

const TEAM = "myteam";
const HOST = "myteam.cloudflareaccess.com";
const AUD = "aud-123";
const alg = { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" };
let good: CryptoKeyPair;
let other: CryptoKeyPair;
let jwksCalls = 0;
const realFetch = globalThis.fetch;

const b64url = (bytes: Uint8Array) => Buffer.from(bytes).toString("base64url");
const enc = (o: unknown) => b64url(new TextEncoder().encode(JSON.stringify(o)));

async function token(claims: Record<string, unknown> = {}, opts: { key?: CryptoKey; header?: Record<string, unknown> } = {}) {
  const head = enc({ alg: "RS256", kid: "k1", ...opts.header });
  const body = enc({ aud: [AUD], iss: `https://${HOST}`, exp: Math.floor(Date.now() / 1000) + 300, ...claims });
  const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", opts.key ?? good.privateKey, new TextEncoder().encode(head + "." + body));
  return `${head}.${body}.${b64url(new Uint8Array(sig))}`;
}

before(async () => {
  good = (await crypto.subtle.generateKey(alg, true, ["sign", "verify"])) as CryptoKeyPair;
  other = (await crypto.subtle.generateKey(alg, true, ["sign", "verify"])) as CryptoKeyPair;
  const jwk = await crypto.subtle.exportKey("jwk", good.publicKey);
  globalThis.fetch = (async (input: string) => {
    if (input === `https://${HOST}/cdn-cgi/access/certs`) {
      jwksCalls++;
      return Response.json({ keys: [{ kid: "ec", kty: "EC" }, { kid: "k1", kty: "RSA", n: jwk.n, e: jwk.e }] });
    }
    return new Response("down", { status: 500 });
  }) as typeof fetch;
});
after(() => {
  globalThis.fetch = realFetch;
});

const env = { NEWSLETTER_ACCESS_TEAM: TEAM, NEWSLETTER_ACCESS_AUD: AUD, NEWSLETTER_ADMIN_PASSWORD: "ignored" } as never;
const withJWT = (t: string) => req("/x", { headers: { "cf-access-jwt-assertion": t } });
const status = async (r: Request, e = env) => ((await requireAdmin(r, e)) as Response | null)?.status ?? 200;

test("Access: a valid token passes, from the header or the cookie, keys cached", async () => {
  assert.equal(await status(withJWT(await token())), 200);
  assert.equal(await status(req("/x", { headers: { cookie: `a=b; CF_Authorization=${await token({ aud: AUD })}` } })), 200);
  // The full hostname form of the team is the same host, so the cache holds.
  const full = { NEWSLETTER_ACCESS_TEAM: HOST, NEWSLETTER_ACCESS_AUD: AUD } as never;
  assert.equal(await status(withJWT(await token()), full), 200);
  assert.equal(jwksCalls, 1);
});

test("Access: Basic credentials do not get in when Access is configured", async () => {
  assert.equal(await status(req("/x", { auth: "ignored" })), 401);
});

test("Access: a token signed by another key is refused", async () => {
  assert.equal(await status(withJWT(await token({}, { key: other.privateKey }))), 401);
});

test("Access: wrong audience and wrong issuer are 403", async () => {
  assert.equal(await status(withJWT(await token({ aud: ["someone-else"] }))), 403);
  assert.equal(await status(withJWT(await token({ iss: "https://evil.example.com" }))), 403);
});

test("Access: an expired token, or one without exp, is refused", async () => {
  assert.equal(await status(withJWT(await token({ exp: Math.floor(Date.now() / 1000) - 10 }))), 401);
  assert.equal(await status(withJWT(await token({ exp: undefined }))), 401);
});

test("Access: malformed tokens, other algorithms and unknown keys are refused", async () => {
  assert.equal(await status(req("/x")), 401);
  assert.equal(await status(withJWT("a.b")), 401);
  assert.equal(await status(withJWT("!!.!!.!!")), 401);
  assert.equal(await status(withJWT(await token({}, { header: { alg: "HS256" } }))), 401);
  const res = (await verifyAccess(withJWT(await token({}, { header: { kid: "nope" } })), TEAM, AUD)) as Response;
  assert.equal(res.status, 401);
  assert.match((await res.json()).error, /unknown Access signing key/);
});

test("Access: an unreachable key endpoint is 503, not a pass", async () => {
  const res = (await verifyAccess(withJWT(await token()), "otherteam", AUD)) as Response;
  assert.equal(res.status, 503);
});
