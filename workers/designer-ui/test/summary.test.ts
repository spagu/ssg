// The module summary: counts and fixed sentences, the problems of the
// repository, the configuration, the settings file and the theme — never a
// value, never a person's data. Plus api/module, api/me and api/health.
import { test } from "node:test";
import assert from "node:assert/strict";
import manifest from "../module.json" with { type: "json" };
import { BASE, READ, makeEnv, request, run, site } from "./fixtures.ts";
import { TOKEN } from "./fake-github.ts";
import { PERM } from "../src/permissions.ts";
import { get } from "./helpers.ts";
import { MB, PLAN_LIMITS } from "../portable/repo-provider.ts";

const summary = async (gh: Awaited<ReturnType<typeof site>> | undefined, env = makeEnv()) =>
  (await run(await request(BASE + "/api/module/summary", READ), env, gh)).json();
const codes = (s: { problems: { code: string }[] }) => s.problems.map((p) => p.code);

test("a healthy site: settings set in the panel, theme files, the branch", async () => {
  const gh = await site({ ".ssg.designer.json": '{"title": "T", "mddb": {"lang": "pl"}}' });
  const s = await summary(gh);
  assert.deepEqual(s.metrics.map((m: { key: string; value: number }) => [m.key, m.value]), [["overlaySettings", 2], ["themeFiles", 3]]);
  assert.deepEqual(codes(s), []);
  assert.equal(s.attention[0].href, "/#/");
  assert.match(s.attention[0].text.en, /branch main/);
});

test("problems: no repository, a refused token, a missing repository, an unreachable host", async () => {
  assert.deepEqual(codes(await summary(undefined, makeEnv({ DESIGNER_GIT_REPO: undefined }))), ["repo_not_configured"]);
  for (const [status, code] of [[401, "repo_token_rejected"], [404, "repo_not_found"], [500, "repo_unreachable"]] as const) {
    const gh = await site();
    gh.override = () => new Response("{}", { status });
    assert.deepEqual(codes(await summary(gh)), [code], String(status));
  }
});

test("problems: the configuration unreadable or partly read, the overlay invalid, the theme not editable", async () => {
  assert.deepEqual(codes(await summary(await site({ ".ssg.yaml": null, ".ssg.toml": "x = 1\n" }))), ["config_unreadable", "theme_not_editable"]);
  assert.deepEqual(codes(await summary(await site({ ".ssg.yaml": "template: mytheme\nfeeds: &f []\n" }))), ["config_partly_read"]);
  assert.deepEqual(codes(await summary(await site({ ".ssg.designer.json": "nope" }))), ["overlay_invalid"]);
  assert.deepEqual(codes(await summary(await site({ ".ssg.designer.json": '{"port": "x"}' }))), ["overlay_invalid"]);
});

test("a failure that is not the repository's is not swallowed", async () => {
  const gh = await site();
  // A blob whose body breaks mid-read: not an answer of the host, so no RepoError.
  gh.override = (_m, path) => (path.startsWith("/git/blobs/") ? new Response(new ReadableStream({ pull: (c) => c.error(new TypeError("boom")) })) : undefined);
  const log = console.error;
  console.error = () => {};
  try {
    const res = await run(await request(BASE + "/api/module/summary", READ), makeEnv(), gh);
    assert.equal(res.status, 500);
  } finally {
    console.error = log;
  }
});

test("api/module, api/me, api/health", async () => {
  const gh = await site();
  assert.deepEqual((await get(gh, "/api/module")).body, manifest);
  const me = (await get(gh, "/api/me", ["designer-ui:read", "designer-ui:theme", "editor-ui:read"])).body;
  assert.deepEqual([me.kind, me.user, me.permissions, me.loginUrl, me.logoutUrl, me.refreshUrl], ["user", { id: "u-ada", email: "ada@example.com" }, [PERM.read, PERM.theme], "/auth/login", "/api/auth/logout", "/api/auth/refresh"]);
  const limiter = { limit: async () => ({ success: true }) };
  const health = await (await run(await request(BASE + "/api/health", READ), makeEnv({ RATE_LIMITER: limiter }))).json();
  assert.deepEqual(health, { ok: true, auth: "auth", repository: { configured: true, provider: "github", branch: "main" }, schemaVersion: 1, plan: { plan: "free", limits: PLAN_LIMITS.free, subrequests: 45, maxBody: 8 * MB }, bindings: { assets: true, rateLimiter: true } });
  const missing = await (await run(await request(BASE + "/api/health", READ), makeEnv({ DESIGNER_GIT_TOKEN: undefined, ASSETS: undefined }))).json();
  assert.deepEqual([missing.ok, missing.repository.problem, missing.bindings.assets], [false, "DESIGNER_GIT_TOKEN is not set (npx wrangler secret put DESIGNER_GIT_TOKEN)", false]);
  assert.ok(!JSON.stringify(health).includes(TOKEN));
  const noRepo = await run(await request(BASE + "/api/settings", READ), makeEnv({ DESIGNER_GIT_REPO: undefined }));
  assert.deepEqual([noRepo.status, (await noRepo.json()).error], [503, "repo_not_configured"]);
});
