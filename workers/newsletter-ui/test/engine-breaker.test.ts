// The hard-bounce circuit breaker: a server that refuses everyone as "user
// unknown" pauses the campaign and suppresses nobody; ordinary bounces below
// the threshold are suppressed as before.
import { test } from "node:test";
import assert from "node:assert/strict";
import { transition } from "../src/campaign-state.ts";
import { breakerFor, notePermanent } from "../src/engine-breaker.ts";
import { engineHooks } from "../src/engine-hooks.ts";
import { permanent } from "../src/mail/_mime.ts";
import { campaignRow, fakeMailer, perPerson, planError, pump, runJob, sendRows, started, subId, truth } from "./engine-fixtures.ts";
import type { EngineEnv } from "./engine-fixtures.ts";

const UNKNOWN = "smtp 550 at RCPT TO: 5.1.1 <x> user unknown";
const FULL = "smtp 552 at RCPT TO: 5.2.2 mailbox full";
const addr = (i: number) => `${subId(i)}@example.com`;
const suppressed = (env: EngineEnv) => Number((env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM suppressions").get() as { n: number }).n);

test("a server answering 550 to everyone: the campaign pauses, nobody is suppressed, resume tries them again", async (t) => {
  const hooks: unknown[] = [];
  const original = engineHooks.onPermanentFailure;
  t.mock.method(engineHooks, "onPermanentFailure", async (e: never, f: never) => {
    hooks.push(f);
    return original(e, f);
  });
  const { env, id, jobs } = await started(10);
  let broken = true;
  const mail = fakeMailer(() => (broken ? permanent(UNKNOWN) : { ok: true }));
  await runJob(env, jobs[0], mail.open);
  assert.equal(mail.stats.calls, 5, "stopped at the fifth refusal (5 of 5 messages)");
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "paused");
  assert.match(String(planError(env, id)), /^bounces: 5 of 5 messages in one batch were refused as unknown mailboxes; nothing was suppressed/);
  assert.equal(suppressed(env), 0);
  assert.deepEqual(hooks, [], "no permanent-failure hook ran for the tripped batch");
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { retry: 5 }, "handed back without counting the attempt; nobody else claimed");
  assert.ok(sendRows(env.NEWSLETTER_DB, id).every((r) => r.attempts === 0 && r.error === null));

  broken = false; // the admin fixes the server and resumes
  assert.equal(await transition(env as never, id, "paused", "sending"), true);
  await pump(env, { open: mail.open });
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "sent");
  assert.equal(perPerson(mail.sent).size, 10);
  assert.ok([...perPerson(mail.sent).values()].every((n) => n === 1));
  assert.equal(suppressed(env), 0);
});

test("below the threshold the breaker stays closed and real bounces are suppressed", async () => {
  // 4 hard of 10: under the minimum count.
  const few = await started(10);
  const m1 = fakeMailer((m) => ([1, 2, 3, 4].map(addr).includes(m.to) ? permanent(UNKNOWN) : { ok: true }));
  await runJob(few.env, few.jobs[0], m1.open);
  assert.equal(campaignRow(few.env.NEWSLETTER_DB, few.id).status, "sending");
  assert.deepEqual(truth(few.env.NEWSLETTER_DB, few.id), { failed: 4, sent: 6 });
  assert.equal(suppressed(few.env), 4);

  // 5 hard, but only after 6 accepted: 5 of 11 is under half.
  const share = await started(12, { NEWSLETTER_SEND_BATCH: "12" });
  const m2 = fakeMailer((m) => ([7, 8, 9, 10, 11].map(addr).includes(m.to) ? permanent(UNKNOWN) : { ok: true }));
  await runJob(share.env, share.jobs[0], m2.open);
  assert.equal(campaignRow(share.env.NEWSLETTER_DB, share.id).status, "sending");
  assert.equal(suppressed(share.env), 5);

  // Mailbox-full refusals are not hard bounces: they never trip it.
  const full = await started(6);
  await runJob(full.env, full.jobs[0], fakeMailer(() => permanent(FULL)).open);
  assert.equal(campaignRow(full.env.NEWSLETTER_DB, full.id).status, "sending");
  assert.deepEqual(truth(full.env.NEWSLETTER_DB, full.id), { failed: 6 });
});

test("the thresholds are settings; a mixed batch trips on its running share", async () => {
  const { env, id, jobs } = await started(10, { NEWSLETTER_BOUNCE_BREAKER_MIN: "2", NEWSLETTER_BOUNCE_BREAKER_PERCENT: "40" });
  const mail = fakeMailer((m) => ([1, 3].map(addr).includes(m.to) ? permanent(UNKNOWN) : m.to === addr(2) ? permanent(FULL) : { ok: true }));
  await runJob(env, jobs[0], mail.open);
  assert.equal(mail.stats.calls, 3, "2 hard of 3 messages ≥ 40 %");
  assert.equal(campaignRow(env.NEWSLETTER_DB, id).status, "paused");
  assert.deepEqual(truth(env.NEWSLETTER_DB, id), { retry: 2, failed: 1 }, "only the hard bounces go back; the soft one stays failed");
  assert.equal(suppressed(env), 0, "not even the mailbox-full count is applied from a tripped batch");
  assert.equal(Number((env.NEWSLETTER_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM soft_bounces").get() as { n: number }).n), 0);
});

test("breakerFor: defaults, and a percentage above 100 is capped", () => {
  assert.deepEqual(breakerFor({} as never), { min: 5, percent: 50, attempted: 0, hard: [], failures: [] });
  const b = breakerFor({ NEWSLETTER_BOUNCE_BREAKER_MIN: "1", NEWSLETTER_BOUNCE_BREAKER_PERCENT: "250" } as never);
  assert.equal(b.percent, 100);
  b.attempted = 1;
  assert.equal(notePermanent(b, { email: "a@example.com", subscriberId: "a", campaignId: "c", detail: UNKNOWN }), true);
});
