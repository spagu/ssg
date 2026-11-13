// The cron side of the send engine, once a minute:
//   1. start due campaigns: `scheduled` with scheduled_at <= now, won with the
//      atomic transition scheduled → sending (two overlapping ticks: one wins);
//   2. for every `sending` campaign:
//      - create its plan on first sight (cursor '', total_count counted);
//      - re-queue ranges that are parked (the campaign was resumed) or queued
//        but untouched for the stale timeout (the job was lost);
//      - plan the next ranges, keeping about one minute of work queued per
//        campaign: ceil(rate / batch size) ranges, at most 100;
//      - finish it when every range is planned and done;
//   3. re-queue lost re-send ranges of `sent` campaigns (an admin re-sending
//      uncertain messages, engine-uncertain.ts) the same way;
//   4. recount live campaigns, drop old rate rows.
// Re-queuing never risks a double send: the ledger (engine-claim.ts) decides
// who still needs a message, not the number of jobs in flight.

import { fromRow } from "./campaign-model";
import type { Campaign, CampaignRow } from "./campaign-model";
import { transition } from "./campaign-state";
import { jobFor } from "./engine-batches";
import type { SendJob } from "./engine-batches";
import { finishIfDone, refreshLiveCounters } from "./engine-finish";
import { engineConfig, pruneRate } from "./engine-rate";
import type { EngineConfig } from "./engine-rate";
import { countRecipients } from "./engine-recipients";
import type { Env } from "./lib";
import { segmentWhere } from "./segment";

const MAX_AHEAD = 100; // ranges per campaign per tick: one sendBatch call
const MAX_STARTS = 20; // campaigns started per tick

interface Plan {
  cursor: string;
  planned_all: number;
}

interface OpenBatch {
  campaign_id: string;
  after_id: string;
  until_id: string;
  status: string;
  updated_at: string;
}

export async function planTick(env: Env): Promise<void> {
  if (!env.NEWSLETTER_QUEUE) {
    console.warn("newsletter-ui: NEWSLETTER_QUEUE is not bound; campaigns cannot be sent");
    return;
  }
  const cfg = engineConfig(env);
  const db = env.NEWSLETTER_DB;
  const now = new Date().toISOString();
  const { results: due } = await db
    .prepare("SELECT id FROM campaigns WHERE status = 'scheduled' AND scheduled_at <= ? ORDER BY scheduled_at LIMIT ?")
    .bind(now, MAX_STARTS)
    .all<{ id: string }>();
  for (const { id } of due) await transition(env, id, "scheduled", "sending");

  const { results: live } = await db.prepare("SELECT * FROM campaigns WHERE status = 'sending'").all<CampaignRow>();
  for (const row of live) {
    try {
      await advance(env, fromRow(row), cfg);
    } catch (e) {
      console.error("newsletter-ui: planning campaign", row.id, e);
    }
  }
  const { results: resends } = await db
    .prepare(
      `SELECT b.campaign_id, b.after_id, b.until_id, b.status, b.updated_at FROM send_batches b
         JOIN campaigns c ON c.id = b.campaign_id WHERE c.status = 'sent' AND b.status != 'done'`,
    )
    .all<OpenBatch>();
  await queueJobs(env, await requeueStale(env, resends, cfg));
  await refreshLiveCounters(env);
  await pruneRate(env);
}

// requeueStale moves parked ranges, and queued ones untouched for the stale
// timeout (their job was lost), back to queued and returns their jobs.
// Guarded on the row as read: of two ticks, one re-queues it.
async function requeueStale(env: Env, open: OpenBatch[], cfg: EngineConfig): Promise<SendJob[]> {
  const jobs: SendJob[] = [];
  const now = new Date().toISOString();
  const staleBefore = Date.now() - cfg.staleMs;
  for (const b of open) {
    if (b.status !== "parked" && Date.parse(b.updated_at) >= staleBefore) continue;
    const r = await env.NEWSLETTER_DB
      .prepare("UPDATE send_batches SET status = 'queued', updated_at = ? WHERE campaign_id = ? AND after_id = ? AND status = ? AND updated_at = ?")
      .bind(now, b.campaign_id, b.after_id, b.status, b.updated_at)
      .run();
    if (r.meta.changes === 1) jobs.push(jobFor(b.campaign_id, b.after_id, b.until_id));
  }
  return jobs;
}

async function queueJobs(env: Env, jobs: SendJob[]): Promise<void> {
  for (let i = 0; i < jobs.length; i += MAX_AHEAD) {
    await env.NEWSLETTER_QUEUE?.sendBatch(jobs.slice(i, i + MAX_AHEAD).map((body) => ({ body })));
  }
}

export const aheadOf = (cfg: EngineConfig): number => Math.min(MAX_AHEAD, Math.max(1, Math.ceil(cfg.rate / cfg.batchSize)));

async function advance(env: Env, c: Campaign, cfg: EngineConfig): Promise<void> {
  const db = env.NEWSLETTER_DB;
  const plan = await planOf(env, c);
  const { results: open } = await db
    .prepare("SELECT campaign_id, after_id, until_id, status, updated_at FROM send_batches WHERE campaign_id = ? AND status != 'done'")
    .bind(c.id)
    .all<OpenBatch>();
  const jobs = await requeueStale(env, open, cfg);
  let outstanding = open.length;
  let plannedAll = plan.planned_all === 1;
  const need = aheadOf(cfg) - outstanding;
  if (!plannedAll && need > 0) {
    const planned = await planMore(env, c, plan, need * cfg.batchSize, cfg.batchSize);
    jobs.push(...planned.jobs);
    outstanding += planned.jobs.length;
    plannedAll = planned.all;
  }
  await queueJobs(env, jobs);
  if (plannedAll && outstanding === 0) await finishIfDone(env, c.id);
}

// planOf reads the campaign's plan, creating it (and counting total_count)
// the first time the campaign is seen in `sending`.
async function planOf(env: Env, c: Campaign): Promise<Plan> {
  const db = env.NEWSLETTER_DB;
  const read = () => db.prepare("SELECT cursor, planned_all FROM send_plans WHERE campaign_id = ?").bind(c.id).first<Plan>();
  const found = await read();
  if (found) return found;
  const now = new Date().toISOString();
  const r = await db
    .prepare("INSERT OR IGNORE INTO send_plans (campaign_id, cursor, planned_all, created_at, updated_at) VALUES (?, '', 0, ?, ?)")
    .bind(c.id, now, now)
    .run();
  if (r.meta.changes === 1) {
    await db.prepare("UPDATE campaigns SET total_count = ? WHERE id = ?").bind(await countRecipients(env, c.segment), c.id).run();
  }
  return (await read()) as Plan;
}

// planMore reads the next `want` subscriber ids after the cursor and cuts them
// into ranges of `size`. Moving the cursor and inserting the ranges is one
// transaction; the cursor move is guarded on the value read, and each insert
// on the cursor having reached this plan's end — so a racing tick either
// plans the same ranges (ignored: primary key) or nothing.
async function planMore(env: Env, c: Campaign, plan: Plan, want: number, size: number): Promise<{ jobs: SendJob[]; all: boolean }> {
  const db = env.NEWSLETTER_DB;
  const { sql, binds } = segmentWhere(c.segment);
  const { results } = await db
    .prepare(`SELECT id FROM subscribers WHERE ${sql} AND id > ? ORDER BY id LIMIT ?`)
    .bind(...binds, plan.cursor, want)
    .all<{ id: string }>();
  const ids = results.map((r) => r.id);
  const all = ids.length < want;
  const ranges: SendJob[] = [];
  for (let i = 0; i < ids.length; i += size) {
    ranges.push(jobFor(c.id, i === 0 ? plan.cursor : ids[i - 1], ids[Math.min(i + size, ids.length) - 1]));
  }
  const cursor = ids.length ? ids[ids.length - 1] : plan.cursor;
  const now = new Date().toISOString();
  const res = await db.batch([
    db
      .prepare("UPDATE send_plans SET cursor = ?, planned_all = ?, updated_at = ? WHERE campaign_id = ? AND cursor = ? AND planned_all = 0")
      .bind(cursor, all ? 1 : 0, now, c.id, plan.cursor),
    ...ranges.map((r) =>
      db
        .prepare(
          `INSERT OR IGNORE INTO send_batches (campaign_id, after_id, until_id, status, created_at, updated_at)
           SELECT ?1, ?2, ?3, 'queued', ?4, ?4 WHERE EXISTS (SELECT 1 FROM send_plans WHERE campaign_id = ?1 AND cursor = ?5)`,
        )
        .bind(c.id, r.after, r.until, now, cursor),
    ),
  ]);
  if (res[0].meta.changes !== 1) return { jobs: [], all: false };
  return { jobs: ranges.filter((_, k) => res[k + 1].meta.changes === 1), all };
}
