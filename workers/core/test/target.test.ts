// Where requests and links may point, and the registry as the build writes
// it. The site build puts functions/_ssg-modules.json next to api/; here a
// resolve hook stands in for that file with test/fixtures/ssg-modules.json
// (only in this test process), so the real import in _registry.ts runs.
import { test } from "node:test";
import assert from "node:assert/strict";
import { registerHooks } from "node:module";
import { cleanBase, joinPath, targetURL } from "../functions/api/core/_target.ts";

const REGISTRY_TS = new URL("../functions/api/core/_registry.ts", import.meta.url).href;
const FIXTURE = new URL("fixtures/ssg-modules.json", import.meta.url).href;
registerHooks({
  resolve(specifier, context, next) {
    if (specifier === "../../_ssg-modules.json" && context.parentURL === REGISTRY_TS) return { url: FIXTURE, format: "json", shortCircuit: true };
    return next(specifier, context);
  },
});
const { loadRegistry } = await import("../functions/api/core/_registry.ts");

test("the registry the build writes is imported and read", async () => {
  assert.deepEqual(await loadRegistry(), {
    ok: true,
    registry: {
      format: 1,
      modules: [{ id: "shop", kind: null, base: "/shop", bundled: false, version: null, manifest: null }],
      available: [{ id: "contact-form", kind: "pages-function", title: "Contact form" }],
      theme: null,
    },
  });
});

test("cleanBase: the root, plain paths with or without a trailing slash; nothing else", () => {
  const cases: [string, string | null][] = [
    ["/", ""],
    ["/shop", "/shop"],
    ["/shop/", "/shop"],
    ["/a/b.c~d_e-f", "/a/b.c~d_e-f"],
    ["shop", null],
    ["//evil.example.com", null],
    ["/a/../b", null],
    ["/a/./b", null],
    ["/a//b", null],
    ["/a\\b", null],
    ["https://evil.example.com", null],
    ["/a b", null],
    ["/a?x=1", null],
  ];
  for (const [base, want] of cases) assert.equal(cleanBase(base), want, base);
});

test("joinPath keeps a #fragment; refuses a path that could leave the host", () => {
  assert.equal(joinPath("/", "/comments-admin.html"), "/comments-admin.html");
  assert.equal(joinPath("/newsletter-admin", "/#/campaigns"), "/newsletter-admin/#/campaigns");
  for (const path of ["//evil.example.com", "relative", "/a\\b", "/a b"]) assert.equal(joinPath("/", path), null, path);
  assert.equal(joinPath("x", "/a"), null);
});

test("targetURL: the caller's origin and exactly the joined path, or nothing", () => {
  const origin = "https://example.com";
  assert.equal(targetURL(origin, "/shop", "/api/module")?.href, "https://example.com/shop/api/module");
  for (const [base, path] of [
    ["/", "/api/x?y=1"],
    ["/", "/api/x#y"],
    ["/", "/api/é"],
    ["/", "/api/%2e%2e/x"],
    ["//evil.example.com", "/x"],
  ]) {
    assert.equal(targetURL(origin, base, path), null, `${base} ${path}`);
  }
});
