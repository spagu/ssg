// Backward-compatibility suite for the moderation endpoints, written before
// GO-143 touched this template (it had no tests): every answer of
// /api/comments/admin and /api/comments/import — status, headers, body — and
// what each action leaves in the table, in both auth modes. Later work on this
// worker (GO-136, GO-148) must keep this file passing unedited. Run from the
// repository root:
//   node --import ./workers/comments/test/resolve.mjs --test 'workers/comments/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import * as admin from "../functions/api/comments/admin.ts";
import * as importer from "../functions/api/comments/import.ts";
import { AUD, JSON_HEADERS, PASSWORD, TEAM, accessIssuer, basic, call, seed, sqliteD1 } from "./harness.ts";

const A = "/api/comments/admin";
const I = "/api/comments/import";
const auth = { authorization: basic(PASSWORD) };
const basicEnv = (extra: Record<string, unknown> = {}) => ({ COMMENTS_DB: sqliteD1(), COMMENTS_ADMIN_PASSWORD: PASSWORD, ...extra });
const answer = (status: number, body: unknown) => ({ status, headers: JSON_HEADERS, body: JSON.stringify(body) });
const post = (body: unknown) => ({ method: "POST", headers: { ...auth, "content-type": "application/json" }, body: typeof body === "string" ? body : JSON.stringify(body) });

test("compat: Basic mode refusals, identical on GET and POST of both endpoints", async () => {
  const env = basicEnv();
  const cases: [string, Record<string, unknown>, Record<string, string>, unknown][] = [
    ["no password configured", { COMMENTS_DB: sqliteD1() }, auth, answer(503, { error: "moderation not configured" })],
    ["no credentials", env, {}, answer(401, { error: "unauthorized" })],
    ["wrong password", env, { authorization: basic("nope") }, answer(401, { error: "unauthorized" })],
    ["malformed Basic", env, { authorization: "Basic %%%" }, answer(401, { error: "unauthorized" })],
    ["not Basic", env, { authorization: `Bearer ${PASSWORD}` }, answer(401, { error: "unauthorized" })],
  ];
  for (const [name, e, headers, want] of cases) {
    assert.deepEqual(await call(admin.onRequestGet, { path: A, headers, env: e }), want, `GET admin: ${name}`);
    assert.deepEqual(await call(admin.onRequestPost, { method: "POST", path: A, headers, body: "{}", env: e }), want, `POST admin: ${name}`);
    assert.deepEqual(await call(importer.onRequestPost, { method: "POST", path: I, headers, body: "{}", env: e }), want, `POST import: ${name}`);
  }
});

test("compat: without the D1 binding an authorised call is 503", async () => {
  const env = { COMMENTS_ADMIN_PASSWORD: PASSWORD };
  const want = answer(503, { error: "comments not configured" });
  assert.deepEqual(await call(admin.onRequestGet, { path: A, headers: auth, env }), want);
  assert.deepEqual(await call(admin.onRequestPost, { path: A, ...post({ action: "purge" }), env }), want);
  assert.deepEqual(await call(importer.onRequestPost, { path: I, ...post({ items: [] }), env }), want);
});

test("compat: GET lists one status, newest first; pending by default; a bad status is 400", async () => {
  const env = basicEnv();
  seed(env.COMMENTS_DB, [
    { id: "p1", status: "pending", created_at: "2026-01-01T00:00:00.000Z" },
    { id: "p2", status: "pending", created_at: "2026-01-02T00:00:00.000Z" },
    { id: "s1", status: "spam", created_at: "2026-01-03T00:00:00.000Z" },
  ]);
  const row = (id: string, status: string, day: string) => ({ id, url: "/blog/post/", author: `Author ${id}`, body: `Body ${id}`, status, created_at: `2026-01-0${day}T00:00:00.000Z` });
  assert.deepEqual(await call(admin.onRequestGet, { path: A, headers: auth, env }), answer(200, { status: "pending", count: 2, comments: [row("p2", "pending", "2"), row("p1", "pending", "1")] }));
  assert.deepEqual(await call(admin.onRequestGet, { path: A + "?status=spam", headers: auth, env }), answer(200, { status: "spam", count: 1, comments: [row("s1", "spam", "3")] }));
  assert.deepEqual(await call(admin.onRequestGet, { path: A + "?status=approved", headers: auth, env }), answer(200, { status: "approved", count: 0, comments: [] }));
  assert.deepEqual(await call(admin.onRequestGet, { path: A + "?status=deleted", headers: auth, env }), answer(400, { error: "status must be pending, spam or approved" }));
});

test("compat: POST approve, spam and delete change the row; bad input is 400 or 422", async () => {
  const env = basicEnv();
  seed(env.COMMENTS_DB, [
    { id: "a", status: "pending", created_at: "2026-01-01T00:00:00.000Z" },
    { id: "b", status: "pending", created_at: "2026-01-01T00:00:00.000Z" },
    { id: "c", status: "pending", created_at: "2026-01-01T00:00:00.000Z" },
  ]);
  const ok = answer(200, { ok: true });
  assert.deepEqual(await call(admin.onRequestPost, { path: A, ...post({ id: "a", action: "approve" }), env }), ok);
  assert.deepEqual(await call(admin.onRequestPost, { path: A, ...post({ id: "b", action: "spam" }), env }), ok);
  assert.deepEqual(await call(admin.onRequestPost, { path: A, ...post({ id: "c", action: "delete" }), env }), ok);
  assert.deepEqual(await call(admin.onRequestPost, { path: A, ...post({ id: "missing", action: "approve" }), env }), ok);
  assert.deepEqual(env.COMMENTS_DB.rows(), [
    { id: "a", url: "/blog/post/", author: "Author a", status: "approved" },
    { id: "b", url: "/blog/post/", author: "Author b", status: "spam" },
  ]);
  assert.deepEqual(await call(admin.onRequestPost, { path: A, ...post("not json"), env }), answer(400, { error: "invalid JSON" }));
  assert.deepEqual(await call(admin.onRequestPost, { path: A, ...post({ action: "approve" }), env }), answer(422, { error: "id is required" }));
  assert.deepEqual(await call(admin.onRequestPost, { path: A, ...post({ id: "a", action: "ban" }), env }), answer(400, { error: "action must be approve, spam or delete" }));
});

test("compat: POST purge deletes old spam and pending only, and only with a retention set", async () => {
  const env = basicEnv();
  seed(env.COMMENTS_DB, [
    { id: "old-spam", status: "spam", created_at: "2000-01-01T00:00:00.000Z" },
    { id: "old-pending", status: "pending", created_at: "2000-01-01T00:00:00.000Z" },
    { id: "old-approved", status: "approved", created_at: "2000-01-01T00:00:00.000Z" },
    { id: "new-spam", status: "spam", created_at: new Date().toISOString() },
  ]);
  assert.deepEqual(await call(admin.onRequestPost, { path: A, ...post({ action: "purge" }), env }), answer(200, { ok: true, purged: 0 }));
  const kept = { ...env, COMMENTS_RETENTION_DAYS: "30" };
  assert.deepEqual(await call(admin.onRequestPost, { path: A, ...post({ action: "purge" }), env: kept }), answer(200, { ok: true, purged: 2 }));
  assert.deepEqual(env.COMMENTS_DB.rows().map((r) => r.id), ["new-spam", "old-approved"]);
});

test("compat: import inserts valid rows once, counts the rest, and refuses bad payloads", async () => {
  const env = basicEnv();
  const items = [
    { url: "/a/", author: "Ada", body: "First!", created_at: "2021-05-01T10:00:00Z" },
    { url: "/b/", author: "Bob", body: "Held", status: "pending", email: "bob@example.com" },
    { url: "https://example.com/off-site", author: "Eve", body: "x" },
    { url: "/c/", author: "", body: "no author" },
  ];
  assert.deepEqual(
    await call(importer.onRequestPost, { path: I, ...post({ items, defaultStatus: "spam" }), env }),
    answer(200, { ok: true, total: 4, imported: 2, duplicate: 0, invalid: 2 }),
  );
  assert.deepEqual(
    await call(importer.onRequestPost, { path: I, ...post({ items: items.slice(0, 1) }), env }),
    answer(200, { ok: true, total: 1, imported: 0, duplicate: 1, invalid: 0 }),
  );
  assert.deepEqual(
    env.COMMENTS_DB.rows().map((r) => [r.url, r.status]).sort(),
    [["/a/", "spam"], ["/b/", "pending"]],
  );
  assert.deepEqual(await call(importer.onRequestPost, { path: I, ...post("{"), env }), answer(400, { error: "invalid JSON" }));
  assert.deepEqual(await call(importer.onRequestPost, { path: I, ...post({ items: "x" }), env }), answer(422, { error: "items must be an array" }));
  assert.deepEqual(
    await call(importer.onRequestPost, { path: I, ...post({ items: new Array(1001).fill({}) }), env }),
    answer(413, { error: "too many items (max 1000 per request — chunk the export)" }),
  );
});

test("compat: Access mode — a valid token passes, everything else is refused as before", async () => {
  const issuer = await accessIssuer();
  const restore = issuer.install();
  try {
    // A password is set too: Access takes precedence, so Basic must not work.
    const env = basicEnv({ COMMENTS_ACCESS_TEAM: TEAM, COMMENTS_ACCESS_AUD: AUD });
    const get = (headers: Record<string, string>) => call(admin.onRequestGet, { path: A, headers, env });
    assert.deepEqual(await get({ "cf-access-jwt-assertion": await issuer.token() }), answer(200, { status: "pending", count: 0, comments: [] }));
    assert.deepEqual(await get({ cookie: `CF_Authorization=${await issuer.token()}` }), answer(200, { status: "pending", count: 0, comments: [] }));
    assert.deepEqual(await get(auth), answer(401, { error: "Cloudflare Access required" }));
    assert.deepEqual(await get({ "cf-access-jwt-assertion": "a.b" }), answer(401, { error: "malformed Access token" }));
    assert.deepEqual(await get({ "cf-access-jwt-assertion": await issuer.token({ aud: ["other"] }) }), answer(403, { error: "wrong Access audience" }));
    assert.deepEqual(await get({ "cf-access-jwt-assertion": await issuer.token({ iss: "https://elsewhere.example.com" }) }), answer(403, { error: "wrong Access issuer" }));
    assert.deepEqual(await get({ "cf-access-jwt-assertion": await issuer.token({ exp: 1 }) }), answer(401, { error: "Access token expired" }));
    const forged = (await issuer.token()).replace(/\.[^.]+$/, ".AAAA");
    assert.deepEqual(await get({ "cf-access-jwt-assertion": forged }), answer(401, { error: "invalid Access signature" }));
  } finally {
    restore();
  }
});
