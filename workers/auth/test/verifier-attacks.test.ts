// The portable verifier (GO-135) against the attack corpus, and the bearer-vs-cookie rules.
import { test } from "node:test";
import assert from "node:assert/strict";
import { b64urlEncode } from "../functions/api/auth/_lib.ts";
import { LEEWAY_SECONDS, MAX_TOKEN, verifySession } from "../portable/session-verifier.ts";
import { KEYS, newKeyPair } from "./fakes.ts";
import { KEYSET, NOW, NOW_MS, PAGE, config, deps, keySet, part, sign, withBearer, withCookie } from "./verifier-fixtures.ts";

const reasonOf = async (req: Request, c = config()) => {
  const v = await verifySession(req, c, deps);
  return v.ok ? "ok" : v.reason;
};

test("verifySession: a good cookie token gives the user, perms, session and expiry", async () => {
  const v = await verifySession(withCookie(await sign()), config(), deps);
  assert.deepEqual(v, {
    ok: true,
    user: { id: "u1", email: "ada@example.com" },
    perms: ["demo:read"],
    sessionId: "s1",
    expiresAt: new Date((NOW + 600) * 1000).toISOString(),
    kind: "user",
  });
  const edge = await sign({ exp: NOW - LEEWAY_SECONDS + 1, iat: NOW + LEEWAY_SECONDS });
  assert.equal(await reasonOf(withCookie(edge)), "ok", "inside the leeway on both ends");
  assert.equal(await reasonOf(withCookie(await sign({ kind: "user" }))), "ok", "an explicit user kind");
  const far = await verifySession(withCookie(await sign({ exp: 1e300 })), config(), deps);
  assert.ok(far.ok && far.expiresAt === "+275760-09-13T00:00:00.000Z", "an absurd exp is clamped, never a RangeError");
});

test("verifySession: the default clock is Date.now", async () => {
  const live = await sign({ iat: Math.floor(Date.now() / 1000), exp: Math.floor(Date.now() / 1000) + 60 });
  assert.equal((await verifySession(withCookie(live), config())).ok, true);
});

test("verifySession: the attack corpus is refused, each for its reason", async () => {
  const good = await sign();
  const [h, p, s] = good.split(".");
  const kid = KEYSET.signing.kid;
  const hmacKey = await crypto.subtle.importKey("raw", new TextEncoder().encode(KEYS.pub), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  const hsUnsigned = `${part({ alg: "HS256", typ: "JWT", kid })}.${p}`;
  const hs = `${hsUnsigned}.${b64urlEncode(new Uint8Array(await crypto.subtle.sign("HMAC", hmacKey, new TextEncoder().encode(hsUnsigned))))}`;
  const stranger = await keySet((await newKeyPair()).priv);
  const strangerToken = await sign({}, stranger);
  const altered = part({ ...JSON.parse(Buffer.from(p, "base64url").toString()), perms: ["*"] });

  const corpus: [string, string, string][] = [
    ["alg none, empty signature", `${part({ alg: "none", typ: "JWT" })}.${p}.`, "bad_signature"],
    ["alg none with our kid", `${part({ alg: "none", typ: "JWT", kid })}.${p}.${s}`, "bad_signature"],
    ["alg NONE", `${part({ alg: "NONE", kid })}.${p}.${s}`, "bad_signature"],
    ["HS256 with the public key as the HMAC secret", hs, "bad_signature"],
    ["RS256 header", `${part({ alg: "RS256", kid })}.${p}.${s}`, "bad_signature"],
    ["crit header", `${part({ alg: "ES256", kid, crit: ["exp"] })}.${p}.${s}`, "bad_signature"],
    ["no kid", `${part({ alg: "ES256" })}.${p}.${s}`, "unknown_key"],
    ["numeric kid", `${part({ alg: "ES256", kid: 1 })}.${p}.${s}`, "unknown_key"],
    ["unknown kid", `${part({ alg: "ES256", kid: "someone-else" })}.${p}.${s}`, "unknown_key"],
    ["signed by another key", strangerToken, "unknown_key"],
    ["another key's signature under our kid", `${h}.${p}.${strangerToken.split(".")[2]}`, "bad_signature"],
    ["tampered payload", `${h}.${altered}.${s}`, "bad_signature"],
    ["short signature", `${h}.${p}.${s.slice(0, -4)}`, "bad_signature"],
    ["extra segment", `${good}.extra`, "malformed"],
    ["missing segment", `${h}.${p}`, "malformed"],
    ["one segment", h, "malformed"],
    ["empty payload", `${h}..${s}`, "malformed"],
    ["not base64url", `!!.${p}.${s}`, "malformed"],
    ["padded signature", `${h}.${p}.${s}=`, "malformed"],
    ["impossible base64 length", `${h}.${p}.${s}AAA`, "malformed"],
    ["header is an array", `${part([1])}.${p}.${s}`, "malformed"],
    ["header is not UTF-8", `${b64urlEncode(new Uint8Array([0xff, 0xfe]))}.${p}.${s}`, "malformed"],
    ["header is not JSON", `${b64urlEncode(new TextEncoder().encode("{oops"))}.${p}.${s}`, "malformed"],
    ["oversized token", `${h}.${part({ pad: "x".repeat(MAX_TOKEN) })}.${s}`, "malformed"],
    ["wrong audience", await sign({ aud: "evil.example.com" }), "wrong_audience"],
    ["array audience", await sign({ aud: ["example.com"] }), "wrong_audience"],
    ["wrong issuer", await sign({ iss: "https://evil.example.com/api/auth" }), "wrong_issuer"],
    ["expired", await sign({ exp: NOW - LEEWAY_SECONDS }), "expired"],
    ["issued in the future", await sign({ iat: NOW + LEEWAY_SECONDS + 1 }), "not_yet_valid"],
    ["newer contract version", await sign({ ver: 2 }), "unsupported_contract"],
    ["no contract version", await sign({ ver: undefined }), "unsupported_contract"],
    ["string contract version", await sign({ ver: "1" }), "unsupported_contract"],
    ["unknown kind", await sign({ kind: "robot" }), "unsupported_contract"],
    ["exp not a number", await sign({ exp: "never" }), "malformed"],
    ["iat missing", await sign({ iat: undefined }), "malformed"],
    ["sub not a string", await sign({ sub: 7 }), "malformed"],
    ["sid missing", await sign({ sid: undefined }), "malformed"],
    ["email null", await sign({ email: null }), "malformed"],
    ["perms a string", await sign({ perms: "owner" }), "malformed"],
    ["perms with a number", await sign({ perms: ["a", 1] }), "malformed"],
  ];
  for (const [name, token, reason] of corpus) assert.equal(await reasonOf(withCookie(token)), reason, name);
  assert.equal(await reasonOf(withCookie(good, PAGE.replace("example.com", "other.example.com"))), "wrong_audience", "audience defaults to the request host");
  assert.equal(await reasonOf(withCookie(good, "https://other.example.com/"), config({ audience: "example.com" })), "ok", "a configured audience");
});

test("channels: the cookie carries user tokens only, a bearer header service tokens only", async () => {
  const user = await sign();
  const service = await sign({ kind: "service", email: "", perms: ["demo:purge"] });
  assert.equal(await reasonOf(withCookie(user)), "ok");
  assert.equal(await reasonOf(withBearer(user)), "wrong_kind", "a user token copied out of its HttpOnly cookie");
  assert.equal(await reasonOf(withCookie(service)), "wrong_kind", "a service token never rides in a browser");
  const v = await verifySession(withBearer(service), config(), deps);
  assert.ok(v.ok && v.kind === "service" && v.perms[0] === "demo:purge");
  // A bearer header is used alone: a valid cookie next to a bad bearer does not rescue it.
  assert.equal(await reasonOf(withCookie(user, PAGE, { authorization: "Bearer junk" })), "malformed");
  assert.equal(await reasonOf(withCookie(user, PAGE, { authorization: "bearer" })), "no_session", "an empty bearer is no session");
  // Any other scheme (a consumer's Basic password mode) is not ours: the cookie is read.
  assert.equal(await reasonOf(withCookie(user, PAGE, { authorization: "Basic YWRtaW46eA==" })), "ok");
  assert.equal(await reasonOf(withCookie(user, PAGE, { authorization: "Bearertoken" })), "ok", "not the Bearer scheme");
});

test("cookies: absent, empty, look-alike names, the first of two", async () => {
  const good = await sign();
  const cookie = (c: string) => new Request(PAGE, { headers: { cookie: c } });
  assert.equal(await reasonOf(new Request(PAGE)), "no_session");
  assert.equal(await reasonOf(cookie("__Host-ssg-auth=")), "no_session");
  assert.equal(await reasonOf(cookie(`x__Host-ssg-auth=${good}`)), "no_session");
  assert.equal(await reasonOf(cookie(`__Host-ssg-auth-refresh=${good}`)), "no_session");
  assert.equal(await reasonOf(cookie(`__Host-ssg-auth=${good}`)), "ok");
  assert.equal(await reasonOf(cookie(`a=b;__Host-ssg-auth = ${good} ;c=d`)), "ok");
  assert.equal(await reasonOf(cookie(`__Host-ssg-auth=junk; __Host-ssg-auth=${good}`)), "malformed", "the first, as auth reads it");
});

test("unusable configuration: not_configured before any token is read", async () => {
  const good = withCookie(await sign());
  for (const c of [
    config({ configProblem: "DEMO_AUTH_ISSUER is not set" }),
    config({ issuer: "" }),
    config({ loginUrl: "" }),
    config({ publicKey: "" }),
    config({ jwksUrl: "/api/auth/jwks" }),
    config({ publicKey: "{oops" }),
    config({ publicKey: KEYS.priv }),
  ]) {
    assert.equal(await reasonOf(good, c), "not_configured", JSON.stringify(c).slice(0, 80));
  }
  assert.equal(await reasonOf(new Request(PAGE), config({ issuer: "" })), "not_configured", "even without a token");
  assert.equal(NOW_MS, NOW * 1000);
});
