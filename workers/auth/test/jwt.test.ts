// Signing keys, the JWKS, and the token verifier — including the attack corpus
// and the key rotation procedure from the README.
import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { loadKeys, thumbprint } from "../functions/api/auth/_keys.ts";
import type { KeySet } from "../functions/api/auth/_keys.ts";
import { CONTRACT_VERSION, LEEWAY_SECONDS, signToken, verifyToken } from "../functions/api/auth/_jwt.ts";
import type { AccessClaims } from "../functions/api/auth/_jwt.ts";
import { b64urlEncode } from "../functions/api/auth/_lib.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { KEYS, newKeyPair } from "./fakes.ts";

const NOW = 1_800_000_000;
const claims = (over: Partial<AccessClaims> = {}): AccessClaims => ({
  iss: "https://example.com/api/auth",
  aud: "example.com",
  sub: "u1",
  sid: "s1",
  email: "ada@example.com",
  iat: NOW,
  exp: NOW + 600,
  ver: CONTRACT_VERSION,
  perms: [],
  ...over,
});
const opts = (keys: KeySet) => ({ keys: keys.verify, issuer: "https://example.com/api/auth", audience: "example.com", nowSeconds: NOW });
const keysOf = async (env: Partial<Env>) => (await loadKeys(env as Env)) as KeySet;
const part = (v: unknown) => b64urlEncode(new TextEncoder().encode(JSON.stringify(v)));

test("loadKeys: a usable key, its thumbprint id, public members only in the JWKS", async () => {
  const keys = await keysOf({ AUTH_SIGNING_KEY: KEYS.priv });
  const { x, y } = JSON.parse(KEYS.pub);
  const want = createHash("sha256").update(JSON.stringify({ crv: "P-256", kty: "EC", x, y })).digest("base64url");
  assert.equal(keys.signing.kid, want, "the RFC 7638 thumbprint, as scripts/auth-keys.mjs prints it");
  assert.equal(await thumbprint(x, y), want);
  assert.deepEqual(keys.jwks, [{ kty: "EC", crv: "P-256", x, y, kid: want, alg: "ES256", use: "sig" }]);
  assert.doesNotMatch(JSON.stringify(keys.jwks), new RegExp(KEYS.d));
  assert.equal(loadKeys({ AUTH_SIGNING_KEY: KEYS.priv } as Env), loadKeys({ AUTH_SIGNING_KEY: KEYS.priv } as Env), "cached per configuration");
});

test("loadKeys: missing, malformed or mismatched keys give null — never a generated key", async () => {
  const other = await newKeyPair();
  const mixed = JSON.stringify({ ...JSON.parse(KEYS.priv), d: other.d });
  for (const raw of [
    undefined,
    "",
    "not json",
    "null",
    JSON.stringify({ kty: "RSA", crv: "P-256", x: "a", y: "b", d: "c" }),
    JSON.stringify({ kty: "EC", crv: "P-384", x: "a", y: "b", d: "c" }),
    KEYS.pub,
    JSON.stringify({ ...JSON.parse(KEYS.priv), d: "!!" }),
    mixed,
  ]) {
    assert.equal(await loadKeys({ AUTH_SIGNING_KEY: raw } as Env), null, String(raw));
  }
});

test("loadKeys: a private key that does not sign for its public half is refused", async () => {
  // Node already refuses such a JWK at import; a runtime that accepts it is
  // caught by the sign-and-verify probe, simulated here.
  const subtle = crypto.subtle as { verify: SubtleCrypto["verify"] };
  const real = subtle.verify;
  subtle.verify = async () => false;
  try {
    assert.equal(await loadKeys({ AUTH_SIGNING_KEY: KEYS.priv, AUTH_PREVIOUS_PUBLIC_KEY: "probe" } as Env), null);
  } finally {
    subtle.verify = real;
  }
});

test("loadKeys: the previous public key is listed; malformed ignored, broken fails closed, duplicate once", async () => {
  const old = await newKeyPair();
  const both = await keysOf({ AUTH_SIGNING_KEY: KEYS.priv, AUTH_PREVIOUS_PUBLIC_KEY: old.pub });
  assert.equal(both.jwks.length, 2);
  assert.equal(both.verify.size, 2);
  assert.equal(both.jwks[0].kid, both.signing.kid, "current key first");
  assert.equal((await keysOf({ AUTH_SIGNING_KEY: KEYS.priv, AUTH_PREVIOUS_PUBLIC_KEY: "{oops" })).jwks.length, 1);
  assert.equal((await keysOf({ AUTH_SIGNING_KEY: KEYS.priv, AUTH_PREVIOUS_PUBLIC_KEY: KEYS.pub })).jwks.length, 1);
  const offCurve = JSON.stringify({ kty: "EC", crv: "P-256", x: b64urlEncode(new Uint8Array(32).fill(1)), y: b64urlEncode(new Uint8Array(32).fill(2)) });
  assert.equal(await loadKeys({ AUTH_SIGNING_KEY: KEYS.priv, AUTH_PREVIOUS_PUBLIC_KEY: offCurve } as Env), null);
});

test("signToken / verifyToken: a good token, with clock leeway", async () => {
  const keys = await keysOf({ AUTH_SIGNING_KEY: KEYS.priv });
  const t = await signToken(keys.signing, claims());
  const header = JSON.parse(atob(t.split(".")[0].replaceAll("-", "+").replaceAll("_", "/")));
  assert.deepEqual(header, { alg: "ES256", typ: "JWT", kid: keys.signing.kid });
  const v = await verifyToken(t, opts(keys));
  assert.ok(v.ok);
  assert.deepEqual(v.ok && v.claims, claims());
  const edge = await signToken(keys.signing, claims({ exp: NOW - LEEWAY_SECONDS + 1, iat: NOW + LEEWAY_SECONDS }));
  assert.ok((await verifyToken(edge, opts(keys))).ok, "inside the leeway on both ends");
});

test("verifyToken: the attack corpus is refused, each for its reason", async () => {
  const keys = await keysOf({ AUTH_SIGNING_KEY: KEYS.priv });
  const good = await signToken(keys.signing, claims());
  const [h, p, s] = good.split(".");
  const kid = keys.signing.kid;
  const sign = (c: Partial<AccessClaims> | Record<string, unknown>) => signToken(keys.signing, { ...claims(), ...c } as AccessClaims);

  // HS256 with the PUBLIC key as the HMAC secret — the classic confusion.
  const hmacKey = await crypto.subtle.importKey("raw", new TextEncoder().encode(KEYS.pub), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  const hsUnsigned = `${part({ alg: "HS256", typ: "JWT", kid })}.${p}`;
  const hs = `${hsUnsigned}.${b64urlEncode(new Uint8Array(await crypto.subtle.sign("HMAC", hmacKey, new TextEncoder().encode(hsUnsigned))))}`;
  const stranger = await newKeyPair();
  const strangerKeys = await keysOf({ AUTH_SIGNING_KEY: stranger.priv });
  const altered = part({ ...claims(), sub: "admin" });

  const corpus: [string, string][] = [
    [`${part({ alg: "none", typ: "JWT" })}.${p}.`, "algorithm"],
    [`${part({ alg: "none", typ: "JWT", kid })}.${p}.${s}`, "algorithm"],
    [`${part({ alg: "NONE", kid })}.${p}.${s}`, "algorithm"],
    [hs, "algorithm"],
    [`${part({ alg: "RS256", kid })}.${p}.${s}`, "algorithm"],
    [`${part({ alg: "ES256", kid, crit: ["exp"] })}.${p}.${s}`, "algorithm"],
    [`${part({ alg: "ES256" })}.${p}.${s}`, "unknown_key"],
    [`${part({ alg: "ES256", kid: "someone-else" })}.${p}.${s}`, "unknown_key"],
    [await signToken(strangerKeys.signing, claims()), "unknown_key"],
    [`${part({ alg: "ES256", typ: "JWT", kid: keys.signing.kid })}.${p}.${(await signToken(strangerKeys.signing, claims())).split(".")[2]}`, "signature"],
    [`${h}.${altered}.${s}`, "signature"],
    [`${h}.${p}.${s.slice(0, -4)}`, "signature"],
    [`${h}.${p}.${s}.extra`, "malformed"],
    [`${h}.${p}`, "malformed"],
    [`${h}..${s}`, "malformed"],
    [`!!.${p}.${s}`, "malformed"],
    [`${part([1])}.${p}.${s}`, "malformed"],
    [`${b64urlEncode(new Uint8Array([0xff, 0xfe]))}.${p}.${s}`, "malformed"],
    [`${h}.${p}.${s}+`, "malformed"],
    [await sign({ aud: "evil.example.com" }), "audience"],
    [await sign({ aud: ["example.com"] } as never), "audience"],
    [await sign({ iss: "https://evil.example.com/api/auth" }), "issuer"],
    [await sign({ exp: NOW - LEEWAY_SECONDS }), "expired"],
    [await sign({ iat: NOW + LEEWAY_SECONDS + 1 }), "future"],
    [await sign({ ver: 2 }), "version"],
    [await sign({ ver: undefined } as never), "version"],
    [await sign({ exp: "never" } as never), "claims"],
    [await sign({ iat: undefined } as never), "claims"],
    [await sign({ sub: 7 } as never), "claims"],
    [await sign({ sid: undefined } as never), "claims"],
    [await sign({ email: null } as never), "claims"],
    [await sign({ perms: "owner" } as never), "claims"],
    [await sign({ perms: ["a", 1] } as never), "claims"],
  ];
  for (const [token, reason] of corpus) {
    const v = await verifyToken(token, opts(keys));
    assert.deepEqual(v, { ok: false, reason }, token.slice(0, 60));
  }
});

test("key rotation: an old-key token verifies while the old public key is listed, not after", async () => {
  const old = await newKeyPair();
  const before = await keysOf({ AUTH_SIGNING_KEY: old.priv });
  const oldToken = await signToken(before.signing, claims());
  // Step 1: new key signs, old public key listed.
  const during = await keysOf({ AUTH_SIGNING_KEY: KEYS.priv, AUTH_PREVIOUS_PUBLIC_KEY: old.pub });
  assert.ok((await verifyToken(oldToken, opts(during))).ok);
  assert.ok((await verifyToken(await signToken(during.signing, claims()), opts(during))).ok);
  assert.notEqual(during.signing.kid, before.signing.kid);
  // Step 2: after the token lifetime, the old public key is removed.
  const after = await keysOf({ AUTH_SIGNING_KEY: KEYS.priv });
  assert.deepEqual(await verifyToken(oldToken, opts(after)), { ok: false, reason: "unknown_key" });
});
