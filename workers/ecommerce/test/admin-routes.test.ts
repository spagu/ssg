// GO-155: every admin route needs exactly one permission, and the table that
// says which (_permissions.ts) is checked against the route files themselves:
// each exported onRequest<Method> under functions/api/shop/admin/ is a route,
// and a route the table does not know fails here — as does an entry without a
// route, or a permission module.json does not declare. Then, for every route,
// generated: no session → 401 (a page → 302 to the login), a session without
// the permission → 403 naming it, a session with exactly it → through, and
// the shop wildcard → through. Handlers are stubbed: this is the gate.
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { ADMIN_ROUTES, PERMISSIONS, SESSION, needFor } from "../functions/api/shop/_permissions";
import { AUTH_VARS, ORIGIN, ask, shopEnv, throughGate } from "./admin-fixtures";

const ADMIN = new URL("../functions/api/shop/admin/", import.meta.url).pathname;
const manifest = JSON.parse(readFileSync(new URL("../module.json", import.meta.url), "utf8")) as { permissions: { name: string }[] };

// routesOnDisk: "<METHOD> <path>" for every handler a route file exports,
// following Pages' file routing ([id] → :id, index.ts → its directory).
function routesOnDisk(dir = ADMIN, base = "/api/shop/admin"): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir).sort()) {
    const full = join(dir, name);
    const segment = name.replace(/^\[(.+)\]$/, ":$1").replace(/\.ts$/, "").replace(/^\[(.+)\]$/, ":$1");
    if (statSync(full).isDirectory()) {
      out.push(...routesOnDisk(full, `${base}/${segment}`));
      continue;
    }
    if (name.startsWith("_") || !name.endsWith(".ts")) continue;
    const path = name === "index.ts" ? base : `${base}/${segment}`;
    for (const [, method] of readFileSync(full, "utf8").matchAll(/export const onRequest(Get|Post|Put|Patch|Delete)\b/g)) {
      out.push(`${method.toUpperCase()} ${path}`);
    }
  }
  return out;
}

const DISK = routesOnDisk();
const concrete = (key: string) => key.split(" ")[1]!.replace(/:[a-z]+/g, "x1");

describe("the route → permission table", () => {
  it("knows every route file, and nothing that is not one", () => {
    assert.ok(DISK.length >= 25, `only ${DISK.length} routes found`);
    assert.deepEqual([...DISK].sort(), Object.keys(ADMIN_ROUTES).sort());
  });

  it("maps every route to exactly one declared permission (or the session, for /me only)", () => {
    assert.deepEqual([...PERMISSIONS], manifest.permissions.map((p) => p.name));
    for (const [route, need] of Object.entries(ADMIN_ROUTES)) {
      assert.equal(typeof need, "string", route);
      if (need === SESSION) assert.equal(route, "GET /api/shop/admin/me");
      else assert.ok((PERMISSIONS as readonly string[]).includes(need), `${route}: ${need} is not declared`);
    }
  });

  it("never lets ecommerce:read change anything", () => {
    for (const [route, need] of Object.entries(ADMIN_ROUTES)) {
      if (!route.startsWith("GET ")) assert.notEqual(need, "ecommerce:read", route);
    }
  });

  it("needFor: HEAD reads as GET, a trailing slash is the same route, the rest is unknown", () => {
    assert.equal(needFor("HEAD", "/api/shop/admin/orders"), "ecommerce:read");
    assert.equal(needFor("POST", "/api/shop/admin/orders/abc/refund/"), "ecommerce:refund");
    assert.equal(needFor("GET", "/api/shop/admin/orders//refund"), null);
    assert.equal(needFor("OPTIONS", "/api/shop/admin/orders"), null);
    assert.equal(needFor("GET", "/api/shop/admin/orders/abc/refund"), null);
  });
});

describe("generated: every route behind the gate", () => {
  for (const [route, need] of Object.entries(ADMIN_ROUTES)) {
    const [method] = route.split(" ") as [string];
    const changes = method !== "GET";
    const path = concrete(route);
    const call = async (perms: string[] | undefined, headers: Record<string, string> = {}) =>
      (await throughGate(await ask(path, { method, perms, panel: changes, headers, body: changes ? "{}" : undefined }), shopEnv(AUTH_VARS))).res;

    it(`${route} [${need}]`, async () => {
      const none = await call(undefined);
      assert.equal(none.status, 401, "no session");
      if (!changes) {
        const page = await call(undefined, { accept: "text/html" });
        assert.equal(page.status, 302, "a page without a session goes to the login");
        assert.ok(page.headers.get("location")?.startsWith(`${ORIGIN}/auth/login?next=`));
      }
      const other = await call(["comments:moderate"]);
      if (need === SESSION) {
        assert.equal(other.status, 200, "a session is enough");
        return;
      }
      assert.equal(other.status, 403, "a session without the permission");
      assert.equal(((await other.json()) as { permission: string }).permission, need);
      assert.equal((await call([need])).status, 200, "a session with exactly the permission");
      assert.equal((await call(["ecommerce:*"])).status, 200, "the shop wildcard");
      for (const p of PERMISSIONS.filter((x) => x !== need)) assert.equal((await call([p])).status, 403, `${p} is not ${need}`);
    });
  }
});
