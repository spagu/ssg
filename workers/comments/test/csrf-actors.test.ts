// auth mode's CSRF defence for state-changing requests, the password mode left
// as it was, and who did what: comment_events (GO-136).
import { test } from "node:test";
import assert from "node:assert/strict";
import * as admin from "../functions/api/comments/admin.ts";
import * as importer from "../functions/api/comments/import.ts";
import { csrfProblem } from "../functions/api/comments/_admin-auth.ts";
import { AUD, PASSWORD, TEAM, accessIssuer, basic, call, seed, sqliteD1 } from "./harness.ts";
import type { SqliteD1 } from "./harness.ts";
import { COOKIE, authEnv, mint, session } from "./auth-tokens.ts";

const A = "/api/comments/admin";
const ALL = ["comments:*"];
const json = { "content-type": "application/json" };
const approve = JSON.stringify({ id: "c1", action: "approve" });
const db = () => {
  const d = sqliteD1();
  seed(d, [
    { id: "c1", status: "pending", created_at: "2000-01-01T00:00:00.000Z" },
    { id: "c2", status: "pending", created_at: "2000-01-01T00:00:00.000Z" },
    { id: "c3", status: "pending", created_at: "2000-01-01T00:00:00.000Z" },
  ]);
  return d;
};
const events = (d: SqliteD1) =>
  d.sqlite.prepare("SELECT comment_id, action, actor, mode, detail FROM comment_events ORDER BY action, comment_id").all().map((r) => ({ ...r }));
const postAdmin = (env: Record<string, unknown>, headers: Record<string, string>, body = approve) =>
  call(admin.onRequestPost, { method: "POST", path: A, headers: { ...json, ...headers }, body, env });

test("csrf: the rules, one by one", () => {
  const req = (h: Record<string, string>) => new Request("https://example.com/api/comments/admin", { method: "POST", headers: h });
  assert.equal(csrfProblem(req({ "x-ssg-auth": "1" })), null);
  assert.equal(csrfProblem(req({ "x-ssg-auth": "1", origin: "https://example.com", "sec-fetch-site": "same-origin" })), null);
  assert.equal(csrfProblem(req({})), "csrf_check_failed");
  assert.equal(csrfProblem(req({ "x-ssg-auth": "yes" })), "csrf_check_failed");
  assert.equal(csrfProblem(req({ "x-ssg-auth": "1", origin: "https://evil.example.com" })), "cross_origin");
  assert.equal(csrfProblem(req({ "x-ssg-auth": "1", origin: "null" })), "cross_origin");
  assert.equal(csrfProblem(req({ "x-ssg-auth": "1", "sec-fetch-site": "same-site" })), "cross_origin");
});

test("csrf: auth mode refuses a cookie POST without the header or from elsewhere, before changing anything", async () => {
  const d = db();
  const env = authEnv({ COMMENTS_DB: d });
  const cookie = { cookie: `${COOKIE}=${await mint(ALL)}` };
  const cases: [Record<string, string>, string][] = [
    [cookie, "csrf_check_failed"],
    [{ ...cookie, origin: "https://example.com" }, "csrf_check_failed"],
    [{ ...cookie, "x-ssg-auth": "1", origin: "https://evil.example.com" }, "cross_origin"],
    [{ ...cookie, "x-ssg-auth": "1", "sec-fetch-site": "cross-site" }, "cross_origin"],
  ];
  for (const [headers, error] of cases) {
    const got = await postAdmin(env, headers);
    assert.deepEqual([got.status, JSON.parse(got.body)], [403, { error }]);
    const imp = await call(importer.onRequestPost, { method: "POST", path: "/api/comments/import", headers: { ...json, ...headers }, body: '{"items":[]}', env });
    assert.equal(imp.status, 403);
  }
  assert.equal(d.rows().find((r) => r.id === "c1")!.status, "pending");
  // GETs change nothing and need no header.
  assert.equal((await call(admin.onRequestGet, { path: A, headers: cookie, env })).status, 200);
  // With the header (and no Origin, as some clients send) it goes through.
  assert.equal((await postAdmin(env, { ...cookie, "x-ssg-auth": "1" })).status, 200);
});

test("csrf: a service token rides a bearer header, which no browser attaches — no header needed; a user token there is refused", async () => {
  const env = authEnv({ COMMENTS_DB: db() });
  const svc = await postAdmin(env, { authorization: `Bearer ${await mint(ALL, { kind: "service", email: "" })}` });
  assert.equal(svc.status, 200);
  const user = await postAdmin(env, { authorization: `Bearer ${await mint(ALL)}`, "x-ssg-auth": "1" });
  assert.deepEqual([user.status, JSON.parse(user.body)], [401, { error: "unauthorized", reason: "invalid_session" }]);
});

test("csrf: the password mode is unchanged — no header, any Origin, as before", async () => {
  const env = { COMMENTS_DB: db(), COMMENTS_ADMIN_PASSWORD: PASSWORD };
  const got = await postAdmin(env, { authorization: basic(PASSWORD), origin: "https://evil.example.com", "sec-fetch-site": "cross-site" });
  assert.deepEqual([got.status, JSON.parse(got.body)], [200, { ok: true }]);
});

test("actors: auth mode records the account id for approve, spam, delete, purge and import", async () => {
  const d = db();
  seed(d, [{ id: "old", status: "spam", created_at: "1999-01-01T00:00:00.000Z" }]);
  const env = authEnv({ COMMENTS_DB: d, COMMENTS_RETENTION_DAYS: "30" });
  const h = session(await mint(ALL, { sub: "acct-7" }), true);
  for (const body of [{ id: "c1", action: "approve" }, { id: "c2", action: "spam" }, { id: "c3", action: "delete" }, { id: "gone", action: "approve" }]) {
    assert.equal((await postAdmin(env, h, JSON.stringify(body))).status, 200);
  }
  assert.deepEqual(JSON.parse((await postAdmin(env, h, '{"action":"purge"}')).body), { ok: true, purged: 2 });
  assert.deepEqual(JSON.parse((await postAdmin(env, h, '{"action":"purge"}')).body), { ok: true, purged: 0 });
  const items = JSON.stringify({ items: [{ url: "/a/", author: "Ada", body: "Hi", created_at: "2021-01-01T00:00:00Z" }, { url: "/b/", author: "No body" }] });
  for (let i = 0; i < 2; i++) await call(importer.onRequestPost, { method: "POST", path: "/api/comments/import", headers: { ...json, ...h }, body: items, env });
  const by = { actor: "acct-7", mode: "auth" };
  assert.deepEqual(events(d), [
    { comment_id: "c1", action: "approve", ...by, detail: null },
    { comment_id: "c3", action: "delete", ...by, detail: null },
    { comment_id: null, action: "import", ...by, detail: "1" },
    { comment_id: null, action: "purge", ...by, detail: "2" },
    { comment_id: "c2", action: "spam", ...by, detail: null },
  ]);
  assert.deepEqual(d.rows().map((r) => `${r.url} ${r.status}`).sort(), ["/a/ approved", "/blog/post/ approved"]);
});

test("actors: the password mode records 'password'; Access the token's email, or 'access' without one", async () => {
  const d = db();
  await postAdmin({ COMMENTS_DB: d, COMMENTS_ADMIN_PASSWORD: PASSWORD }, { authorization: basic(PASSWORD) });
  const issuer = await accessIssuer();
  const restore = issuer.install();
  try {
    const env = { COMMENTS_DB: d, COMMENTS_ACCESS_TEAM: TEAM, COMMENTS_ACCESS_AUD: AUD };
    // Access-mode writes carry the CSRF header like the panel sends (security review SR-10).
    const panel = { "x-ssg-auth": "1" };
    await postAdmin(env, { ...panel, "cf-access-jwt-assertion": await issuer.token({ email: "mod@example.com" }) }, JSON.stringify({ id: "c2", action: "spam" }));
    await postAdmin(env, { ...panel, "cf-access-jwt-assertion": await issuer.token() }, JSON.stringify({ id: "c3", action: "delete" }));
  } finally {
    restore();
  }
  assert.deepEqual(events(d), [
    { comment_id: "c1", action: "approve", actor: "password", mode: "password", detail: null },
    { comment_id: "c3", action: "delete", actor: "access", mode: "access", detail: null },
    { comment_id: "c2", action: "spam", actor: "mod@example.com", mode: "access", detail: null },
  ]);
});
