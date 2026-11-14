// GO-151, the corners: races between a check and its write, failures of the
// audit write and of hashing, odd provider answers, and the JSON forms of the
// routes the pages use as plain forms (methods-json.test.ts).
import { test } from "node:test";
import assert from "node:assert/strict";
import { emitEvent } from "../functions/api/auth/_audit.ts";
import { linkIdentity, removeIdentity } from "../functions/api/auth/_provider-identities.ts";
import { jwksCache, verifyIdToken } from "../functions/api/auth/_provider-oidc.ts";
import { providerFetch } from "../functions/api/auth/_provider-fetch.ts";
import { providerName } from "../functions/api/auth/_provider.ts";
import { sessionMethod } from "../functions/api/auth/_provider-policy.ts";
import { acceptWith } from "../functions/api/auth/_provider-signin.ts";
import { google } from "../functions/api/auth/providers/_google.ts";
import { Browser, call, makeEnv, useClock } from "./fakes.ts";
import { invite, tokenOf } from "./go134.ts";
import { loggedIn, roleId, signedIn } from "./seed.ts";
import { before, callbackGet, count, errorOf, fakeProviders, providerEnv, providerUser, roundTrip, startAt } from "./go151.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

test("audit: a failed write is logged by kind and the request goes on", async () => {
  const env = makeEnv();
  const logged: string[] = [];
  const real = console.error;
  console.error = (m: string) => logged.push(m);
  try {
    env.AUTH_DB.state.failOn = /auth_events/;
    await emitEvent(env, { kind: "signin" }, 0);
    assert.deepEqual(logged, ["auth: audit event signin not recorded"]);
  } finally {
    console.error = real;
  }
});

test("data layer: linking races and refusals; removing an unknown identity; a database error is not swallowed", async () => {
  useClock();
  const env = providerEnv();
  const ada = (await loggedIn(env)).id;
  const id = (sub: string) => ({ subject: sub, email: "", emailVerified: false, name: "" });
  assert.equal(await linkIdentity(env, ada, "github", id("1"), 0), "ok");
  assert.equal(await linkIdentity(env, ada, "github", id("1"), 0), "ok", "the same pair again");
  assert.equal(await linkIdentity(env, ada, "github", id("2"), 0), "provider_linked");
  const bea = (await loggedIn(env, "bea@example.com")).id;
  // Someone else links the pair between the check and the insert.
  before(env, /^INSERT INTO identities/, () => env.AUTH_DB.sqlite.exec(`INSERT INTO identities (id, provider, subject, user_id, linked_at) VALUES ('x', 'google', 'g', '${ada}', 't')`));
  assert.equal(await linkIdentity(env, bea, "google", id("g"), 0), "in_use");
  assert.equal(await removeIdentity(env, ada, "nope"), "not_found");
  env.AUTH_DB.state.failOn = /^DELETE FROM identities/;
  await assert.rejects(removeIdentity(env, ada, "x"), /forced failure/);
  env.AUTH_DB.state.failOn = /^INSERT INTO identities/;
  await assert.rejects(linkIdentity(env, bea, "apple", id("z"), 0), /forced failure/);
  env.AUTH_DB.state.failOn = null;
  env.AUTH_DB.sqlite.exec("DELETE FROM session_methods");
  assert.equal(await sessionMethod(env, "any"), "password", "a session from before GO-151");
  assert.equal(providerName("myspace"), "myspace");
});

test("ID tokens: a payload that is not JSON, a key without n or with a bad exponent, a numeric aud", async () => {
  const clock = useClock();
  jwksCache.clear();
  const { rsaKey, signJwt } = await import("./providers-jwt.ts");
  const key = await rsaKey("k");
  let served: unknown[] = [];
  const real = globalThis.fetch;
  globalThis.fetch = (async () => new Response(JSON.stringify({ keys: served }))) as typeof fetch;
  const check = (u: string) => ({ jwksUrl: `https://k.example.com/${u}`, hosts: ["k.example.com"], algs: ["RS256"], audience: "aud", nonce: "n", now: clock.now, issuer: () => true });
  try {
    assert.equal((await verifyIdToken("eyJhbGciOiJSUzI1NiJ9.bm90IGpzb24.AAAA", check("a"))).ok, false);
    const t = await signJwt({}, { sub: "s", aud: "aud", nonce: "n", exp: Math.floor(clock.now / 1000) + 60 }, key);
    served = [{ kty: "RSA", kid: "k", e: "AQAB" }];
    assert.deepEqual(await verifyIdToken(t, check("b")), { ok: false, detail: "ID token: RSA key shorter than 2048 bits" });
    served = [{ ...key.jwk, e: undefined }];
    assert.deepEqual(await verifyIdToken(t, check("c")), { ok: false, detail: "ID token: key cannot be imported" });
    served = [key.jwk];
    const odd = await signJwt({}, { sub: "s", aud: 7, nonce: "n", exp: Math.floor(clock.now / 1000) + 60 }, key);
    assert.deepEqual(await verifyIdToken(odd, check("d")), { ok: false, detail: "ID token audience is wrong" });
    assert.equal((await providerFetch("https://k.example.com/x", ["k.example.com"], { form: { a: "b" } })).ok, true, "a form without a method is a POST");
  } finally {
    globalThis.fetch = real;
  }
});

test("provider answers without an address, odd ids, a broken e-mail list", async () => {
  useClock();
  jwksCache.clear();
  const env = providerEnv();
  const fake = fakeProviders();
  try {
    for (const p of ["apple", "microsoft"] as const) {
      await providerUser(env, `${p}@example.com`, p, `s-${p}`);
      assert.equal(errorOf((await roundTrip(env, fake, new Browser(), p, { sub: `s-${p}` })).location), null, p);
    }
    await providerUser(env, "gh@example.com", "github", "13");
    fake.tweak = { api: { id: 13 } };
    assert.equal(errorOf((await roundTrip(env, fake, new Browser(), "github", { sub: "13" })).location), null, "no name, no login");
    for (const [p, api] of [["github", { id: "12" }], ["github", { id: -1 }], ["facebook", { id: "abc" }], ["facebook", { id: 12 }]] as const) {
      fake.tweak = { api };
      assert.equal(errorOf((await roundTrip(env, fake, new Browser(), p, { sub: "12" })).location), "provider_failed", `${p} ${JSON.stringify(api)}`);
    }
    fake.tweak = { emails: "broken" };
    assert.equal(errorOf((await roundTrip(env, fake, new Browser(), "github", { sub: "12" })).location), "provider_failed");
  } finally {
    fake.restore();
  }
});

test("flows: a role removed before the invitation is accepted, a session too large, two links at once, an invite flow without its hash", async () => {
  useClock();
  jwksCache.clear();
  const env = providerEnv();
  const fake = fakeProviders();
  try {
    const boss = await signedIn(env, "boss@example.com", "owner");
    const token = tokenOf((await invite(env, boss.browser, { email: "n@example.com", roleIds: [roleId(env as never, "newsletter-viewer")], delivery: "link" })).body.link);
    env.AUTH_DB.sqlite.exec("DELETE FROM roles WHERE name = 'newsletter-viewer'");
    assert.equal((await roundTrip(env, fake, new Browser(), "google", { sub: "n1" }, { purpose: "invite", token })).location, "/auth/?welcome=1&skipped=1");
    const huge = providerEnv({ AUTH_ISSUER: "https://example.com/" + "x".repeat(4000) } as Partial<Env>);
    await providerUser(huge, "big@example.com", "google", "big");
    const quiet = console.error;
    console.error = () => {};
    try {
      assert.equal(errorOf((await roundTrip(huge, fake, new Browser(), "google", { sub: "big" })).location), "permissions_too_large");
    } finally {
      console.error = quiet;
    }
    const { browser } = await loggedIn(env, "two@example.com");
    const a = await startAt(env, browser, "github", { purpose: "link" });
    const first = await fake.authorize(a.headers.get("location") as string, { sub: "501" });
    const handleA = browser.jar.get("__Host-ssg-auth-flow-github") as string;
    const b = await startAt(env, browser, "github", { purpose: "link" });
    const second = await fake.authorize(b.headers.get("location") as string, { sub: "502" });
    const handleB = browser.jar.get("__Host-ssg-auth-flow-github") as string;
    browser.jar.set("__Host-ssg-auth-flow-github", handleA);
    assert.equal((await call(callbackGet, env, first.path, { browser, csrf: false, params: { provider: "github" } })).headers.get("location"), "/auth/account?ok=method_linked");
    browser.jar.set("__Host-ssg-auth-flow-github", handleB);
    assert.equal(errorOf((await call(callbackGet, env, second.path, { browser, csrf: false, params: { provider: "github" } })).headers.get("location") as string), "provider_already_linked");
    const ctx = { request: new Request("https://example.com/"), env, keys: null, s: { loginUrl: "/auth/login" }, now: Date.now() } as never;
    const res = await acceptWith(ctx, google, { subject: "z", email: "", emailVerified: false, name: "" }, { purpose: "invite", invite_hash: null } as never);
    assert.equal(errorOf(res.headers.get("location") as string), "invitation_invalid");
  } finally {
    fake.restore();
  }
});

test("flows: an invitation withdrawn between its check and its acceptance", async () => {
  useClock();
  jwksCache.clear();
  const env = providerEnv();
  const fake = fakeProviders();
  try {
    const boss = await signedIn(env, "boss@example.com", "owner");
    const token = tokenOf((await invite(env, boss.browser, { email: "n@example.com", delivery: "link" })).body.link);
    const browser = new Browser();
    const s = await startAt(env, browser, "google", { purpose: "invite", token });
    const back = await fake.authorize(s.headers.get("location") as string, { sub: "w1" });
    before(env, /^SELECT id FROM roles WHERE id IN/, () => env.AUTH_DB.sqlite.exec("UPDATE invitations SET revoked_at = 'x'"));
    assert.equal(errorOf((await call(callbackGet, env, back.path, { browser, csrf: false, params: { provider: "google" } })).headers.get("location") as string), "invitation_invalid");
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM users WHERE email = 'n@example.com'"), 0);
  } finally {
    fake.restore();
  }
});
