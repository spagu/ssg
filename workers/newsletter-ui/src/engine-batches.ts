// A queue job and the send_batches row behind it.
//
// A job never carries addresses: it names a keyset range of subscribers.id,
// (after, until], planned by the cron. `from` (optional) is where to resume
// reading inside the range when it held more rows than one read takes. The
// row (campaign_id, after_id) records the range's state:
//   queued  a job for it is (or should be) on the queue
//   parked  its job found the campaign paused; resuming re-queues it at once
//   done    every recipient in it reached a final state
// retry_only = 1 marks a range reopened to re-send `retry` rows only (an admin
// re-sending uncertain messages): its job reads the ledger, not the segment.
// A queued row untouched for the stale timeout is re-queued by the cron, which
// covers a job lost anywhere (dead-lettered, dropped, a crash before sending).

import { transition } from "./campaign-state";
import { refreshCounters } from "./engine-finish";
import { SEND_JOB } from "./engine-rate";
import type { Job } from "./jobs";
import type { Env } from "./lib";

export interface SendJob extends Job {
  campaignId: string;
  after: string;
  until: string;
  from?: string;
}

export const jobFor = (campaignId: string, after: string, until: string): SendJob => ({ type: SEND_JOB, campaignId, after, until });

const str = (v: unknown): v is string => typeof v === "string";

// parseJob validates a job body; the queue holds what was sent, nothing more.
export function parseJob(job: Job): SendJob | null {
  const { campaignId, after, until, from } = job;
  if (!str(campaignId) || !campaignId || !str(after) || !str(until) || (from !== undefined && !str(from))) return null;
  return job as SendJob;
}

// batchStatus reads the range's row: its state, and whether it was reopened
// for re-sends only (engine-uncertain.ts). null when there is no such range.
export async function batchStatus(env: Env, j: SendJob): Promise<{ status: string; retryOnly: boolean } | null> {
  const row = await env.NEWSLETTER_DB.prepare("SELECT status, retry_only FROM send_batches WHERE campaign_id = ? AND after_id = ?")
    .bind(j.campaignId, j.after)
    .first<{ status: string; retry_only: number }>();
  return row ? { status: row.status, retryOnly: Number(row.retry_only) === 1 } : null;
}

// setBatch moves the row; `from` limits the move to rows in that state.
export async function setBatch(env: Env, j: SendJob, status: string, from?: string): Promise<void> {
  await env.NEWSLETTER_DB.prepare(
    `UPDATE send_batches SET status = ?1, updated_at = ?2 WHERE campaign_id = ?3 AND after_id = ?4${from ? " AND status = ?5" : ""}`,
  )
    .bind(status, new Date().toISOString(), j.campaignId, j.after, ...(from ? [from] : []))
    .run();
}

// closeBatch marks a range whose job finished as done — unless a row in it
// is (again) waiting in `retry`, e.g. an admin re-sent an uncertain message
// while this job was running, or someone left the segment while a retry was
// pending. Then the range stays `queued` and becomes re-send only, so the
// cron's stale check runs it once more for exactly those rows. Same single
// statement as a plain "done".
export async function closeBatch(env: Env, j: SendJob): Promise<void> {
  await env.NEWSLETTER_DB.prepare(
    `UPDATE send_batches SET
       status = CASE WHEN EXISTS (${PENDING_RETRY}) THEN 'queued' ELSE 'done' END,
       retry_only = CASE WHEN EXISTS (${PENDING_RETRY}) THEN 1 ELSE retry_only END,
       updated_at = ?3
     WHERE campaign_id = ?1 AND after_id = ?2`,
  )
    .bind(j.campaignId, j.after, new Date().toISOString())
    .run();
}

const PENDING_RETRY = `SELECT 1 FROM campaign_sends cs WHERE cs.campaign_id = ?1 AND cs.status = 'retry'
    AND cs.subscriber_id > send_batches.after_id AND cs.subscriber_id <= send_batches.until_id`;

// requeue puts the job back (the rest of a range) and marks the row fresh, so
// the cron does not count it as lost while it waits.
export async function requeue(env: Env, j: SendJob, delaySeconds: number): Promise<void> {
  await env.NEWSLETTER_QUEUE?.send(j, delaySeconds > 0 ? { delaySeconds } : undefined);
  await setBatch(env, j, "queued");
}

// stopBatch handles a campaign that is no longer `sending`: a paused one
// parks the range (resume re-queues it), a final one closes it.
export async function stopBatch(env: Env, j: SendJob, campaignStatus: string): Promise<void> {
  if (campaignStatus === "paused") return setBatch(env, j, "parked", "queued");
  await setBatch(env, j, "done");
  if (campaignStatus === "cancelled") await refreshCounters(env, j.campaignId);
}

// pauseFor stops a campaign loudly instead of burning through the list: a
// render error or a missing setting would fail every recipient the same way.
// The reason is kept in send_plans.error for the panel.
export async function pauseFor(env: Env, j: SendJob, reason: string): Promise<void> {
  console.error("newsletter-ui: campaign paused by the send engine", j.campaignId, reason);
  await transition(env, j.campaignId, "sending", "paused");
  await env.NEWSLETTER_DB.prepare("UPDATE send_plans SET error = ?, updated_at = ? WHERE campaign_id = ?")
    .bind(reason, new Date().toISOString(), j.campaignId)
    .run();
  await setBatch(env, j, "parked", "queued");
}
