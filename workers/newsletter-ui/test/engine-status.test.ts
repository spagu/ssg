// The panel's view of sending, the dead-letter consumer, and the rate counter.
import { test } from "node:test";
import assert from "node:assert/strict";
import worker from "../src/index.ts";
import { planTick } from "../src/engine-plan.ts";
import {
  DEFAULT_BATCH, DEFAULT_RATE, engineConfig, giveBack, minuteOf, pruneRate, rateUsage, secondsToNextMinute, takeBudget,
} from "../src/engine-rate.ts";
import { handleDeadLetters, isDeadLetterQueue } from "../src/engine-status.ts";
import { addCampaign, engineEnv, fakeMailer, pump, sendJobs } from "./engine-fixtures.ts";
import { fakeBatch } from "./fakes.ts";
import { api } from "./subscriber-seed.ts";

test("GET /api/engine/status and /api/campaigns/:id/progress: numbers from the tables", async () => {
  const env = await engineEnv(25);
  const id = addCampaign(env.NEWSLETTER_DB, { subject: "Spring" });
  addCampaign(env.NEWSLETTER_DB, { status: "draft", scheduled_at: null });
  await planTick(env as never);
  const [job, ...rest] = sendJobs(env);
  env.NEWSLETTER_QUEUE.sent.length = 0;
  const { sendBatch } = await import("../src/engine-send.ts");
  let n = 0;
  await sendBatch(job as never, env as never, fakeMailer(() => (n++ === 0 ? { ok: false, temporary: false, detail: "550" } : { ok: true })).open);
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE subscribers SET status = 'unsubscribed' WHERE id = 'sub-00011'").run();
  await handleDeadLetters(fakeBatch([{ type: "newsletter.send-batch", campaignId: id }], "ssg-newsletter-send-dlq") as never, env as never);

  const res = await api(env, "GET", "/api/engine/status");
  assert.equal(res.status, 200);
  const body = await res.json();
  assert.equal(body.campaigns.length, 1);
  assert.deepEqual({ ...body.campaigns[0], startedAt: null }, {
    id, subject: "Spring", status: "sending", total: 25, sent: 9, failed: 1, skipped: 0, claimed: 0, uncertain: 0, remaining: 15,
    error: null, startedAt: null, finishedAt: null,
  });
  assert.equal(body.deadLetters.count, 1);
  assert.ok(body.deadLetters.lastAt);
  assert.deepEqual({ ...body.rate, minute: "" }, { perMinute: 60, minute: "", used: 10 });
  assert.deepEqual([body.batchSize, body.staleMinutes, body.maxAttempts, body.queue], [10, 30, 3, true]);

  const p = await api(env, "GET", `/api/campaigns/${id}/progress`);
  assert.equal(p.status, 200);
  assert.equal((await p.json()).sent, 9);
  const missing = await api(env, "GET", "/api/campaigns/nope/progress");
  assert.equal(missing.status, 404);
  assert.deepEqual(await missing.json(), { error: "not_found" });

  env.NEWSLETTER_QUEUE.sent.push(...rest.map((body) => ({ body })));
  await pump(env, { open: fakeMailer().open });
  const done = await (await api(env, "GET", `/api/campaigns/${id}/progress`)).json();
  assert.deepEqual([done.status, done.sent, done.failed, done.skipped, done.remaining], ["sent", 23, 1, 0, 1], "remaining counts who left the list before their turn");
  const idle = await (await api(env, "GET", "/api/engine/status")).json();
  assert.deepEqual(idle.campaigns, []);
  assert.equal(idle.uncertainTotal, 0);
  // uncertainTotal counts across campaigns, finished ones included.
  const ins = env.NEWSLETTER_DB.sqlite.prepare("INSERT INTO campaign_sends (campaign_id, subscriber_id, status, attempts, claimed_at) VALUES (?, ?, 'uncertain', 1, '2026-01-01T00:00:00.000Z')");
  ins.run(id, "sub-00030");
  ins.run("other-campaign", "sub-00031");
  assert.equal((await (await api(env, "GET", "/api/engine/status")).json()).uncertainTotal, 2);
});

test("status on an empty database, without a queue", async () => {
  const env = await engineEnv(0, { NEWSLETTER_QUEUE: undefined });
  const body = await (await api(env, "GET", "/api/engine/status")).json();
  assert.deepEqual(body.deadLetters, { count: 0, lastAt: null });
  assert.equal(body.uncertainTotal, 0);
  assert.equal(body.queue, false);
});

test("the worker routes the dead-letter queue to the recorder, everything else to the jobs", async () => {
  const env = await engineEnv(0);
  const dead = fakeBatch([{ type: "newsletter.send-batch", campaignId: "c1" }, null], "ssg-newsletter-send-dlq");
  await worker.queue(dead as never, env as never);
  assert.ok(dead.messages.every((m) => m.acked));
  const rows = env.NEWSLETTER_DB.sqlite.prepare("SELECT campaign_id, job, attempts FROM send_dead_letters ORDER BY campaign_id").all();
  assert.deepEqual(rows.map((r) => ({ ...r })), [
    { campaign_id: null, job: "null", attempts: 1 },
    { campaign_id: "c1", job: '{"type":"newsletter.send-batch","campaignId":"c1"}', attempts: 1 },
  ]);
  const live = fakeBatch([{ type: "unknown" }]);
  await worker.queue(live as never, env as never);
  assert.equal(live.messages[0].acked, true);
  assert.equal(isDeadLetterQueue("my-dlq", { NEWSLETTER_DEAD_LETTER_QUEUE: "my-dlq" } as never), true);
  assert.equal(isDeadLetterQueue("ssg-newsletter-send-dlq", { NEWSLETTER_DEAD_LETTER_QUEUE: "my-dlq" } as never), false);
});

test("a dead letter that cannot be written is retried", async () => {
  const env = await engineEnv(0);
  env.NEWSLETTER_DB.state.failOn = /INSERT INTO send_dead_letters/;
  const original = console.error;
  console.error = () => {};
  const dead = fakeBatch([{ type: "x" }], "ssg-newsletter-send-dlq");
  try {
    await handleDeadLetters(dead as never, env as never);
  } finally {
    console.error = original;
  }
  assert.equal(dead.messages[0].acked, false);
  assert.deepEqual(dead.messages[0].retried, {});
});

test("engineConfig: defaults, overrides, batch never above the rate", () => {
  assert.deepEqual(engineConfig({} as never), { rate: DEFAULT_RATE, batchSize: DEFAULT_BATCH, staleMs: 30 * 60_000, maxAttempts: 3 });
  assert.deepEqual(
    engineConfig({ NEWSLETTER_SEND_RATE: "5", NEWSLETTER_SEND_BATCH: "100", NEWSLETTER_SEND_STALE_MINUTES: "20", NEWSLETTER_SEND_MAX_ATTEMPTS: "5" } as never),
    { rate: 5, batchSize: 5, staleMs: 20 * 60_000, maxAttempts: 5 },
  );
  assert.equal(engineConfig({ NEWSLETTER_SEND_RATE: "0", NEWSLETTER_SEND_BATCH: "x" } as never).rate, DEFAULT_RATE);
});

test("rate counter: partial grants, nothing for 0, a lowered rate gives nothing back, prune", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-13T10:00:45.500Z") });
  assert.equal(minuteOf(), "2026-11-13T10:00");
  assert.equal(secondsToNextMinute(), 15);
  assert.equal(secondsToNextMinute(Date.parse("2026-11-13T10:00:00.000Z")), 60);
  const env = await engineEnv(0);
  assert.deepEqual(await takeBudget(env as never, 0, 10), { minute: "2026-11-13T10:00", granted: 0 });
  assert.equal((await takeBudget(env as never, 7, 10)).granted, 7);
  const g = await takeBudget(env as never, 7, 10);
  assert.equal(g.granted, 3);
  assert.equal((await takeBudget(env as never, 1, 10)).granted, 0);
  assert.equal((await takeBudget(env as never, 5, 4)).granted, 0, "rate lowered below what was used");
  assert.equal((await rateUsage(env as never)).used, 10);
  await giveBack(env as never, g, 0);
  await giveBack(env as never, g, 2);
  assert.equal((await rateUsage(env as never)).used, 8);
  t.mock.timers.tick(2 * 3_600_000);
  assert.equal((await rateUsage(env as never)).used, 0);
  await takeBudget(env as never, 1, 10);
  await pruneRate(env as never);
  assert.deepEqual(env.NEWSLETTER_DB.sqlite.prepare("SELECT minute FROM send_rate").all().map((r) => r.minute), ["2026-11-13T12:00"]);
});

test("progress of a campaign that never started: zeros and nulls", async () => {
  const env = await engineEnv(0);
  const id = addCampaign(env.NEWSLETTER_DB, { status: "draft", scheduled_at: null });
  const body = await (await api(env, "GET", `/api/campaigns/${id}/progress`)).json();
  assert.deepEqual([body.total, body.sent, body.claimed, body.remaining, body.startedAt, body.error], [0, 0, 0, 0, null, null]);
});

test("the registered job handler is the send engine (a malformed job is dropped with a warning)", async () => {
  const env = await engineEnv(0);
  const warned: unknown[] = [];
  const original = console.warn;
  console.warn = (...x: unknown[]) => void warned.push(x[0]);
  const batch = fakeBatch([{ type: "newsletter.send-batch" }]);
  try {
    await worker.queue(batch as never, env as never);
  } finally {
    console.warn = original;
  }
  assert.equal(batch.messages[0].acked, true);
  assert.match(String(warned[0]), /malformed send job/);
});
