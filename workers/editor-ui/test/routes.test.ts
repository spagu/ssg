// Every route × (no session, a session without the permission, a session
// with it); the panel files; the pipeline's own answers (base path, 404,
// 405, 308, rate limit, errors from an operation).
import { test } from "node:test";
import assert from "node:assert/strict";
import manifest from "../module.json" with { type: "json" };
import { BASE, GIT_VARS, ORIGIN, makeEnv, request, run, site } from "./fixtures.ts";
import { TOKEN } from "./fake-github.ts";
import { PERM } from "../src/permissions.ts";
import { OpError, errorResponse, retryAfter } from "../portable/ops.ts";
import type { Route } from "../portable/router.ts";
import { matchPath } from "../portable/router.ts";
import { ROUTES } from "../src/routes.ts";
import { MB, PLAN_LIMITS, RepoError } from "../portable/repo-provider.ts";

const OTHER = ["newsletter:read", "editor-ui:publish", "editor-ui:media", "editor-ui:delete"]; // everything but read

const SEED = (await site()).store.refs.get("main")!;

// A working request for the reading routes with an input (GO-170); the
// writing route has its own suite (changes.test.ts).
const SAMPLE: Record<string, string> = {
  "/api/content/:path": "/api/content/" + encodeURIComponent("content/blog/posts/hello.md"),
  "/api/content/new-path": "/api/content/new-path?section=content%2Fblog%2Fposts&title=Hi",
  "/api/media/new-path": "/api/media/new-path?name=Cat.png&type=image%2Fpng&hash=1a2b3c4d",
  "/api/media/references": "/api/media/references?path=" + encodeURIComponent("content/blog/media/a.png"),
  // GO-172: the seed commit's id — site() is deterministic, so it is the same in the test below.
  "/api/revisions/:path": "/api/revisions/" + encodeURIComponent("content/blog/posts/hello.md"),
  "/api/revisions/:path/diff": "/api/revisions/" + encodeURIComponent("content/blog/posts/hello.md") + "/diff?from=" + SEED,
  "/api/revisions/:path/:commit": "/api/revisions/" + encodeURIComponent("content/blog/posts/hello.md") + "/" + SEED,
};

test("every reading route: no session 401, without editor-ui:read 403 naming it, with it 200 — refusals byte for byte alike", async () => {
  const gh = await site();
  const refusals = new Set<string>();
  for (const r of ROUTES.filter((x) => x.method === "GET")) {
    const path = SAMPLE[r.path] ?? r.path;
    const none = await run(await request(BASE + path, null), makeEnv(), gh);
    assert.equal(none.status, 401, r.path);
    const without = await run(await request(BASE + path, OTHER), makeEnv(), gh);
    assert.equal(without.status, 403, r.path);
    const body = await without.text();
    refusals.add(body);
    assert.deepEqual(JSON.parse(body), { error: "forbidden", permission: PERM.read });
    assert.equal(without.headers.get("cache-control"), "no-store");
    const ok = await run(await request(BASE + path, [PERM.read]), makeEnv(), gh);
    assert.equal(ok.status, 200, r.path);
    assert.equal(ok.headers.get("x-robots-tag"), "noindex, nofollow");
    assert.equal(ok.headers.get("content-type"), "application/json; charset=utf-8");
  }
  assert.equal(refusals.size, 1, "the manifest and summary refuse exactly as the admin routes");
});

test("wildcards grant through the verifier's can: editor-ui:* and *", async () => {
  const gh = await site();
  for (const perms of [["editor-ui:*"], ["*"]]) assert.equal((await run(await request(BASE + "/api/me", perms), makeEnv(), gh)).status, 200);
});

test("GET /api/module serves module.json", async () => {
  const res = await run(await request(BASE + "/api/module", [PERM.read]));
  assert.deepEqual(await res.json(), manifest);
});

test("GET /api/me: identity, kind, this module's permissions only, the auth endpoints", async () => {
  const res = await run(await request(BASE + "/api/me", ["editor-ui:read", "editor-ui:media", "newsletter:send"]));
  const me = await res.json();
  assert.deepEqual(me.user, { id: "u-ada", email: "ada@example.com" });
  assert.equal(me.kind, "user");
  assert.deepEqual(me.permissions, ["editor-ui:read", "editor-ui:media"]);
  assert.equal(me.loginUrl, "/auth/login");
  assert.equal(me.logoutUrl, "/api/auth/logout");
  assert.equal(me.refreshUrl, "/api/auth/refresh");
  assert.ok(Date.parse(me.sessionExpiresAt) > Date.now());
  const all = await (await run(await request(BASE + "/api/me", ["editor-ui:*"]))).json();
  assert.deepEqual(all.permissions, manifest.permissions.map((p) => p.name));
});

test("GET /api/health: configuration only, never the token", async () => {
  const limiter = { limit: async () => ({ success: true }) };
  const ok = await run(await request(BASE + "/api/health", [PERM.read]), makeEnv({ EDITOR_MEDIA_DIR: "static/img", EDITOR_CONTENT_DIRS: " ", RATE_LIMITER: limiter }));
  const body = await ok.json();
  assert.deepEqual(body, {
    ok: true,
    auth: "auth",
    repository: { configured: true, provider: "github", branch: "main" },
    overrides: { contentDirs: false, mediaDir: true },
    plan: { plan: "free", limits: PLAN_LIMITS.free, subrequests: 45, maxBody: 8 * MB },
    bindings: { assets: true, rateLimiter: true },
  });
  const paid = await (await run(await request(BASE + "/api/health", [PERM.read]), makeEnv({ EDITOR_PLAN: "paid" }))).json();
  assert.deepEqual(paid.plan, { plan: "paid", limits: PLAN_LIMITS.paid, subrequests: 125, maxBody: 28 * MB });
  const missing = await (await run(await request(BASE + "/api/health", [PERM.read]), makeEnv({ EDITOR_GIT_TOKEN: undefined, ASSETS: undefined }))).json();
  assert.equal(missing.ok, false);
  assert.deepEqual(missing.repository, { configured: false, problem: "EDITOR_GIT_TOKEN is not set (npx wrangler secret put EDITOR_GIT_TOKEN)" });
  assert.equal(missing.bindings.assets, false);
  const text = JSON.stringify(await (await run(await request(BASE + "/api/health", [PERM.read]), makeEnv({ EDITOR_GIT_TOKEN: "x y" }))).json());
  assert.ok(!text.includes(TOKEN) && !text.includes("x y"));
});

test("the panel: served for editor-ui:read with the security headers, 403 without, 308 from the bare base", async () => {
  const page = await run(await request(BASE + "/", [PERM.read]));
  assert.equal(await page.text(), "asset /");
  assert.equal(page.headers.get("x-frame-options"), "DENY");
  assert.match(page.headers.get("content-security-policy")!, /script-src 'self'; style-src 'self'/);
  assert.equal(await (await run(await request(BASE + "/css/base.css", [PERM.read]))).text(), "asset /css/base.css");
  const without = await run(await request(BASE + "/", OTHER, { headers: { accept: "text/html" } }));
  assert.equal(without.status, 403);
  assert.deepEqual(await without.json(), { error: "forbidden", permission: PERM.read });
  const bare = await run(await request(BASE + "?lang=pl", [PERM.read]));
  assert.equal(bare.status, 308);
  assert.equal(bare.headers.get("location"), "/editor/?lang=pl");
  assert.equal((await run(await request(BASE + "/", [PERM.read]), makeEnv({ ASSETS: undefined }))).status, 404);
});

test("base path: outside → 404, a custom base, an invalid one fails closed, the whole host", async () => {
  assert.equal((await run(await request("/elsewhere", [PERM.read]))).status, 404);
  assert.equal((await run(await request("/editorial", [PERM.read]))).status, 404);
  assert.equal((await run(await request("/admin/edit/api/module", [PERM.read]), makeEnv({ EDITOR_BASE: "admin/edit/" }))).status, 200);
  const bad = await run(await request("/x/../api/module", [PERM.read]), makeEnv({ EDITOR_BASE: "/x/.." }));
  assert.equal(bad.status, 503);
  assert.equal((await bad.json()).error, "bad_base");
  assert.equal((await run(await request("/api/module", [PERM.read]), makeEnv({ EDITOR_BASE: "/" }))).status, 200);
});

test("unknown API path 404, wrong method 405 with allow", async () => {
  assert.equal((await run(await request(BASE + "/api/nothing", [PERM.read]))).status, 404);
  assert.equal((await run(await request(BASE + "/api", [PERM.read]))).status, 404);
  const res = await run(await request(BASE + "/api/module", [PERM.read], { method: "DELETE", headers: { origin: ORIGIN } }));
  assert.equal(res.status, 405);
  assert.equal(res.headers.get("allow"), "GET");
});

test("RATE_LIMITER: keyed per IP, 429 with retry-after when exceeded, before the gate", async () => {
  const keys: string[] = [];
  let success = true;
  const env = makeEnv({ RATE_LIMITER: { limit: async ({ key }: { key: string }) => (keys.push(key), { success }) } });
  const r = await request(BASE + "/api/module", [PERM.read], { headers: { "cf-connecting-ip": "203.0.113.9" } });
  assert.equal((await run(r, env)).status, 200);
  success = false;
  const limited = await run(await request(BASE + "/api/module", null), env);
  assert.equal(limited.status, 429);
  assert.equal(limited.headers.get("retry-after"), "60");
  assert.deepEqual(keys, ["editor-ui:203.0.113.9", "editor-ui:unknown"]);
});

test("operation errors: OpError, RepoError, anything else → 500 without its text", async () => {
  const fail = (err: unknown): Route => ({ method: "GET", path: "/api/fail", op: { name: "fail", permission: PERM.read, run: () => { throw err; } } });
  const call = async (err: unknown) => run(await request(BASE + "/api/fail", [PERM.read]), makeEnv(), undefined, [fail(err)]);
  const op = await call(new OpError("repo_not_configured", 503, "EDITOR_GIT_REPO is not set"));
  assert.deepEqual([op.status, await op.json()], [503, { error: "repo_not_configured", detail: "EDITOR_GIT_REPO is not set" }]);
  const bare = await call(new OpError("gone", 410));
  assert.deepEqual(await bare.json(), { error: "gone" });
  const conflict = await call(new RepoError("conflict", { conflicts: [{ path: "a.md", currentBlobId: null, headCommit: "c" }] }));
  assert.deepEqual([conflict.status, await conflict.json()], [409, { error: "conflict", conflicts: [{ path: "a.md", currentBlobId: null, headCommit: "c" }] }]);
  const limited = await call(new RepoError("rate_limited", { detail: "slow down", retryAt: new Date(Date.now() + 90_000).toISOString() }));
  assert.equal(limited.status, 429);
  assert.ok(Number(limited.headers.get("retry-after")) >= 89);
  assert.deepEqual(Object.keys(await limited.json()), ["error", "detail", "retryAt"]);
  const logged: unknown[] = [];
  const log = console.error;
  console.error = (...a: unknown[]) => void logged.push(a);
  try {
    const boom = await call(new Error(`secret ${GIT_VARS.EDITOR_GIT_TOKEN}`));
    assert.deepEqual([boom.status, await boom.json()], [500, { error: "internal_error" }]);
  } finally {
    console.error = log;
  }
  assert.equal(logged.length, 1);
  assert.equal(errorResponse(new OpError("forbidden", 403, undefined, { permission: PERM.delete }), "x").status, 403);
});

test("a route's input and respond hooks", async () => {
  const echo: Route = {
    method: "GET", path: "/api/echo/:name",
    op: { name: "echo", permission: PERM.read, run: (_c, input: string) => input },
    input: (_c, params) => params.name,
    respond: (out: string) => new Response(out, { status: 201 }),
  };
  const res = await run(await request(BASE + "/api/echo/a%20b", [PERM.read]), makeEnv(), undefined, [echo]);
  assert.deepEqual([res.status, await res.text(), res.headers.get("x-frame-options")], [201, "a b", "DENY"]);
  assert.equal((await run(await request(BASE + "/api/echo/%E0", [PERM.read]), makeEnv(), undefined, [echo])).status, 404);
});

test("matchPath: segments, captures, empty and malformed values", () => {
  assert.deepEqual(matchPath("/api/x/:id", "/api/x/7"), { id: "7" });
  assert.equal(matchPath("/api/x/:id", "/api/x/"), null);
  assert.equal(matchPath("/api/x", "/api/y"), null);
  assert.equal(matchPath("/api/x", "/api/x/y"), null);
});

test("retryAfter: from the reset time, at least 1 s, 60 when unknown", () => {
  assert.equal(retryAfter(new Date(10_500).toISOString(), 1_000), "10");
  assert.equal(retryAfter(new Date(0).toISOString(), 5_000), "1");
  assert.equal(retryAfter(undefined), "60");
});
