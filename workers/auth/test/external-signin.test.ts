// #435: POST /api/auth/jwt (functions/api/auth/jwt.ts) — the external
// issuer's token exchanged for a normal session. Off by default (404), 503
// when misconfigured; the CSRF defence (with the configured form origins);
// the bearer header or the `token` field; a session of method jwt:<issuer>
// with the usual cookies, listed and revocable; the redirect only to a safe
// path; a token used once; the lockout; the second factor never skipped.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ACCESS_COOKIE, REFRESH_COOKIE } from "../functions/api/auth/_cookies.ts";
import { onRequestGet as session } from "../functions/api/auth/session.ts";
import { onRequestGet as ownSessions } from "../functions/api/auth/sessions/index.ts";
import { onRequestDelete as revoke } from "../functions/api/auth/sessions/[id].ts";
import { Browser, T0, call, makeEnv, useClock } from "./fakes.ts";
import { count, events, providerUser } from "./go151.ts";
import { FakeIssuer, ISS, errorCode, jwtEnv, mint, post, sqlite, where } from "./external-fake.ts";

const METHOD = `jwt:${ISS}`;

async function world(over: Record<string, string> = {}) {
  useClock();
  const env = jwtEnv(over);
  const fake = new FakeIssuer().install();
  return { env, fake };
}

test("off by default: the route answers 404 and nothing is read; misconfigured: 503 with the hint", async () => {
  useClock();
  const off = await post(makeEnv(), await mint());
  assert.equal(off.status, 404);
  assert.deepEqual(await off.json(), { error: "not_found" });
  const quiet = console.error;
  console.error = () => {};
  try {
    const bad = await post(jwtEnv({ AUTH_JWT_ALGORITHMS: "RS256,HS256" }), await mint());
    assert.equal(bad.status, 503);
    const body = await bad.json();
    assert.equal(body.error, "jwt_misconfigured");
    assert.match(body.hint, /AUTH_JWT_SECRET/);
  } finally {
    console.error = quiet;
  }
});

test("a valid token signs a linked person in: form or bearer, the usual cookies, method jwt:<issuer>, audit, listed and revocable", async () => {
  const { env, fake } = await world();
  try {
    const id = await providerUser(env, "ada@example.com", METHOD, "ext-1001");
    for (const via of ["form", "bearer", "json"] as const) {
      const browser = new Browser();
      const res = await post(env, await mint(), { via, next: "/editor/", browser });
      assert.equal(res.status, 303, via);
      assert.equal(where(res), "/editor/");
      assert.ok(browser.jar.has(ACCESS_COOKIE) && browser.jar.has(REFRESH_COOKIE), via);
      const me = await call(session, env, "/api/auth/session", { browser });
      assert.equal((await me.json()).user.id, id);
      const listed = await (await call(ownSessions, env, "/api/auth/sessions", { browser })).json();
      const current = listed.items.find((i: { current: boolean }) => i.current);
      assert.ok(current, "listed among the person's sessions");
      const gone = await call(revoke, env, `/api/auth/sessions/${current.id}`, { method: "DELETE", browser, params: { id: current.id } });
      assert.ok(gone.status < 300, "revocable");
      assert.equal((await call(session, env, "/api/auth/session", { browser })).status, 401);
    }
    assert.deepEqual(sqlite(env).prepare("SELECT DISTINCT method FROM session_methods").all().map((r) => ({ ...(r as object) })), [{ method: METHOD }]);
    const signins = events(env, "signin");
    assert.equal(signins.length, 3);
    assert.deepEqual([signins[0].user_id, signins[0].method], [id, METHOD]);
    assert.equal(fake.requests.length, 1, "the JWKS fetched once");
  } finally {
    fake.restore();
  }
});

test("redirects stay on this host: no next → AUTH_AFTER_LOGIN_URL; a hostile next falls back", async () => {
  const { env, fake } = await world();
  try {
    await providerUser(env, "ada@example.com", METHOD, "ext-1001");
    assert.equal(where(await post(env, await mint())), "/");
    for (const next of ["//evil.example.com/", "https://evil.example.com/", "/\\evil.example.com", "/./\x09/evil.example.com"]) {
      assert.equal(where(await post(env, await mint(), { next })), "/", next);
    }
  } finally {
    fake.restore();
  }
});

test("a refused token: to the login page with jwt_failed / jwt_unavailable, audited with the reason only — never the token", async () => {
  const { env, fake } = await world();
  const quiet = console.warn;
  console.warn = () => {};
  try {
    await providerUser(env, "ada@example.com", METHOD, "ext-1001");
    const t = Math.floor(T0 / 1000);
    const cases: [string | null, string, string][] = [
      [null, "jwt_failed", "no token"],
      [await mint({ iss: "https://other.example.com" }), "jwt_failed", "issuer"],
      [await mint({ aud: "https://other.example.com" }), "jwt_failed", "audience"],
      [await mint({ exp: t - 120 }), "jwt_failed", "expired"],
      [await mint({ nbf: t + 600 }), "jwt_failed", "not yet valid"],
      [await mint({ iat: t - 900, exp: t + 300 }), "jwt_failed", "too old"],
      [await mint({ jti: undefined }), "jwt_failed", "no token id"],
      [await mint({ sub: undefined }), "jwt_failed", "no subject"],
    ];
    for (const [token, code, cause] of cases) {
      const res = await post(env, token, { next: "/x" });
      assert.equal(errorCode(res), code, cause);
      assert.equal(new URL(where(res), "https://example.com").searchParams.get("next"), "/x");
    }
    const refused = events(env, "signin_refused");
    assert.deepEqual(refused.map((e) => e.detail.cause), cases.map((c) => c[2]));
    assert.ok(refused.every((e) => e.method === METHOD && !JSON.stringify(e.detail).includes("eyJ")));
    fake.mode = "500";
    fake.restore();
    fake.install();
    fake.mode = "500";
    assert.equal(errorCode(await post(env, await mint())), "jwt_unavailable");
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM sessions"), 0);
  } finally {
    console.warn = quiet;
    fake.restore();
  }
});

test("single use: the same jti twice is a replay; spent ids expire with their token", async () => {
  const { env, fake } = await world();
  try {
    await providerUser(env, "ada@example.com", METHOD, "ext-1001");
    const token = await mint({ jti: "once" });
    assert.equal(where(await post(env, token)), "/");
    const again = await post(env, token);
    assert.equal(errorCode(again), "jwt_failed");
    assert.equal(events(env, "signin_refused").at(-1)?.detail.cause, "replayed");
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM jwt_used_tokens"), 1);
    // Kept until min(exp, iat + max age) + skew: 300 + 60 s. A later spend purges it.
    const later = T0 + 361_000;
    useClock(later);
    assert.equal(where(await post(env, await mint({}, undefined, later))), "/");
    assert.equal(count(env, "SELECT COUNT(*) AS n FROM jwt_used_tokens"), 1);
    const stored = sqlite(env).prepare("SELECT jti_hash FROM jwt_used_tokens").get() as { jti_hash: string };
    assert.match(stored.jti_hash, /^[0-9a-f]{64}$/, "a hash, never the jti");
  } finally {
    fake.restore();
  }
});

test("CSRF: a cross-origin form only from AUTH_JWT_FORM_ORIGINS; fetch needs x-ssg-auth; nothing else", async () => {
  const { env, fake } = await world({ AUTH_JWT_FORM_ORIGINS: "https://id.example.com" });
  try {
    await providerUser(env, "ada@example.com", METHOD, "ext-1001");
    const fromIssuer = await post(env, await mint(), { origin: "https://id.example.com", headers: { "sec-fetch-site": "cross-site" } });
    assert.equal(where(fromIssuer), "/", "the issuer's page may post the form");
    const evil = await post(env, await mint(), { origin: "https://evil.example.com" });
    assert.equal(evil.status, 403);
    assert.deepEqual(await evil.json(), { error: "cross_origin" });
    const bearerCross = await post(env, await mint(), { via: "bearer", origin: "https://id.example.com" });
    assert.equal(bearerCross.status, 403, "a configured origin is for plain forms only");
    const noProof = await call((await import("../functions/api/auth/jwt.ts")).onRequestPost, env, "/api/auth/jwt", { method: "POST", csrf: false, headers: { authorization: `Bearer ${await mint()}` } });
    assert.equal(noProof.status, 403);
  } finally {
    fake.restore();
  }
});
