// openapi.yaml describes exactly the routes in src/routes.ts: a route added
// without documenting it, or a documented one that no longer exists, fails
// here. No YAML library (no npm dependencies): the file keeps one fixed
// layout — paths at two spaces, methods at four — and this reads only that.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { ROUTES } from "../src/routes.ts";

const yaml = readFileSync(new URL("../openapi.yaml", import.meta.url), "utf8");

// operations lists "METHOD /path" for every operation under `paths:`, with
// OpenAPI's {param} written as the router's :param.
export function operations(text: string): string[] {
  const out: string[] = [];
  let inPaths = false;
  let path = "";
  for (const line of text.split("\n")) {
    if (/^\S/.test(line)) inPaths = line === "paths:";
    if (!inPaths) continue;
    const p = /^ {2}(\/\S*):\s*$/.exec(line);
    if (p) path = p[1].replace(/\{([A-Za-z]+)\}/g, ":$1");
    const m = /^ {4}(get|put|post|patch|delete|head|options):\s*$/.exec(line);
    if (m) out.push(`${m[1].toUpperCase()} ${path}`);
  }
  return out.sort();
}

test("openapi.yaml documents every route in ROUTES, and nothing else", () => {
  const documented = operations(yaml);
  const routed = ROUTES.map((r) => `${r.method} ${r.path}`).sort();
  assert.deepEqual(documented.filter((o) => !routed.includes(o)), [], "documented but not routed");
  assert.deepEqual(routed.filter((o) => !documented.includes(o)), [], "routed but not documented");
  assert.equal(new Set(documented).size, documented.length, "no operation twice");
});

test("openapi.yaml: 3.1, the CSRF header on every state-changing /api/ operation, hooks public", () => {
  assert.match(yaml, /^openapi: 3\.1\.\d+$/m);
  const blocks = yaml.split(/\n(?= {2}\/)/).slice(1);
  for (const block of blocks) {
    const path = /^ {2}(\S+):/.exec(block)?.[1] ?? "";
    for (const op of block.split(/\n(?= {4}(?:get|post|patch|delete|head):)/).slice(1)) {
      const method = op.trimStart().split(":")[0];
      if (path.startsWith("/api/") && !["get", "head"].includes(method)) {
        assert.match(op, /parameters\/Csrf/, `${method} ${path} lacks the CSRF header`);
      }
      if (path.startsWith("/hooks/")) assert.match(op, /security: \[\]/, `${method} ${path} must be public`);
    }
  }
});

test("operations(): reads only paths, ignores schema properties named like methods", () => {
  const text = "paths:\n  /a/{id}:\n    get:\n    post:\ncomponents:\n  /b:\n    get:\n";
  assert.deepEqual(operations(text), ["GET /a/:id", "POST /a/:id"]);
});

// permissions(): "METHOD /path" → the operation's x-permission, as written
// ("newsletter:read", "[newsletter:import, newsletter:suppress]", "session").
export function permissions(text: string): Map<string, string> {
  const out = new Map<string, string>();
  let [inPaths, path, op] = [false, "", ""];
  for (const line of text.split("\n")) {
    if (/^\S/.test(line)) inPaths = line === "paths:";
    if (!inPaths) continue;
    const p = /^ {2}(\/\S*):\s*$/.exec(line);
    if (p) path = p[1].replace(/\{([A-Za-z]+)\}/g, ":$1");
    const m = /^ {4}(get|put|post|patch|delete|head|options):\s*$/.exec(line);
    if (m) op = `${m[1].toUpperCase()} ${path}`;
    const x = /^ {6}x-permission: (.+)$/.exec(line);
    if (x) out.set(op, x[1].trim());
  }
  return out;
}

test("openapi.yaml: each operation's x-permission is the registry's (GO-137)", () => {
  const documented = permissions(yaml);
  for (const r of ROUTES) {
    const want = typeof r.permission === "string" ? r.permission : `[${r.permission.join(", ")}]`;
    assert.equal(documented.get(`${r.method} ${r.path}`), want, `${r.method} ${r.path}`);
  }
  assert.equal(documented.size, ROUTES.length, "one x-permission per operation");
  assert.match(yaml, /authSession:\n {6}type: apiKey\n {6}in: cookie\n {6}name: __Host-ssg-auth/);
});

test("permissions(): reads x-permission per operation, only under paths", () => {
  const text = "paths:\n  /a:\n    get:\n      x-permission: newsletter:read\n    post:\n      x-permission: [a:b, c:d]\ncomponents:\n  /b:\n    get:\n      x-permission: x\n";
  assert.deepEqual([...permissions(text)], [["GET /a", "newsletter:read"], ["POST /a", "[a:b, c:d]"]]);
});
