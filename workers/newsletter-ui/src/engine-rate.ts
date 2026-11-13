// The send engine's settings and its pace.
//
// Pace is NEWSLETTER_SEND_RATE messages per minute for the whole worker (all
// campaigns, all consumers together). The budget is one counter row per UTC
// minute in `send_rate`, taken with a single atomic upsert, so two consumers
// running at once can never hand out more than the rate between them. A
// consumer that gets less than it asked for sends what it got and puts the
// rest back on the queue for the next minute — it never sleeps in the worker.
//
// Batch size is the number of messages one queue job may send. It is bounded
// by the per-invocation limits of the plan, because one job runs in one
// invocation (wrangler.toml sets max_batch_size = 1 for that reason):
//   - Workers Free: 50 subrequests and 50 D1 queries per invocation. An HTTP
//     transport is one fetch per message; the engine adds about 2 D1 queries
//     per message and about 10 per job, so 10 messages ≈ 30 D1 queries + 10
//     fetches. Default 10.
//   - Workers Paid: 1000 D1 queries and 10 000 subrequests (by default) per
//     invocation; 100 messages ≈ 210 queries is comfortably inside.
// (Cloudflare's Workers and D1 limits pages, checked 2026-11-13.)
// The batch never exceeds the rate: a job bigger than a minute's budget could
// never finish in one go.

import type { Env } from "./lib";
import { intVar } from "./lib";

export const SEND_JOB = "newsletter.send-batch";

export const DEFAULT_RATE = 60; // per minute: a pace any provider tier and most SMTP servers accept
export const DEFAULT_BATCH = 10; // safe on Workers Free; 100 on Workers Paid
export const DEFAULT_STALE_MINUTES = 30; // > the 15-minute wall-clock limit of a queue consumer
export const DEFAULT_MAX_ATTEMPTS = 3;

export interface EngineConfig {
  rate: number; // messages per minute
  batchSize: number; // messages per queue job
  staleMs: number; // a claim or a queued batch older than this is abandoned
  maxAttempts: number; // tries per recipient
}

export function engineConfig(env: Env): EngineConfig {
  const rate = intVar(env.NEWSLETTER_SEND_RATE, DEFAULT_RATE);
  return {
    rate,
    batchSize: Math.min(intVar(env.NEWSLETTER_SEND_BATCH, DEFAULT_BATCH), rate),
    staleMs: intVar(env.NEWSLETTER_SEND_STALE_MINUTES, DEFAULT_STALE_MINUTES) * 60_000,
    maxAttempts: intVar(env.NEWSLETTER_SEND_MAX_ATTEMPTS, DEFAULT_MAX_ATTEMPTS),
  };
}

// minuteOf is the counter row's key: the UTC minute, "2026-11-13T12:34".
export const minuteOf = (t: number = Date.now()): string => new Date(t).toISOString().slice(0, 16);

// secondsToNextMinute is the delay that lands a deferred job in a fresh budget.
export const secondsToNextMinute = (t: number = Date.now()): number => 60 - Math.floor((t % 60_000) / 1000);

export interface Grant {
  minute: string;
  granted: number;
}

// takeBudget reserves up to `want` messages of this minute's budget and
// returns how many it got (0 when the minute is used up). One statement: in
// an UPDATE every right-hand side reads the row as it was, so `granted` is
// computed from the old `used` while `used` is raised — atomically, whoever
// else is reserving at the same moment. MAX keeps a lowered rate from giving
// budget back.
export async function takeBudget(env: Env, want: number, rate: number): Promise<Grant> {
  const minute = minuteOf();
  if (want <= 0) return { minute, granted: 0 };
  const row = await env.NEWSLETTER_DB.prepare(
    `INSERT INTO send_rate (minute, used, granted) VALUES (?1, MIN(?2, ?3), MIN(?2, ?3))
     ON CONFLICT(minute) DO UPDATE SET
       granted = MAX(used, MIN(?3, used + ?2)) - used,
       used = MAX(used, MIN(?3, used + ?2))
     RETURNING granted`,
  )
    .bind(minute, want, rate)
    .first<{ granted: number }>();
  return { minute, granted: Number((row as { granted: number }).granted) }; // RETURNING always yields the row
}

// giveBack returns reserved budget that was not used (recipients skipped at
// the last check, a batch stopped by a pause).
export async function giveBack(env: Env, grant: Grant, unused: number): Promise<void> {
  if (unused <= 0) return;
  await env.NEWSLETTER_DB.prepare("UPDATE send_rate SET used = MAX(0, used - ?) WHERE minute = ?").bind(unused, grant.minute).run();
}

// rateUsage is this minute's count, for the status endpoint.
export async function rateUsage(env: Env): Promise<{ minute: string; used: number }> {
  const minute = minuteOf();
  const row = await env.NEWSLETTER_DB.prepare("SELECT used FROM send_rate WHERE minute = ?").bind(minute).first<{ used: number }>();
  return { minute, used: row ? Number(row.used) : 0 };
}

// pruneRate drops counter rows older than an hour; the cron calls it.
export async function pruneRate(env: Env): Promise<void> {
  await env.NEWSLETTER_DB.prepare("DELETE FROM send_rate WHERE minute < ?").bind(minuteOf(Date.now() - 3_600_000)).run();
}
