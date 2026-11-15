// The template's own files agree with the code: openapi.yaml lists exactly
// the routes, PERM is module.json's list, the wrangler file keeps the token a
// secret and every request through the worker, the panel page needs no
// inline code (the CSP forbids it).
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import manifest from "../module.json" with { type: "json" };
import { DECLARED, PERM, held } from "../src/permissions.ts";
import { ROUTES } from "../src/routes.ts";
import { CSP, DEFAULT_BASE } from "../src/http.ts";

const read = (p: string) => readFileSync(new URL(p, import.meta.url), "utf8");

// operations lists "METHOD /path" under `paths:` (paths at two spaces,
// methods at four — the layout workers/modules.test.ts reads too).
export function operations(text: string): string[] {
  const out: string[] = [];
  let inPaths = false;
  let path = "";
  for (const line of text.split("\n")) {
    if (/^\S/.test(line)) inPaths = line === "paths:";
    if (!inPaths) continue;
    const p = /^ {2}(\/\S*):\s*$/.exec(line);
    if (p) path = p[1].replace(/\{([A-Za-z]+)\}/g, ":$1");
    const m = /^ {4}(get|put|post|patch|delete):\s*$/.exec(line);
    if (m) out.push(`${m[1].toUpperCase()} ${path}`);
  }
  return out.sort();
}

test("openapi.yaml documents every route and nothing else, each with the operation's permission", () => {
  const yaml = read("../openapi.yaml");
  assert.deepEqual(operations(yaml), ROUTES.map((r) => `${r.method} ${r.path}`).sort());
  for (const r of ROUTES) {
    const at = r.path.replace(/:([A-Za-z]+)/g, "{$1}");
    const block = yaml.slice(yaml.indexOf(`\n  ${at}:\n`) + 1).split(/\n {2}\//)[0];
    assert.match(block, new RegExp(`x-permission: ${r.op.permission}\\n`), r.path);
  }
  assert.match(yaml, /^openapi: 3\.1\.\d+$/m);
  assert.match(yaml, new RegExp(`^  version: ${manifest.version}$`, "m"), "the API version is the module's");
});

test("PERM is module.json's permission list, and held() keeps only those", () => {
  assert.deepEqual(Object.values(PERM), DECLARED);
  assert.deepEqual(held(["designer-ui:read", "editor-ui:read", "designer-ui:nope"]), ["designer-ui:read"]);
  assert.deepEqual(held(["*"]), DECLARED);
  assert.deepEqual(held(["designer-ui:*"]), DECLARED);
  for (const n of manifest.nav) assert.ok(DECLARED.includes(n.permission));
});

test("the two sensitive permissions say so in every language's description", () => {
  for (const p of manifest.permissions.filter((x) => x.name === PERM.advanced || x.name === PERM.theme)) {
    assert.match(p.description.en, /^Sensitive:/);
    assert.match(p.description.pl, /^Wrażliwe:/);
  }
});

test("wrangler.toml: the base, the token only as a secret, every request through the worker, placeholders", () => {
  const toml = read("../wrangler.toml");
  assert.match(toml, new RegExp(`^DESIGNER_BASE = "${DEFAULT_BASE}"$`, "m"));
  assert.match(toml, /^run_worker_first = true$/m);
  assert.match(toml, /^main = "src\/index\.ts"$/m);
  assert.doesNotMatch(toml, /^DESIGNER_GIT_TOKEN\s*=/m, "the repository token is never a [vars] entry");
  assert.match(toml, /npx wrangler secret put DESIGNER_GIT_TOKEN/);
  for (const v of ["DESIGNER_GIT_REPO", "DESIGNER_GIT_BRANCH", "DESIGNER_AUTH_ISSUER", "DESIGNER_AUTH_LOGIN_URL", "DESIGNER_AUTH_PUBLIC_KEY", "DESIGNER_AUTH_JWKS_URL", "DESIGNER_AUTH_AUDIENCE"]) {
    assert.match(toml, new RegExp(`\\b${v}\\b`), v);
  }
  assert.match(toml, /<example\.com>/);
});

test("the panel page: no inline script or style, never indexed, the shared navigation", () => {
  const html = read("../public/index.html");
  assert.doesNotMatch(html, /<script(?![^>]*\bsrc=)[^>]*>/, "no inline script");
  assert.doesNotMatch(html, /<style|\sstyle=|\son[a-z]+=/i, "no inline style or handler");
  assert.match(html, /<meta name="robots" content="noindex, nofollow, noarchive">/);
  assert.match(html, /<ssg-admin-nav current="designer-ui">/);
  assert.match(html, /<html lang="en">/);
  assert.match(CSP, /script-src 'self'; style-src 'self'/);
});
