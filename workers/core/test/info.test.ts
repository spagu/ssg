// GET /api/core/info (build.json present, absent, unusable; problems from the
// summaries; nothing from core's environment), the shared navigation's fields
// of /api/core/modules (home, signOut) and the quiet refusal it asks for.
import { after, beforeEach, test } from "node:test";
import assert from "node:assert/strict";
import * as info from "../functions/api/core/info.ts";
import * as overview from "../functions/api/core/overview.ts";
import * as modules from "../functions/api/core/modules.ts";
import { buildJob, readBuild } from "../functions/api/core/_build.ts";
import { useRegistry } from "../functions/api/core/_registry.ts";
import { clearStatusCache } from "../functions/api/core/_status.ts";
import { AUTH_LOGOUT, HOME, signOutPath } from "../functions/api/core/_visible.ts";
import { REGISTRY, SCENARIOS } from "./demo.ts";
import { AUTH_ENV, ORIGIN, PASSWORD, PW_ENV, basic, call, entry, fakeFetch, freshIP, registry, session } from "./harness.ts";

const pw = () => ({ authorization: basic(PASSWORD), "cf-connecting-ip": freshIP() });
function routes(name: string, patch: Record<string, Response> = {}) {
  const out: Record<string, Response> = {};
  for (const [path, a] of Object.entries(SCENARIOS[name]())) out[ORIGIN + path] = a.raw !== undefined ? new Response(a.raw, { status: a.status }) : Response.json(a.body, { status: a.status });
  for (const [path, r] of Object.entries(patch)) out[ORIGIN + path] = r;
  return out;
}
async function get(env: Record<string, unknown> = PW_ENV, headers: Record<string, string> = pw()) {
  const r = await call(info.onRequest, { path: "/api/core/info?lang=de", headers, env });
  assert.equal(r.status, 200, r.body);
  return { body: r.json(), raw: r.body };
}

beforeEach(() => {
  useRegistry(REGISTRY);
  clearStatusCache();
});
after(() => useRegistry());

test("with build.json: the build facts, field by field; modules with versions and contracts; core listed", async () => {
  const f = fakeFetch(routes("good"));
  try {
    const { body } = await get();
    assert.deepEqual(body.build, { generator: "ssg", version: "1.9.9", buildTime: "2026-11-13T07:12:00Z", finished: "2026-11-13T07:12:09Z", durationSeconds: 8.6, pages: 412, files: 1288, content: { post: 120, page: 14 } });
    assert.deepEqual([body.buildState, body.buildOption, body.mode, body.lang], ["ok", "build_report", "password", "de"]);
    assert.deepEqual(body.registry, { format: 1, modules: 7 });
    assert.deepEqual(body.modules.find((m: { id: string }) => m.id === "comments"), { id: "comments", title: "Kommentare", kind: "pages-function", version: "1.4.0", contract: 1, status: "registered" });
    assert.ok(body.modules.some((m: { id: string }) => m.id === "core"));
    assert.deepEqual(body.problems, []);
    const build = f.sent.find((s) => s.url.endsWith("/build.json"))!;
    assert.deepEqual([build.redirect, Object.keys(build.headers).sort()], ["manual", ["authorization"]]);
  } finally {
    f.restore();
  }
});

test("without build.json: build null and the option to turn on; problems gathered from every module", async () => {
  const f = fakeFetch(routes("todo"));
  try {
    const { body } = await get();
    assert.deepEqual([body.build, body.buildState], [null, "absent"]);
    assert.deepEqual(body.problems.map((p: { module: string; code: string }) => `${p.module}:${p.code}`), ["newsletter:no_turnstile_secret", "kitchen-log:no_photo_bucket"]);
    assert.match(body.problems[0].text, /TURNSTILE_SECRET ist nicht gesetzt/);
  } finally {
    f.restore();
  }
});

test("a build report that is not one, or cannot be had", async () => {
  for (const [res, state] of [
    [Response.json({ hello: "world" }), "invalid"],
    [Response.json([1, 2]), "invalid"],
    [new Response("{", { status: 200 }), "invalid"],
    [new Response("", { status: 500 }), "unreadable"],
  ] as const) {
    clearStatusCache();
    const f = fakeFetch(routes("good", { "/build.json": res }));
    try {
      assert.equal((await get()).body.buildState, state);
    } finally {
      f.restore();
    }
  }
});

test("readBuild keeps known fields of the right type only", () => {
  const got = readBuild({ version: "1.9.9<script>", buildTime: "2026-11-13T07:12:00Z", finished: "not a date", durationSeconds: -1, pages: 1.5, files: "9", content: { post: 3, "Bad Key": 1, page: -2 }, secret: "x" });
  assert.deepEqual(got, { generator: null, version: null, buildTime: "2026-11-13T07:12:00Z", finished: null, durationSeconds: null, pages: null, files: null, content: { post: 3 } });
  assert.equal(readBuild({ generator: "ssg" }), null, "neither version nor time: not a report");
  assert.equal(readBuild({ version: "1.0.0", content: [1] })?.version, "1.0.0");
});

test("CORE_BUILD_REPORT_PATH names another file; anything but a plain path falls back", () => {
  const path = (v?: string) => (buildJob({ CORE_BUILD_REPORT_PATH: v }) as { path: string }).path;
  assert.deepEqual([path(undefined), path(" "), path("/reports/build.json"), path("https://evil.example.com/x"), path("//evil"), path("/a?x"), path("/a/../b")], [
    "/build.json", "/build.json", "/reports/build.json", "/build.json", "/build.json", "/build.json", "/a/../b",
  ]);
});

test("nothing from core's environment is in the answer — not a password, key, team or path", async () => {
  const secrets = { ...PW_ENV, CORE_ADMIN_PASSWORD: "correct-horse-battery", CORE_ACCESS_TEAM: "", CORE_BUILD_REPORT_PATH: "/hidden-report-name.json", CORE_PROBE_TIMEOUT_MS: "1999" };
  const f = fakeFetch(routes("good", { "/hidden-report-name.json": Response.json({ version: "1.9.9" }) }));
  try {
    const { raw, body } = await get(secrets, { authorization: basic("correct-horse-battery"), "cf-connecting-ip": freshIP() });
    assert.equal(body.build.version, "1.9.9");
    for (const v of Object.values(secrets).filter((v) => v.length > 3)) assert.ok(!raw.includes(v), `${v} leaked`);
  } finally {
    f.restore();
  }
  const auth = fakeFetch(routes("good"));
  try {
    const { raw } = await get(AUTH_ENV, { cookie: await session(["*"]) });
    for (const [k, v] of Object.entries(AUTH_ENV)) if (k !== "CORE_AUTH_LOGIN_URL") assert.ok(!raw.includes(v), `${k} leaked`);
  } finally {
    auth.restore();
  }
});

test("/modules for the navigation: home, and signOut only in the auth mode", async () => {
  const f = fakeFetch(routes("good"));
  try {
    const pwd = (await call(modules.onRequest, { path: "/api/core/modules", headers: pw(), env: PW_ENV })).json();
    assert.deepEqual([pwd.home, pwd.signOut, HOME], ["/core/", null, "/core/"]);
    const a = (await call(modules.onRequest, { path: "/api/core/modules", headers: { cookie: await session(["*"]) }, env: AUTH_ENV })).json();
    assert.deepEqual([a.home, a.signOut, a.user.email], ["/core/", AUTH_LOGOUT, "ada@example.com"]);
  } finally {
    f.restore();
  }
});

test("signOutPath: the auth module's base, else a same-origin issuer, else none", () => {
  const auth = { mode: "auth", user: null, perms: [] } as const;
  const none = registry();
  assert.equal(signOutPath({ mode: "password", user: null, perms: null }, REGISTRY as never, {}, ORIGIN), null);
  assert.equal(signOutPath(auth, registry(entry("auth", null, "/accounts")) as never, {}, ORIGIN), "/accounts/api/auth/logout");
  assert.equal(signOutPath(auth, none as never, { CORE_AUTH_ISSUER: `${ORIGIN}/api/auth` }, ORIGIN), "/api/auth/logout");
  assert.equal(signOutPath(auth, none as never, { CORE_AUTH_ISSUER: "https://auth.example.net/api/auth" }, ORIGIN), null);
  assert.equal(signOutPath(auth, none as never, { CORE_AUTH_ISSUER: "not a url" }, ORIGIN), null);
  assert.equal(signOutPath(auth, none as never, {}, ORIGIN), null);
});

test("x-core-quiet: a refusal carries no password challenge, so drawing the bar never opens a dialog", async () => {
  const loud = await call(modules.onRequest, { path: "/api/core/modules", headers: { "cf-connecting-ip": freshIP() }, env: PW_ENV });
  assert.match(loud.headers["www-authenticate"], /Basic/);
  const quiet = await call(modules.onRequest, { path: "/api/core/modules", headers: { "x-core-quiet": "1", "cf-connecting-ip": freshIP() }, env: PW_ENV });
  assert.deepEqual([quiet.status, quiet.headers["www-authenticate"], quiet.json().error], [401, undefined, "unauthorized"]);
  assert.equal(quiet.headers["content-security-policy"]?.includes("default-src 'none'"), true, "still secured");
  const ok = await call(modules.onRequest, { path: "/api/core/modules", headers: { "x-core-quiet": "1", ...pw() }, env: PW_ENV });
  assert.equal(ok.status, 200, "the header changes refusals only");
});

test("overview and info without a registry: 503 registry_missing, like every route", async () => {
  useRegistry();
  for (const handler of [overview.onRequest, info.onRequest]) {
    const r = await call(handler, { path: "/api/core/x", headers: pw(), env: PW_ENV });
    assert.deepEqual([r.status, r.json().error], [503, "registry_missing"]);
  }
});

test("info: a module core has no manifest for has no contract to show", async () => {
  useRegistry(registry(entry("ghost", null, "/ghost")));
  const f = fakeFetch({});
  try {
    const { body } = await get();
    assert.deepEqual(body.modules, [{ id: "ghost", title: "ghost", kind: null, version: null, contract: null, status: "unreachable" }]);
  } finally {
    f.restore();
  }
});
