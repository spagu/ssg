// GO-151, ID tokens: for Google, Microsoft and Apple every forged or foreign
// token is refused — wrong audience, wrong issuer, expired, issued in the
// future, an unknown key, the right key id with another key, alg "none", HS256
// with the provider's public key, no subject. Microsoft's per-tenant issuer;
// the JWKS cache, its refetch and its cool-down; the key checks.
import { test } from "node:test";
import assert from "node:assert/strict";
import { JWKS_COOLDOWN_MS, JWKS_TTL_MS, jwksCache, verifyIdToken } from "../functions/api/auth/_provider-oidc.ts";
import { CONSUMERS_TENANT, tenantAllows } from "../functions/api/auth/providers/_microsoft.ts";
import { Browser, useClock } from "./fakes.ts";
import { MS_TENANT, OIDC, OTHER_KEY } from "./providers-fake.ts";
import type { Pid, Tweak } from "./providers-fake.ts";
import { rsaKey, signJwt } from "./providers-jwt.ts";
import { errorOf, events, fakeProviders, providerEnv, providerUser, roundTrip } from "./go151.ts";

const PAT = { sub: "4711", email: "pat@example.com", verified: true };

async function attempt(p: Pid, tweak: Tweak, over = {}) {
  useClock();
  jwksCache.clear();
  const env = providerEnv(over);
  await providerUser(env, "pat@example.com", p, PAT.sub);
  const fake = fakeProviders();
  fake.tweak = tweak;
  const browser = new Browser();
  try {
    const { location } = await roundTrip(env, fake, browser, p, PAT);
    return { error: errorOf(location), signedIn: browser.jar.has("__Host-ssg-auth"), env, fake };
  } finally {
    fake.restore();
  }
}

const ATTACKS: [string, Tweak][] = [
  ["another audience", { claims: (c) => (c.aud = "someone-elses-client") }],
  ["several audiences, azp not ours", { claims: (c) => (c.aud = [c.aud, "other"]) }],
  ["another issuer", { claims: (c) => (c.iss = "https://evil.example.com") }],
  ["expired", { claims: (c) => (c.exp = (c.iat as number) - 3600) }],
  ["no exp", { claims: (c) => delete c.exp }],
  ["issued in the future", { claims: (c) => (c.iat = (c.iat as number) + 3600) }],
  ["no subject", { claims: (c) => (c.sub = "") }],
  ["no nonce", { claims: (c) => delete c.nonce }],
  ["an unknown key id", { header: { kid: "not-in-the-jwks" } }],
  ["no key id", { header: { kid: undefined } }],
  ["the right key id, another key", { signWith: OTHER_KEY }],
  ["a broken signature", { forge: "bad-signature" }],
  ['alg "none"', { forge: "none" }],
  ["HS256 with the public key as the secret", { forge: "hs256-public" }],
  ["RS512, not on the list", { header: { alg: "RS512" } }],
];

for (const p of OIDC) {
  for (const [name, tweak] of ATTACKS) {
    test(`${p}: an ID token with ${name} is refused`, async () => {
      const r = await attempt(p, tweak);
      assert.deepEqual([r.error, r.signedIn], ["provider_failed", false]);
      assert.ok(events(r.env, "signin_refused")[0].detail.cause, "the reason is recorded");
    });
  }
  test(`${p}: several audiences with azp = our client is accepted`, async () => {
    const r = await attempt(p, { claims: (c) => ((c.azp = c.aud), (c.aud = [c.aud, "other"])) });
    assert.deepEqual([r.error, r.signedIn], [null, true]);
  });
  test(`${p}: a JWKS without our key, an empty JWKS answer and a weak key are refused`, async () => {
    const weak = await rsaKey(p === "google" ? "g1" : p === "microsoft" ? "m1" : "a1", 1024);
    for (const jwks of [{ keys: [] }, { nope: 1 }, { keys: [{ ...weak.jwk }] }, { keys: [{ ...weak.jwk, kty: "EC" }] }]) {
      assert.equal((await attempt(p, { jwks })).error, "provider_failed");
    }
  });
}

test("microsoft: the issuer is the tid-templated one, and AUTH_MICROSOFT_TENANT restricts the tenant", async () => {
  const other = "99999999-8888-7777-6666-555555555555";
  assert.equal((await attempt("microsoft", { claims: (c) => (c.iss = "https://login.microsoftonline.com/common/v2.0") })).error, "provider_failed");
  assert.equal((await attempt("microsoft", { claims: (c) => (c.tid = other) })).error, "provider_failed", "iss names another tenant than tid");
  assert.equal((await attempt("microsoft", { claims: (c) => ((c.tid = other), (c.iss = `https://login.microsoftonline.com/${other}/v2.0`)) })).error, null, "common: any tenant");
  assert.equal((await attempt("microsoft", {}, { AUTH_MICROSOFT_TENANT: other })).error, "provider_failed", "a single tenant refuses the others");
  assert.equal((await attempt("microsoft", {}, { AUTH_MICROSOFT_TENANT: MS_TENANT.toUpperCase() })).error, null);
  assert.equal((await attempt("microsoft", { claims: (c) => (c.tid = "not-a-guid") })).error, "provider_failed");
  assert.equal(tenantAllows("consumers", CONSUMERS_TENANT), true);
  assert.equal(tenantAllows("consumers", MS_TENANT), false);
  assert.equal(tenantAllows("organizations", CONSUMERS_TENANT), false);
  assert.equal(tenantAllows("organizations", MS_TENANT), true);
  const r = await attempt("microsoft", {}, { AUTH_MICROSOFT_TENANT: other });
  assert.ok(r.fake.calls.some((c) => c.url === `https://login.microsoftonline.com/${other}/discovery/v2.0/keys`), "a single tenant reads its own JWKS");
});

test("JWKS: cached per isolate; an unknown kid refetches once, then not again within the cool-down; refetched after the TTL", async () => {
  const clock = useClock();
  jwksCache.clear();
  const key = await rsaKey("k1");
  const rotated = await rsaKey("k2");
  let served = [key.jwk];
  let fetches = 0;
  const real = globalThis.fetch;
  globalThis.fetch = (async () => (fetches++, new Response(JSON.stringify({ keys: served })))) as typeof fetch;
  const check = () => ({ jwksUrl: "https://keys.example.com/jwks", hosts: ["keys.example.com"], algs: ["RS256"], audience: "aud", nonce: "n", now: clock.now, issuer: () => true });
  const token = async (k: typeof key) => signJwt({}, { sub: "s", aud: "aud", nonce: "n", exp: Math.floor(clock.now / 1000) + 600 }, k);
  try {
    assert.equal((await verifyIdToken(await token(key), check())).ok, true);
    assert.equal((await verifyIdToken(await token(key), check())).ok, true);
    assert.equal(fetches, 1, "cached");
    served = [key.jwk, rotated.jwk];
    assert.equal((await verifyIdToken(await token(rotated), check())).ok, false, "a new kid right after a fetch: still the cached set");
    assert.equal(fetches, 1, "no refetch within the cool-down");
    clock.advance(JWKS_COOLDOWN_MS);
    assert.equal((await verifyIdToken(await token(rotated), check())).ok, true);
    assert.equal(fetches, 2, "after the cool-down an unknown kid refetches");
    const unknown = await rsaKey("k9");
    assert.equal((await verifyIdToken(await token(unknown), check())).ok, false);
    assert.equal(fetches, 2, "and invented kids within the cool-down do not");
    clock.advance(JWKS_TTL_MS);
    await verifyIdToken(await token(key), check());
    assert.equal(fetches, 3, "after the TTL");
    globalThis.fetch = (async () => new Response("nope", { status: 500 })) as typeof fetch;
    jwksCache.clear();
    assert.deepEqual(await verifyIdToken(await token(key), check()), { ok: false, detail: "ID token: keys.example.com/jwks: answer is not JSON (status 500)" });
    globalThis.fetch = (async () => new Response(JSON.stringify({ keys: [] }), { status: 404 })) as typeof fetch;
    assert.deepEqual(await verifyIdToken(await token(key), check()), { ok: false, detail: "ID token: JWKS: status 404, no key list" });
  } finally {
    globalThis.fetch = real;
  }
});

test("verifyIdToken: malformed tokens, and keys whose own alg or use disagree", async () => {
  const clock = useClock();
  jwksCache.clear();
  const key = await rsaKey("k1");
  const real = globalThis.fetch;
  let served: unknown[] = [key.jwk];
  globalThis.fetch = (async () => new Response(JSON.stringify({ keys: served }))) as typeof fetch;
  const check = { jwksUrl: "https://keys.example.com/a", hosts: ["keys.example.com"], algs: ["RS256"], audience: "aud", nonce: "n", now: clock.now, issuer: () => true };
  try {
    for (const bad of ["a.b", "!!.e30.x", "e30.!!.x", "W10.e30.", `${btoa('{"alg":"RS256"}')}.e30.AAAA`]) assert.equal((await verifyIdToken(bad, check)).ok, false, bad);
    const t = await signJwt({}, { sub: "s", aud: "aud", nonce: "n", exp: Math.floor(clock.now / 1000) + 60 }, key);
    for (const [i, k] of [{ ...key.jwk, alg: "RS512" }, { ...key.jwk, use: "enc" }, { ...key.jwk, n: "@@" }].entries()) {
      served = [k];
      jwksCache.clear();
      assert.equal((await verifyIdToken(t, { ...check, jwksUrl: `https://keys.example.com/${i}` })).ok, false);
    }
  } finally {
    globalThis.fetch = real;
  }
});
