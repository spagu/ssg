// The three admin modes side by side (GO-136): nothing configured, the
// password, Cloudflare Access, auth; a half-configured auth; auth and the
// password set together; Access and auth set together — and what each answers
// on GET /api/comments/admin/whoami.
import { test } from "node:test";
import assert from "node:assert/strict";
import * as admin from "../functions/api/comments/admin.ts";
import * as whoami from "../functions/api/comments/admin/whoami.ts";
import { AUD, PASSWORD, TEAM, accessIssuer, basic, call, sqliteD1 } from "./harness.ts";
import { authEnv, keyPair, mint, session } from "./auth-tokens.ts";

const A = "/api/comments/admin";
const W = "/api/comments/admin/whoami";
const ALL = ["comments:read", "comments:moderate", "comments:delete", "comments:import"];
const get = (path: string, headers: Record<string, string>, env: Record<string, unknown>) => call(path === W ? whoami.onRequestGet : admin.onRequestGet, { path, headers, env });
const body = (o: { body: string }) => JSON.parse(o.body);

test("modes: nothing configured — 503 as today, also from whoami", async () => {
  for (const path of [A, W]) {
    const got = await get(path, { authorization: basic(PASSWORD) }, { COMMENTS_DB: sqliteD1() });
    assert.equal(got.status, 503);
    assert.deepEqual(body(got), { error: "moderation not configured" });
  }
});

test("modes: password — whoami names the mode and every permission; a wrong one is today's 401", async () => {
  const env = { COMMENTS_DB: sqliteD1(), COMMENTS_ADMIN_PASSWORD: PASSWORD };
  const ok = await get(W, { authorization: basic(PASSWORD) }, env);
  assert.equal(ok.status, 200);
  assert.deepEqual(body(ok), { mode: "password", user: null, permissions: ALL });
  const no = await get(W, { authorization: basic("nope") }, env);
  assert.deepEqual([no.status, body(no)], [401, { error: "unauthorized" }]);
});

test("modes: Access — whoami carries the Access email; a token without one has no user", async () => {
  const issuer = await accessIssuer();
  const restore = issuer.install();
  try {
    const env = { COMMENTS_ACCESS_TEAM: TEAM, COMMENTS_ACCESS_AUD: AUD, COMMENTS_ADMIN_PASSWORD: PASSWORD, ...authEnv() };
    const withEmail = await get(W, { "cf-access-jwt-assertion": await issuer.token({ email: "mod@example.com" }) }, env);
    assert.deepEqual(body(withEmail), { mode: "access", user: { id: "mod@example.com", email: "mod@example.com" }, permissions: ALL });
    const without = await get(W, { "cf-access-jwt-assertion": await issuer.token() }, env);
    assert.deepEqual(body(without), { mode: "access", user: null, permissions: ALL });
    // Access wins over auth: an auth session alone is not Access, and gets Access's 401, without a login URL.
    const cookie = await get(W, session(await mint(ALL)), env);
    assert.deepEqual([cookie.status, body(cookie)], [401, { error: "Cloudflare Access required" }]);
  } finally {
    restore();
  }
});

test("modes: auth — whoami names the account and its permissions, wildcards expanded", async () => {
  const env = authEnv({ COMMENTS_DB: sqliteD1() });
  const reader = await get(W, session(await mint(["comments:read", "newsletter:send"])), env);
  assert.equal(reader.status, 200);
  assert.deepEqual(body(reader), { mode: "auth", user: { id: "user-1", email: "ada@example.com" }, permissions: ["comments:read"] });
  const all = await get(W, session(await mint(["comments:*"])), env);
  assert.deepEqual(body(all).permissions, ALL);
  const none = await get(W, session(await mint([])), env);
  assert.deepEqual([none.status, body(none).permissions], [200, []]);
});

test("modes: auth — without a session whoami says where to sign in; a page gets the redirect", async () => {
  const env = authEnv({ COMMENTS_DB: sqliteD1() });
  const api = await get(W, {}, env);
  assert.equal(api.status, 401);
  assert.ok(api.headers.some(([k, v]) => k === "www-authenticate" && v === 'Bearer realm="ssg-auth"'));
  assert.deepEqual(body(api), { error: "unauthorized", reason: "no_session", mode: "auth", login: "https://example.com/auth/login?next=%2Fcomments-admin.html" });
  const expired = await get(W, session(await mint(ALL, { exp: 1, iat: 0 })), env);
  assert.equal(body(expired).reason, "expired");
  const page = await get(W, { accept: "text/html" }, env);
  assert.equal(page.status, 302);
  assert.equal(page.headers.find(([k]) => k === "location")![1], "https://example.com/auth/login?next=%2Fapi%2Fcomments%2Fadmin%2Fwhoami");
});

test("modes: auth — refusals from a bad token, a wrong issuer or another key", async () => {
  const env = authEnv({ COMMENTS_DB: sqliteD1() });
  const other = await keyPair();
  for (const token of ["not-a-token", await mint(ALL, { iss: "https://elsewhere.example.com" }), await mint(ALL, {}, other), await mint(ALL, { aud: "other.example.com" })]) {
    const got = await get(A, session(token), env);
    assert.deepEqual([got.status, body(got)], [401, { error: "unauthorized", reason: "invalid_session" }]);
  }
});

test("modes: half-configured auth is the verifier's 503 — never a fall-through to the password", async () => {
  const env = { COMMENTS_DB: sqliteD1(), COMMENTS_ADMIN_PASSWORD: PASSWORD, COMMENTS_AUTH_ISSUER: "https://example.com/api/auth" };
  const want = { error: "not_configured", detail: "COMMENTS_AUTH_LOGIN_URL is not set; set exactly one of COMMENTS_AUTH_PUBLIC_KEY and COMMENTS_AUTH_JWKS_URL" };
  for (const path of [A, W]) {
    const got = await get(path, { authorization: basic(PASSWORD) }, env);
    assert.deepEqual([got.status, body(got)], [503, want], path);
  }
  const post = await call(admin.onRequestPost, { method: "POST", path: A, headers: { authorization: basic(PASSWORD) }, body: '{"action":"purge"}', env });
  assert.deepEqual([post.status, body(post)], [503, want]);
});

test("modes: auth and the password both set — auth wins and the password is refused", async () => {
  const env = authEnv({ COMMENTS_DB: sqliteD1(), COMMENTS_ADMIN_PASSWORD: PASSWORD });
  const pw = await get(A, { authorization: basic(PASSWORD) }, env);
  assert.deepEqual([pw.status, body(pw)], [401, { error: "unauthorized", reason: "no_session" }]);
  const post = await call(admin.onRequestPost, { method: "POST", path: A, headers: { authorization: basic(PASSWORD) }, body: '{"action":"purge"}', env });
  assert.equal(post.status, 401);
  const ok = await get(A, session(await mint(["comments:read"])), env);
  assert.equal(ok.status, 200);
});

test("modes: auth with a JWKS URL — keys fetched from the same origin; an unreachable one is 503", async () => {
  const keys = await keyPair();
  const real = globalThis.fetch;
  const seen: string[] = [];
  globalThis.fetch = (async (url: string) => {
    seen.push(String(url));
    if (String(url) === "https://example.com/api/auth/jwks") return new Response(JSON.stringify({ keys: [JSON.parse(keys.publicJwk)] }));
    return new Response("down", { status: 500 });
  }) as typeof fetch;
  try {
    const env = (url: string) => authEnv({ COMMENTS_DB: sqliteD1(), COMMENTS_AUTH_PUBLIC_KEY: undefined, COMMENTS_AUTH_JWKS_URL: url });
    const ok = await get(A, session(await mint(["comments:read"], {}, keys)), env("/api/auth/jwks"));
    assert.equal(ok.status, 200);
    const down = await get(A, session(await mint(["comments:read"], {}, keys)), env("/api/auth/other-jwks"));
    assert.deepEqual([down.status, body(down)], [503, { error: "auth_unavailable" }]);
    assert.deepEqual(seen, ["https://example.com/api/auth/jwks", "https://example.com/api/auth/other-jwks"]);
  } finally {
    globalThis.fetch = real;
  }
});
