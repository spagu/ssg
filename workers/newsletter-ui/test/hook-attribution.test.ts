// #460: bounces and complaints attributed to a campaign exactly — by the
// campaign tag a Resend event carries, or by the engine for its own refusals —
// and never by address or time. Counts only.
import { test } from "node:test";
import assert from "node:assert/strict";
import { CAMPAIGN_TAG, knownCampaigns, resendCampaign } from "../src/bounce-attribution.ts";
import { EXACT_DETAIL, NONE_DETAIL, campaignBounces } from "../src/stats-bounces.ts";
import { findSuppression } from "../src/suppress.ts";
import { permanent } from "../src/mail/_mime.ts";
import { HARD, campaignWithSends, counts, resend, tagged } from "./attribution-fixtures.ts";
import { campaignRow, fakeMailer, runJob, started } from "./engine-fixtures.ts";
import { hookEnv, mcEvent, rfc9421Headers, hook } from "./hook-fixtures.ts";
import { api } from "./subscriber-seed.ts";

const ID = "0f8fad5b-d9cb-469f-a165-70867728950e";

test("resendCampaign reads data.tags[newsletter_campaign] in the shape of a campaign id only", () => {
  const p = (tags: unknown) => ({ type: "email.bounced", data: { tags } });
  assert.equal(resendCampaign(p({ [CAMPAIGN_TAG]: ID })), ID);
  assert.equal(resendCampaign(p({ [CAMPAIGN_TAG]: ID.toUpperCase() })), null, "randomUUID writes lower case");
  assert.equal(resendCampaign(p({ [CAMPAIGN_TAG]: `${ID}' OR 1=1` })), null);
  assert.equal(resendCampaign(p({ [CAMPAIGN_TAG]: 42 })), null);
  assert.equal(resendCampaign(p({ category: "x" })), null);
  assert.equal(resendCampaign(p([{ name: CAMPAIGN_TAG, value: ID }])), null, "the send API's list shape is not what the webhook returns");
  assert.equal(resendCampaign({ data: {} }), null);
  assert.equal(resendCampaign(null), null);
  assert.equal(resendCampaign("x"), null);
});

test("knownCampaigns keeps ids of existing campaigns, one statement", async () => {
  const env = await hookEnv();
  const id = campaignWithSends(env.NEWSLETTER_DB, "sent", 0);
  assert.deepEqual(await knownCampaigns(env as never, [null, null]), new Set());
  assert.deepEqual(await knownCampaigns(env as never, [id, ID, id, null]), new Set([id]));
});

test("a tagged Resend bounce, complaint and soft bounce count for their campaign; delivered and replays do not", async () => {
  const env = await hookEnv();
  const d1 = env.NEWSLETTER_DB;
  const id = campaignWithSends(d1, "sent", 10);
  assert.equal((await resend(env, tagged("email.bounced", "a@example.com", id, HARD), "msg_1")).body.applied, 1);
  assert.equal((await resend(env, tagged("email.bounced", "a@example.com", id, HARD), "msg_1")).body.duplicate, 1, "a retry counts once");
  await resend(env, tagged("email.bounced", "b@example.com", id, { type: "Transient", message: "mailbox full" }));
  await resend(env, tagged("email.complained", "c@example.com", id));
  await resend(env, tagged("email.delivered", "d@example.com", id));
  await resend(env, tagged("email.opened", "d@example.com", id));
  assert.deepEqual(counts(d1, id), { "resend:complaint": 1, "resend:hard": 1, "resend:soft": 1 });
  assert.equal((await findSuppression(env as never, "a@example.com"))?.reason, "hard", "suppression works as before");
  assert.equal(campaignRow(d1, id).status, "sent", "a finished campaign changes counts only");
});

test("a tag naming no campaign, or not a campaign id, is not counted; the address is still suppressed", async () => {
  const env = await hookEnv();
  const d1 = env.NEWSLETTER_DB;
  await resend(env, tagged("email.bounced", "a@example.com", ID, HARD));
  await resend(env, tagged("email.bounced", "b@example.com", "not-a-campaign", HARD));
  await resend(env, tagged("email.bounced", "c@example.com", null, HARD, {}));
  assert.equal(Number((d1.sqlite.prepare("SELECT COUNT(*) AS n FROM campaign_bounces").get() as { n: number }).n), 0);
  assert.equal(Number((d1.sqlite.prepare("SELECT COUNT(*) AS n FROM suppressions").get() as { n: number }).n), 3);
});

test("MailChannels events name no campaign: suppressed, never attributed", async () => {
  const env = await hookEnv();
  const d1 = env.NEWSLETTER_DB;
  campaignWithSends(d1, "sending", 10);
  const body = JSON.stringify([mcEvent("hard-bounced", ["a@example.com"], { status: "550", reason: "5.1.1 user unknown" }), mcEvent("processed", ["b@example.com"])]);
  const res = await hook(env, "mailchannels", body, await rfc9421Headers(body));
  assert.deepEqual(await res.json(), { ok: true, applied: 1, duplicate: 0, ignored: 1 });
  assert.equal(Number((d1.sqlite.prepare("SELECT COUNT(*) AS n FROM campaign_bounces").get() as { n: number }).n), 0);
});

test("the engine's own refusals while sending count for the campaign (source smtp)", async () => {
  const { env, id, jobs } = await started(10);
  const refusals = [permanent("smtp 550 at RCPT TO: 5.1.1 user unknown"), permanent("smtp 552 at RCPT TO: 5.2.2 full")];
  const mail = fakeMailer((_m, i) => refusals[i] ?? { ok: true });
  await runJob(env, jobs[0], mail.open);
  assert.deepEqual(counts(env.NEWSLETTER_DB, id), { "smtp:hard": 1, "smtp:soft": 1 });
});

test("campaign stats: exact counts with attribution, nulls with the reason without", async () => {
  const env = await hookEnv({ NEWSLETTER_RESEND_API_KEY: "re_test" });
  const d1 = env.NEWSLETTER_DB;
  const id = campaignWithSends(d1, "sent", 4);
  const stats = async (cid: string) => (await (await api(env, "GET", `/api/campaigns/${cid}/stats`)).json()).bounces;
  const exact = { attribution: "exact", detail: EXACT_DETAIL };
  assert.deepEqual(await stats(id), { hard: 0, soft: 0, complaints: 0, ...exact }, "Resend tags every campaign message");
  await resend(env, tagged("email.bounced", "a@example.com", id, HARD));
  await resend(env, tagged("email.bounced", "b@example.com", id, HARD));
  await resend(env, tagged("email.complained", "c@example.com", id));
  assert.deepEqual(await stats(id), { hard: 2, soft: 0, complaints: 1, ...exact });
  // MailChannels names no campaign: null with the reason, unless something was attributed.
  const other = campaignWithSends(d1, "sent", 1);
  const mc = { ...env, NEWSLETTER_RESEND_API_KEY: "", NEWSLETTER_MAILCHANNELS_API_KEY: "mc" };
  assert.deepEqual(await campaignBounces(mc as never, other), { hard: null, soft: null, complaints: null, attribution: "none", detail: NONE_DETAIL });
  assert.equal((await campaignBounces(mc as never, id)).hard, 2, "counts attributed earlier still show");
  const smtp = { ...env, NEWSLETTER_RESEND_API_KEY: "", NEWSLETTER_SMTP_HOST: "mail.example.com" };
  assert.deepEqual(await campaignBounces(smtp as never, other), { hard: 0, soft: 0, complaints: 0, ...exact });
  const unset = { ...env, NEWSLETTER_RESEND_API_KEY: "" };
  assert.equal((await campaignBounces(unset as never, other)).attribution, "none", "no transport configured");
});

test("deleting a cancelled campaign deletes its counts and breaker state", async () => {
  const env = await hookEnv();
  const d1 = env.NEWSLETTER_DB;
  const id = campaignWithSends(d1, "sending", 100);
  await resend(env, tagged("email.bounced", "a@example.com", id, HARD));
  d1.sqlite.prepare("UPDATE campaigns SET status = 'cancelled' WHERE id = ?").run(id);
  assert.equal((await api(env, "DELETE", `/api/campaigns/${id}`)).status, 204);
  const left = (t: string) => Number((d1.sqlite.prepare(`SELECT COUNT(*) AS n FROM ${t} WHERE campaign_id = ?`).get(id) as { n: number }).n);
  assert.deepEqual([left("campaign_bounces"), left("hook_breaker")], [0, 0]);
});
