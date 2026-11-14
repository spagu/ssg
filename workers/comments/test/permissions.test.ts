// The permission table of auth mode (GO-136), generated from OPERATIONS — the
// one source of truth in _admin-auth.ts. For every operation: no session → the
// verifier's 401 (and, for a GET a browser opens, the redirect to the login
// page); a session holding every OTHER declared permission → 403 naming the
// missing one; a session holding only this one (or comments:*, or *) → works.
import { test } from "node:test";
import assert from "node:assert/strict";
import * as admin from "../functions/api/comments/admin.ts";
import * as importer from "../functions/api/comments/import.ts";
import * as manifest from "../functions/api/comments/module/index.ts";
import * as summary from "../functions/api/comments/module/summary.ts";
import { OPERATIONS, PERMISSIONS } from "../functions/api/comments/_admin-auth.ts";
import type { Operation } from "../functions/api/comments/_admin-auth.ts";
import { call, seed, sqliteD1 } from "./harness.ts";
import { authEnv, mint, session } from "./auth-tokens.ts";

type Handler = Parameters<typeof call>[0];
interface Route { handler: Handler; method: "GET" | "POST"; path: string; body?: unknown }

// ROUTES says how each operation is called; a new operation without a route fails below.
const ROUTES: Record<Operation, Route> = {
  list: { handler: admin.onRequestGet, method: "GET", path: "/api/comments/admin?status=pending" },
  approve: { handler: admin.onRequestPost, method: "POST", path: "/api/comments/admin", body: { id: "c1", action: "approve" } },
  spam: { handler: admin.onRequestPost, method: "POST", path: "/api/comments/admin", body: { id: "c1", action: "spam" } },
  delete: { handler: admin.onRequestPost, method: "POST", path: "/api/comments/admin", body: { id: "c1", action: "delete" } },
  purge: { handler: admin.onRequestPost, method: "POST", path: "/api/comments/admin", body: { action: "purge" } },
  import: { handler: importer.onRequestPost, method: "POST", path: "/api/comments/import", body: { items: [{ url: "/a/", author: "Ada", body: "Hi" }] } },
  manifest: { handler: manifest.onRequestGet, method: "GET", path: "/api/comments/module" },
  summary: { handler: summary.onRequestGet, method: "GET", path: "/api/comments/module/summary" },
};

const env = () => {
  const db = sqliteD1();
  seed(db, [{ id: "c1", status: "pending", created_at: "2000-01-01T00:00:00.000Z" }]);
  return authEnv({ COMMENTS_DB: db, COMMENTS_RETENTION_DAYS: "30", COMMENTS_TURNSTILE_SECRET: "ts" });
};
const run = (op: Operation, headers: Record<string, string>) => {
  const r = ROUTES[op];
  const write: Record<string, string> = r.method === "POST" ? { "content-type": "application/json" } : {};
  return call(r.handler, { method: r.method, path: r.path, headers: { ...write, ...headers }, body: r.body === undefined ? undefined : JSON.stringify(r.body), env: env() });
};

test("permissions: every operation has a route and a declared permission", () => {
  assert.deepEqual(Object.keys(ROUTES).sort(), Object.keys(OPERATIONS).sort());
  for (const p of Object.values(OPERATIONS)) assert.ok((PERMISSIONS as readonly string[]).includes(p), p);
  assert.deepEqual(new Set(Object.values(OPERATIONS)), new Set(PERMISSIONS), "every declared permission guards something");
});

for (const [op, permission] of Object.entries(OPERATIONS) as [Operation, string][]) {
  const route = ROUTES[op];
  const write = route.method === "POST";

  test(`permissions: ${op} without a session → 401 (API)${write ? "" : " / login redirect (page)"}`, async () => {
    const api = await run(op, {});
    assert.equal(api.status, 401);
    assert.deepEqual(JSON.parse(api.body), { error: "unauthorized", reason: "no_session" });
    assert.ok(api.headers.some(([k, v]) => k === "www-authenticate" && v === 'Bearer realm="ssg-auth"'));
    if (write) return;
    const page = await run(op, { accept: "text/html" });
    assert.equal(page.status, 302);
    const to = new URL(page.headers.find(([k]) => k === "location")![1]);
    assert.equal(to.origin + to.pathname, "https://example.com/auth/login");
    assert.equal(to.searchParams.get("next"), route.path);
  });

  test(`permissions: ${op} with every permission but ${permission} → 403 naming it`, async () => {
    const others = PERMISSIONS.filter((p) => p !== permission);
    const got = await run(op, session(await mint(others), write));
    assert.equal(got.status, 403);
    assert.deepEqual(JSON.parse(got.body), { error: "forbidden", permission });
  });

  test(`permissions: ${op} with ${permission} alone, comments:* or * → works`, async () => {
    for (const perms of [[permission], ["comments:*"], ["*"]]) {
      const got = await run(op, session(await mint(perms), write));
      assert.equal(got.status, 200, `${perms}: ${got.body}`);
    }
  });
}
