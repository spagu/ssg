// GO-155: who gets into the shop's admin API. The mode matrix — nothing set,
// the shared password, Cloudflare Access, `auth`, a half-configured `auth`,
// `auth` with the password still set — then CSRF in the cookie modes, the
// password's lockout, and the audit log's actor per mode.
import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { csrfProblem, modeOf, permit } from "../functions/api/shop/_auth";
import { onRequestPut as putSettings } from "../functions/api/shop/admin/settings";
import { ACCESS_VARS, AUTH_VARS, ORIGIN, PASSWORD, ask, serveAccessKeys, shopEnv, throughGate } from "./admin-fixtures";

const ME = "/api/shop/admin/me";
const STATS = "/api/shop/admin/stats";
let restore: () => void = () => {};
afterEach(() => restore());

async function status(request: Request, vars: Record<string, unknown>): Promise<number> {
  return (await throughGate(request, shopEnv(vars))).res.status;
}

describe("the mode the configuration selects", () => {
  it("is Access, then auth, then the password, else none", () => {
    assert.equal(modeOf(shopEnv()), "none");
    assert.equal(modeOf(shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD })), "password");
    assert.equal(modeOf(shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD, SHOP_AUTH_LOGIN_URL: "/auth/login" })), "auth");
    assert.equal(modeOf(shopEnv({ ...AUTH_VARS, ...ACCESS_VARS })), "access");
    // Half of Access is not Access: the next mode decides.
    assert.equal(modeOf(shopEnv({ SHOP_ACCESS_TEAM: "myteam", SHOP_ADMIN_PASSWORD: PASSWORD })), "password");
  });
});

describe("nothing configured", () => {
  it("is a 503 naming every way in, never open", async () => {
    const { res } = await throughGate(await ask(STATS), shopEnv());
    assert.equal(res.status, 503);
    const body = (await res.json()) as { error: string; message: string };
    assert.equal(body.error, "admin_not_configured");
    assert.match(body.message, /SHOP_ACCESS_TEAM.*SHOP_AUTH_\*.*SHOP_ADMIN_PASSWORD/);
  });

  it("without the database bound is a 503 before anything else", async () => {
    const env = { ...shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD }), SHOP_DB: undefined };
    const { res } = await throughGate(await ask(STATS, { password: PASSWORD }), env as never);
    assert.equal(res.status, 503);
    assert.equal(((await res.json()) as { error: string }).error, "not_configured");
  });
});

describe("the shared password", () => {
  const vars = { SHOP_ADMIN_PASSWORD: PASSWORD };

  it("lets the right password in with every permission, and names nobody", async () => {
    const { res, data } = await throughGate(await ask(STATS, { password: PASSWORD }), shopEnv(vars));
    assert.equal(res.status, 200);
    assert.deepEqual(data.admin, { mode: "password", user: null, perms: ["ecommerce:*"], actor: "password" });
  });

  it("refuses no password, a wrong one, another scheme and an undecodable one — without a browser dialog", async () => {
    for (const a of [{}, { password: "wrong" }, { headers: { authorization: "Bearer x" } }, { headers: { authorization: "Basic %%%" } }]) {
      const { res } = await throughGate(await ask(STATS, a), shopEnv(vars));
      assert.equal(res.status, 401);
      assert.equal(res.headers.get("www-authenticate"), null);
      assert.equal(((await res.json()) as { error: string }).error, "unauthorized");
    }
  });

  it("takes a password outside ASCII, as curl -u sends it (UTF-8)", async () => {
    const env = shopEnv({ SHOP_ADMIN_PASSWORD: "zażółć gęślą jaźń" });
    assert.equal((await throughGate(await ask(STATS, { password: "zażółć gęślą jaźń" }), env)).res.status, 200);
  });

  it("needs no CSRF header: the credential travels in a header only a script adds", async () => {
    assert.equal(await status(await ask("/api/shop/admin/outbox", { method: "POST", password: PASSWORD, body: "{}" }), vars), 200);
  });
});

describe("Cloudflare Access", () => {
  it("lets a verified person in with every permission, as their address", async () => {
    restore = serveAccessKeys();
    const { res, data } = await throughGate(await ask(STATS, { access: true }), shopEnv(ACCESS_VARS));
    assert.equal(res.status, 200);
    assert.deepEqual(data.admin, {
      mode: "access",
      user: { id: "owner@example.com", email: "owner@example.com" },
      perms: ["ecommerce:*"],
      actor: "access:owner@example.com",
    });
  });

  it("refuses a request without the token, and ignores the password", async () => {
    restore = serveAccessKeys();
    const { res } = await throughGate(await ask(STATS, { password: PASSWORD }), shopEnv({ ...ACCESS_VARS, SHOP_ADMIN_PASSWORD: PASSWORD }));
    assert.equal(res.status, 401);
    assert.equal(((await res.json()) as { error: string }).error, "access_required");
  });

  it("wins over a configured auth mode", async () => {
    restore = serveAccessKeys();
    const { data } = await throughGate(await ask(STATS, { access: true }), shopEnv({ ...ACCESS_VARS, ...AUTH_VARS }));
    assert.equal(data.admin.mode, "access");
  });
});

describe("auth", () => {
  it("lets a session in with exactly its permissions, as the account", async () => {
    const { res, data } = await throughGate(await ask(STATS, { perms: ["ecommerce:read"] }), shopEnv(AUTH_VARS));
    assert.equal(res.status, 200);
    assert.deepEqual(data.admin, {
      mode: "auth",
      user: { id: "u-ada", email: "ada@example.com" },
      perms: ["ecommerce:read"],
      actor: "auth:u-ada ada@example.com",
    });
  });

  it("answers an API call without a session 401, and a page 302 to the login", async () => {
    const api = await throughGate(await ask(STATS), shopEnv(AUTH_VARS));
    assert.equal(api.res.status, 401);
    assert.equal(((await api.res.json()) as { reason: string }).reason, "no_session");
    const page = await throughGate(await ask(STATS, { headers: { accept: "text/html" } }), shopEnv(AUTH_VARS));
    assert.equal(page.res.status, 302);
    assert.equal(page.res.headers.get("location"), `${ORIGIN}/auth/login?next=%2Fapi%2Fshop%2Fadmin%2Fstats`);
  });

  it("is a 503 naming what is missing when half configured — never a fall-through to the password", async () => {
    const { res } = await throughGate(await ask(STATS, { password: PASSWORD }), shopEnv({ SHOP_AUTH_LOGIN_URL: "/auth/login", SHOP_ADMIN_PASSWORD: PASSWORD }));
    assert.equal(res.status, 503);
    const body = (await res.json()) as { error: string; detail: string };
    assert.equal(body.error, "not_configured");
    assert.match(body.detail, /SHOP_AUTH_ISSUER is not set/);
  });

  it("refuses the shared password while it is on", async () => {
    assert.equal(await status(await ask(STATS, { password: PASSWORD }), { ...AUTH_VARS, SHOP_ADMIN_PASSWORD: PASSWORD }), 401);
  });

  // GO-157 accepts service tokens on the routes listed for machines, with the
  // exact permission (machine-tokens.test.ts); a wildcard alone is refused.
  it("refuses a machine's service token that holds only the shop wildcard (GO-157)", async () => {
    const { authToken } = await import("./admin-fixtures");
    const bearer = await authToken(["ecommerce:*"], { kind: "service" });
    const { res } = await throughGate(await ask(STATS, { headers: { authorization: `Bearer ${bearer}` } }), shopEnv(AUTH_VARS));
    assert.equal(res.status, 403);
    assert.deepEqual((await res.json()) as { error: string; permission: string }, {
      error: "forbidden",
      permission: "ecommerce:read",
      message: "A service token needs the permission ecommerce:read by its exact name; a wildcard does not count for machines.",
    });
  });
});

describe("CSRF in the cookie modes", () => {
  const change = { method: "POST", body: "{}" };

  for (const [name, vars, cred] of [
    ["auth", AUTH_VARS, { perms: ["ecommerce:orders"] }],
    ["Access", ACCESS_VARS, { access: true }],
  ] as const) {
    it(`${name}: a change needs the header and this origin; a read does not`, async () => {
      restore = serveAccessKeys();
      const path = "/api/shop/admin/outbox";
      const bodyOf = async (r: Request) => (await (await throughGate(r, shopEnv(vars))).res.json()) as { error?: string };
      assert.equal((await bodyOf(await ask(path, { ...change, ...cred }))).error, "csrf_check_failed");
      assert.equal((await bodyOf(await ask(path, { ...change, ...cred, headers: { "x-ssg-auth": "1", origin: "https://evil.example.com" } }))).error, "cross_origin");
      assert.equal((await bodyOf(await ask(path, { ...change, ...cred, headers: { "x-ssg-auth": "1", "sec-fetch-site": "same-site" } }))).error, "cross_origin");
      assert.equal(await status(await ask(path, { ...change, ...cred, panel: true }), vars), 200);
      assert.equal(await status(await ask(path, { ...change, ...cred, headers: { "x-ssg-auth": "1", "sec-fetch-site": "same-origin" } }), vars), 200);
      assert.equal(await status(await ask("/api/shop/admin/outbox", { ...cred, perms: name === "auth" ? ["ecommerce:read"] : undefined }), vars), 200);
    });
  }

  it("csrfProblem: header, Origin and Sec-Fetch-Site rules", () => {
    const r = (h: Record<string, string>) => new Request(`${ORIGIN}/x`, { method: "POST", headers: h });
    assert.equal(csrfProblem(r({ "x-ssg-auth": "1" })), null);
    assert.equal(csrfProblem(r({ "x-ssg-auth": "2" })), "csrf_check_failed");
    assert.equal(csrfProblem(r({ "x-ssg-auth": "1", origin: "null" })), "cross_origin");
  });
});

describe("permissions and unknown routes", () => {
  it("permit: a session-only need passes; a missing permission is named", async () => {
    const admin = { mode: "auth" as const, user: null, perms: ["ecommerce:read"], actor: "auth:x" };
    assert.equal(permit(admin, "session"), null);
    assert.equal(permit(admin, "ecommerce:read"), null);
    const res = permit(admin, "ecommerce:refund") as Response;
    assert.equal(res.status, 403);
    const body = (await res.json()) as { error: string; permission: string; message: string };
    assert.equal(body.permission, "ecommerce:refund");
    assert.match(body.message, /ecommerce:refund/);
  });

  it("a route the table does not know is refused in every mode, after sign-in", async () => {
    assert.equal(await status(await ask("/api/shop/admin/users"), { SHOP_ADMIN_PASSWORD: PASSWORD }), 401);
    for (const path of ["/api/shop/admin/users", "/api/shop/admin/auth/login", "/api/shop/admin/password"]) {
      const { res } = await throughGate(await ask(path, { method: "POST", password: PASSWORD, body: "{}" }), shopEnv({ SHOP_ADMIN_PASSWORD: PASSWORD }));
      assert.equal(res.status, 404, path);
    }
  });
});

describe("the audit log names who did it", () => {
  const change = (a: Record<string, unknown>) => ask("/api/shop/admin/settings", { method: "PUT", body: JSON.stringify({ "shop.name": "Example Press" }), ...a });

  for (const [mode, vars, cred, actor] of [
    ["password", { SHOP_ADMIN_PASSWORD: PASSWORD }, { password: PASSWORD }, "password"],
    ["Access", ACCESS_VARS, { access: true, panel: true }, "access:owner@example.com"],
    ["auth", AUTH_VARS, { perms: ["ecommerce:settings"], panel: true }, "auth:u-ada ada@example.com"],
  ] as const) {
    it(`${mode}: ${actor}`, async () => {
      restore = serveAccessKeys();
      const env = shopEnv(vars);
      const { res } = await throughGate(await change(cred), env, (ctx) => putSettings(ctx as never) as Promise<Response>);
      assert.equal(res.status, 200);
      const row = (await env.SHOP_DB.prepare("SELECT actor FROM audit_log WHERE action = 'settings.update'").first()) as { actor: string };
      assert.equal(row.actor, actor);
    });
  }
});
