// The cron side: starting, pacing, re-queuing lost work, finishing.
import { test } from "node:test";
import assert from "node:assert/strict";
import { transition } from "../src/campaign-state.ts";
import { aheadOf, planTick } from "../src/engine-plan.ts";
import { engineConfig } from "../src/engine-rate.ts";
import { addCampaign, campaignRow, engineEnv, fakeMailer, perPerson, pump, sendJobs, subId } from "./engine-fixtures.ts";

test("a zero-recipient campaign finishes on its first tick", async () => {
  const env = await engineEnv(0);
  const id = addCampaign(env.NEWSLETTER_DB);
  await planTick(env as never);
  const c = campaignRow(env.NEWSLETTER_DB, id);
  assert.equal(c.status, "sent");
  assert.deepEqual([c.total_count, c.sent_count, c.failed_count], [0, 0, 0]);
  assert.ok(c.started_at && c.finished_at);
  assert.equal(env.NEWSLETTER_QUEUE.sent.length, 0);
});

test("start: only due campaigns; total counted; about a minute of work queued as keyset ranges", async () => {
  const env = await engineEnv(100);
  const due = addCampaign(env.NEWSLETTER_DB);
  const later = addCampaign(env.NEWSLETTER_DB, { scheduled_at: "2999-01-01T00:00:00.000Z" });
  const draft = addCampaign(env.NEWSLETTER_DB, { status: "draft", scheduled_at: null });
  await planTick(env as never);
  assert.equal(campaignRow(env.NEWSLETTER_DB, due).status, "sending");
  assert.equal(campaignRow(env.NEWSLETTER_DB, due).total_count, 100);
  assert.equal(campaignRow(env.NEWSLETTER_DB, later).status, "scheduled");
  assert.equal(campaignRow(env.NEWSLETTER_DB, draft).status, "draft");
  const jobs = sendJobs(env);
  assert.equal(jobs.length, 6, "ceil(60 / 10) ranges ahead at the defaults");
  assert.deepEqual(jobs[0], { type: "newsletter.send-batch", campaignId: due, after: "", until: subId(10) });
  assert.deepEqual(jobs[1], { type: "newsletter.send-batch", campaignId: due, after: subId(10), until: subId(20) });
  assert.ok(jobs.every((j) => !("email" in j)), "a job never carries addresses");
  // The next tick adds nothing while those six are fresh.
  await planTick(env as never);
  assert.equal(env.NEWSLETTER_QUEUE.sent.length, 6);
});

test("aheadOf: at least one, at most 100 ranges", () => {
  assert.equal(aheadOf({ ...engineConfig({} as never) }), 6);
  assert.equal(aheadOf({ rate: 1, batchSize: 1, staleMs: 0, maxAttempts: 1 }), 1);
  assert.equal(aheadOf({ rate: 100000, batchSize: 10, staleMs: 0, maxAttempts: 1 }), 100);
});

test("two planners racing on the same campaign: one start, each range queued once", async () => {
  const env = await engineEnv(35);
  const id = addCampaign(env.NEWSLETTER_DB);
  const log: string[] = [];
  const original = console.error;
  console.error = (...a: unknown[]) => void log.push(a.join(" "));
  try {
    await Promise.all([planTick(env as never), planTick(env as never), planTick(env as never)]);
  } finally {
    console.error = original;
  }
  assert.deepEqual(log, []);
  const keys = sendJobs(env).map((j) => j.after);
  assert.equal(new Set(keys).size, keys.length, "no range queued twice");
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).total_count, 35);
  const mail = fakeMailer();
  await pump(env, { open: mail.open });
  assert.equal(perPerson(mail.sent).size, 35);
  assert.ok([...perPerson(mail.sent).values()].every((n) => n === 1));
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
});

test("a lost job is re-queued after the stale timeout, once, and nobody is mailed twice", async () => {
  const env = await engineEnv(15);
  const id = addCampaign(env.NEWSLETTER_DB);
  await planTick(env as never);
  const first = sendJobs(env);
  env.NEWSLETTER_QUEUE.sent.length = 0; // the queue lost both jobs
  const mail = fakeMailer();
  await pump(env, { open: mail.open });
  assert.equal(mail.sent.length, 0, "nothing until the ranges go stale");
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE send_batches SET updated_at = '2000-01-01T00:00:00.000Z'").run();
  await Promise.all([planTick(env as never), planTick(env as never)]);
  assert.deepEqual(sendJobs(env).map((j) => j.after).sort(), first.map((j) => j.after).sort(), "each stale range re-queued once");
  await pump(env, { open: mail.open });
  assert.equal(perPerson(mail.sent).size, 15);
  assert.ok([...perPerson(mail.sent).values()].every((n) => n === 1));
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
});

test("a dead-lettered job is recorded and its range still completes", async () => {
  const env = await engineEnv(3);
  const id = addCampaign(env.NEWSLETTER_DB);
  let down = true;
  const mail = fakeMailer(() => (down ? { ok: false, temporary: true, detail: "resend returned 503" } : { ok: true }));
  const stats = await pump(env, { open: mail.open, maxRetries: 1 });
  assert.equal(stats.deadLettered, 1);
  assert.equal((env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM send_dead_letters").get() as { n: number }).n, 1);
  down = false;
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE send_batches SET updated_at = '2000-01-01T00:00:00.000Z'").run();
  await pump(env, { open: mail.open });
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
  assert.equal(perPerson(mail.sent).size, 3);
});

test("finish waits for claimed rows; sent/failed counters come from the ledger", async () => {
  const env = await engineEnv(2);
  const id = addCampaign(env.NEWSLETTER_DB);
  await planTick(env as never);
  env.NEWSLETTER_QUEUE.sent.length = 0;
  const db = env.NEWSLETTER_DB.sqlite;
  db.prepare("UPDATE send_batches SET status = 'done'").run();
  db.prepare("INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, claimed_at) VALUES (?, ?, 'claimed', 1, ?)").run(id, subId(1), new Date().toISOString());
  db.prepare("INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, claimed_at) VALUES (?, ?, 'failed', 1, ?)").run(id, subId(2), new Date().toISOString());
  db.prepare("UPDATE campaigns SET sent_count = 99 WHERE id = ?").run(id);
  await planTick(env as never);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sending");
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).sent_count, 0, "live counters are recounted, not trusted");
  db.prepare("UPDATE campaign_sends SET status = 'sent' WHERE subscriber_id = ?").run(subId(1));
  await planTick(env as never);
  const c = campaignRow(env.NEWSLETTER_DB, id);
  assert.equal(c.status, "sent");
  assert.deepEqual([c.sent_count, c.failed_count], [1, 1]);
});

test("a campaign that lost its plan row gets one; a failing campaign does not stop the others", async () => {
  const env = await engineEnv(2);
  const a = addCampaign(env.NEWSLETTER_DB, { id: "a", segment: '{"languages":["de"],"tags":[],"tagMode":"any"}' });
  const b = addCampaign(env.NEWSLETTER_DB, { id: "b" });
  await transition(env as never, a, "scheduled", "sending");
  env.NEWSLETTER_DB.state.failOn = /SELECT cursor, planned_all FROM send_plans/;
  const log: unknown[] = [];
  const original = console.error;
  console.error = (...x: unknown[]) => void log.push(x[0]);
  try {
    await planTick(env as never);
  } finally {
    console.error = original;
    env.NEWSLETTER_DB.state.failOn = null;
  }
  assert.equal(log.length, 2);
  await planTick(env as never);
  assert.equal(campaignRow(env.NEWSLETTER_DB, a).status, "sent", "nobody speaks German: done at once");
  assert.equal(sendJobs(env).filter((j) => j.campaignId === b).length, 1);
});

test("without the queue binding the cron warns and changes nothing", async () => {
  const env = await engineEnv(1, { NEWSLETTER_QUEUE: undefined });
  const id = addCampaign(env.NEWSLETTER_DB);
  const warned: unknown[] = [];
  const original = console.warn;
  console.warn = (...x: unknown[]) => void warned.push(x[0]);
  try {
    await planTick(env as never);
  } finally {
    console.warn = original;
  }
  assert.match(String(warned[0]), /NEWSLETTER_QUEUE is not bound/);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "scheduled");
});

test("counters of a campaign cancelled in the last hour are closed by the cron", async () => {
  const env = await engineEnv(1);
  const id = addCampaign(env.NEWSLETTER_DB);
  await planTick(env as never);
  await transition(env as never, id, "sending", "cancelled");
  env.NEWSLETTER_DB.sqlite
    .prepare("INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, claimed_at) VALUES (?, ?, 'sent', 1, ?)")
    .run(id, subId(1), new Date().toISOString());
  await planTick(env as never);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).sent_count, 1);
});
