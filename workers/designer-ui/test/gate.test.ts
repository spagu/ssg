// The gate and the pipeline: `auth` is the only mode. Missing or incomplete
// DESIGNER_AUTH_* → 503 on every path (the panel's files too); no session →
// the verifier's own 401 / login redirect; the channel per kind (cookie =
// person, bearer = service); CSRF on a cookie session's writes; base path,
// 404/405/308, the rate limit, errors of an operation.
import { test } from "node:test";
import assert from "node:assert/strict";
import { BASE, COOKIE, GIT_VARS, ISSUER, LOGIN, ORIGIN, makeEnv, request, run, site, token } from "./fixtures.ts";
import { NOT_CONFIGURED, authConfig, csrfRefusal } from "../src/gate.ts";
import type { Caller } from "../src/gate.ts";
import { OpError, errorResponse, retryAfter } from "../src/ops.ts";
import type { Route } from "../src/router.ts";
import { PERM } from "../src/permissions.ts";
import { RepoError } from "../portable/repo-provider.ts";
import handler from "../src/index.ts";

const PATHS = ["/api/module", "/api/health", "/api/settings", "/api/theme", "/", "/css/base.css", "", "/api/nothing"];

test("no DESIGNER_AUTH_* at all: 503 auth_not_configured on every path, panel included", async () => {
  const env = makeEnv({ DESIGNER_AUTH_ISSUER: undefined, DESIGNER_AUTH_PUBLIC_KEY: undefined, DESIGNER_AUTH_LOGIN_URL: undefined });
  for (const p of PATHS) {
    const res = await run(await request(BASE + p, ["*"]), env);
    assert.equal(res.status, 503, p);
    assert.deepEqual(await res.json(), { error: "auth_not_configured", detail: NOT_CONFIGURED });
    assert.equal(res.headers.get("x-frame-options"), "DENY");
  }
});

test("half-configured auth: 503 naming what is missing — never open", async () => {
  for (const vars of [{ DESIGNER_AUTH_ISSUER: undefined }, { DESIGNER_AUTH_PUBLIC_KEY: undefined }, { DESIGNER_AUTH_JWKS_URL: "/api/auth/jwks" }]) {
    const res = await run(await request(BASE + "/api/settings", ["*"]), makeEnv(vars));
    assert.equal(res.status, 503);
    assert.match((await res.json()).detail, /DESIGNER_AUTH_/);
  }
  assert.equal(authConfig(makeEnv() as never).ok, true);
  assert.deepEqual(authConfig({} as never), { ok: false, detail: NOT_CONFIGURED });
  assert.match((authConfig({ DESIGNER_AUTH_ISSUER: ISSUER } as never) as { detail: string }).detail, /DESIGNER_AUTH_LOGIN_URL is not set/);
});

test("no session: an API call gets 401 JSON, a page is sent to sign in with a safe next", async () => {
  const res = await run(await request(BASE + "/api/settings", null));
  assert.deepEqual([res.status, await res.json()], [401, { error: "unauthorized", reason: "no_session" }]);
  const page = await run(await request(BASE + "/?x=1", null, { headers: { accept: "text/html" } }));
  assert.equal(page.status, 302);
  const to = new URL(page.headers.get("location")!);
  assert.equal(to.origin + to.pathname, ORIGIN + LOGIN);
  assert.equal(to.searchParams.get("next"), "/designer/?x=1");
});

test("channel per kind: a person by cookie, a script by bearer — never swapped", async () => {
  const gh = await site();
  assert.equal((await (await run(await request(BASE + "/api/me", [PERM.read]), makeEnv(), gh)).json()).kind, "user");
  assert.equal((await (await run(await request(BASE + "/api/me", [PERM.read], { kind: "service" }), makeEnv(), gh)).json()).kind, "service");
  const personAsBearer = new Request(ORIGIN + BASE + "/api/me", { headers: { authorization: `Bearer ${await token([PERM.read])}` } });
  assert.deepEqual(await (await run(personAsBearer)).json(), { error: "unauthorized", reason: "invalid_session" });
  const serviceAsCookie = new Request(ORIGIN + BASE + "/api/me", { headers: { cookie: `${COOKIE}=${await token([PERM.read], { kind: "service" })}` } });
  assert.deepEqual(await (await run(serviceAsCookie)).json(), { error: "unauthorized", reason: "invalid_session" });
});

const WRITE: Route = { method: "POST", path: "/api/probe", op: { name: "probe", permission: PERM.settings, run: () => ({ done: true }) } };

test("CSRF: a cookie session's write needs this origin; a bearer write needs nothing", async () => {
  const send = async (headers: Record<string, string>, kind: "user" | "service" = "user") =>
    run(await request(BASE + "/api/probe", [PERM.settings], { method: "POST", headers, kind }), makeEnv(), undefined, [WRITE]);
  const foreign: Record<string, string>[] = [{}, { origin: "https://evil.example" }, { "sec-fetch-site": "same-site" }, { origin: ORIGIN, "sec-fetch-site": "cross-site" }];
  for (const headers of foreign) {
    const res = await send(headers);
    assert.deepEqual([res.status, (await res.json()).error], [403, "cross_origin"], JSON.stringify(headers));
  }
  const own: Record<string, string>[] = [{ origin: ORIGIN }, { "sec-fetch-site": "same-origin" }];
  for (const headers of own) assert.deepEqual(await (await send(headers)).json(), { done: true });
  assert.equal((await send({ origin: "https://evil.example" }, "service")).status, 200, "bearer cannot be forged by a page");
  const caller: Caller = { kind: "user", user: { id: "u", email: "u@example.com" }, perms: [], sessionId: "s", expiresAt: "" };
  assert.equal(csrfRefusal(new Request(ORIGIN + "/", { method: "HEAD", headers: { origin: "https://evil.example" } }), caller), null);
});

test("the panel: served for designer-ui:read, 403 without, 308 from the bare base, 404 without assets", async () => {
  const page = await run(await request(BASE + "/", [PERM.read]));
  assert.equal(await page.text(), "asset /");
  assert.equal(page.headers.get("x-frame-options"), "DENY");
  const without = await run(await request(BASE + "/", [PERM.theme], { headers: { accept: "text/html" } }));
  assert.deepEqual([without.status, await without.json()], [403, { error: "forbidden", permission: PERM.read }]);
  const bare = await run(await request(BASE + "?lang=pl", [PERM.read]));
  assert.deepEqual([bare.status, bare.headers.get("location")], [308, "/designer/?lang=pl"]);
  assert.equal((await run(await request(BASE + "/", [PERM.read]), makeEnv({ ASSETS: undefined }))).status, 404);
});

test("base path: outside 404, a custom base, an invalid one fails closed, the whole host", async () => {
  assert.equal((await run(await request("/designers", [PERM.read]))).status, 404);
  assert.equal((await run(await request("/admin/look/api/module", [PERM.read]), makeEnv({ DESIGNER_BASE: "admin/look/" }))).status, 200);
  const bad = await run(await request("/x/../api/module", [PERM.read]), makeEnv({ DESIGNER_BASE: "/x/.." }));
  assert.deepEqual([bad.status, (await bad.json()).error], [503, "bad_base"]);
  assert.equal((await run(await request("/api/module", [PERM.read]), makeEnv({ DESIGNER_BASE: "/" }))).status, 200);
});

test("unknown API path 404, wrong method 405 with allow", async () => {
  assert.equal((await run(await request(BASE + "/api/nothing", [PERM.read]))).status, 404);
  const res = await run(await request(BASE + "/api/settings", [PERM.read], { method: "DELETE", headers: { origin: ORIGIN } }));
  assert.deepEqual([res.status, res.headers.get("allow")], [405, "GET"]);
});

test("RATE_LIMITER: keyed per IP under the module's id, 429 with retry-after", async () => {
  const keys: string[] = [];
  let success = true;
  const env = makeEnv({ RATE_LIMITER: { limit: async ({ key }: { key: string }) => (keys.push(key), { success }) } });
  assert.equal((await run(await request(BASE + "/api/module", [PERM.read], { headers: { "cf-connecting-ip": "203.0.113.9" } }), env)).status, 200);
  success = false;
  const limited = await run(await request(BASE + "/api/module", null), env);
  assert.deepEqual([limited.status, limited.headers.get("retry-after")], [429, "60"]);
  assert.deepEqual(keys, ["designer-ui:203.0.113.9", "designer-ui:unknown"]);
});

test("operation errors: OpError, forbidden with keys, RepoError, anything else → 500 without its text", async () => {
  const fail = (err: unknown): Route => ({ method: "GET", path: "/api/fail", op: { name: "fail", permission: PERM.read, run: () => { throw err; } } });
  const call = async (err: unknown) => run(await request(BASE + "/api/fail", [PERM.read]), makeEnv(), undefined, [fail(err)]);
  assert.deepEqual(await (await call(new OpError("gone", 410))).json(), { error: "gone" });
  const keys = await call(new OpError("forbidden", 403, undefined, { permission: PERM.advanced, keys: ["hooks"] }));
  assert.deepEqual([keys.status, await keys.json()], [403, { error: "forbidden", permission: PERM.advanced, keys: ["hooks"] }]);
  const conflict = await call(new RepoError("conflict", { detail: "busy", conflicts: [] }));
  assert.deepEqual([conflict.status, await conflict.json()], [409, { error: "conflict", detail: "busy", conflicts: [] }]);
  const limited = await call(new RepoError("rate_limited", { retryAt: new Date(Date.now() + 90_000).toISOString() }));
  assert.ok(Number(limited.headers.get("retry-after")) >= 89);
  assert.equal((await call(new RepoError("rate_limited"))).headers.get("retry-after"), "60");
  const logged: unknown[] = [];
  const log = console.error;
  console.error = (...a: unknown[]) => void logged.push(a);
  try {
    const boom = await call(new Error(`secret ${GIT_VARS.DESIGNER_GIT_TOKEN}`));
    assert.deepEqual([boom.status, await boom.json()], [500, { error: "internal_error" }]);
  } finally {
    console.error = log;
  }
  assert.equal(logged[0] && (logged[0] as unknown[])[0], "designer-ui:");
  assert.equal(errorResponse(new OpError("forbidden", 403, undefined, { permission: PERM.theme }), "x").status, 403);
  assert.equal(retryAfter(new Date(1_000).toISOString(), 5_000), "1");
});

test("a route's respond hook, and the Worker's entry point", async () => {
  const echo: Route = { method: "GET", path: "/api/echo", op: { name: "echo", permission: PERM.read, run: () => "x" }, respond: (out: string) => new Response(out, { status: 201 }) };
  const res = await run(await request(BASE + "/api/echo", [PERM.read]), makeEnv(), undefined, [echo]);
  assert.deepEqual([res.status, await res.text(), res.headers.get("x-frame-options")], [201, "x", "DENY"]);
  assert.equal((await handler.fetch(new Request(ORIGIN + "/elsewhere"), makeEnv() as never)).status, 404);
});
