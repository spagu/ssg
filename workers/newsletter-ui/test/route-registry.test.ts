// GO-137: the route registry states, for every route, who may call it — and
// the table below is generated from it: every route × {no credentials, a
// session without the permission, a session with exactly that permission, the
// owner `*`}. A new route without a permission, or with one module.json does
// not declare, fails here; so does a state-changing route an account holding
// only newsletter:read could reach. Handlers are stubbed: this is the gate.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { handleFetch } from "../src/app.ts";
import { DECLARED, PUBLIC, SESSION, missing, needed } from "../src/permissions.ts";
import type { Route } from "../src/router.ts";
import { ROUTES } from "../src/routes.ts";
import { authEnv, session } from "./auth-fixtures.ts";
import { fakeCtx, req } from "./fakes.ts";

const B = "/newsletter-admin";
const manifest = JSON.parse(readFileSync(new URL("../module.json", import.meta.url), "utf8"));
const READ = "newsletter:read";
const changes = (r: Route) => !["GET", "HEAD"].includes(r.method);
const stubbed: Route[] = ROUTES.map((r) => ({ ...r, handler: () => Response.json({ ok: true }) }));
const pathOf = (r: Route) => B + r.path.replace(/:[a-z]+/gi, "x1");

async function status(r: Route, perms: string[] | null): Promise<number> {
  const init = { method: r.method, body: changes(r) ? "{}" : undefined, headers: { "content-type": "application/json" } };
  const request = perms === null ? req(pathOf(r), init) : await session(pathOf(r), perms, init);
  return (await handleFetch(request, authEnv() as never, fakeCtx() as never, stubbed)).status;
}

test("registry: every route states its permission, from module.json or a marker", () => {
  assert.deepEqual([...DECLARED], manifest.permissions.map((p: { name: string }) => p.name));
  for (const r of ROUTES) {
    const where = `${r.method} ${r.path}`;
    assert.ok(r.permission !== undefined && r.permission.length > 0, `${where} has no permission`);
    const list = typeof r.permission === "string" ? [r.permission] : [...r.permission];
    for (const p of list) assert.ok(DECLARED.includes(p) || p === SESSION || p === PUBLIC, `${where}: ${p} is not declared in module.json`);
    assert.equal(r.path.startsWith("/hooks/"), r.permission === PUBLIC, `${where}: PUBLIC is for /hooks/ only, and every hook is PUBLIC`);
  }
  assert.deepEqual(ROUTES.filter((r) => r.permission === SESSION).map((r) => r.path), ["/api/me"], "SESSION stays rare");
});

test("registry: every state-changing /api/ route names its audit action or says it changes nothing", () => {
  const actions = new Set<string>();
  for (const r of ROUTES.filter((x) => x.path.startsWith("/api/"))) {
    const where = `${r.method} ${r.path}`;
    if (!changes(r)) {
      assert.equal(r.audit, undefined, `${where}: a read is never audited`);
      continue;
    }
    assert.ok(r.audit === false || (typeof r.audit === "string" && /^[a-z]+\.[a-z_]+$/.test(r.audit)), `${where} needs audit: "<thing>.<verb>" or false`);
    if (typeof r.audit === "string") {
      assert.ok(!actions.has(r.audit), `${r.audit} twice`);
      actions.add(r.audit);
      assert.ok(!needed(r.permission).every((p) => p === READ), `${where} changes state with newsletter:read`);
    }
  }
});

test("an account holding only newsletter:read cannot call any state-changing route", async () => {
  const changing = ROUTES.filter((r) => typeof r.audit === "string");
  assert.ok(changing.length >= 20);
  for (const r of changing) assert.equal(await status(r, [READ]), 403, `${r.method} ${r.path}`);
});

// The generated table. Expected: hooks are public; SESSION routes need a
// session only; everything else is 401 / 403 / allowed / allowed.
for (const r of ROUTES) {
  const need = needed(r.permission);
  const name = `${r.method} ${r.path} [${typeof r.permission === "string" ? r.permission : r.permission.join(" + ")}]`;
  test(`matrix: ${name}`, async () => {
    if (r.permission === PUBLIC) {
      assert.equal(await status(r, null), 200, "no credentials");
      return;
    }
    assert.equal(await status(r, null), 401, "no credentials");
    const other = await status(r, ["comments:moderate"]);
    assert.equal(other, need.length ? 403 : 200, "a session without the permission");
    assert.equal(await status(r, need), 200, "a session with exactly the permission");
    assert.equal(await status(r, ["*"]), 200, "the owner");
    for (const p of need) {
      if (need.length > 1) assert.equal(await status(r, need.filter((x) => x !== p)), 403, `without ${p}`);
      assert.equal(await status(r, [p.split(":")[0] + ":*"]), 200, "the worker wildcard");
    }
  });
}

test("a missing permission is answered with its name", async () => {
  const route = ROUTES.find((r) => r.path === "/api/campaigns/:id/schedule") as Route;
  const res = await handleFetch(await session(pathOf(route), [READ], { method: "POST", body: "{}" }), authEnv() as never, fakeCtx() as never, stubbed);
  assert.equal(res.status, 403);
  assert.deepEqual(await res.json(), { error: "forbidden", permission: "newsletter:send" });
  assert.equal(res.headers.get("x-content-type-options"), "nosniff");
});

test("missing(): all-granting modes, an undeclared need fails closed, several needs", () => {
  const all = { mode: "password" as const, user: null, perms: "all" as const };
  const ada = { mode: "auth" as const, user: { id: "u", email: "u@example.com" }, perms: ["newsletter:import"] };
  assert.equal(missing(all, undefined), null);
  assert.equal(missing(ada, undefined), "undeclared");
  assert.equal(missing(ada, ["newsletter:import", "newsletter:suppress"]), "newsletter:suppress");
  assert.equal(missing(ada, SESSION), null);
});
