// What the panel shows about sending, read from the tables only (a queue
// cannot be asked what it holds):
//   GET {base}/api/engine/status          campaigns in progress, dead letters,
//                                          this minute's rate use, the settings
//   GET {base}/api/campaigns/:id/progress  one campaign's numbers
// and the dead-letter consumer that fills send_dead_letters.

import type { Job } from "./jobs";
import type { Env } from "./lib";
import { json } from "./lib";
import { engineConfig, rateUsage } from "./engine-rate";
import type { RouteContext } from "./router";
import { ensureSchema } from "./schema";

export const DEFAULT_DLQ = "ssg-newsletter-send-dlq";

export interface Progress {
  id: string;
  subject: string;
  status: string;
  total: number; // recipients counted when the campaign started
  sent: number;
  failed: number;
  skipped: number; // unsubscribed or suppressed between start and their turn
  claimed: number; // being sent right now, or waiting for a retry
  uncertain: number; // in flight when a consumer died: maybe delivered, never re-sent automatically
  remaining: number; // total minus sent, failed, skipped and uncertain (never below 0)
  error: string | null; // why the engine paused it, if it did
  startedAt: string | null;
  finishedAt: string | null;
}

const PROGRESS = `SELECT c.id, c.subject, c.status, c.total_count, c.started_at, c.finished_at, p.error,
    SUM(s.status = 'sent') AS sent, SUM(s.status = 'failed') AS failed, SUM(s.status = 'skipped') AS skipped,
    SUM(s.status IN ('claimed', 'retry')) AS claimed, SUM(s.status = 'uncertain') AS uncertain
  FROM campaigns c
  LEFT JOIN send_plans p ON p.campaign_id = c.id
  LEFT JOIN campaign_sends s ON s.campaign_id = c.id`;

function toProgress(r: Record<string, unknown>): Progress {
  const n = (k: string) => Number(r[k] ?? 0);
  return {
    id: r.id as string,
    subject: r.subject as string,
    status: r.status as string,
    total: n("total_count"),
    sent: n("sent"),
    failed: n("failed"),
    skipped: n("skipped"),
    claimed: n("claimed"),
    uncertain: n("uncertain"),
    remaining: Math.max(0, n("total_count") - n("sent") - n("failed") - n("skipped") - n("uncertain")),
    error: (r.error as string | null) ?? null,
    startedAt: (r.started_at as string | null) ?? null,
    finishedAt: (r.finished_at as string | null) ?? null,
  };
}

// GET /api/campaigns/:id/progress → Progress, or 404 not_found.
export async function campaignProgress({ env, params }: RouteContext): Promise<Response> {
  const row = await env.NEWSLETTER_DB.prepare(`${PROGRESS} WHERE c.id = ? GROUP BY c.id`).bind(params.id).first<Record<string, unknown>>();
  return row ? json(toProgress(row)) : json({ error: "not_found" }, 404);
}

// GET /api/engine/status → {campaigns, deadLetters: {count, lastAt}, rate:
// {perMinute, minute, used}, batchSize, staleMinutes, maxAttempts, queue}.
export async function engineStatus({ env }: RouteContext): Promise<Response> {
  const db = env.NEWSLETTER_DB;
  const { results } = await db
    .prepare(`${PROGRESS} WHERE c.status IN ('sending', 'paused') GROUP BY c.id ORDER BY c.started_at`)
    .all<Record<string, unknown>>();
  const dead = (await db.prepare("SELECT COUNT(*) AS n, MAX(received_at) AS last FROM send_dead_letters").first()) as { n: number; last: string | null };
  // Across every campaign, finished ones included: an uncertain send waits
  // for the admin whatever state its campaign is in.
  const uncertain = (await db.prepare("SELECT COUNT(*) AS n FROM campaign_sends WHERE status = 'uncertain'").first()) as { n: number };
  const cfg = engineConfig(env);
  const usage = await rateUsage(env);
  return json({
    campaigns: results.map(toProgress),
    deadLetters: { count: Number(dead.n), lastAt: dead.last },
    uncertainTotal: Number(uncertain.n),
    rate: { perMinute: cfg.rate, minute: usage.minute, used: usage.used },
    batchSize: cfg.batchSize,
    staleMinutes: cfg.staleMs / 60_000,
    maxAttempts: cfg.maxAttempts,
    queue: !!env.NEWSLETTER_QUEUE,
  });
}

export const isDeadLetterQueue = (queue: string, env: Env): boolean => queue === (env.NEWSLETTER_DEAD_LETTER_QUEUE || DEFAULT_DLQ);

// handleDeadLetters records every job that ran out of retries and acks it.
// The work is not lost with it: a range left `queued` is re-queued by the
// cron after the stale timeout. A row that cannot be written is retried.
export async function handleDeadLetters(batch: MessageBatch<Job>, env: Env): Promise<void> {
  await ensureSchema(env);
  for (const msg of batch.messages) {
    const body = msg.body as Partial<Job> | null;
    const campaignId = body && typeof body.campaignId === "string" ? body.campaignId : null;
    try {
      await env.NEWSLETTER_DB.prepare("INSERT INTO send_dead_letters (id, campaign_id, job, attempts, received_at) VALUES (?, ?, ?, ?, ?)")
        .bind(crypto.randomUUID(), campaignId, JSON.stringify(msg.body ?? null), msg.attempts, new Date().toISOString())
        .run();
      msg.ack();
    } catch (e) {
      console.error("newsletter-ui: recording a dead letter", e);
      msg.retry();
    }
  }
}
