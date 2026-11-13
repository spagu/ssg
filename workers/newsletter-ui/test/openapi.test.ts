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
