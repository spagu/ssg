// #460: webhook health — verified webhook events counted per hour and kind
// (delivered included), shown in /api/health and /api/settings, totals only;
// a warning when a webhook-reporting transport has been silent since the last
// campaign started.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ACTIVITY_RETENTION_MS, WEBHOOK_GRACE_MS, webhookActivity } from "../src/hook-activity.ts";
import { HARD, campaignWithSends, resend, tagged } from "./attribution-fixtures.ts";
import { hook, hookEnv, mcEvent, resendEvent, rfc9421Headers, svixHeaders } from "./hook-fixtures.ts";
import { api } from "./subscriber-seed.ts";

const rows = (env: Awaited<ReturnType<typeof hookEnv>>) =>
  env.NEWSLETTER_DB.sqlite.prepare("SELECT provider, kind, count FROM hook_activity ORDER BY provider, kind").all().map((r) => ({ ...r }));
const hourOf = (ms: number) => new Date(ms).toISOString().slice(0, 13);
const none = { delivered: 0, hard: 0, soft: 0, complaint: 0, other: 0 };

test("every verified event is counted by kind, delivered and ignored ones included; addresses never", async () => {
  const env = await hookEnv();
  await resend(env, resendEvent("email.delivered", "a@example.com"));
  await resend(env, resendEvent("email.delivered", "b@example.com"));
  await resend(env, resendEvent("email.opened", "b@example.com"));
  await resend(env, tagged("email.bounced", "c@example.com", null, HARD, {}));
  const body = JSON.stringify([mcEvent("delivered", ["d@example.com"]), mcEvent("complained", ["e@example.com"]), mcEvent("soft-bounced", ["f@example.com"])]);
  assert.equal((await hook(env, "mailchannels", body, await rfc9421Headers(body))).status, 200);
  assert.deepEqual(rows(env), [
    { provider: "mailchannels", kind: "complaint", count: 1 },
    { provider: "mailchannels", kind: "delivered", count: 1 },
    { provider: "mailchannels", kind: "other", count: 1 },
    { provider: "resend", kind: "delivered", count: 2 },
    { provider: "resend", kind: "hard", count: 1 },
    { provider: "resend", kind: "other", count: 1 },
  ]);
  const a = await webhookActivity(env as never);
  assert.deepEqual(a.last24h, { delivered: 3, hard: 1, soft: 0, complaint: 1, other: 2 });
  assert.deepEqual(a.last7d, a.last24h);
  assert.ok(Date.now() - Date.parse(a.lastReceivedAt as string) < 60_000);
  assert.doesNotMatch(JSON.stringify(a), /@/);
});

test("an unsigned or unparsable request leaves no trace", async () => {
  const env = await hookEnv();
  const body = resendEvent("email.delivered", "a@example.com");
  const headers = await svixHeaders(body);
  assert.equal((await hook(env, "resend", body, { ...headers, "svix-signature": "v1,AAAA" })).status, 401);
  assert.equal((await hook(env, "resend", "{", await svixHeaders("{"))).status, 400);
  assert.deepEqual(rows(env), []);
});

test("24 hours and 7 days by whole UTC hours; rows past 8 days are pruned on write", async () => {
  const env = await hookEnv();
  const d1 = env.NEWSLETTER_DB;
  const now = Date.now();
  const put = d1.sqlite.prepare("INSERT INTO hook_activity (hour, provider, kind, count, last_at) VALUES (?, 'resend', 'delivered', ?, ?)");
  put.run(hourOf(now - 23 * 3_600_000), 1, new Date(now - 23 * 3_600_000).toISOString());
  put.run(hourOf(now - 30 * 3_600_000), 10, new Date(now - 30 * 3_600_000).toISOString());
  put.run(hourOf(now - 7.5 * 86_400_000), 100, new Date(now - 7.5 * 86_400_000).toISOString());
  put.run(hourOf(now - ACTIVITY_RETENTION_MS - 3_600_000), 1000, new Date(now - ACTIVITY_RETENTION_MS - 3_600_000).toISOString());
  const a = await webhookActivity(env as never, now);
  assert.deepEqual([a.last24h.delivered, a.last7d.delivered], [1, 11]);
  assert.equal(a.lastReceivedAt, new Date(now - 23 * 3_600_000).toISOString());
  await resend(env, resendEvent("email.delivered", "a@example.com"));
  assert.deepEqual(
    d1.sqlite.prepare("SELECT count FROM hook_activity ORDER BY hour").all().map((r) => Number(r.count)),
    [100, 10, 1, 1],
    "the 1000 past retention is gone",
  );
});

test("warning: a webhook transport, a campaign started past the grace time, no webhook since", async () => {
  const now = Date.now();
  const startedAt = new Date(now - WEBHOOK_GRACE_MS - 60_000).toISOString();
  const env = await hookEnv({ NEWSLETTER_RESEND_API_KEY: "re_test" });
  const d1 = env.NEWSLETTER_DB;
  let a = await webhookActivity(env as never, now);
  assert.deepEqual([a.reportsByWebhook, a.lastCampaignStartedAt, a.warning], [true, null, null], "no campaign yet");
  campaignWithSends(d1, "sending", 1, { started_at: startedAt });
  a = await webhookActivity(env as never, now);
  assert.equal(a.warning, "no_webhook_since_campaign");
  assert.equal(a.lastCampaignStartedAt, startedAt);
  assert.equal((await webhookActivity(env as never, now - 120_000)).warning, null, "still within the grace time");
  d1.sqlite.prepare("INSERT INTO hook_activity (hour, provider, kind, count, last_at) VALUES ('2026-01-01T00', 'resend', 'delivered', 1, '2026-01-01T00:00:00.000Z')").run();
  assert.equal((await webhookActivity(env as never, now)).warning, "no_webhook_since_campaign", "one before the campaign does not count");
  await resend(env, resendEvent("email.delivered", "a@example.com"));
  assert.equal((await webhookActivity(env as never)).warning, null);
  const smtp = { ...env, NEWSLETTER_RESEND_API_KEY: "", NEWSLETTER_SMTP_HOST: "mail.example.com" };
  d1.sqlite.exec("DELETE FROM hook_activity");
  assert.deepEqual([(await webhookActivity(smtp as never, now)).reportsByWebhook, (await webhookActivity(smtp as never, now)).warning], [false, null]);
});

test("health and settings show the same totals, behind admin auth", async () => {
  const env = await hookEnv({ NEWSLETTER_MAILCHANNELS_API_KEY: "mc" });
  await resend(env, resendEvent("email.delivered", "a@example.com"));
  const health = await (await api(env, "GET", "/api/health")).json();
  const settings = await (await api(env, "GET", "/api/settings")).json();
  assert.deepEqual(health.webhookActivity.last24h, { ...none, delivered: 1 });
  assert.deepEqual(settings.webhookActivity, health.webhookActivity);
  assert.equal(health.webhookActivity.reportsByWebhook, true);
});
