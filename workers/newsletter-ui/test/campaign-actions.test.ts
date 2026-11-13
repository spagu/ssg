// Preview, test send (and its limit), schedule and the plain state moves,
// through the real pipeline on real SQL. The transport is Resend on a fake
// fetch, so the test sees exactly what would have been sent.
import { test } from "node:test";
import assert from "node:assert/strict";
import { PREVIEW_TOKEN, hasUnsubscribe } from "../src/campaign-actions.ts";
import { senderOf } from "../src/campaign-model.ts";
import { TEST_KEY, TEST_LIMIT, takeTestSend } from "../src/campaign-limit.ts";
import { call, fakeFetch, mailEnv } from "./campaign-fixtures.ts";

const UNSUB = `https://example.com/api/newsletter/unsubscribe?token=${PREVIEW_TOKEN}`;
type Env = ReturnType<typeof mailEnv>;
const create = async (env: Env, body: unknown = { subject: "Hello {{language}}", body: "Hi **there**, [site]({{site_url}})" }) =>
  (await call(env, "POST", "", body)).body;
const statusOf = (env: Env, id: string) =>
  (env.NEWSLETTER_DB.sqlite.prepare("SELECT status FROM campaigns WHERE id = ?").get(id) as { status: string }).status;
const setStatus = (env: Env, id: string, status: string) =>
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET status = ? WHERE id = ?").run(status, id);

test("preview: rendered parts + recipients by language", async () => {
  const env = mailEnv();
  const c = await create(env);
  const r = await call(env, "POST", `/${c.id}/preview`, { language: "pl" });
  assert.equal(r.status, 200);
  assert.equal(r.body.subject, "Hello pl");
  assert.match(r.body.html, /<html lang="pl"/);
  assert.match(r.body.html, /Hi <strong>there<\/strong>/);
  assert.ok(r.body.text.includes(`Wypisz się: ${UNSUB}`));
  assert.equal(r.body.headers["List-Unsubscribe"], `<${UNSUB}>`);
  assert.deepEqual(r.body.recipients, { total: 2, byLanguage: { en: 1, pl: 1 } });
  assert.equal((await call(env, "POST", `/${c.id}/preview`)).body.subject, "Hello en", "no body = en");
  assert.equal((await call(env, "POST", `/${c.id}/preview`, { language: "<x>" })).status, 400);
  assert.equal((await call(env, "POST", "/missing/preview", {})).status, 404);
  const noSite = await call(mailEnv({ NEWSLETTER_SITE_URL: undefined }), "POST", `/${c.id}/preview`, {});
  assert.equal(noSite.status, 404, "other database: not found first");
  const env2 = mailEnv({ NEWSLETTER_SITE_URL: "http://example.com" });
  const c2 = await create(env2);
  const r2 = await call(env2, "POST", `/${c2.id}/preview`, {});
  assert.equal(r2.status, 503);
  assert.equal(r2.body.error, "no_site_url");
});

test("test send: the preview, byte for byte, with [TEST] in the subject", async () => {
  const env = mailEnv();
  const c = await create(env, { subject: "Hi", body: "Body", fromName: "Campaign Name" });
  const f = fakeFetch();
  try {
    const r = await call(env, "POST", `/${c.id}/test`, { to: "me@example.com", language: "de" });
    assert.equal(r.status, 200);
    assert.deepEqual(r.body, { ok: true, transport: "resend" });
    const preview = (await call(env, "POST", `/${c.id}/preview`, { language: "de" })).body;
    const sent = f.calls[0].body;
    assert.equal(f.calls[0].url, "https://api.resend.com/emails");
    assert.equal(sent.to, "me@example.com");
    assert.equal(sent.subject, `[TEST] ${preview.subject}`);
    assert.equal(sent.html, preview.html);
    assert.equal(sent.text, preview.text);
    assert.deepEqual(sent.headers, preview.headers);
    assert.equal(sent.from, "Campaign Name <news@example.com>");
  } finally {
    f.restore();
  }
});

test("test send: refusals and transport failure", async () => {
  const env = mailEnv();
  const c = await create(env);
  for (const body of [undefined, {}, { to: "nope" }, { to: 5 }]) {
    assert.equal((await call(env, "POST", `/${c.id}/test`, body)).body.error, "invalid_to");
  }
  assert.equal((await call(env, "POST", `/${c.id}/test`, { to: "a@example.com", language: "x y" })).status, 400);
  assert.equal((await call(env, "POST", "/missing/test", { to: "a@example.com" })).status, 404);
  const cases: [Record<string, unknown>, number, string][] = [
    [{ NEWSLETTER_SITE_URL: undefined }, 503, "no_site_url"],
    [{ NEWSLETTER_RESEND_API_KEY: undefined }, 503, "no_transport"],
    [{ NEWSLETTER_MAIL_FROM: undefined }, 400, "no_sender"],
  ];
  for (const [over, status, error] of cases) {
    const e = mailEnv(over);
    const x = await create(e);
    const r = await call(e, "POST", `/${x.id}/test`, { to: "a@example.com" });
    assert.deepEqual([r.status, r.body.error], [status, error]);
  }
  const f = fakeFetch(() => new Response("down", { status: 503 }));
  try {
    const r = await call(env, "POST", `/${c.id}/test`, { to: "a@example.com" });
    assert.equal(r.status, 502);
    assert.deepEqual(r.body, { error: "send_failed", detail: "resend returned 503", temporary: true });
  } finally {
    f.restore();
  }
});

test("test send limit: 10 per hour, then 429; the window reopens", async () => {
  const env = mailEnv();
  const c = await create(env);
  const f = fakeFetch();
  try {
    for (let i = 0; i < TEST_LIMIT; i++) assert.equal((await call(env, "POST", `/${c.id}/test`, { to: "a@example.com" })).status, 200, `send ${i}`);
    const r = await call(env, "POST", `/${c.id}/test`, { to: "b@example.com" });
    assert.equal(r.status, 429);
    assert.equal(r.body.error, "test_limit");
    assert.equal(r.headers.get("retry-after"), "3600");
    assert.equal(f.calls.length, TEST_LIMIT, "the refused one never reached the transport");
  } finally {
    f.restore();
  }
  const db = env.NEWSLETTER_DB;
  const later = new Date(Date.now() + 61 * 60 * 1000);
  assert.equal(await takeTestSend(env as never, later), true, "a new window");
  const row = db.sqlite.prepare("SELECT value FROM settings WHERE key = ?").get(TEST_KEY) as { value: string };
  assert.deepEqual(JSON.parse(row.value), { since: later.toISOString(), count: 1 });
  // Concurrent takers can never exceed the limit.
  const results = await Promise.all(Array.from({ length: 20 }, () => takeTestSend(env as never, later)));
  assert.equal(results.filter(Boolean).length, TEST_LIMIT - 1);
});

test("schedule: validates everything, then draft → scheduled", async () => {
  const env = mailEnv();
  const c = await create(env);
  const r = await call(env, "POST", `/${c.id}/schedule`, {});
  assert.equal(r.status, 200);
  assert.equal(r.body.status, "scheduled");
  assert.ok(Math.abs(Date.parse(r.body.scheduledAt) - Date.now()) < 5000, "omitted = now");
  assert.deepEqual(r.body.recipients, { total: 2, byLanguage: { en: 1, pl: 1 } });
  // Scheduled again: not from this state.
  const again = await call(env, "POST", `/${c.id}/schedule`, {});
  assert.deepEqual([again.status, again.body.error], [409, "invalid_transition"]);

  const future = await create(env);
  const at = "2030-05-01T10:00:00.000Z";
  assert.equal((await call(env, "POST", `/${future.id}/schedule`, { at })).body.scheduledAt, at);
  const past = await create(env);
  const p = await call(env, "POST", `/${past.id}/schedule`, { at: "2001-01-01T00:00:00Z" });
  assert.ok(Date.parse(p.body.scheduledAt) > Date.parse("2020-01-01"), "a past time = now");
  for (const at2 of ["tomorrow", 5]) assert.equal((await call(env, "POST", `/${past.id}/schedule`, { at: at2 })).body.error, "invalid_input");
  assert.equal((await call(env, "POST", "/missing/schedule", {})).status, 404);
});

test("schedule refusals: each missing piece has its own answer", async () => {
  const cases: [Record<string, unknown>, Record<string, unknown>, number, string][] = [
    [{ NEWSLETTER_SITE_URL: undefined }, {}, 503, "no_site_url"],
    [{ NEWSLETTER_RESEND_API_KEY: undefined }, {}, 503, "no_transport"],
    [{ NEWSLETTER_MAIL_FROM: undefined }, {}, 400, "no_sender"],
    [{}, { subject: " " }, 400, "empty_subject"],
    [{}, { subject: "S", body: "  " }, 400, "empty_body"],
    [{}, { subject: "S", body: "B", segment: { languages: ["de"] } }, 400, "empty_segment"],
  ];
  for (const [over, body, status, error] of cases) {
    const env = mailEnv(over);
    const c = await create(env, { subject: "S", body: "B", ...body });
    const r = await call(env, "POST", `/${c.id}/schedule`, {});
    assert.deepEqual([r.status, r.body.error], [status, error], error);
    assert.equal(statusOf(env, c.id), "draft");
  }
  // A sender on the campaign is enough without NEWSLETTER_MAIL_FROM.
  const env = mailEnv({ NEWSLETTER_MAIL_FROM: undefined });
  const c = await create(env, { subject: "S", body: "B", fromEmail: "editor@example.com" });
  assert.equal((await call(env, "POST", `/${c.id}/schedule`, {})).status, 200);
  // A variable smuggled in behind the API's back is still caught.
  const env2 = mailEnv();
  const d = await create(env2);
  env2.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET body = 'Hi {{name}}' WHERE id = ?").run(d.id);
  const u = await call(env2, "POST", `/${d.id}/schedule`, {});
  assert.deepEqual([u.status, u.body.error, u.body.detail], [400, "unknown_variable", "{{name}}"]);
});

test("schedule: two concurrent requests, one scheduled campaign", async () => {
  const env = mailEnv();
  const c = await create(env);
  const results = await Promise.all([call(env, "POST", `/${c.id}/schedule`, {}), call(env, "POST", `/${c.id}/schedule`, {})]);
  assert.deepEqual(results.map((r) => r.status).sort(), [200, 409]);
});

test("unschedule, pause, resume, cancel: only from their own states", async () => {
  const env = mailEnv();
  const c = await create(env);
  const actions: [string, string, string[]][] = [
    ["unschedule", "draft", ["scheduled"]],
    ["pause", "paused", ["sending"]],
    ["resume", "sending", ["paused"]],
    ["cancel", "cancelled", ["scheduled", "sending", "paused"]],
  ];
  for (const [action, to, from] of actions) {
    for (const status of ["draft", "scheduled", "sending", "paused", "cancelled", "sent"]) {
      setStatus(env, c.id, status);
      const r = await call(env, "POST", `/${c.id}/${action}`);
      if (from.includes(status)) {
        assert.equal(r.status, 200, `${action} from ${status}`);
        assert.equal(r.body.status, to);
      } else {
        assert.deepEqual([r.status, r.body.error], [409, "invalid_transition"], `${action} from ${status}`);
        assert.equal(statusOf(env, c.id), status);
      }
    }
    assert.equal((await call(env, "POST", `/missing/${action}`)).status, 404);
  }
  setStatus(env, c.id, "scheduled");
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE campaigns SET scheduled_at = 'x' WHERE id = ?").run(c.id);
  assert.equal((await call(env, "POST", `/${c.id}/unschedule`)).body.scheduledAt, null);
});

test("schedule: a body-less request schedules now; a race lost after the checks is 409", async () => {
  const env = mailEnv();
  const c = await create(env);
  assert.equal((await call(env, "POST", `/${c.id}/schedule`)).status, 200);
  // Someone else moves the campaign between the checks and the UPDATE.
  const d = await create(env);
  const db = env.NEWSLETTER_DB;
  const prepare = db.prepare;
  db.prepare = (sql: string) => {
    if (sql.startsWith("UPDATE campaigns SET status")) setStatus(env, d.id, "cancelled");
    return prepare(sql);
  };
  const r = await call(env, "POST", `/${d.id}/schedule`, {});
  db.prepare = prepare;
  assert.deepEqual([r.status, r.body.error], [409, "invalid_transition"]);
  assert.equal(statusOf(env, d.id), "cancelled");
});

test("hasUnsubscribe: html, text and header are each required", () => {
  const url = "https://example.com/api/newsletter/unsubscribe?token=t";
  const ok = { subject: "s", html: url, text: url, headers: { "List-Unsubscribe": `<${url}>` } };
  assert.equal(hasUnsubscribe(ok, url), true);
  assert.equal(hasUnsubscribe({ ...ok, html: "" }, url), false);
  assert.equal(hasUnsubscribe({ ...ok, text: "" }, url), false);
  assert.equal(hasUnsubscribe({ ...ok, headers: {} }, url), false);
});

test("senderOf: campaign first, then the environment, else nobody", () => {
  const c = { fromEmail: null, fromName: null } as never;
  assert.deepEqual(senderOf(c, { NEWSLETTER_MAIL_FROM: "news@example.com" } as never), { from: "news@example.com", fromName: "" });
  assert.equal(senderOf(c, {} as never), null);
  assert.deepEqual(senderOf({ fromEmail: "e@example.com", fromName: "E" } as never, { NEWSLETTER_MAIL_FROM_NAME: "X" } as never), { from: "e@example.com", fromName: "E" });
});
