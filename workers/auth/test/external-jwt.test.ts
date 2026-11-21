// #435: verifying the external issuer's token. Each algorithm on the list
// verifies with its own key family; every claim check refuses; `none`, an
// algorithm off the list, HS256 with the public key as its secret, `crit`
// and keys the token itself points at (jku / jwk) never get through.
import { test } from "node:test";
import assert from "node:assert/strict";
import { externalLookup } from "../functions/api/auth/_external-config.ts";
import type { ExternalSetup } from "../functions/api/auth/_external-config.ts";
import { MAX_TOKEN, verifyExternalToken } from "../functions/api/auth/_external-jwt.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { AUD, BASE_ENV, FakeIssuer, ISS, RS, SECRET, T0, claims, hmac, mint, sign, testKey } from "./external-fake.ts";
import type { TestKey } from "./external-fake.ts";

const setupOf = (v: Record<string, string>): ExternalSetup => (externalLookup(v as unknown as Env) as { setup: ExternalSetup }).setup;
const staticSetup = (key: TestKey | string, over: Record<string, string> = {}) =>
  setupOf({ AUTH_JWT_ISSUER: ISS, AUTH_JWT_AUDIENCE: AUD, AUTH_JWT_PUBLIC_KEY: typeof key === "string" ? key : JSON.stringify(key.jwk), ...over });
const reason = async (token: string, s: ExternalSetup, now = T0) => {
  const v = await verifyExternalToken(token, s, now);
  return v.ok ? "ok" : v.reason;
};

test("each asymmetric algorithm verifies with its key — JWK, JWK Set or PEM — and the claims come back", async () => {
  for (const alg of ["RS256", "PS256", "ES256", "ES384", "ES512", "EdDSA"]) {
    const key = await testKey(alg);
    const token = await mint({}, key);
    const v = await verifyExternalToken(token, staticSetup(key), T0);
    assert.ok(v.ok, `${alg}: ${v.ok ? "" : v.reason}`);
    assert.equal(v.claims.sub, "ext-1001");
    assert.equal(await reason(token, staticSetup(key.spkiPem)), "ok", `${alg} from PEM`);
    assert.equal(await reason(token, staticSetup(JSON.stringify({ keys: [key.jwk] }))), "ok", `${alg} from a JWK Set`);
  }
});

test("HS256/HS384/HS512 with the shared secret only", async () => {
  const s = setupOf({ AUTH_JWT_ISSUER: ISS, AUTH_JWT_AUDIENCE: AUD, AUTH_JWT_SECRET: SECRET, AUTH_JWT_ALGORITHMS: "HS256,HS384,HS512" });
  for (const alg of ["HS256", "HS384", "HS512"]) assert.equal(await reason(await hmac(claims(), SECRET, alg), s), "ok", alg);
  assert.equal(await reason(await hmac(claims(), "x".repeat(32)), s), "signature", "another secret");
  const only256 = setupOf({ AUTH_JWT_ISSUER: ISS, AUTH_JWT_AUDIENCE: AUD, AUTH_JWT_SECRET: SECRET, AUTH_JWT_ALGORITHMS: "HS256" });
  assert.equal(await reason(await hmac(claims(), SECRET, "HS512"), only256), "algorithm", "HS512 not on the list");
});

test("the algorithm attacks: none, off the list, HS256 with the public key, crit — refused before any key is used", async () => {
  const s = staticSetup(RS);
  assert.equal(await reason(await sign(claims(), RS, {}, "none"), s), "malformed", "alg none has no signature at all");
  assert.equal(await reason(`${(await sign(claims(), RS, {}, "none")).slice(0, -1)}.AAAA`, s), "algorithm", "alg none with a signature");
  assert.equal(await reason(await sign(claims(), RS, {}, "hs256-public"), s), "algorithm", "HS256 with the public key as secret");
  assert.equal(await reason(await mint({}, RS), staticSetup(RS, { AUTH_JWT_ALGORITHMS: "ES256" })), "algorithm", "RS256 off the list");
  assert.equal(await reason(await sign(claims(), RS, { crit: ["exp"] }), s), "algorithm");
  assert.equal(await reason(await sign(claims(), RS, { alg: 5 }), s), "algorithm");
});

test("the key comes from the configuration: jku and jwk headers ignored, a wrong key refused", async () => {
  const s = staticSetup(RS);
  const stranger = await testKey("RS256");
  // A token signed with the attacker's own key, pointing at it: still checked against the configured key.
  const forged = await sign(claims(), stranger, { kid: RS.kid, jku: "https://evil.example.com/jwks.json", jwk: stranger.jwk });
  assert.equal(await reason(forged, s), "signature");
  assert.equal(await reason(await sign(claims(), RS, {}, "bad-signature"), s), "signature");
  const es = await testKey("ES256");
  assert.equal(await reason(await sign(claims(), es, {}, "short-signature"), staticSetup(es)), "signature", "an ECDSA signature of the wrong length");
});

test("which configured key: by kid; a key without kid only when it is the only one; family, alg, use and size must fit", async () => {
  const es = await testKey("ES256");
  const set = (keys: unknown[]) => staticSetup(JSON.stringify({ keys }));
  assert.equal(await reason(await mint({}, RS), set([es.jwk, RS.jwk])), "ok");
  assert.equal(await reason(await sign(claims(), RS, { kid: "nope" }), set([es.jwk, RS.jwk])), "unknown key id");
  const { kid: _drop, ...bare } = RS.jwk;
  assert.equal(await reason(await sign(claims(), RS, { kid: undefined }), set([bare])), "ok", "no kid anywhere, one key");
  assert.equal(await reason(await sign(claims(), RS, { kid: "x" }), set([bare])), "ok", "a kid the only, kid-less key does not contradict");
  assert.equal(await reason(await sign(claims(), RS, { kid: undefined }), set([bare, es.jwk])), "unknown key id", "no kid, two keys");
  assert.equal(await reason(await mint({}, RS), set([{ ...RS.jwk, kty: "EC" }])), "key type does not fit the algorithm");
  assert.equal(await reason(await mint({}, es), set([{ ...es.jwk, crv: "P-384" }])), "key type does not fit the algorithm");
  assert.equal(await reason(await mint({}, RS), set([{ ...RS.jwk, alg: "PS256" }])), "key not meant for this algorithm");
  assert.equal(await reason(await mint({}, RS), set([{ ...RS.jwk, use: "enc" }])), "key not meant for this algorithm");
  assert.equal(await reason(await mint({}, RS), set([{ ...RS.jwk, n: "AQAB" }])), "RSA key shorter than 2048 bits");
  assert.equal(await reason(await mint({}, RS), set([{ ...RS.jwk, n: undefined }])), "RSA key shorter than 2048 bits", "no modulus");
  assert.equal(await reason(await mint({}, RS), set([{ ...RS.jwk, n: "!!" }])), "RSA key shorter than 2048 bits", "a modulus that is not base64url");
  assert.equal(await reason(await mint({}, es), set([{ ...es.jwk, x: "AAAA" }])), "key cannot be imported");
  const weak = await testKey("RS1024");
  assert.equal(await reason(await mint({}, weak), staticSetup(weak.spkiPem)), "RSA key shorter than 2048 bits", "a short PEM key");
  assert.equal(await reason(await mint({}, RS), staticSetup(es.spkiPem)), "key cannot be imported", "an EC PEM for an RSA token");
});

test("the claims: issuer, audience, expiry, not-before, issued-at with its age limit, token id — 60 s of skew", async () => {
  const s = staticSetup(RS);
  const t = Math.floor(T0 / 1000);
  const cases: [Record<string, unknown>, string][] = [
    [{ iss: "https://id.example.com/" }, "issuer"],
    [{ iss: undefined }, "issuer"],
    [{ aud: "https://other.example.com" }, "audience"],
    [{ aud: ["https://other.example.com", AUD] }, "ok"],
    [{ aud: [] }, "audience"],
    [{ exp: undefined }, "no expiry"],
    [{ exp: "9999999999" }, "no expiry"],
    [{ exp: t - 61 }, "expired"],
    [{ exp: t - 59 }, "ok"],
    [{ nbf: t + 61 }, "not yet valid"],
    [{ nbf: t + 59 }, "ok"],
    [{ nbf: "later" }, "not yet valid"],
    [{ nbf: undefined }, "ok"],
    [{ iat: undefined }, "no issued-at"],
    [{ iat: t + 61 }, "issued in the future"],
    [{ iat: t - 300 - 61, exp: t + 300 }, "too old"],
    [{ iat: t - 300 - 59, exp: t + 300 }, "ok"],
    [{ jti: undefined }, "no token id"],
    [{ jti: "" }, "no token id"],
    [{ jti: "x".repeat(256) }, "no token id"],
  ];
  for (const [over, want] of cases) assert.equal(await reason(await mint(over), s), want, JSON.stringify(over));
  assert.equal(await reason(await mint({ iat: t - 50 }), staticSetup(RS, { AUTH_JWT_MAX_AGE_SECONDS: "1" }), T0), "ok", "skew covers a short age limit");
});

test("malformed tokens: missing, not three parts, too long, not JSON, an array payload, an empty signature", async () => {
  const s = staticSetup(RS);
  assert.equal(await reason("", s), "no token");
  assert.equal(await reason("a.b", s), "malformed");
  assert.equal(await reason("a".repeat(MAX_TOKEN + 1), s), "malformed");
  assert.equal(await reason("!!!.e30.AAAA", s), "malformed");
  assert.equal(await reason("e30.W10.AAAA", s), "malformed", "payload []");
  assert.equal(await reason("e30.bm90IGpzb24.AAAA", s), "malformed", "payload not JSON");
  assert.equal(await reason(`${(await mint()).split(".").slice(0, 2).join(".")}.`, s), "malformed");
});

test("the JWKS: fetched, cached, an unknown kid refetched at most once a minute; down → unavailable", async () => {
  const fake = new FakeIssuer().install();
  try {
    const s = setupOf(BASE_ENV);
    assert.equal(await reason(await mint(), s), "ok");
    assert.equal(await reason(await mint(), s), "ok");
    assert.equal(fake.requests.length, 1, "cached");
    const rotated = await testKey("RS256", "rotated");
    fake.keys = [RS, rotated];
    assert.equal(await reason(await mint({}, rotated), s, T0 + 1000), "unknown key id", "inside the cool-down: not asked again");
    assert.equal(await reason(await mint({}, rotated, T0 + 61_000), s, T0 + 61_000), "ok", "after it: refetched");
    assert.equal(fake.requests.length, 2);
    for (const mode of ["500", "throw", "empty", "junk"] as const) {
      fake.restore();
      fake.install();
      fake.mode = mode;
      const quiet = console.warn;
      console.warn = () => {};
      try {
        const v = await verifyExternalToken(await mint(), s, T0);
        assert.deepEqual(v, { ok: false, reason: "keys unavailable", unavailable: true }, mode);
      } finally {
        console.warn = quiet;
      }
    }
  } finally {
    fake.restore();
  }
});
