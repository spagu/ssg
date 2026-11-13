// The send engine against failures: temporary, permanent, render, settings.
import { test } from "node:test";
import assert from "node:assert/strict";
import { engineHooks } from "../src/engine-hooks.ts";
import { planTick } from "../src/engine-plan.ts";
import { sendBatch } from "../src/engine-send.ts";
import { permanent, temporary } from "../src/mail/_mime.ts";
import {
  addCampaign, batchState, campaignRow, fakeMailer, perPerson, planError, pump, runJob, sendJobs, sendRows, started, subId, truth,
} from "./engine-fixtures.ts";

const requeued = async (n: number, overrides: Record<string, unknown> = {}) => {
  const s = await started(n, overrides);
  s.env.NEWSLETTER_QUEUE.sent.push(...s.jobs.map((body) => ({ body })));
  return s;
};

test("temporary failure: retried by the queue, then sent; finished rows are not resent", async () => {
  const { env, id } = await requeued(3);
  let failures = 0;
  const mail = fakeMailer((m) => (m.to === `${subId(2)}@example.com` && failures++ < 1 ? temporary("resend returned 429") : { ok: true }));
  const stats = await pump(env, { open: mail.open });
  assert.equal(stats.retried, 1);
  assert.equal(mail.sent.length, 3);
  assert.ok([...perPerson(mail.sent).values()].every((n) => n === 1));
  const row = sendRows(env.NEWSLETTER_DB, id)[1];
  assert.equal(row.attempts, 2);
  assert.equal(row.status, "sent");
  assert.equal(row.error, null);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
});

test("temporary failures up to max attempts end failed; the campaign still finishes", async () => {
  const { env, id } = await requeued(2, { NEWSLETTER_SEND_MAX_ATTEMPTS: "2" });
  const mail = fakeMailer((m) => (m.to === `${subId(1)}@example.com` ? temporary("smtp: 451 try later") : { ok: true }));
  await pump(env, { open: mail.open });
  const [row] = sendRows(env.NEWSLETTER_DB, id);
  assert.equal(row.status, "failed");
  assert.equal(row.attempts, 2);
  assert.match(String(row.error), /451 try later \(gave up after 2 attempts\)/);
  const c = campaignRow(env.NEWSLETTER_DB, id);
  assert.equal(c.status, "sent");
  assert.deepEqual([c.sent_count, c.failed_count], [1, 1]);
});

test("permanent failure: failed with the detail, the hook is called; a throwing hook does not stop the batch", async (t) => {
  const calls: unknown[] = [];
  t.mock.method(engineHooks, "onPermanentFailure", async (_env: unknown, f: unknown) => {
    calls.push(f);
    throw new Error("hook broke");
  });
  const { env, id, jobs } = await started(3);
  const mail = fakeMailer((m) => (m.to !== `${subId(3)}@example.com` ? permanent("smtp: 550 5.1.1 user unknown") : { ok: true }));
  await runJob(env, jobs[0], mail.open);
  assert.equal(calls.length, 2);
  assert.deepEqual(calls[0], { email: `${subId(1)}@example.com`, subscriberId: subId(1), campaignId: id, detail: "smtp: 550 5.1.1 user unknown" });
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { failed: 2, sent: 1 });
  assert.equal(sendRows(env.NEWSLETTER_DB, id)[0].error, "smtp: 550 5.1.1 user unknown");
});

test("the default hook does nothing", async () => {
  assert.equal(await engineHooks.onPermanentFailure({} as never, { email: "a@example.com", subscriberId: "s", campaignId: "c", detail: "x" }), undefined);
});

test("a render error pauses the campaign with the reason and sends nothing", async () => {
  const { env, id, jobs } = await started(3, {}, { body: "Hi {{name}}" });
  const mail = fakeMailer();
  await runJob(env, jobs[0], mail.open);
  assert.equal(mail.sent.length, 0);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "paused");
  assert.match(String(planError(env, id)), /^render: unknown variable \{\{name\}\}/);
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { retry: 1 }, "the one claim handed back, no attempt counted; nobody else was claimed");
  assert.ok(sendRows(env.NEWSLETTER_DB, id).every((r) => r.attempts === 0));
  assert.deepEqual(batchState(env, id), ["parked"]);
});

test("missing settings or a refused transport pause the campaign; an unreachable one retries", async () => {
  for (const [overrides, reason] of [
    [{ NEWSLETTER_SITE_URL: "http://example.com" }, /^config: NEWSLETTER_SITE_URL/],
    [{ NEWSLETTER_MAIL_FROM: undefined }, /^config: no sender/],
  ] as const) {
    const { env, id, jobs } = await started(1, overrides);
    await runJob(env, jobs[0], fakeMailer().open);
    assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "paused");
    assert.match(String(planError(env, id)), reason);
  }
  const { env, id, jobs } = await started(2);
  await runJob(env, jobs[0], async () => permanent("smtp: 535 authentication failed"));
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "paused");
  assert.equal(planError(env, id), "transport: smtp: 535 authentication failed");
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), {}, "nobody was claimed");

  const again = await started(2);
  await assert.rejects(runJob(again.env, again.jobs[0], async () => temporary("smtp: connection refused")), /mail transport unavailable/);
  assert.equal((again.env.NEWSLETTER_DB.sqlite.prepare("SELECT used FROM send_rate").get() as { used: number }).used, 0, "budget given back");
});

test("a mailer that fails to close is logged, not fatal", async () => {
  const { env, id, jobs } = await started(1);
  const m = fakeMailer();
  const open = async (e: unknown) => {
    const o = (await m.open(e as never)) as { ok: true; mailer: { send: unknown; close: () => Promise<void> } };
    o.mailer.close = async () => {
      throw new Error("quit failed");
    };
    return o;
  };
  await runJob(env, jobs[0], open);
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 1 });
});

test("every message carries the renderer's headers and the reader's language (Resend transport)", async () => {
  const { env, jobs } = await started(0);
  const db = env.NEWSLETTER_DB.sqlite;
  db.prepare(
    `INSERT INTO subscribers (id, email, status, language, tags, consent_text, consent_at, token, created_at, updated_at)
     VALUES ('p1', 'pl@example.com', 'confirmed', 'pl', '', 'yes', '2026-01-01', 'tok-pl-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx', '2026-01-01', '2026-01-01')`,
  ).run();
  assert.equal(jobs.length, 0);
  const id = addCampaign(env.NEWSLETTER_DB);
  await planTick(env as never);
  const [job] = sendJobs(env);
  const calls: { url: string; body: Record<string, unknown> }[] = [];
  const original = globalThis.fetch;
  globalThis.fetch = (async (url: string, init: RequestInit) => {
    calls.push({ url: String(url), body: JSON.parse(String(init.body)) });
    return new Response("{}", { status: 200 });
  }) as typeof fetch;
  try {
    await sendBatch(job as never, env as never);
  } finally {
    globalThis.fetch = original;
  }
  assert.equal(calls.length, 1);
  const body = calls[0].body as { to: string; subject: string; text: string; headers: Record<string, string> };
  assert.equal(body.to, "pl@example.com");
  assert.equal(body.subject, "Hello pl");
  assert.match(body.text, /Wypisz|wypis/i);
  assert.deepEqual(body.headers, {
    "List-Unsubscribe": "<https://example.com/api/newsletter/unsubscribe?token=tok-pl-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx>",
    "List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
    "List-Id": "<newsletter.example.com>",
    Precedence: "bulk",
  });
  assert.equal(truth(env.NEWSLETTER_DB, id).sent, 1);
});
