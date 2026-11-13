// The fetch pipeline: base path, hooks, rate limit, auth, CSRF, routes, assets,
// and the security headers on every kind of response.
import { test } from "node:test";
import assert from "node:assert/strict";
import { CSRF_HEADER, handleFetch } from "../src/app.ts";
import type { Route } from "../src/router.ts";
import { CSP } from "../src/lib.ts";
import { MIGRATIONS } from "../src/schema.ts";
import { PASSWORD, fakeAssets, fakeCtx, fakeD1, fakeRateLimiter, makeEnv, req } from "./fakes.ts";

const B = "/newsletter-admin";
const seen: string[] = [];
const routes: Route[] = [
  { method: "GET", path: "/api/health", handler: () => new Response("health"), needsSubscribers: false },
  { method: "GET", path: "/api/things/:id", handler: ({ params }) => Response.json(params) },
  { method: "POST", path: "/api/things", handler: () => new Response("made", { status: 201 }) },
  { method: "GET", path: "/api/boom", handler: () => Promise.reject(new Error("secret sql")) },
  { method: "POST", path: "/hooks/test", handler: ({ request }) => (seen.push(request.method), new Response("hooked")) },
];
const run = (r: Request, env: Record<string, unknown> = makeEnv()) => handleFetch(r, env as never, fakeCtx() as never, routes);

function assertSecure(res: Response) {
  assert.equal(res.headers.get("cache-control"), "no-store");
  assert.equal(res.headers.get("x-robots-tag"), "noindex, nofollow");
  assert.equal(res.headers.get("content-security-policy"), CSP);
  assert.equal(res.headers.get("referrer-policy"), "no-referrer");
  assert.equal(res.headers.get("x-content-type-options"), "nosniff");
}

test("outside the base path is 404; an invalid base is 503", async () => {
  assert.equal((await run(req("/"))).status, 404);
  assert.equal((await run(req("/newsletter-administrator/api/health", { auth: PASSWORD }))).status, 404);
  assert.equal((await run(req(B + "/api/health"), makeEnv({ NEWSLETTER_UI_BASE: "/a b" }))).status, 503);
});

test("a custom base path moves everything", async () => {
  const env = makeEnv({ NEWSLETTER_UI_BASE: "/ops/mail/" });
  assert.equal(await (await run(req("/ops/mail/api/health", { auth: PASSWORD }), env)).text(), "health");
  assert.equal((await run(req(B + "/api/health", { auth: PASSWORD }), env)).status, 404);
});

test("hooks skip auth, CSRF and the rate limiter; unknown hooks are 404", async () => {
  const env = makeEnv({ RATE_LIMITER: fakeRateLimiter(0) });
  const res = await run(req(B + "/hooks/test", { method: "POST", csrf: false }), env);
  assert.equal(await res.text(), "hooked");
  assert.deepEqual(seen, ["POST"]);
  assertSecure(res);
  assert.equal((await run(req(B + "/hooks/nope", { method: "POST" }), env)).status, 404);
  // A hook path cannot reach an /api route, and /api cannot reach a hook.
  // "/hooks/../api/health" is normalised by URL to /api/health: limited, not a hook.
  assert.equal((await run(req(B + "/hooks/../api/health"), env)).status, 429);
  assert.equal((await run(req(B + "/api/hooks/test", { method: "POST", auth: PASSWORD }), makeEnv())).status, 404);
});

test("the rate limiter bounds everything else, before auth", async () => {
  const limiter = fakeRateLimiter(1);
  const env = makeEnv({ RATE_LIMITER: limiter });
  assert.equal((await run(req(B + "/api/health", { auth: PASSWORD }), env)).status, 200);
  const res = await run(req(B + "/api/health", { auth: PASSWORD }), env);
  assert.equal(res.status, 429);
  assert.equal(res.headers.get("retry-after"), "60");
  assert.deepEqual([...limiter.counts.keys()], ["newsletter-ui:203.0.113.7"]);
});

test("an unreachable database is 503 with the binding hint", async () => {
  const d1 = fakeD1();
  d1.state.failOn = /CREATE TABLE/;
  const res = await run(req(B + "/api/health", { auth: PASSWORD }), makeEnv({ NEWSLETTER_DB: d1 }));
  assert.equal(res.status, 503);
  assert.match((await res.json()).hint, /NEWSLETTER_DB/);
});

test("auth applies to the API and to panel files alike", async () => {
  for (const path of ["/api/health", "/", "/index.html", "/app.js", ""]) {
    const res = await run(req(B + path));
    assert.equal(res.status, 401, path);
    assertSecure(res);
  }
  const env = makeEnv({ NEWSLETTER_ADMIN_PASSWORD: undefined });
  assert.equal((await run(req(B + "/", { auth: "x" }), env)).status, 503);
});

test("CSRF: a state-changing request without the header is 403", async () => {
  const res = await run(req(B + "/api/things", { method: "POST", auth: PASSWORD, csrf: false }));
  assert.equal(res.status, 403);
  assert.match((await res.json()).error, new RegExp(CSRF_HEADER));
  const wrong = req(B + "/api/things", { method: "POST", auth: PASSWORD, csrf: false, headers: { [CSRF_HEADER]: "yes" } });
  assert.equal((await run(wrong)).status, 403);
  assert.equal((await run(req(B + "/api/things", { method: "POST", auth: PASSWORD }))).status, 201);
  assert.equal((await run(req(B + "/api/things/1", { method: "HEAD", auth: PASSWORD, csrf: false }))).status, 405);
});

test("API: params, 404, 405 with Allow, generic 500", async () => {
  const env = makeEnv();
  assert.deepEqual(await (await run(req(B + "/api/things/a%2Fb", { auth: PASSWORD }), env)).json(), { id: "a/b" });
  assert.equal((await run(req(B + "/api/nope", { auth: PASSWORD }), env)).status, 404);
  assert.equal((await run(req(B + "/api", { auth: PASSWORD }), env)).status, 404);
  const m = await run(req(B + "/api/things", { method: "DELETE", auth: PASSWORD }), env);
  assert.equal(m.status, 405);
  assert.equal(m.headers.get("allow"), "POST");
  const boom = await run(req(B + "/api/boom", { auth: PASSWORD }), env);
  assert.equal(boom.status, 500);
  assert.doesNotMatch(await boom.text(), /secret sql/);
  assertSecure(boom);
});

test("API: missing subscribers table is a readable 503, health still answers", async () => {
  const env = makeEnv({ NEWSLETTER_DB: fakeD1() });
  const res = await run(req(B + "/api/things/1", { auth: PASSWORD }), env);
  assert.equal(res.status, 503);
  assert.match((await res.json()).hint, /deploy the `newsletter` template/);
  assert.equal((await run(req(B + "/api/health", { auth: PASSWORD }), env)).status, 200);
});

test("the bare base redirects to base/ keeping the query", async () => {
  const res = await run(req(B + "?tab=campaigns", { auth: PASSWORD }));
  assert.equal(res.status, 308);
  assert.equal(res.headers.get("location"), B + "/?tab=campaigns");
  assertSecure(res);
});

test("panel files come from ASSETS with the base stripped, re-headered", async () => {
  const assets = fakeAssets({ "/index.html": "<!doctype html>", "/app.css": "body{}" });
  const env = makeEnv({ ASSETS: assets });
  const page = await run(req(B + "/", { auth: PASSWORD }), env);
  assert.equal(await page.text(), "<!doctype html>");
  assertSecure(page);
  assert.equal((await run(req(B + "/app.css", { auth: PASSWORD }), env)).status, 200);
  assert.equal((await run(req(B + "/missing.js", { auth: PASSWORD }), env)).status, 404);
  assert.deepEqual(assets.requested, ["/", "/app.css", "/missing.js"]);
  const none = await run(req(B + "/", { auth: PASSWORD }), makeEnv({ ASSETS: undefined }));
  assert.equal(none.status, 404);
  assertSecure(none);
});

test("the default registry serves health", async () => {
  const env = makeEnv();
  const res = await handleFetch(req(B + "/api/health", { auth: PASSWORD }), env as never, fakeCtx() as never);
  assert.equal(res.status, 200);
  assertSecure(res);
  const body = await res.json();
  assert.deepEqual(body, {
    ok: true,
    schemaVersion: MIGRATIONS[MIGRATIONS.length - 1].version, // the latest migration
    subscribersTable: true,
    auth: "basic",
    bindings: { queue: true, rateLimiter: false, assets: true },
  });
  const bare = makeEnv({ NEWSLETTER_DB: fakeD1(), NEWSLETTER_QUEUE: undefined, ASSETS: undefined, RATE_LIMITER: fakeRateLimiter() });
  const missing = await (await handleFetch(req(B + "/api/health", { auth: PASSWORD }), bare as never, fakeCtx() as never)).json();
  assert.equal(missing.ok, false);
  assert.match(missing.hint, /newsletter/);
  assert.deepEqual(missing.bindings, { queue: false, rateLimiter: true, assets: false });
});

test("health names the auth mode without revealing secrets", async () => {
  const { health } = await import("../src/health.ts");
  const { ensureSchema } = await import("../src/schema.ts");
  const env = makeEnv({ NEWSLETTER_ACCESS_TEAM: "t", NEWSLETTER_ACCESS_AUD: "a" });
  await ensureSchema(env as never);
  const body = await (await health({ env } as never)).json();
  assert.equal(body.auth, "access");
  assert.doesNotMatch(JSON.stringify(body), new RegExp(PASSWORD));
});
