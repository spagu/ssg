// What every core response carries, whatever produced it; the CSRF header
// rule; methods; the registry's absence and shape; the manifest route.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import * as modules from "../functions/api/core/modules.ts";
import * as status from "../functions/api/core/status.ts";
import * as mod from "../functions/api/core/module/index.ts";
import { SECURITY_HEADERS, intVar } from "../functions/api/core/_lib.ts";
import { checkRegistry, loadRegistry, useRegistry } from "../functions/api/core/_registry.ts";
import { route } from "../functions/api/core/_route.ts";
import { PASSWORD, PW_ENV, basic, call, freshIP, registry } from "./harness.ts";

const auth = () => ({ authorization: basic(PASSWORD), "cf-connecting-ip": freshIP() });
const assertSecure = (headers: Record<string, string>, what: string) => {
  for (const [k, v] of Object.entries(SECURITY_HEADERS)) assert.equal(headers[k], v, `${what}: ${k}`);
  assert.equal(headers["cache-control"], "no-store");
  assert.match(headers["x-robots-tag"], /noindex/);
};

test("registry missing (the template directory has none) → 503 registry_missing with the fix, on both routes", async () => {
  useRegistry();
  for (const handler of [modules.onRequest, status.onRequest]) {
    const r = await call(handler, { path: "/api/core/x", headers: auth(), env: PW_ENV });
    assert.equal(r.status, 503);
    assert.equal(r.json().error, "registry_missing");
    assert.match(r.json().detail, /workers\/core under workers:/);
    assertSecure(r.headers, "registry_missing");
  }
});

test("a registry that is not one, or of a newer format, is refused with what to do", async () => {
  const cases: [unknown, string, RegExp][] = [
    [null, "registry_invalid", /not a module registry/],
    [{ format: "1", modules: [] }, "registry_invalid", /not a module registry/],
    [{ format: 2, modules: [] }, "registry_unsupported", /format 2, this core reads format 1 at most/],
    [{ format: 1, modules: {} }, "registry_invalid", /modules is not a list/],
    [{ format: 1, modules: [{ id: "Bad" }] }, "registry_invalid", /modules\[0\] has no valid id/],
    [{ format: 1, modules: [7] }, "registry_invalid", /modules\[0\] has no valid id/],
    [{ format: 1, modules: [{ id: "x", base: 1 }] }, "registry_invalid", /modules\[0\] \(x\) has no base path/],
  ];
  try {
    for (const [raw, error, detail] of cases) {
      useRegistry(raw);
      const r = await call(modules.onRequest, { path: "/api/core/modules", headers: auth(), env: PW_ENV });
      assert.deepEqual([r.status, r.json().error], [503, error]);
      assert.match(r.json().detail, detail);
      assert.match(r.json().detail, /rebuild the site/);
    }
  } finally {
    useRegistry();
  }
});

test("checkRegistry normalises entries and ignores fields it does not know", async () => {
  assert.deepEqual(checkRegistry({ format: 1, modules: [{ id: "a", base: "/", later: 1 }], available: [], theme: null }), {
    format: 1,
    modules: [{ id: "a", kind: null, base: "/", bundled: false, version: null, manifest: null }],
    available: [],
    theme: null,
  });
  useRegistry(registry());
  assert.deepEqual(await loadRegistry(), { ok: true, registry: { format: 1, modules: [], available: [], theme: null } });
  useRegistry();
});

test("every answer carries the security headers: success, refusal, misconfiguration, method, CSRF, crash", async () => {
  const boom = route({
    GET: async () => {
      throw new Error("secret detail");
    },
  });
  const cases: [Parameters<typeof call>[0], Parameters<typeof call>[1], number, string | null][] = [
    [mod.onRequest, { path: "/api/core/module", headers: auth(), env: PW_ENV }, 200, null],
    [mod.onRequest, { method: "HEAD", path: "/api/core/module", headers: auth(), env: PW_ENV }, 200, null],
    [mod.onRequest, { path: "/api/core/module", env: PW_ENV }, 401, "unauthorized"],
    [mod.onRequest, { path: "/api/core/module", env: {} }, 503, "signin_not_configured"],
    [mod.onRequest, { method: "POST", path: "/api/core/module", headers: auth(), env: PW_ENV }, 403, "csrf_header_missing"],
    [mod.onRequest, { method: "DELETE", path: "/api/core/module", headers: { ...auth(), "x-core-admin": "yes" }, env: PW_ENV }, 403, "csrf_header_missing"],
    [mod.onRequest, { method: "POST", path: "/api/core/module", headers: { ...auth(), "x-core-admin": "1" }, env: PW_ENV }, 405, "method_not_allowed"],
    [boom, { path: "/api/core/boom", headers: auth(), env: PW_ENV }, 500, "internal_error"],
  ];
  for (const [handler, init, want, error] of cases) {
    const r = await call(handler, init);
    assert.equal(r.status, want, `${init.method ?? "GET"} ${init.path}`);
    assertSecure(r.headers, `${want}`);
    if (error) assert.equal(r.json().error, error);
    if (want === 405) assert.equal(r.headers.allow, "GET");
    if (want === 500) assert.doesNotMatch(r.body, /secret detail/);
  }
});

test("a state-changing route runs only with the CSRF header, after sign-in", async () => {
  const posted = route({ POST: async () => new Response("done") });
  const r = await call(posted, { method: "POST", path: "/api/core/x", headers: { ...auth(), "x-core-admin": "1" }, env: PW_ENV });
  assert.deepEqual([r.status, r.body], [200, "done"]);
  assertSecure(r.headers, "plain handler response");
  const anon = await call(posted, { method: "POST", path: "/api/core/x", headers: { "x-core-admin": "1" }, env: PW_ENV });
  assert.equal(anon.status, 401, "the CSRF header is no credential");
});

test("GET /api/core/module answers core's own module.json", async () => {
  const r = await call(mod.onRequest, { path: "/api/core/module", headers: auth(), env: PW_ENV });
  assert.deepEqual(r.json(), JSON.parse(readFileSync(new URL("../module.json", import.meta.url), "utf8")));
  assert.equal(r.headers["content-type"], "application/json; charset=utf-8");
});

test("intVar: positive integers only, else the default", () => {
  assert.deepEqual([intVar("5", 1), intVar("0", 1), intVar("-3", 1), intVar("x", 1), intVar(undefined, 2)], [5, 1, 1, 1, 2]);
});
