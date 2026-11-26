// #460 end to end through the real Resend transport: every campaign message
// carries the campaign tag, a webhook that returns it is attributed, and hard
// bounces reported that way pause a campaign still sending. A test send does
// not carry the tag.
import { test } from "node:test";
import assert from "node:assert/strict";
import { CAMPAIGN_TAG } from "../src/bounce-attribution.ts";
import { hookBreakerReason } from "../src/hook-breaker.ts";
import { HARD, counts, planErrorOf, resend, statusOf, tagged } from "./attribution-fixtures.ts";
import { call, fakeFetch } from "./campaign-fixtures.ts";
import { addCampaign, runJob, started, subId } from "./engine-fixtures.ts";
import { RESEND_SECRET } from "./hook-fixtures.ts";

const LIMITS = { NEWSLETTER_RESEND_WEBHOOK_SECRET: RESEND_SECRET, NEWSLETTER_BOUNCE_BREAKER_MIN: "3", NEWSLETTER_BOUNCE_BREAKER_PERCENT: "30" };

test("a campaign sent through Resend: tagged messages, attributed bounces, the breaker pauses it", async (t) => {
  const f = fakeFetch();
  t.after(f.restore);
  const { env, id, jobs } = await started(20, LIMITS);
  await runJob(env, jobs[0], undefined); // the default opener: the real Resend transport
  assert.equal(f.calls.length, 10);
  for (const c of f.calls) {
    assert.equal(c.url, "https://api.resend.com/emails");
    assert.deepEqual(c.body.tags, [{ name: CAMPAIGN_TAG, value: id }]);
  }
  assert.equal(statusOf(env.NEWSLETTER_DB, id), "sending", "the rest of the list is still queued");

  // The provider reports three of the ten as unknown mailboxes.
  for (const i of [1, 2]) await resend(env, tagged("email.bounced", `${subId(i)}@example.com`, id, HARD));
  assert.equal(statusOf(env.NEWSLETTER_DB, id), "sending", "2 < MIN");
  await resend(env, tagged("email.bounced", `${subId(3)}@example.com`, id, HARD));
  assert.equal(statusOf(env.NEWSLETTER_DB, id), "paused", "3 of 10 sent = 30 %");
  assert.equal(planErrorOf(env.NEWSLETTER_DB, id), hookBreakerReason(3, 10));
  assert.deepEqual(counts(env.NEWSLETTER_DB, id), { "resend:hard": 3 });

  // The next range finds the campaign paused and sends nothing.
  await runJob(env, jobs[1], undefined);
  assert.equal(f.calls.length, 10);
});

test("a test send carries no campaign tag", async (t) => {
  const f = fakeFetch();
  t.after(f.restore);
  const { env } = await started(0);
  const draft = addCampaign(env.NEWSLETTER_DB, { status: "draft" });
  const res = await call(env, "POST", `/${draft}/test`, { to: "editor@example.com" });
  assert.equal(res.status, 200);
  assert.equal(f.calls.length, 1);
  assert.equal("tags" in f.calls[0].body, false);
});
