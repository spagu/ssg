// The queue consumer of the send engine: one job = one keyset range of one
// campaign. Order of business:
//   1. the range's row: gone or done → nothing to do (a duplicate delivery);
//   2. the campaign's status: paused → park the range, cancelled/sent → close
//      it; no message goes out either way;
//   3. the recipients: segment + suppressions (or, for a range reopened for
//      re-sends, the ledger's `retry` rows), then the ledger sorts them into
//      to-do, busy (another consumer is mid-send) and abandoned (a stale claim
//      becomes `uncertain`, never re-sent by the engine);
//   4. the rate budget for this minute; whatever it does not cover goes back
//      on the queue with a delay to the next minute;
//   5. deliver (engine-deliver.ts): claim + last check, render, send, record;
//   6. what next: temporary failures → throw, so the queue retries the job
//      (finished rows are skipped by the ledger); the rest of a range → re-
//      queued; rows busy elsewhere → leave the range queued for the cron's
//      stale check; otherwise the range is done.
// A `sent` campaign is still served for a range reopened for re-sends: the
// campaign stays `sent` while an admin re-sends its uncertain messages.

import { loadCampaign, senderOf } from "./campaign-model";
import { batchStatus, closeBatch, parseJob, pauseFor, requeue, setBatch, stopBatch } from "./engine-batches";
import { settleAbandoned, sortRows } from "./engine-claim";
import { defaultOpener, deliver } from "./engine-deliver";
import type { Opener } from "./engine-deliver";
import { engineConfig, giveBack, secondsToNextMinute, takeBudget } from "./engine-rate";
import { selectRange, selectRetries } from "./engine-recipients";
import { refreshCounters } from "./engine-finish";
import type { Job } from "./jobs";
import type { Env } from "./lib";
import { siteOf } from "./render";
import { ensureSchema } from "./schema";

export async function sendBatch(job: Job, env: Env, open: Opener = defaultOpener): Promise<void> {
  const j = parseJob(job);
  if (!j) {
    console.warn("newsletter-ui: malformed send job dropped", JSON.stringify(job));
    return;
  }
  await ensureSchema(env);
  const state = await batchStatus(env, j);
  if (state === null || state.status === "done") return;
  const c = await loadCampaign(env, j.campaignId);
  if (!c) return setBatch(env, j, "done");
  const resendOnly = state.retryOnly;
  if (c.status !== "sending" && !(c.status === "sent" && resendOnly)) return stopBatch(env, j, c.status);

  const site = siteOf(env);
  const sender = senderOf(c, env);
  if (!site) return pauseFor(env, j, "config: NEWSLETTER_SITE_URL is not an https:// origin");
  if (!sender) return pauseFor(env, j, "config: no sender (campaign fromEmail or NEWSLETTER_MAIL_FROM)");

  const cfg = engineConfig(env);
  const window = cfg.batchSize * 2; // a range holds batchSize ids when planned; sign-ups since may add a few
  const from = j.from ?? j.after;
  const range = resendOnly ? await selectRetries(env, c.id, from, j.until, window) : await selectRange(env, c.segment, from, j.until, window);
  const sorted = await sortRows(env, c.id, range.rows, cfg.staleMs, cfg.maxAttempts);
  await settleAbandoned(env, c.id, sorted.abandoned.map((r) => r.id), cfg.staleMs, cfg.maxAttempts);

  const wanted = sorted.todo.slice(0, cfg.batchSize);
  const grant = await takeBudget(env, wanted.length, cfg.rate);
  const rows = wanted.slice(0, grant.granted);
  const tally = { handed: 0 };
  if (rows.length) {
    try {
      const done = await deliver({ env, job: j, campaign: c, live: c.status, rows, cfg, site, sender, open, tally });
      if (done.stopped) return;
      if (done.temporary) throw new Error(`${done.temporary} recipient(s) failed temporarily; the queue retries the batch`);
    } finally {
      await giveBack(env, grant, grant.granted - tally.handed);
    }
  }

  if (sorted.todo.length > rows.length) {
    // Out of budget for this minute → next minute; only past the batch size → now.
    return requeue(env, j, grant.granted < wanted.length ? secondsToNextMinute() : 0);
  }
  if (sorted.busy.length) return setBatch(env, j, "queued");
  if (range.full) return requeue(env, { ...j, from: range.lastId as string }, 0);
  await closeBatch(env, j);
  // A finished campaign's counters are no longer recounted by the cron.
  if (c.status === "sent") await refreshCounters(env, c.id);
}
