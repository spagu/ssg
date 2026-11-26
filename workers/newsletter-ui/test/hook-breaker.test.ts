// #460: hard bounces reported by webhook for a campaign still sending feed the
// bounce breaker (NEWSLETTER_BOUNCE_BREAKER_MIN / _PERCENT of the messages
// sent); a finished campaign's reports change counts only.
import { test } from "node:test";
import assert from "node:assert/strict";
import { transition } from "../src/campaign-state.ts";
import { hookBreakerReason, noteWebhookHard, webhookHard } from "../src/hook-breaker.ts";
import { HARD, addSent, campaignWithSends, counts, planErrorOf, resend, statusOf, tagged } from "./attribution-fixtures.ts";
import { hookEnv } from "./hook-fixtures.ts";

const breaker = (d1: { sqlite: { prepare: (s: string) => { get: (...a: unknown[]) => unknown } } }, id: string) =>
  d1.sqlite.prepare("SELECT hard, sent_base FROM hook_breaker WHERE campaign_id = ?").get(id) as { hard: number; sent_base: number } | undefined;
const bounce = (env: unknown, id: string, i: number) => resend(env, tagged("email.bounced", `gone${i}@example.com`, id, HARD));

test("webhookHard: new hard bounces per attributed campaign, webhook sources only", () => {
  const b = (kind: string, campaignId: string | null, source = "resend") => ({ id: null, kind, emails: [], source, detail: "", campaignId }) as never;
  const got = webhookHard([b("hard", "A"), b("hard", "A"), b("hard", "B"), b("soft", "A"), b("complaint", "A"), b("hard", null), b("hard", "A", "smtp")]);
  assert.deepEqual([...got], [["A", 2], ["B", 1]]);
});

test("a sending campaign pauses at MIN hard bounces that are PERCENT % of the messages sent", async () => {
  const env = await hookEnv({ NEWSLETTER_BOUNCE_BREAKER_MIN: "3", NEWSLETTER_BOUNCE_BREAKER_PERCENT: "20" });
  const d1 = env.NEWSLETTER_DB;
  const id = campaignWithSends(d1, "sending", 10);
  await bounce(env, id, 1);
  await bounce(env, id, 2);
  assert.equal(statusOf(d1, id), "sending", "2 < MIN");
  await bounce(env, id, 3);
  assert.equal(statusOf(d1, id), "paused", "3 ≥ MIN and 3 of 10 ≥ 20 %");
  assert.equal(planErrorOf(d1, id), hookBreakerReason(3, 10));
  assert.deepEqual({ ...breaker(d1, id) }, { hard: 0, sent_base: 10 }, "a new count starts at the trip");
  assert.deepEqual(counts(d1, id), { "resend:hard": 3 });

  // Bounces arriving while paused count for the campaign, not for the breaker.
  await bounce(env, id, 4);
  await bounce(env, id, 5);
  assert.deepEqual({ ...breaker(d1, id) }, { hard: 0, sent_base: 10 });
  assert.deepEqual(counts(d1, id), { "resend:hard": 5 });

  // Resumed: judged on what comes after the trip only.
  assert.equal(await transition(env as never, id, "paused", "sending"), true);
  addSent(d1, id, 10, 40);
  for (let i = 6; i <= 9; i++) await bounce(env, id, i);
  assert.equal(statusOf(d1, id), "sending", "4 of the 40 sent since = 10 % < 20 %");
  for (let i = 10; i <= 13; i++) await bounce(env, id, i);
  assert.equal(statusOf(d1, id), "paused", "8 of 40 = 20 %");
  assert.equal(planErrorOf(d1, id), hookBreakerReason(8, 40));
});

test("below PERCENT the campaign keeps sending; defaults are MIN 5 and 50 %", async () => {
  const env = await hookEnv();
  const d1 = env.NEWSLETTER_DB;
  const id = campaignWithSends(d1, "sending", 20);
  for (let i = 1; i <= 9; i++) await bounce(env, id, i);
  assert.equal(statusOf(d1, id), "sending", "9 of 20 < 50 %");
  await bounce(env, id, 10);
  assert.equal(statusOf(d1, id), "paused", "10 of 20 = 50 %");
});

test("a finished, paused or cancelled campaign: counts only, the breaker is untouched", async () => {
  const env = await hookEnv({ NEWSLETTER_BOUNCE_BREAKER_MIN: "1", NEWSLETTER_BOUNCE_BREAKER_PERCENT: "1" });
  const d1 = env.NEWSLETTER_DB;
  for (const status of ["sent", "paused", "cancelled"]) {
    const id = campaignWithSends(d1, status, 2);
    await bounce(env, id, 1);
    assert.equal(statusOf(d1, id), status);
    assert.equal(breaker(d1, id), undefined);
    assert.deepEqual(counts(d1, id), { "resend:hard": 1 });
    assert.equal(planErrorOf(d1, id), null);
  }
});

test("two reports racing past the threshold pause the campaign once", async () => {
  const env = await hookEnv({ NEWSLETTER_BOUNCE_BREAKER_MIN: "2", NEWSLETTER_BOUNCE_BREAKER_PERCENT: "1" });
  const d1 = env.NEWSLETTER_DB;
  const id = campaignWithSends(d1, "sending", 10);
  const results = await Promise.all([noteWebhookHard(env as never, id, 2), noteWebhookHard(env as never, id, 2)]);
  assert.deepEqual(results.toSorted((a, b) => Number(a) - Number(b)), [false, true]);
  assert.equal(statusOf(d1, id), "paused");
});
