// The router: path params, 404, method mismatch. Run from the repository root:
//   node --import ./workers/newsletter-ui/test/resolve.mjs --test 'workers/newsletter-ui/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { match, matchPath } from "../src/router.ts";
import type { Route } from "../src/router.ts";
import { ROUTES } from "../src/routes.ts";

const ok = () => new Response("ok");
const table: Route[] = [
  { method: "GET", path: "/api/campaigns", handler: ok },
  { method: "GET", path: "/api/campaigns/:id", handler: ok },
  { method: "PATCH", path: "/api/campaigns/:id", handler: ok },
  { method: "POST", path: "/api/campaigns/:id/sends/:sendId", handler: ok },
];

test("matchPath captures and decodes params", () => {
  assert.deepEqual(matchPath("/api/campaigns/:id", "/api/campaigns/abc%20d"), { id: "abc d" });
  assert.deepEqual(matchPath("/api/campaigns/:id/sends/:sendId", "/api/campaigns/1/sends/2"), { id: "1", sendId: "2" });
  assert.deepEqual(matchPath("/api/health", "/api/health"), {});
});

test("matchPath refuses other lengths, literals, empty and malformed params", () => {
  assert.equal(matchPath("/api/campaigns/:id", "/api/campaigns"), null);
  assert.equal(matchPath("/api/campaigns/:id", "/api/campaigns/1/x"), null);
  assert.equal(matchPath("/api/campaigns/:id", "/api/other/1"), null);
  assert.equal(matchPath("/api/campaigns/:id", "/api/campaigns/"), null);
  assert.equal(matchPath("/api/campaigns/:id", "/api/campaigns/%E0%A4%A"), null);
});

test("match finds the route for the method", () => {
  const m = match(table, "PATCH", "/api/campaigns/7");
  assert.equal(m.kind, "found");
  if (m.kind === "found") {
    assert.equal(m.route.method, "PATCH");
    assert.deepEqual(m.params, { id: "7" });
  }
});

test("match reports a known path with the wrong method, with Allow", () => {
  assert.deepEqual(match(table, "DELETE", "/api/campaigns/7"), { kind: "method", allow: ["GET", "PATCH"] });
});

test("match reports an unknown path", () => {
  assert.deepEqual(match(table, "GET", "/api/nope"), { kind: "none" });
});

test("the registry has health, open without subscribers", () => {
  const m = match(ROUTES, "GET", "/api/health");
  assert.equal(m.kind, "found");
  if (m.kind === "found") assert.equal(m.route.needsSubscribers, false);
});
