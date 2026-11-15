// GO-157: `auth` service tokens — a script, CI or a cron job — on the shop's
// admin API. Accepted only on the routes _permissions.ts lists for machines
// (reads, chosen one by one), only as a Bearer token, only with the route's
// permission by its exact name; never needing the CSRF header. Everything else
// a token asks for is refused, with an answer that says why.
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as exportCsv } from "../functions/api/shop/admin/export";
import { onRequestGet as stats } from "../functions/api/shop/admin/stats";
import { authenticate } from "../functions/api/shop/_auth";
import { ADMIN_ROUTES, MACHINE_ROUTES, MODULE_ROUTES, machineSafe } from "../functions/api/shop/_permissions";
import { onRequestGet as manifest } from "../functions/api/shop/module/index";
import { onRequestGet as summary } from "../functions/api/shop/module/summary";
import { AUTH_VARS, COOKIE, PASSWORD, ask, authToken, shopEnv, throughGate } from "./admin-fixtures";

const service = async (perms: string[]) => `Bearer ${await authToken(perms, { kind: "service", sub: "svc-ci", email: "" })}`;
const asMachine = async (path: string, perms: string[], init: { method?: string; headers?: Record<string, string> } = {}) =>
  ask(path, { method: init.method, headers: { authorization: await service(perms), ...init.headers }, body: init.method && init.method !== "GET" ? "{}" : undefined });
const errorOf = async (res: Response) => ((await res.json()) as { error: string }).error;

describe("the machine route list", () => {
  it("is a short list of reads the tables know", () => {
    const known = { ...ADMIN_ROUTES, ...MODULE_ROUTES };
    for (const route of MACHINE_ROUTES) {
      assert.ok(route in known, `${route} is not a route`);
      assert.ok(route.startsWith("GET "), `${route} changes the shop: decide again and document it before listing it`);
    }
    assert.deepEqual([...MACHINE_ROUTES].sort(), ["GET /api/shop/admin/export", "GET /api/shop/admin/stats", "GET /api/shop/module", "GET /api/shop/module/summary"]);
    assert.equal(machineSafe("HEAD", "/api/shop/admin/stats/"), true);
    assert.equal(machineSafe("GET", "/api/shop/admin/orders"), false);
    assert.equal(machineSafe("POST", "/api/shop/admin/export"), false);
  });
});

describe("a service token on a machine route", () => {
  it("exports and reads the sales figures with the exact permission, recorded under its own name", async () => {
    const env = shopEnv(AUTH_VARS);
    const csv = await throughGate(await asMachine("/api/shop/admin/export?type=orders", ["ecommerce:export"]), env, (ctx) => exportCsv(ctx as never) as Promise<Response>);
    assert.equal(csv.res.status, 200);
    assert.match(csv.res.headers.get("content-type") ?? "", /text\/csv/);
    assert.equal(csv.data.admin.actor, "service:svc-ci");
    assert.equal(csv.data.admin.service, true);
    const row = await env.SHOP_DB.prepare("SELECT actor FROM audit_log WHERE action = 'export'").first<{ actor: string }>();
    assert.equal(row?.actor, "service:svc-ci");

    const figures = await throughGate(await asMachine("/api/shop/admin/stats", ["ecommerce:read"]), env, (ctx) => stats(ctx as never) as Promise<Response>);
    assert.equal(figures.res.status, 200);
    assert.equal(((await figures.res.json()) as { window: string }).window, "30d");
  });

  it("reads the module's manifest and summary", async () => {
    const env = shopEnv(AUTH_VARS);
    for (const [path, route] of [["/api/shop/module", manifest], ["/api/shop/module/summary", summary]] as const) {
      const res = await route({ request: await asMachine(path, ["ecommerce:read"]), env } as never);
      assert.equal(res.status, 200, path);
    }
  });

  it("needs the permission by name: a wildcard or another permission is refused", async () => {
    for (const perms of [["ecommerce:*"], ["*"], ["ecommerce:read"]]) {
      const { res } = await throughGate(await asMachine("/api/shop/admin/export", perms), shopEnv(AUTH_VARS));
      assert.equal(res.status, 403, perms.join());
      assert.deepEqual(await res.json(), {
        error: "forbidden",
        permission: "ecommerce:export",
        message: "A service token needs the permission ecommerce:export by its exact name; a wildcard does not count for machines.",
      });
    }
  });
});

describe("a service token anywhere else", () => {
  it("is refused on every other route, reads and changes alike, even holding the permission", async () => {
    const env = shopEnv(AUTH_VARS);
    for (const [method, path, perm] of [
      ["GET", "/api/shop/admin/orders", "ecommerce:read"],
      ["GET", "/api/shop/admin/me", "ecommerce:read"],
      ["POST", "/api/shop/admin/orders/o1/refund", "ecommerce:refund"],
      ["PUT", "/api/shop/admin/settings", "ecommerce:settings"],
    ] as const) {
      const { res } = await throughGate(await asMachine(path, [perm], { method }), env);
      assert.equal(res.status, 403, `${method} ${path}`);
      assert.equal(await errorOf(res), "service_token_not_accepted", `${method} ${path}`);
    }
  });

  it("an unknown route is still a 404", async () => {
    const { res } = await throughGate(await asMachine("/api/shop/admin/nothing", ["ecommerce:read"]), shopEnv(AUTH_VARS));
    assert.equal(res.status, 404);
  });
});

describe("how a service token travels", () => {
  it("needs no CSRF header: a Bearer token is no credential a browser attaches by itself", async () => {
    const request = await asMachine("/api/shop/admin/outbox", ["ecommerce:orders"], { method: "POST", headers: { origin: "https://elsewhere.example.com" } });
    const admin = await authenticate(request, shopEnv(AUTH_VARS));
    assert.ok(!(admin instanceof Response));
    assert.equal(admin.service, true);
    // A person's session on the same request needs the header and this origin.
    const person = await authenticate(await ask("/api/shop/admin/outbox", { method: "POST", perms: ["ecommerce:orders"], body: "{}" }), shopEnv(AUTH_VARS));
    assert.ok(person instanceof Response);
    assert.equal(person.status, 403);
  });

  it("only as a Bearer token; a person's token only as the cookie", async () => {
    const env = shopEnv(AUTH_VARS);
    const cookie = await ask("/api/shop/admin/stats", { headers: { cookie: `${COOKIE}=${await authToken(["ecommerce:read"], { kind: "service" })}` } });
    assert.equal((await throughGate(cookie, env)).res.status, 401, "a service token in the cookie");
    const bearer = await ask("/api/shop/admin/stats", { headers: { authorization: `Bearer ${await authToken(["ecommerce:read"])}` } });
    assert.equal((await throughGate(bearer, env)).res.status, 401, "a person's token as a bearer");
  });

  it("means nothing in the password mode", async () => {
    const { res } = await throughGate(await asMachine("/api/shop/admin/stats", ["ecommerce:read"]), shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD }));
    assert.equal(res.status, 401);
  });
});
