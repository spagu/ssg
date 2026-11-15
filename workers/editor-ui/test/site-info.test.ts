// GET /api/site (GO-173): the sections a new article can go in, languages,
// shortcode names, the media folder and limits — for a repository site, an
// MDDB site with and without a repository, and a site whose images have no
// published folder.
import { test } from "node:test";
import assert from "node:assert/strict";
import { BASE, makeEnv, request, run, site } from "./fixtures.ts";
import { MDDB_VARS, bothEnv, mddbOnlyEnv, world, worldFetch } from "./mddb-fixtures.ts";
import { handleFetch } from "../portable/app.ts";
import { PERM } from "../src/permissions.ts";
import { PLAN_LIMITS } from "../portable/repo-provider.ts";

const READ = [PERM.read];

test("a repository site: sections then loose content folders, languages, shortcodes, media and limits", async () => {
  const gh = await site();
  const res = await run(await request(BASE + "/api/site", READ), makeEnv(), gh);
  assert.equal(res.status, 200);
  const body = await res.json();
  assert.deepEqual(body.sections.map((s: { path: string }) => s.path), ["content/blog/pages", "content/blog/posts", "docs", "content/blog"]);
  assert.ok(body.sections.every((s: { origin: string }) => s.origin === "git"));
  assert.deepEqual(body.sections.find((s: { path: string }) => s.path === "content/blog").type, null);
  assert.deepEqual([body.languages, body.defaultLanguage, body.shortcodes], [["en", "pl"], "en", ["note"]]);
  assert.deepEqual(body.media, { dir: "content/blog/media", urlBase: "/media/" });
  assert.deepEqual([body.plan, body.limits], ["free", PLAN_LIMITS.free]);
  const paid = await (await run(await request(BASE + "/api/site", READ), makeEnv({ EDITOR_PLAN: "paid" }), gh)).json();
  assert.deepEqual([paid.plan, paid.limits], ["paid", PLAN_LIMITS.paid]);
});

test("a media folder ssg does not publish gives media: null", async () => {
  const gh = await site();
  const body = await (await run(await request(BASE + "/api/site", READ), makeEnv({ EDITOR_MEDIA_DIR: "uploads" }), gh)).json();
  assert.equal(body.media, null);
});

test("MDDB beside the repository adds one section per collection; MDDB alone answers only those", async () => {
  const w = await world();
  const both = await handleFetch(await request(BASE + "/api/site", READ), bothEnv() as never, { fetch: worldFetch(w) });
  const paths = (await both.json()).sections.map((s: { path: string; origin: string }) => `${s.origin}:${s.path}`);
  assert.deepEqual(paths.filter((p: string) => p.startsWith("mddb:mddb:")), ["mddb:mddb:blog", "mddb:mddb:kb"]);
  const alone = await world({ git: false });
  const res = await handleFetch(await request(BASE + "/api/site", READ), mddbOnlyEnv() as never, { fetch: worldFetch(alone) });
  assert.deepEqual(await res.json(), { sections: [{ path: "mddb:blog", type: null, origin: "mddb" }], languages: [], defaultLanguage: null, shortcodes: [], media: null, plan: "free", limits: PLAN_LIMITS.free });
  const twice = await handleFetch(await request(BASE + "/api/site", READ), mddbOnlyEnv({ EDITOR_MDDB_URL: "ftp://x" }) as never, { fetch: worldFetch(alone) });
  assert.deepEqual((await twice.json()).sections, [], "MDDB set up wrongly: no sections, the list says why");
});

test("neither a repository nor MDDB: 503 naming the repository settings; MDDB_VARS alone are not a repository", async () => {
  const res = await run(await request(BASE + "/api/site", READ), makeEnv({ EDITOR_GIT_REPO: undefined, EDITOR_GIT_TOKEN: undefined }));
  assert.equal(res.status, 503);
  assert.equal((await res.json()).error, "repo_not_configured");
  assert.ok(MDDB_VARS.EDITOR_MDDB_URL);
});
