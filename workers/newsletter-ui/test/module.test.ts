// GET {base}/api/module (GO-143): the manifest behind the same gates as every
// /api/ route, byte for byte module.json — and module.json agrees with the
// panel it describes: the views its navigation names exist, their labels are
// the panel's own words, and the permissions are the split GO-137 enforces.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { handleFetch } from "../src/app.ts";
import { ROUTES } from "../src/routes.ts";
import { ROUTES as VIEWS, parse } from "../public/js/core/router.js";
import { PASSWORD, fakeCtx, fakeD1, makeEnv, req } from "./fakes.ts";

const B = "/newsletter-admin";
const manifest = JSON.parse(readFileSync(new URL("../module.json", import.meta.url), "utf8"));
const run = (r: Request, env: Record<string, unknown> = makeEnv()) => handleFetch(r, env as never, fakeCtx() as never);

test("module: with admin credentials the manifest is module.json, with the panel's security headers", async () => {
  const res = await run(req(B + "/api/module", { auth: PASSWORD }));
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("content-type"), "application/json; charset=utf-8");
  assert.equal(res.headers.get("cache-control"), "no-store");
  assert.equal(res.headers.get("x-robots-tag"), "noindex, nofollow");
  assert.deepEqual(await res.json(), manifest);
});

test("module: refused exactly as /api/health is — no credentials, wrong ones, no auth configured", async () => {
  const cases: [string, Record<string, unknown>, string | undefined][] = [
    ["no credentials", makeEnv(), undefined],
    ["wrong password", makeEnv(), "wrong"],
    ["no admin auth configured", makeEnv({ NEWSLETTER_ADMIN_PASSWORD: undefined }), PASSWORD],
  ];
  for (const [name, env, auth] of cases) {
    const health = await run(req(B + "/api/health", { auth }), env);
    const mod = await run(req(B + "/api/module", { auth }), env);
    assert.notEqual(mod.status, 200, name);
    assert.equal(mod.status, health.status, name);
    assert.deepEqual([...mod.headers].sort(), [...health.headers].sort(), name);
    assert.equal(await mod.text(), await health.text(), name);
  }
});

test("module: answers without the subscribers table; other methods are 405", async () => {
  const bare = makeEnv({ NEWSLETTER_DB: fakeD1() });
  assert.equal((await run(req(B + "/api/module", { auth: PASSWORD }), bare)).status, 200);
  const post = await run(req(B + "/api/module", { method: "POST", auth: PASSWORD }));
  assert.equal(post.status, 405);
  assert.equal(post.headers.get("allow"), "GET");
});

test("module.json: every endpoint is a GET route that needs no subscribers table", () => {
  for (const path of Object.values(manifest.endpoints)) {
    const route = ROUTES.find((r) => r.method === "GET" && r.path === path);
    assert.ok(route, `${path} is not a GET route`);
    assert.equal(route.needsSubscribers, false, `${path} must answer on a fresh install`);
  }
});

test("module.json: each nav entry opens a panel view and is labelled as the panel labels it", async () => {
  const views = new Set(VIEWS.map(([name]: [string]) => name));
  for (const item of manifest.nav) {
    const view = parse(item.path.slice(item.path.indexOf("#")));
    assert.ok(views.has(view.name), `${item.path} opens no view`);
    for (const lang of Object.keys(item.label)) {
      const strings = (await import(`../public/js/strings/${lang}.js`)).default;
      assert.equal(item.label[lang], strings[`nav.${view.section}`], `${item.path} (${lang})`);
    }
  }
  const title = Object.fromEntries(await Promise.all(Object.keys(manifest.title).map(async (l) => [l, (await import(`../public/js/strings/${l}.js`)).default["app.name"]])));
  assert.deepEqual(manifest.title, title, "the title is the panel's own name");
});

test("module.json: the permissions are GO-137's split, one per level of trust", () => {
  assert.deepEqual(
    manifest.permissions.map((p: { name: string }) => p.name),
    ["newsletter:read", "newsletter:edit", "newsletter:send", "newsletter:import", "newsletter:export", "newsletter:erase", "newsletter:suppress", "newsletter:settings"],
  );
});
