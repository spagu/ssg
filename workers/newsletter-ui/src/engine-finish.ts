// Closing a campaign: counters taken from the ledger, and the move to `sent`
// once nothing is left to do.
//
// sent_count / failed_count are never incremented message by message: they
// are recounted from campaign_sends, so a retried batch, a crash or two
// consumers racing cannot make them drift from what the table says.

import { transition } from "./campaign-state";
import type { Env } from "./lib";

const COUNTERS = `UPDATE campaigns SET
    sent_count = (SELECT COUNT(*) FROM campaign_sends WHERE campaign_id = campaigns.id AND status = 'sent'),
    failed_count = (SELECT COUNT(*) FROM campaign_sends WHERE campaign_id = campaigns.id AND status = 'failed')`;

// refreshCounters recounts one campaign.
export async function refreshCounters(env: Env, campaignId: string): Promise<void> {
  await env.NEWSLETTER_DB.prepare(`${COUNTERS} WHERE id = ?`).bind(campaignId).run();
}

// refreshLiveCounters recounts every campaign in progress, and those cancelled
// in the last hour (a message in flight when the cancel landed still counts).
export async function refreshLiveCounters(env: Env): Promise<void> {
  await env.NEWSLETTER_DB.prepare(
    `${COUNTERS} WHERE status IN ('sending', 'paused') OR (status = 'cancelled' AND finished_at >= ?)`,
  )
    .bind(new Date(Date.now() - 3_600_000).toISOString())
    .run();
}

// finishIfDone moves a campaign whose ranges are all planned and done to
// `sent`, with final counters, provided no recipient is still claimed or
// waiting for a retry. The caller has checked the ranges. Returns whether
// this call finished it.
export async function finishIfDone(env: Env, campaignId: string): Promise<boolean> {
  const db = env.NEWSLETTER_DB;
  const open = await db
    .prepare("SELECT COUNT(*) AS n FROM campaign_sends WHERE campaign_id = ? AND status IN ('claimed', 'retry')")
    .bind(campaignId)
    .first<{ n: number }>();
  if (Number(open?.n) > 0) return false;
  await db.prepare(`${COUNTERS} WHERE id = ? AND status = 'sending'`).bind(campaignId).run();
  return transition(env, campaignId, "sending", "sent");
}
