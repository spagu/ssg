// GO-155: the panel's sign-in logic that runs without a DOM — session.js (what
// the account may see, the same-origin login guard) and api.js (headers per
// mode, one refresh in auth mode, the 403 that names a permission) — and the
// page itself: no login e-mail field, no user management, no inline code.
import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import * as session from "../public/ecommerce-admin/session.js";
import * as client from "../public/ecommerce-admin/api.js";

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
  client.configure({ mode: "password" });
  client.setPassword("");
  session.setPermissions([]);
});

const ORIGIN = "https://shop.example.com";

describe("session.js", () => {
  it("loginTarget follows a same-origin URL only, as a path", () => {
    assert.equal(session.loginTarget("/auth/login?next=%2Fecommerce-admin%2F", ORIGIN), "/auth/login?next=%2Fecommerce-admin%2F");
    assert.equal(session.loginTarget(`${ORIGIN}/auth/login#x`, ORIGIN), "/auth/login#x");
    for (const bad of ["https://evil.example.com/login", "//evil.example.com/login", "javascript:alert(1)", "", null, 42]) {
      assert.equal(session.loginTarget(bad, ORIGIN), null, String(bad));
    }
    assert.equal(session.loginTarget("/x", "not a base"), null, "an unparsable URL is not followed");
  });

  it("may, firstView: only what the permissions allow", () => {
    session.setPermissions("not a list");
    assert.equal(session.may("ecommerce:read"), false);
    assert.equal(session.firstView("dashboard"), null);
    session.setPermissions(["ecommerce:settings"]);
    assert.equal(session.firstView("dashboard"), "modules");
    assert.equal(session.firstView("settings"), "settings");
    assert.equal(session.firstView("nonsense"), "modules");
  });

  it("applyNeeds marks what the account may not use, the root included", () => {
    session.setPermissions(["ecommerce:read"]);
    const node = (needs: string) => {
      const n = { dataset: { needs }, denied: false, matches: () => true, toggleAttribute: (_: string, on: boolean) => void (n.denied = on) };
      return n;
    };
    const refund = node("ecommerce:refund");
    const read = node("ecommerce:read");
    const root = Object.assign(node("ecommerce:orders"), { querySelectorAll: () => [refund, read] });
    session.applyNeeds(root);
    assert.deepEqual([root.denied, refund.denied, read.denied], [true, true, false]);
    session.applyNeeds({ matches: () => false, querySelectorAll: () => [read] });
    session.applyNeeds({}); // a text-like node: nothing to do
  });

  it("names who is signed in and what is missing", () => {
    assert.equal(session.signedInAs({ mode: "auth", user: { email: "ada@example.com" } }), "ada@example.com");
    assert.equal(session.signedInAs({ mode: "password", user: null }), "Shared password");
    assert.equal(session.signedInAs({ mode: "access", user: null }), "Signed in");
    assert.match(session.missingPermission("ecommerce:refund"), /ecommerce:refund/);
  });
});

type Seen = { url: string; init: RequestInit & { headers: Record<string, string> } };
function serve(...answers: Array<Response | Error>): Seen[] {
  const seen: Seen[] = [];
  globalThis.fetch = (async (url: string, init: Seen["init"]) => {
    seen.push({ url, init });
    const next = answers.shift() as Response | Error;
    if (next instanceof Error) throw next;
    return next;
  }) as never;
  return seen;
}
const reply = (status: number, body?: unknown) => new Response(body === undefined ? "" : JSON.stringify(body), { status });

describe("api.js", () => {
  it("password mode: Basic (UTF-8) and the CSRF header on every request; JSON bodies", async () => {
    client.setPassword("gęś");
    const seen = serve(reply(200, { ok: 1 }), reply(200));
    assert.deepEqual(await client.api.post("/x", { a: 1 }), { ok: 1 });
    assert.equal(seen[0]!.init.headers.authorization, `Basic ${Buffer.from(":gęś").toString("base64")}`);
    assert.equal(seen[0]!.init.headers["x-ssg-auth"], "1");
    assert.equal(seen[0]!.init.headers["content-type"], "application/json");
    assert.equal(seen[0]!.init.body, '{"a":1}');
    assert.equal(await client.api.del("/x"), null);
  });

  it("auth mode: no Basic header; a 401 refreshes once and retries", async () => {
    client.configure({ mode: "auth", refreshUrl: "/api/auth/refresh" });
    client.setPassword("ignored");
    const seen = serve(reply(401, {}), reply(200), reply(200, { done: true }));
    assert.deepEqual(await client.api.get("/x"), { done: true });
    assert.equal(seen[0]!.init.headers.authorization, undefined);
    assert.equal(seen[1]!.url, "/api/auth/refresh");
    assert.equal(seen[1]!.init.headers["x-ssg-auth"], "1");
  });

  it("a 401 that survives the refresh signs the panel out with what the API said", async () => {
    client.configure({ mode: "auth", refreshUrl: "/api/auth/refresh" });
    let told: unknown = null;
    client.setSignedOutHandler((body: unknown) => (told = body));
    serve(reply(401, {}), reply(200), reply(401, { mode: "auth", loginUrl: "/auth/login" }));
    await assert.rejects(client.api.get("/x"), (e: client.ApiError) => e.status === 401 && e.code === "unauthenticated");
    assert.deepEqual(told, { mode: "auth", loginUrl: "/auth/login" });
  });

  it("a 403 carries the missing permission; other errors their code, or a plain one", async () => {
    serve(reply(403, { error: "forbidden", permission: "ecommerce:refund", message: "needs ecommerce:refund" }), reply(500, "nope"), new Response("<html>", { status: 502 }));
    await assert.rejects(client.api.post("/x", {}), (e: client.ApiError) => e.permission === "ecommerce:refund" && e.code === "forbidden" && e.message === "needs ecommerce:refund");
    await assert.rejects(client.api.get("/x"), (e: client.ApiError) => e.code === "http_error" && e.permission === null && /500/.test(e.message));
    await assert.rejects(client.api.get("/x"), (e: client.ApiError) => e.code === "http_error" && /502/.test(e.message));
  });

  it("uploads FormData untouched, and whoami never throws for a status", async () => {
    const seen = serve(reply(200, {}), reply(200, {}), reply(200, {}), reply(401, { error: "unauthorized" }));
    const form = new FormData();
    await client.api.upload("/f", form);
    assert.equal(seen[0]!.init.body, form);
    assert.equal(seen[0]!.init.headers["content-type"], undefined);
    await client.api.patch("/p", {});
    await client.api.put("/q", {});
    assert.deepEqual(await client.api.whoami(), { ok: false, status: 401, body: { error: "unauthorized" } });
    assert.equal(seen[3]!.url, client.ME);
  });

  it("refreshSession: none without a URL, false when it fails or cannot be reached", async () => {
    client.configure({ mode: "auth" });
    assert.equal(await client.refreshSession(), false);
    serve(reply(401), new Error("offline"));
    assert.equal(await client.refreshSession("/api/auth/refresh"), false);
    assert.equal(await client.refreshSession("/api/auth/refresh"), false);
  });
});

describe("the page", () => {
  const html = readFileSync(new URL("../public/ecommerce-admin/index.html", import.meta.url), "utf8");
  const js = readFileSync(new URL("../public/ecommerce-admin/admin.js", import.meta.url), "utf8");

  it("asks for the shared password only — no e-mail, no accounts, no bootstrap", () => {
    assert.match(html, /id="login-password"/);
    for (const gone of [/login-email/, /view-users/, /data-view="users"/, /BOOTSTRAP/, /people\.js/]) assert.doesNotMatch(html + js, gone);
  });

  it("has no inline script, style or handler, and keeps its meta tags", () => {
    assert.doesNotMatch(html, /<style|\sstyle=|\son[a-z]+=/i);
    for (const [, body] of html.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)) assert.equal(body!.trim(), "");
    assert.match(html, /<meta name="robots" content="noindex, nofollow, noarchive">/);
    assert.match(html, /<meta name="description" content="[^"]{20,}">/);
  });

  it("marks actions with a permission the API also requires", () => {
    const needs = [...html.matchAll(/data-needs="([^"]+)"/g)].map((m) => m[1]);
    assert.deepEqual(needs, ["ecommerce:products", "ecommerce:export", "ecommerce:products", "ecommerce:orders"]);
  });
});
