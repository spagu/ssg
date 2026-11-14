// Sign-in: the three modes, their order, refusal when none or half of one is
// configured, and Access end to end with a real RSA key. Run through the
// manifest route, which needs no registry.
import { test } from "node:test";
import assert from "node:assert/strict";
import * as mod from "../functions/api/core/module/index.ts";
import { signIn } from "../functions/api/core/_signin.ts";
import { ACCESS_ENV, AUTH_ENV, AUTH_ISSUER, PASSWORD, PW_ENV, TEAM, accessIssuer, basic, call, fakeFetch, freshIP, session } from "./harness.ts";

const M = "/api/core/module";
const issuer = await accessIssuer([{ kid: "ec", kty: "EC", n: "", e: "" }]);
const get = (env: Record<string, unknown>, headers: Record<string, string> = {}) => call(mod.onRequest, { path: M, headers, env });

test("none configured → 503 signin_not_configured, never open", async () => {
  for (const env of [{}, { CORE_ADMIN_PASSWORD: "" }, { CORE_ACCESS_TEAM: " ", CORE_ACCESS_AUD: "" }]) {
    const r = await get(env, { authorization: basic(PASSWORD) });
    assert.equal(r.status, 503);
    assert.equal(r.json().error, "signin_not_configured");
    assert.match(r.json().detail, /CORE_ADMIN_PASSWORD/);
  }
});

test("Access configured by halves → 503 naming the missing variable, even with a password set", async () => {
  const a = await get({ CORE_ACCESS_TEAM: TEAM, ...PW_ENV }, { authorization: basic(PASSWORD) });
  assert.equal(a.status, 503);
  assert.equal(a.json().error, "access_misconfigured");
  assert.match(a.json().detail, /CORE_ACCESS_AUD is not set/);
  const b = await get({ CORE_ACCESS_AUD: "x" });
  assert.match(b.json().detail, /CORE_ACCESS_TEAM is not set/);
});

test("Access: a valid token (header or cookie) signs in with its address; Access wins over auth and password", async () => {
  const restore = issuer.install();
  try {
    const env = { ...ACCESS_ENV, ...PW_ENV, CORE_AUTH_ISSUER: "x" };
    const token = await issuer.token();
    assert.equal((await get(env, { "cf-access-jwt-assertion": token })).status, 200);
    assert.equal((await get(env, { cookie: `a=b; CF_Authorization=${token}` })).status, 200);
    const got = await signIn(new Request("https://example.com/", { headers: { "cf-access-jwt-assertion": token } }), env);
    assert.deepEqual(got, { ok: true, caller: { mode: "access", user: { email: "ada@example.com" }, perms: null } });
    const anon = await signIn(new Request("https://example.com/", { headers: { "cf-access-jwt-assertion": await issuer.token({ email: 7 }) } }), env);
    assert.deepEqual(anon.ok && anon.caller.user, null);
    // The password is not a way around Access.
    assert.equal((await get(env, { authorization: basic(PASSWORD) })).json().error, "access_required");
    assert.equal(issuer.certsCalls(), 1, "the JWKS is fetched once per isolate and cached");
  } finally {
    restore();
  }
});

test("Access: every bad token is refused with its own reason", async () => {
  const restore = issuer.install();
  try {
    const [h, p] = (await issuer.token()).split(".");
    const cases: [string, number, string][] = [
      ["a.b", 401, "access_token_malformed"],
      ["%%%.e30.x", 401, "access_token_malformed"],
      [await issuer.token({}, { alg: "none", kid: "k1" }), 401, "access_token_algorithm"],
      [await issuer.token({}, { alg: "RS256", kid: "nope" }), 401, "access_key_unknown"],
      [await issuer.token({}, { alg: "RS256", kid: "ec" }), 401, "access_key_unknown"],
      [`${h}.${p}.AAAA`, 401, "access_signature_invalid"],
      [await issuer.token({ aud: ["other"] }), 403, "access_audience_wrong"],
      [await issuer.token({ aud: "aud-core", iss: "https://evil.example.com" }), 403, "access_issuer_wrong"],
      [await issuer.token({ exp: 1 }), 401, "access_token_expired"],
      [await issuer.token({ exp: "never" }), 401, "access_token_expired"],
    ];
    for (const [token, status, error] of cases) {
      const r = await get(ACCESS_ENV, { "cf-access-jwt-assertion": token });
      assert.deepEqual([r.status, r.json().error], [status, error], token);
    }
  } finally {
    restore();
  }
});

test("Access: a JWKS that cannot be fetched → 503; a team given as a hostname is used as is", async () => {
  const { sent, restore } = fakeFetch({ "https://down.example.com/cdn-cgi/access/certs": new Response("no", { status: 500 }) });
  try {
    const token = await issuer.token();
    for (const team of ["down.example.com", "gone-team"]) {
      const r = await get({ CORE_ACCESS_TEAM: team, CORE_ACCESS_AUD: "aud-core" }, { "cf-access-jwt-assertion": token });
      assert.deepEqual([r.status, r.json().error], [503, "access_unreachable"]);
    }
    assert.deepEqual(sent.map((s) => s.url), ["https://down.example.com/cdn-cgi/access/certs", "https://gone-team.cloudflareaccess.com/cdn-cgi/access/certs"]);
  } finally {
    restore();
  }
});

test("auth: half configured → 503 not_configured naming what is missing, before the password is considered", async () => {
  const cases: [Record<string, string>, RegExp][] = [
    [{ CORE_AUTH_LOGIN_URL: "/auth/login" }, /CORE_AUTH_ISSUER is not set; set exactly one of CORE_AUTH_PUBLIC_KEY and CORE_AUTH_JWKS_URL/],
    [{ CORE_AUTH_ISSUER: AUTH_ISSUER, CORE_AUTH_PUBLIC_KEY: "k" }, /CORE_AUTH_LOGIN_URL is not set/],
    [{ ...AUTH_ENV, CORE_AUTH_JWKS_URL: "/api/auth/jwks" }, /set exactly one of/],
    [{ CORE_AUTH_AUDIENCE: "example.com" }, /CORE_AUTH_ISSUER is not set/],
  ];
  for (const [vars, detail] of cases) {
    const r = await get({ ...vars, ...PW_ENV }, { authorization: basic(PASSWORD), cookie: await session(["*"]) });
    assert.deepEqual([r.status, r.json().error], [503, "not_configured"]);
    assert.match(r.json().detail, detail);
  }
});

test("auth: a valid session signs in with who and what they hold; auth wins over the password", async () => {
  const cookie = await session(["comments:read"]);
  const ok = await signIn(new Request("https://example.com/", { headers: { cookie } }), { ...AUTH_ENV, ...PW_ENV });
  assert.deepEqual(ok, { ok: true, caller: { mode: "auth", user: { id: "u1", email: "ada@example.com" }, perms: ["comments:read"] } });
  assert.equal((await get(AUTH_ENV, { cookie })).status, 200);
});

test("auth: no, expired or forged session → the contract's answers (401 JSON for the API, 302 for a page)", async () => {
  const none = await get({ ...AUTH_ENV, ...PW_ENV }, { authorization: basic(PASSWORD) });
  assert.equal(none.status, 401);
  assert.deepEqual(none.json(), { error: "unauthorized", reason: "no_session" });
  assert.equal(none.headers["x-robots-tag"], "noindex, nofollow", "the contract's answer still carries core's headers");
  const old = await get(AUTH_ENV, { cookie: await session(["*"], { exp: 1000, iat: 900 }) });
  assert.deepEqual([old.status, old.json().reason], [401, "expired"]);
  const forged = await get(AUTH_ENV, { cookie: await session(["*"], { iss: "https://evil.example.com" }) });
  assert.deepEqual([forged.status, forged.json().reason], [401, "invalid_session"]);
  const page = await get(AUTH_ENV, { accept: "text/html" });
  assert.equal(page.status, 302);
  assert.equal(page.headers.location, "https://example.com/auth/login?next=%2Fapi%2Fcore%2Fmodule");
});

test("password: no credential, a wrong one, a malformed one → 401 with a Basic challenge; the right one → 200", async () => {
  const ip = freshIP();
  const headers = (h: Record<string, string>) => ({ "cf-connecting-ip": ip, ...h });
  for (const h of [{}, { authorization: basic("nope") }, { authorization: "Bearer x" }, { authorization: "Basic %%%" }]) {
    const r = await get(PW_ENV, headers(h));
    assert.equal(r.status, 401);
    assert.equal(r.headers["www-authenticate"], 'Basic realm="core-admin", charset="UTF-8"');
  }
  const ok = await signIn(new Request("https://example.com/", { headers: headers({ authorization: basic(PASSWORD) }) }), PW_ENV);
  assert.deepEqual(ok, { ok: true, caller: { mode: "password", user: null, perms: null } });
});
