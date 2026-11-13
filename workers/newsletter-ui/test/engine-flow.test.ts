// The send engine against campaign state changes and the rate.
import { test } from "node:test";
import assert from "node:assert/strict";
import { transition } from "../src/campaign-state.ts";
import { planTick } from "../src/engine-plan.ts";
import { batchState, campaignRow, fakeMailer, perPerson, pump, runJob, started, subId, truth } from "./engine-fixtures.ts";

test("pause stops the batch at the next message; resume continues without duplicates", async () => {
  const { env, id, jobs } = await started(30);
  assert.equal(jobs.length, 3);
  let paused = false;
  const mail = fakeMailer(async (_m, i) => {
    if (i === 12 && !paused) {
      paused = true;
      assert.equal(await transition(env as never, id, "sending", "paused"), true);
    }
    return { ok: true };
  });
  for (const j of jobs) await runJob(env, j, mail.open);
  assert.equal(mail.sent.length, 13, "the message being sent when the pause landed is the last one");
  assert.deepEqual(batchState(env, id), ["done", "parked", "parked"]);
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 13, retry: 1 }, "the next row's claim is handed back; nobody after it was claimed");
  // The cron leaves a paused campaign alone.
  await pump(env, { open: mail.open });
  assert.equal(mail.sent.length, 13);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).sent_count, 13, "counters of a paused campaign are kept current");

  assert.equal(await transition(env as never, id, "paused", "sending"), true);
  await pump(env, { open: mail.open });
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
  assert.equal(perPerson(mail.sent).size, 30);
  assert.ok([...perPerson(mail.sent).values()].every((n) => n === 1));
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 30 });
});

test("a job that finds its campaign paused parks the range without sending", async () => {
  const { env, id, jobs } = await started(5);
  await transition(env as never, id, "sending", "paused");
  const mail = fakeMailer();
  await runJob(env, jobs[0], mail.open);
  assert.equal(mail.stats.opens, 0);
  assert.deepEqual(batchState(env, id), ["parked"]);
});

test("cancel: no further message, the range closes, counters are final", async () => {
  const { env, id, jobs } = await started(20);
  const mail = fakeMailer(async (_m, i) => {
    if (i === 2) await transition(env as never, id, "sending", "cancelled");
    return { ok: true };
  });
  await runJob(env, jobs[0], mail.open);
  await runJob(env, jobs[1], mail.open);
  assert.equal(mail.sent.length, 3);
  assert.deepEqual(batchState(env, id), ["done", "done"]);
  const c = campaignRow(env.NEWSLETTER_DB, id);
  assert.equal(c.status, "cancelled");
  assert.equal(c.sent_count, 3);
  await pump(env, { open: mail.open });
  assert.equal(mail.sent.length, 3);
});

test("a deleted campaign or a malformed job: nothing sent, nothing thrown", async () => {
  const { env, id, jobs } = await started(2);
  env.NEWSLETTER_DB.sqlite.prepare("DELETE FROM campaigns WHERE id = ?").run(id);
  const mail = fakeMailer();
  await runJob(env, jobs[0], mail.open);
  assert.deepEqual(batchState(env, id), ["done"]);
  for (const bad of [{ type: "x" }, { ...jobs[0], campaignId: "" }, { ...jobs[0], from: 5 }, { ...jobs[0], until: null }]) await runJob(env, bad, mail.open);
  await runJob(env, { ...jobs[0], campaignId: "nope" }, mail.open);
  assert.equal(mail.stats.opens, 0);
});

test("the rate holds with two consumers at once; the rest waits for the next minute", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-13T10:00:30.000Z") });
  const { env, id, jobs } = await started(20, { NEWSLETTER_SEND_RATE: "15" });
  assert.equal(jobs.length, 2, "batch size is capped by the rate: ranges of 10");
  const mail = fakeMailer();
  await Promise.all(jobs.map((j) => runJob(env, j, mail.open)));
  assert.equal(mail.sent.length, 15);
  const used = env.NEWSLETTER_DB.sqlite.prepare("SELECT used FROM send_rate").get() as { used: number };
  assert.equal(used.used, 15);
  const deferred = [...env.NEWSLETTER_QUEUE.sent];
  assert.equal(deferred.length, 1);
  assert.deepEqual(deferred[0].options, { delaySeconds: 30 });
  // Same minute: nothing more goes out.
  await runJob(env, deferred[0].body, mail.open);
  assert.equal(mail.sent.length, 15);
  t.mock.timers.tick(30_000);
  env.NEWSLETTER_QUEUE.sent.length = 0;
  await runJob(env, deferred[0].body, mail.open);
  assert.equal(mail.sent.length, 20);
  assert.ok([...perPerson(mail.sent).values()].every((n) => n === 1));
  await planTick(env as never);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
});

test("unused budget is given back (skipped recipients do not use the rate)", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-13T10:00:00.000Z") });
  const { env, jobs } = await started(4);
  env.NEWSLETTER_DB.sqlite.prepare("UPDATE subscribers SET status = 'unsubscribed' WHERE id = ?").run(subId(2));
  const mail = fakeMailer(async (_m, i) => {
    if (i === 0) env.NEWSLETTER_DB.sqlite.prepare("UPDATE subscribers SET status = 'unsubscribed' WHERE id = ?").run(subId(3));
    return { ok: true };
  });
  await runJob(env, jobs[0], mail.open);
  assert.equal(mail.sent.length, 2);
  assert.equal((env.NEWSLETTER_DB.sqlite.prepare("SELECT used FROM send_rate").get() as { used: number }).used, 2);
});

test("a range longer than one read (sign-ups since planning) continues where it stopped", async () => {
  const { env, id, jobs } = await started(10, { NEWSLETTER_SEND_BATCH: "2" });
  assert.equal(jobs.length, 5);
  // 5 sign-ups land inside the first range (sub-00001, sub-00002].
  const db = env.NEWSLETTER_DB.sqlite;
  for (const s of ["a", "b", "c", "d", "e"]) {
    db.prepare(
      `INSERT INTO subscribers (id, email, status, language, tags, consent_text, consent_at, token, created_at, updated_at)
       VALUES (?, ?, 'confirmed', 'en', '', 'yes', '2026-01-01', ?, '2026-01-01', '2026-01-01')`,
    ).run(`${subId(1)}${s}`, `x${s}@example.com`, `tok-x${s}`.padEnd(43, "x"));
  }
  env.NEWSLETTER_QUEUE.sent.push(...jobs.map((body) => ({ body })));
  const mail = fakeMailer();
  await pump(env, { open: mail.open });
  assert.equal(perPerson(mail.sent).size, 15);
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
});
