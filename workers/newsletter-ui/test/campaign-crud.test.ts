// Campaign CRUD through the real pipeline (auth, CSRF, routes) on real SQL,
// plus the auth refusal table over every GO-125 route.
import { test } from "node:test";
import assert from "node:assert/strict";
import { handleFetch } from "../src/app.ts";
import { ROUTES } from "../src/routes.ts";
import { PASSWORD, fakeCtx, makeEnv, req } from "./fakes.ts";
import { call, mailEnv } from "./campaign-fixtures.ts";

type Env = ReturnType<typeof makeEnv>;

const B = "/newsletter-admin/api/campaigns";
const create = async (env: Env, body: unknown = { subject: "Hello", body: "Hi" }) => (await call(env, "POST", "", body)).body;

test("create: defaults, validation, unknown variables refused", async () => {
  const env = mailEnv();
  const r = await call(env, "POST", "", {});
  assert.equal(r.status, 201);
  assert.equal(r.body.status, "draft");
  assert.equal(r.body.subject, "");
  assert.equal(r.body.bodyFormat, "markdown");
  assert.deepEqual(r.body.segment, { languages: [], tags: [], tagMode: "any" });
  assert.match(r.body.id, /^[0-9a-f-]{36}$/);
  const full = await call(env, "POST", "", {
    subject: " Weekly {{language}} ",
    body: "<p>x</p>",
    bodyFormat: "html",
    textOverride: "Plain",
    fromEmail: "editor@example.com",
    fromName: " Editor ",
    segment: { languages: ["PL"], tags: ["News"], tagMode: "all" },
  });
  assert.equal(full.status, 201);
  assert.deepEqual(
    [full.body.subject, full.body.bodyFormat, full.body.textOverride, full.body.fromEmail, full.body.fromName, full.body.segment],
    ["Weekly {{language}}", "html", "Plain", "editor@example.com", "Editor", { languages: ["pl"], tags: ["news"], tagMode: "all" }],
  );
  const blank = await call(env, "POST", "", { textOverride: "  ", fromEmail: "", fromName: null });
  assert.deepEqual([blank.body.textOverride, blank.body.fromEmail, blank.body.fromName], [null, null, null]);

  const bad: [unknown, string][] = [
    ["not json", "invalid_json"],
    [[1], "invalid_json"],
    [{ subject: 1 }, "invalid_input"],
    [{ subject: "a\nb" }, "invalid_input"],
    [{ subject: "x".repeat(251) }, "invalid_input"],
    [{ body: 5 }, "invalid_input"],
    [{ body: "x".repeat(256 * 1024 + 1) }, "invalid_input"],
    [{ bodyFormat: "pdf" }, "invalid_input"],
    [{ textOverride: 3 }, "invalid_input"],
    [{ fromEmail: "not an address" }, "invalid_input"],
    [{ fromName: "a\r\nBcc: x@example.com" }, "invalid_input"],
    [{ fromName: "x".repeat(201) }, "invalid_input"],
    [{ segment: { tagMode: "most" } }, "invalid_input"],
    [{ body: "Hi {{first_name}}" }, "unknown_variable"],
    [{ subject: "{{ name }}" }, "unknown_variable"],
    [{ textOverride: "{{x}}" }, "unknown_variable"],
  ];
  for (const [body, error] of bad) {
    const res = await call(env, "POST", "", body);
    assert.equal(res.status, 400, JSON.stringify(body).slice(0, 60));
    assert.equal(res.body.error, error);
  }
  const u = await call(env, "POST", "", { body: "{{first_name}} {{site_url}}" });
  assert.match(u.body.detail, /\{\{first_name\}\}/);
  assert.doesNotMatch(u.body.detail, /site_url/);
});

test("get, list with status filter and cursor", async () => {
  const env = mailEnv();
  assert.deepEqual((await call(env, "GET", "")).body, { items: [], nextCursor: null });
  const made = [];
  for (let i = 0; i < 5; i++) made.push(await create(env, { subject: `c${i}`, body: "long body" }));
  // Same created_at for two rows: the id breaks the tie, nothing is skipped.
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET created_at = '2026-01-01T00:00:00.000Z'").run();
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET status = 'sent' WHERE subject = 'c4'").run();
  const got = await call(env, "GET", `/${made[0].id}`);
  assert.equal(got.body.subject, "c0");
  assert.equal(got.body.body, "long body");
  assert.equal((await call(env, "GET", "/nope")).status, 404);

  const seen: string[] = [];
  let cursor: string | null = "";
  while (cursor !== null) {
    const page: { body: { items: { id: string; body?: string }[]; nextCursor: string | null } } = await call(env, "GET", `?limit=2${cursor ? `&cursor=${cursor}` : ""}`);
    assert.ok(page.body.items.length <= 2);
    for (const it of page.body.items) {
      assert.equal(it.body, undefined, "list omits bodies");
      seen.push(it.id);
    }
    cursor = page.body.nextCursor;
  }
  assert.deepEqual([...seen].sort(), made.map((m) => m.id).sort());
  const sent = await call(env, "GET", "?status=sent");
  assert.deepEqual(sent.body.items.map((i: { subject: string }) => i.subject), ["c4"]);
  assert.equal((await call(env, "GET", "?status=bogus")).body.error, "invalid_input");
  assert.equal((await call(env, "GET", "?cursor=!!")).body.error, "invalid_input");
  assert.equal((await call(env, "GET", `?cursor=${btoa('["a"]')}`)).status, 400);
  assert.equal((await call(env, "GET", "?limit=0")).body.items.length, 5);
});

test("update: draft only, atomic, validated", async () => {
  const env = mailEnv();
  const c = await create(env);
  const r = await call(env, "PATCH", `/${c.id}`, { subject: "New", segment: { tags: ["news"] } });
  assert.equal(r.status, 200);
  assert.equal(r.body.subject, "New");
  assert.equal(r.body.body, "Hi", "untouched fields stay");
  assert.deepEqual(r.body.segment.tags, ["news"]);
  assert.equal((await call(env, "PATCH", `/${c.id}`, {})).status, 200);
  assert.equal((await call(env, "PATCH", `/${c.id}`, { body: "{{oops}}" })).body.error, "unknown_variable");
  assert.equal((await call(env, "PATCH", `/${c.id}`, "[]")).body.error, "invalid_json");
  assert.equal((await call(env, "PATCH", "/missing", { subject: "x" })).status, 404);
  for (const status of ["scheduled", "sending", "paused", "cancelled", "sent"]) {
    env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET status = ? WHERE id = ?").run(status, c.id);
    const res = await call(env, "PATCH", `/${c.id}`, { subject: "Too late" });
    assert.equal(res.status, 409, status);
    assert.equal(res.body.error, "not_draft");
  }
  assert.equal((await call(env, "GET", `/${c.id}`)).body.subject, "New");
});

test("delete: draft and cancelled only, with their send records", async () => {
  const env = mailEnv();
  const db = env.NEWSLETTER_DB.sqlite;
  for (const status of ["draft", "cancelled", "scheduled", "sending", "paused", "sent"]) {
    const c = await create(env);
    db.prepare("UPDATE campaigns SET status = ? WHERE id = ?").run(status, c.id);
    db.prepare("INSERT INTO campaign_sends (campaign_id, subscriber_id, status, claimed_at) VALUES (?, 's1', 'sent', 'x')").run(c.id);
    const res = await handleFetch(req(`${B}/${c.id}`, { method: "DELETE", auth: PASSWORD }), env as never, fakeCtx() as never);
    const sends = (db.prepare("SELECT COUNT(*) AS n FROM campaign_sends WHERE campaign_id = ?").get(c.id) as { n: number }).n;
    if (status === "draft" || status === "cancelled") {
      assert.equal(res.status, 204, status);
      assert.equal(sends, 0);
      assert.equal((await call(env, "GET", `/${c.id}`)).status, 404);
    } else {
      assert.equal(res.status, 409, status);
      assert.equal((await res.json()).error, "not_deletable");
      assert.equal(sends, 1);
    }
  }
  assert.equal((await call(env, "DELETE", "/missing")).status, 404);
});

test("duplicate: a new draft with the same content, from any state", async () => {
  const env = mailEnv();
  const c = await create(env, { subject: "S", body: "B", bodyFormat: "html", textOverride: "T", fromEmail: "e@example.com", fromName: "N", segment: { tags: ["news"] } });
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET status = 'sent', sent_count = 5 WHERE id = ?").run(c.id);
  const d = await call(env, "POST", `/${c.id}/duplicate`);
  assert.equal(d.status, 201);
  assert.notEqual(d.body.id, c.id);
  assert.equal(d.body.status, "draft");
  assert.equal(d.body.sentCount, 0);
  for (const k of ["subject", "body", "bodyFormat", "textOverride", "fromEmail", "fromName", "segment"]) assert.deepEqual(d.body[k], c[k], k);
  assert.equal((await call(env, "POST", "/missing/duplicate")).status, 404);
});

test("an unreadable stored segment reaches nobody", async () => {
  const env = mailEnv();
  const c = await create(env);
  for (const raw of ["not json", '{"tags":["a,b"]}']) {
    env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET segment = ? WHERE id = ?").run(raw, c.id);
    const r = await call(env, "POST", `/${c.id}/preview`, {});
    assert.deepEqual(r.body.recipients, { total: 0, byLanguage: {} });
  }
});

test("auth refusal table: every campaign route needs auth; writes need the CSRF header", async () => {
  const mine = ROUTES.filter((r) => r.path.startsWith("/api/campaigns"));
  // 13 from GO-125 + GET /:id/progress (GO-126) + POST /from-post (GO-130) + GET /:id/stats (GO-129)
  // + GET /:id/uncertain, POST /:id/uncertain/resend, POST /:id/uncertain/dismiss
  assert.equal(mine.length, 19);
  const env = mailEnv();
  const c = await create(env);
  for (const [n, r] of mine.entries()) {
    const path = "/newsletter-admin" + r.path.replace(":id", c.id);
    const noAuth = await handleFetch(req(path, { method: r.method }), env as never, fakeCtx() as never);
    assert.equal(noAuth.status, 401, `${r.method} ${r.path} without auth`);
    // One IP per route: the failed-login lockout would otherwise answer 429.
    const wrong = await handleFetch(req(path, { method: r.method, auth: "wrong", ip: `198.51.100.${n}` }), env as never, fakeCtx() as never);
    assert.equal(wrong.status, 401, `${r.method} ${r.path} wrong password`);
    if (r.method !== "GET") {
      const noCsrf = await handleFetch(req(path, { method: r.method, auth: PASSWORD, csrf: false }), env as never, fakeCtx() as never);
      assert.equal(noCsrf.status, 403, `${r.method} ${r.path} without CSRF header`);
    }
  }
  assert.equal((await call(env, "GET", `/${c.id}`)).body.status, "draft", "nothing was changed by refused requests");
  const unset = makeEnv({ NEWSLETTER_ADMIN_PASSWORD: undefined });
  assert.equal((await handleFetch(req(B, { auth: PASSWORD }), unset as never, fakeCtx() as never)).status, 503);
});
