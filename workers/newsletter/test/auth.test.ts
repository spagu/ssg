// GO-137: the third admin mode of this template. Every admin route × {no
// session, a session without its permission, a session with exactly it, the
// owner}; the mode order and a half-configured set; auth AND a password (the
// password is not accepted); CSRF on the purge; the public endpoints untouched.
import { test } from "node:test";
import assert from "node:assert/strict";
import * as admin from "../functions/api/newsletter/admin.ts";
import * as manifest from "../functions/api/newsletter/module/index.ts";
import * as summary from "../functions/api/newsletter/module/summary.ts";
import * as signup from "../functions/api/newsletter/index.ts";
import { requireAdmin, sameOrigin } from "../functions/api/newsletter/_lib.ts";
import { AUTH_VARS, LOGIN, session, token } from "./auth-fixtures.ts";
import { sqliteD1, withSchemaV1 } from "./sqlite-d1.ts";

type Fn = (ctx: { request: Request; env: unknown; waitUntil?: (p: Promise<unknown>) => void }) => Promise<Response>;
const env = (over: Record<string, unknown> = {}) => ({ NEWSLETTER_DB: withSchemaV1(sqliteD1()), ...AUTH_VARS, ...over });
const POST = { method: "POST", body: '{"action":"purge"}', headers: { "content-type": "application/json", origin: "https://example.com" } };

// The routes and what each needs.
const ROUTES: [string, Fn, string, RequestInit, string][] = [
  ["counts", admin.onRequestGet as unknown as Fn, "/api/newsletter/admin", {}, "newsletter:read"],
  ["CSV export", admin.onRequestGet as unknown as Fn, "/api/newsletter/admin?format=csv", {}, "newsletter:export"],
  ["purge", admin.onRequestPost as unknown as Fn, "/api/newsletter/admin", POST, "newsletter:settings"],
  ["manifest", manifest.onRequestGet as unknown as Fn, "/api/newsletter/module", {}, "newsletter:read"],
  ["summary", summary.onRequestGet as unknown as Fn, "/api/newsletter/module/summary", {}, "newsletter:read"],
];

for (const [name, fn, path, init, need] of ROUTES) {
  test(`matrix: ${name} [${need}]`, async () => {
    const status = async (perms: string[] | null) => (await fn({ request: await session(path, perms, init), env: env() })).status;
    assert.equal(await status(null), 401, "no session");
    assert.equal(await status(["comments:moderate"]), 403, "without the permission");
    assert.equal(await status([need]), 200, "with exactly the permission");
    assert.equal(await status(["*"]), 200, "the owner");
    assert.equal(await status(["newsletter:*"]), 200, "the worker wildcard");
    if (need !== "newsletter:read") assert.equal(await status(["newsletter:read"]), 403, "read only");
  });
}

test("a missing permission is named; a page without a session goes to the login page", async () => {
  const res = await admin.onRequestGet({ request: await session("/api/newsletter/admin?format=csv", ["newsletter:read"]), env: env() } as never);
  assert.deepEqual(await res.json(), { error: "forbidden", permission: "newsletter:export" });
  const page = await admin.onRequestGet({ request: new Request("https://example.com/api/newsletter/admin", { headers: { accept: "text/html" } }), env: env() } as never);
  assert.equal(page.status, 302);
  assert.equal(page.headers.get("location"), `https://example.com${LOGIN}?next=%2Fapi%2Fnewsletter%2Fadmin`);
});

test("half-configured auth is a 503 naming what is missing — the password does not rescue it", async () => {
  const e = { NEWSLETTER_DB: withSchemaV1(sqliteD1()), NEWSLETTER_AUTH_ISSUER: AUTH_VARS.NEWSLETTER_AUTH_ISSUER, NEWSLETTER_ADMIN_PASSWORD: "pw" };
  const res = await admin.onRequestGet({ request: new Request("https://example.com/api/newsletter/admin", { headers: { authorization: `Basic ${btoa("a:pw")}` } }), env: e } as never);
  assert.equal(res.status, 503);
  const body = await res.json();
  assert.equal(body.error, "not_configured");
  assert.match(body.detail, /NEWSLETTER_AUTH_LOGIN_URL/);
});

test("auth AND a password: the password is not accepted, the session is", async () => {
  const e = env({ NEWSLETTER_ADMIN_PASSWORD: "pw" });
  const basic = new Request("https://example.com/api/newsletter/admin", { headers: { authorization: `Basic ${btoa("a:pw")}` } });
  assert.equal((await requireAdmin(basic, e as never, "newsletter:read"))?.status, 401);
  assert.equal(await requireAdmin(await session("/x", ["newsletter:read"]), e as never, "newsletter:read"), null);
});

test("Access configured too: Access decides, as before", async () => {
  const res = await requireAdmin(await session("/x", ["*"]), env({ NEWSLETTER_ACCESS_TEAM: "t", NEWSLETTER_ACCESS_AUD: "a" }) as never, "newsletter:read");
  assert.deepEqual(await res?.json(), { error: "Cloudflare Access required" });
});

test("the Basic lockout's variable names do not switch auth on", async () => {
  const e = { NEWSLETTER_ADMIN_PASSWORD: "pw", NEWSLETTER_AUTH_MAX_FAILURES: "3", NEWSLETTER_AUTH_WINDOW_MINUTES: "5" };
  const basic = new Request("https://example.com/", { headers: { authorization: `Basic ${btoa("a:pw")}` } });
  assert.equal(await requireAdmin(basic, e as never, "newsletter:read"), null);
});

test("CSRF on the purge in auth mode: same origin only; a service token by bearer needs none", async () => {
  const purge = async (headers: Record<string, string>, perms: string[] | null = ["newsletter:settings"]) =>
    (await admin.onRequestPost({ request: await session("/api/newsletter/admin", perms, { method: "POST", body: '{"action":"purge"}', headers }), env: env() } as never)).status;
  assert.equal(await purge({ origin: "https://example.com" }), 200);
  assert.equal(await purge({ "sec-fetch-site": "same-origin" }), 200);
  for (const h of [{}, { origin: "https://evil.example" }, { origin: "null" }, { "sec-fetch-site": "cross-site" }, { origin: "https://example.com", "sec-fetch-site": "same-site" }]) {
    assert.equal(await purge(h), 403, JSON.stringify(h));
  }
  const bearer = { authorization: `Bearer ${await token(["newsletter:settings"], { kind: "service" })}` };
  assert.equal(await purge(bearer, null), 200);
  assert.equal(sameOrigin(new Request("https://example.com/", { headers: { origin: "https://example.com" } })), true);
});

test("the public sign-up endpoint answers exactly as without auth", async () => {
  const call = async (e: Record<string, unknown>) => {
    const request = new Request("https://example.com/api/newsletter", { method: "POST", body: JSON.stringify({ email: "reader@example.com", consent: "yes", consent_text: "Send it" }), headers: { "content-type": "application/json" } });
    const res = await signup.onRequestPost({ request, env: e, waitUntil: () => {} } as never);
    return [res.status, await res.text()];
  };
  assert.deepEqual(await call(env()), await call({ NEWSLETTER_DB: withSchemaV1(sqliteD1()) }));
});
