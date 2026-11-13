// The send engine end to end: cron + queue pumped until idle.
import { test } from "node:test";
import assert from "node:assert/strict";
import { addCampaign, campaignRow, engineEnv, fakeMailer, perPerson, pump, truth } from "./engine-fixtures.ts";

test("a 5 000-subscriber campaign completes: one message each, counters equal the ledger", async () => {
  const env = await engineEnv(5000, { NEWSLETTER_SEND_RATE: "100000", NEWSLETTER_SEND_BATCH: "100" });
  const id = addCampaign(env.NEWSLETTER_DB);
  const mail = fakeMailer();
  const stats = await pump(env, { open: mail.open });
  const c = campaignRow(env.NEWSLETTER_DB, id);
  assert.equal(c.status, "sent");
  const counts = perPerson(mail.sent);
  assert.equal(counts.size, 5000);
  assert.ok([...counts.values()].every((n) => n === 1));
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { sent: 5000 });
  assert.equal(c.total_count, 5000);
  assert.equal(c.sent_count, 5000);
  assert.equal(c.failed_count, 0);
  assert.ok(c.finished_at);
  assert.equal(mail.stats.opens, 50, "one mailer session per batch");
  assert.equal(mail.stats.closes, 50);
  assert.ok(stats.rounds < 10, `rounds: ${stats.rounds}`);
});

test("at the defaults (60/min, batches of 10) the pace holds minute by minute", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: Date.parse("2026-11-13T10:00:00.000Z") });
  const env = await engineEnv(200);
  const id = addCampaign(env.NEWSLETTER_DB);
  const perMinute = new Map<string, number>();
  const mail = fakeMailer(() => {
    const minute = new Date().toISOString().slice(0, 16);
    perMinute.set(minute, (perMinute.get(minute) ?? 0) + 1);
    return { ok: true };
  });
  await pump(env, { open: mail.open, advance: () => t.mock.timers.tick(60_000) });
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
  assert.equal(perPerson(mail.sent).size, 200);
  assert.ok([...perMinute.values()].every((n) => n <= 60), JSON.stringify([...perMinute]));
  assert.ok(perMinute.size >= 4, "200 messages need at least 4 minutes at 60/min");
});
