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
import { CSP } from "../portable/http-kit.ts";
import { DEFAULT_BASE } from "../src/http.ts";

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
    const at = yaml.indexOf(`\n  ${r.path.replace(/:([A-Za-z]+)/g, "{$1}")}:\n`);
    assert.ok(at >= 0, r.path);
    const block = yaml.slice(at + 1).split(/\n {2}\//)[0];
    assert.match(block, new RegExp(`x-permission: ${r.op.permission}\\n`), r.path);
  }
  assert.match(yaml, /^openapi: 3\.1\.\d+$/m);
  assert.match(yaml, new RegExp(`^  version: ${manifest.version}$`, "m"), "the API version is the module's");
});

test("PERM is module.json's permission list, and held() keeps only those", () => {
  assert.deepEqual(Object.values(PERM), DECLARED);
  assert.deepEqual(held(["editor-ui:read", "newsletter:read", "editor-ui:nope"]), ["editor-ui:read"]);
  assert.deepEqual(held(["*"]), DECLARED);
  for (const n of manifest.nav) assert.ok(DECLARED.includes(n.permission));
});

test("wrangler.toml: the base, the token only as a secret, every request through the worker, placeholders", () => {
  const toml = read("../wrangler.toml");
  assert.match(toml, new RegExp(`^EDITOR_BASE = "${DEFAULT_BASE}"$`, "m"));
  assert.match(toml, /^run_worker_first = true$/m);
  assert.match(toml, /^main = "src\/index\.ts"$/m);
  assert.doesNotMatch(toml, /^EDITOR_GIT_TOKEN\s*=/m, "the repository token is never a [vars] entry");
  assert.match(toml, /npx wrangler secret put EDITOR_GIT_TOKEN/);
  for (const v of ["EDITOR_GIT_REPO", "EDITOR_GIT_BRANCH", "EDITOR_AUTH_ISSUER", "EDITOR_AUTH_LOGIN_URL", "EDITOR_AUTH_PUBLIC_KEY", "EDITOR_AUTH_JWKS_URL", "EDITOR_AUTH_AUDIENCE", "EDITOR_CONTENT_DIRS", "EDITOR_MEDIA_DIR"]) {
    assert.match(toml, new RegExp(`\\b${v}\\b`), v);
  }
  assert.match(toml, /<example\.com>/);
});

test("the panel page: no inline script or style, never indexed, the shared navigation", () => {
  const html = read("../public/index.html");
  assert.doesNotMatch(html, /<script(?![^>]*\bsrc=)[^>]*>/, "no inline script");
  assert.doesNotMatch(html, /<style|\sstyle=|\son[a-z]+=/i, "no inline style or handler");
  assert.match(html, /<meta name="robots" content="noindex, nofollow, noarchive">/);
  assert.match(html, /<ssg-admin-nav current="editor-ui">/);
  assert.match(html, /<html lang="en">/);
  assert.match(CSP, /script-src 'self'; style-src 'self'/);
});
