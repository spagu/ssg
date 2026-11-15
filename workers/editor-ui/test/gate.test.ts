// The gate: `auth` is the only mode. Missing or incomplete EDITOR_AUTH_* →
// 503 on every path (the panel's files too); no session → the verifier's
// own 401 / login redirect; the channel per kind (cookie = person, bearer =
// service); CSRF on a cookie session's writes.
import { test } from "node:test";
import assert from "node:assert/strict";
import { BASE, COOKIE, ISSUER, LOGIN, ORIGIN, makeEnv, request, run, site, token } from "./fixtures.ts";
import { NOT_CONFIGURED, authConfig, csrfRefusal } from "../src/gate.ts";
import type { Caller } from "../src/gate.ts";
import type { Route } from "../src/router.ts";
import { PERM } from "../src/permissions.ts";

const PATHS = ["/api/module", "/api/health", "/api/module/summary", "/api/me", "/", "/css/base.css", "", "/api/nothing"];

test("no EDITOR_AUTH_* at all: 503 auth_not_configured on every path, panel included", async () => {
  const env = makeEnv({ EDITOR_AUTH_ISSUER: undefined, EDITOR_AUTH_PUBLIC_KEY: undefined, EDITOR_AUTH_LOGIN_URL: undefined });
  for (const p of PATHS) {
    const res = await run(await request(BASE + p, ["*"]), env);
    assert.equal(res.status, 503, p);
    assert.deepEqual(await res.json(), { error: "auth_not_configured", detail: NOT_CONFIGURED });
    assert.equal(res.headers.get("cache-control"), "no-store");
    assert.equal(res.headers.get("x-frame-options"), "DENY");
  }
});

test("half-configured auth: 503 naming what is missing — never open, whatever the credential", async () => {
  const cases: Record<string, unknown>[] = [
    { EDITOR_AUTH_PUBLIC_KEY: undefined, EDITOR_AUTH_LOGIN_URL: undefined },
    { EDITOR_AUTH_ISSUER: undefined },
    { EDITOR_AUTH_PUBLIC_KEY: undefined },
    { EDITOR_AUTH_JWKS_URL: "/api/auth/jwks" }, // both key sources
    { EDITOR_AUTH_PUBLIC_KEY: undefined, EDITOR_AUTH_JWKS_URL: "http://example.com/jwks" },
  ];
  for (const vars of cases) {
    for (const p of PATHS) {
      const res = await run(await request(BASE + p, ["*"]), makeEnv(vars));
      assert.equal(res.status, 503, `${JSON.stringify(vars)} ${p}`);
      const body = await res.json();
      assert.equal(body.error, "auth_not_configured");
      assert.match(body.detail, /EDITOR_AUTH_/);
    }
  }
});

test("an unusable inline key: the verifier's own 503 not_configured, with the security headers", async () => {
  const res = await run(await request(BASE + "/api/me", ["*"]), makeEnv({ EDITOR_AUTH_PUBLIC_KEY: "{not json" }));
  assert.equal(res.status, 503);
  assert.equal((await res.json()).error, "not_configured");
  assert.equal(res.headers.get("content-security-policy")?.includes("default-src 'none'"), true);
});

test("authConfig: complete set, nothing set, incomplete set", () => {
  assert.equal(authConfig(makeEnv() as never).ok, true);
  assert.deepEqual(authConfig({} as never), { ok: false, detail: NOT_CONFIGURED });
  const half = authConfig({ EDITOR_AUTH_ISSUER: ISSUER } as never);
  assert.equal(half.ok, false);
  assert.match((half as { detail: string }).detail, /EDITOR_AUTH_LOGIN_URL is not set/);
});

test("no session: an API call gets 401 JSON, a page is sent to sign in with a safe next", async () => {
  for (const p of ["/api/me", "/api/module", "/api/health", "/api/module/summary"]) {
    const res = await run(await request(BASE + p, null));
    assert.equal(res.status, 401, p);
    assert.deepEqual(await res.json(), { error: "unauthorized", reason: "no_session" });
  }
  const page = await run(await request(BASE + "/?x=1", null, { headers: { accept: "text/html" } }));
  assert.equal(page.status, 302);
  const to = new URL(page.headers.get("location")!);
  assert.equal(to.origin + to.pathname, ORIGIN + LOGIN);
  assert.equal(to.searchParams.get("next"), "/editor/?x=1");
  assert.equal(page.headers.get("x-robots-tag"), "noindex, nofollow");
});

test("expired and forged sessions are refused by the verifier", async () => {
  const past = Math.floor(Date.now() / 1000) - 3600;
  const expired = await run(await request(BASE + "/api/me", ["*"], { claims: { iat: past - 600, exp: past } }));
  assert.deepEqual(await expired.json(), { error: "unauthorized", reason: "expired" });
  const wrongIssuer = await run(await request(BASE + "/api/me", ["*"], { claims: { iss: "https://example.org/api/auth" } }));
  assert.deepEqual(await wrongIssuer.json(), { error: "unauthorized", reason: "invalid_session" });
});

test("channel per kind: a person by cookie, a script by bearer — never swapped", async () => {
  const gh = await site();
  const user = await run(await request(BASE + "/api/me", [PERM.read]), makeEnv(), gh);
  assert.equal((await user.json()).kind, "user");
  const service = await run(await request(BASE + "/api/me", [PERM.read], { kind: "service" }), makeEnv(), gh);
  assert.equal((await service.json()).kind, "service");
  // A person's token sent as a bearer, a service token in the cookie: wrong_kind → invalid_session.
  const personAsBearer = new Request(ORIGIN + BASE + "/api/me", { headers: { authorization: `Bearer ${await token([PERM.read])}` } });
  assert.deepEqual(await (await run(personAsBearer)).json(), { error: "unauthorized", reason: "invalid_session" });
  const serviceAsCookie = new Request(ORIGIN + BASE + "/api/me", { headers: { cookie: `${COOKIE}=${await token([PERM.read], { kind: "service" })}` } });
  assert.deepEqual(await (await run(serviceAsCookie)).json(), { error: "unauthorized", reason: "invalid_session" });
});

// A state-changing test route: the skeleton has none yet (GO-170 adds them).
const WRITE: Route = { method: "POST", path: "/api/probe", op: { name: "probe", permission: PERM.publish, run: () => ({ done: true }) } };

test("CSRF: a cookie session's write needs this origin; a bearer write needs nothing", async () => {
  const send = async (headers: Record<string, string>, kind: "user" | "service" = "user") =>
    run(await request(BASE + "/api/probe", [PERM.publish], { method: "POST", headers, kind }), makeEnv(), undefined, [WRITE]);
  const foreign: Record<string, string>[] = [{}, { origin: "https://evil.example" }, { "sec-fetch-site": "same-site" }, { origin: ORIGIN, "sec-fetch-site": "cross-site" }];
  for (const headers of foreign) {
    const res = await send(headers);
    assert.equal(res.status, 403, JSON.stringify(headers));
    assert.equal((await res.json()).error, "cross_origin");
  }
  const own: Record<string, string>[] = [{ origin: ORIGIN }, { "sec-fetch-site": "same-origin" }, { origin: ORIGIN, "sec-fetch-site": "same-origin" }];
  for (const headers of own) {
    assert.deepEqual(await (await send(headers)).json(), { done: true }, JSON.stringify(headers));
  }
  assert.equal((await send({}, "service")).status, 200, "bearer: no Origin needed");
  assert.equal((await send({ origin: "https://evil.example" }, "service")).status, 200, "bearer cannot be forged by a page");
});

test("CSRF comes before routing: a foreign write to an unknown path is refused, not routed", async () => {
  const foreign = await run(await request(BASE + "/api/me", [PERM.read], { method: "POST", headers: { origin: "https://evil.example" } }));
  assert.equal(foreign.status, 403);
  const own = await run(await request(BASE + "/api/me", [PERM.read], { method: "POST", headers: { origin: ORIGIN } }));
  assert.equal(own.status, 405);
  assert.equal(own.headers.get("allow"), "GET");
});

test("csrfRefusal: safe methods pass", () => {
  const caller: Caller = { kind: "user", user: { id: "u", email: "u@example.com" }, perms: [], sessionId: "s", expiresAt: "" };
  for (const method of ["GET", "HEAD"]) assert.equal(csrfRefusal(new Request(ORIGIN + "/", { method, headers: { origin: "https://evil.example" } }), caller), null);
});

