// GO-137: the third admin mode. The order Access → auth → password, a
// half-configured set (503, never a weaker mode), auth AND a password (the
// password is not accepted), the answers of the verifier through the pipeline,
// /api/me in every mode, the cross-origin rule, health.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { authenticate, modeOf, requireAdmin } from "../src/auth.ts";
import { AUTH_VARS, ISSUER, LOGIN, authEnv, session, token } from "./auth-fixtures.ts";
import { PASSWORD, fakeCtx, makeEnv, req } from "./fakes.ts";

const B = "/newsletter-admin";
const run = (r: Request, env: Record<string, unknown>) => handleFetch(r, env as never, fakeCtx() as never);

test("modeOf: Access → auth → password; Basic's lockout variables do not switch auth on", () => {
  assert.equal(modeOf(makeEnv() as never), "password");
  assert.equal(modeOf(makeEnv({ NEWSLETTER_AUTH_MAX_FAILURES: "3", NEWSLETTER_AUTH_WINDOW_MINUTES: "5" }) as never), "password");
  assert.equal(modeOf(authEnv() as never), "auth");
  assert.equal(modeOf(authEnv({ NEWSLETTER_ACCESS_TEAM: "t", NEWSLETTER_ACCESS_AUD: "a" }) as never), "access");
  assert.equal(modeOf(makeEnv({ NEWSLETTER_AUTH_LOGIN_URL: LOGIN }) as never), "auth", "one variable is enough to turn it on");
});

test("half-configured auth is a loud 503 naming what is missing — never the password", async () => {
  const cases: Record<string, unknown>[] = [
    { NEWSLETTER_AUTH_ISSUER: ISSUER },
    { NEWSLETTER_AUTH_LOGIN_URL: LOGIN },
    { ...AUTH_VARS, NEWSLETTER_AUTH_PUBLIC_KEY: undefined },
    { ...AUTH_VARS, NEWSLETTER_AUTH_JWKS_URL: "/api/auth/jwks" }, // both key sources
    { ...AUTH_VARS, NEWSLETTER_AUTH_PUBLIC_KEY: "{not json" }, // unusable inline key (seen once a token arrives)
  ];
  for (const vars of cases) {
    const env = makeEnv(vars); // the password is set too: it must not rescue anything
    const r = await session(B + "/api/health", ["*"], { headers: { authorization: `Basic ${btoa("admin:" + PASSWORD)}` } });
    const res = await run(r, env);
    assert.equal(res.status, 503, JSON.stringify(vars));
    const body = await res.json();
    assert.equal(body.error, "not_configured");
    assert.equal(typeof body.detail, "string");
    assert.equal(res.headers.get("x-frame-options"), "DENY", "the panel's security headers");
  }
});

test("auth AND a password: the password is not accepted; the session is", async () => {
  const env = authEnv({ NEWSLETTER_ADMIN_PASSWORD: PASSWORD });
  const basic = await run(req(B + "/api/health", { auth: PASSWORD }), env);
  assert.equal(basic.status, 401);
  assert.deepEqual(await basic.json(), { error: "unauthorized", reason: "no_session" });
  assert.equal(basic.headers.get("www-authenticate"), 'Bearer realm="ssg-auth"', "no Basic prompt");
  assert.equal(env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM auth_attempts").get()!.n, 0, "no password was even compared");
  assert.equal((await run(await session(B + "/api/health", ["newsletter:read"]), env)).status, 200);
});

test("Access configured too: Access decides, as before (auth is the second mode)", async () => {
  const env = authEnv({ NEWSLETTER_ACCESS_TEAM: "team", NEWSLETTER_ACCESS_AUD: "aud" });
  const res = await run(await session(B + "/api/health", ["*"]), env);
  assert.equal(res.status, 401);
  assert.deepEqual(await res.json(), { error: "Cloudflare Access required" });
});

test("no session: an API call is 401 JSON, a page is sent to the login page with next", async () => {
  const env = authEnv();
  const api = await run(req(B + "/api/campaigns"), env);
  assert.equal(api.status, 401);
  assert.equal(api.headers.get("cache-control"), "no-store");
  const page = await run(req(B + "/?x=1", { headers: { accept: "text/html" } }), env);
  assert.equal(page.status, 302);
  assert.equal(page.headers.get("location"), `https://example.com${LOGIN}?next=${encodeURIComponent(B + "/?x=1")}`);
  assert.equal(page.headers.get("content-security-policy")?.startsWith("default-src 'none'"), true);
});

test("a bad or expired token is refused with the contract's coarse reasons", async () => {
  const env = authEnv();
  const [h, p, sig] = (await token(["*"])).split(".");
  const forged = `${h}.${p}.${sig.slice(0, 10)}${sig[10] === "A" ? "B" : "A"}${sig.slice(11)}`; // one signature byte changed
  const cases: [string, string][] = [
    [forged, "invalid_session"],
    [await token(["*"], { exp: Math.floor(Date.now() / 1000) - 120 }), "expired"],
    [await token(["*"], { iss: "https://example.org/api/auth" }), "invalid_session"],
    [await token(["*"], { aud: "example.org" }), "invalid_session"],
  ];
  for (const [t, reason] of cases) {
    const res = await run(req(B + "/api/health", { headers: { cookie: `__Host-ssg-auth=${t}` } }), env);
    assert.equal(res.status, 401, reason);
    assert.equal((await res.json()).reason, reason);
  }
});

test("keys from a JWKS URL that cannot be fetched: 503 auth_unavailable, never open", async () => {
  const realFetch = globalThis.fetch;
  globalThis.fetch = (async () => new Response("down", { status: 500 })) as typeof fetch;
  try {
    const env = authEnv({ NEWSLETTER_AUTH_PUBLIC_KEY: undefined, NEWSLETTER_AUTH_JWKS_URL: "/api/auth/jwks" });
    const res = await run(await session(B + "/api/health", ["*"]), env);
    assert.equal(res.status, 503);
    assert.deepEqual(await res.json(), { error: "auth_unavailable" });
    assert.equal(res.headers.get("retry-after"), "60");
  } finally {
    globalThis.fetch = realFetch;
  }
});

test("/api/me in each mode", async () => {
  const pw = await run(req(B + "/api/me", { auth: PASSWORD }), makeEnv());
  assert.deepEqual(await pw.json(), { mode: "password", user: null, permissions: "all" });
  const editor = await run(await session(B + "/api/me", ["newsletter:read", "newsletter:edit", "comments:moderate"]), authEnv());
  assert.deepEqual(await editor.json(), {
    mode: "auth",
    user: { id: "u-ada", email: "ada@example.com" },
    permissions: ["newsletter:read", "newsletter:edit"],
    loginUrl: LOGIN,
    logoutUrl: "/api/auth/logout",
    refreshUrl: "/api/auth/refresh",
  });
  const wild = await (await run(await session(B + "/api/me", ["newsletter:*"]), authEnv())).json();
  assert.equal(wild.permissions.length, 8, "a wildcard expands to every declared permission");
  const none = await run(await session(B + "/api/me", []), authEnv());
  assert.equal(none.status, 200, "a session alone is enough");
  assert.deepEqual((await none.json()).permissions, []);
});

test("/api/me with Cloudflare Access: everything, nobody named", async () => {
  const { me } = await import("../src/me.ts");
  const res = me({ env: makeEnv(), admin: { mode: "access", user: null, perms: "all" } } as never);
  assert.deepEqual(await res.json(), { mode: "access", user: null, permissions: "all" });
});

test("CSRF in auth mode: the header rule stays, and a cross-origin request is refused", async () => {
  const env = authEnv();
  const perms = ["newsletter:edit"];
  const body = JSON.stringify({ subject: "Hello" });
  const post = (headers: Record<string, string>, csrf = true) => session(B + "/api/campaigns", perms, { method: "POST", body, csrf, headers: { "content-type": "application/json", ...headers } });
  assert.equal((await run(await post({}, false), env)).status, 403, "no x-newsletter-admin");
  for (const headers of [{ origin: "https://evil.example" }, { origin: "null" }, { "sec-fetch-site": "cross-site" }, { "sec-fetch-site": "same-site" }]) {
    const res = await run(await post(headers), env);
    assert.equal(res.status, 403, JSON.stringify(headers));
    assert.deepEqual(await res.json(), { error: "cross-origin request refused" });
  }
  assert.equal((await run(await post({ "sec-fetch-site": "same-origin" }), env)).status, 201);
  // A script with a bearer token sends neither header; only a service token rides there.
  const script = req(B + "/api/campaigns", { method: "POST", body, headers: { authorization: `Bearer ${await token(perms, { kind: "service" })}` } });
  assert.equal((await run(script, env)).status, 201);
});

test("CSRF in password mode: unchanged — a foreign Origin is still only the header's business", async () => {
  const res = await run(req(B + "/api/campaigns", { method: "POST", auth: PASSWORD, body: "{}", headers: { origin: "https://evil.example" } }), makeEnv());
  assert.notEqual(res.status, 403);
});

test("health names the auth mode", async () => {
  const body = await (await run(await session(B + "/api/health", ["newsletter:read"]), authEnv())).json();
  assert.equal(body.auth, "auth");
  assert.doesNotMatch(JSON.stringify(body), /BEGIN|"x":/);
});

test("authenticate / requireAdmin: who is calling, and the yes-or-no form", async () => {
  assert.deepEqual(await authenticate(await session("/x", ["newsletter:read"]), authEnv() as never), {
    mode: "auth", user: { id: "u-ada", email: "ada@example.com" }, perms: ["newsletter:read"],
  });
  assert.equal(await requireAdmin(await session("/x", []), authEnv() as never), null);
  assert.equal(((await requireAdmin(req("/x"), authEnv() as never)) as Response).status, 401);
});
