// Everything that leaves the shop — email, outgoing webhooks, server-side
// tracking — goes through one queue with one retry policy.
//
// Why a queue at all: the row is written in the same step that changes the
// order, so a provider being down cannot lose a buyer's download link. Pages
// Functions have no cron, so the queue is drained opportunistically: right
// after writing, and again on ordinary traffic. A shop nobody visits for a week
// can add the cron worker (ECOM-022), but it should not be required to get an
// email out.

import type { Env } from "./_env";
import { newId, nowISO } from "./_lib";

export type OutboxKind = "email" | "webhook" | "tracking";

export interface OutboxRow {
  id: string;
  kind: OutboxKind;
  target: string;
  payload_json: string;
  attempts: number;
  next_attempt: string;
  last_error: string | null;
  done_at: string | null;
  created_at: string;
}

/** Minutes between attempts. Five tries over roughly a day and a half, then the
 *  row sits in the panel waiting for a human. */
const BACKOFF_MINUTES = [1, 10, 60, 360, 1440];

export async function enqueue(
  env: Env,
  kind: OutboxKind,
  target: string,
  payload: unknown,
): Promise<string> {
  const id = newId();
  await env.SHOP_DB.prepare(
    `INSERT INTO outbox (id, kind, target, payload_json, attempts, next_attempt, last_error, done_at, created_at)
     VALUES (?, ?, ?, ?, 0, ?, NULL, NULL, ?)`,
  )
    .bind(id, kind, target, JSON.stringify(payload), nowISO(), nowISO())
    .run();
  return id;
}

export interface Deliverer {
  (row: OutboxRow, env: Env): Promise<void>;
}

/** Delivers up to `limit` due rows. Each failure is recorded with its reason
 *  and rescheduled; each success is marked done. Never throws — draining is a
 *  side effect of some other request, and it must not fail that request. */
export async function drainOutbox(env: Env, limit = 5, deliver?: Deliverer): Promise<number> {
  const now = nowISO();
  const { results } = await env.SHOP_DB.prepare(
    `SELECT * FROM outbox WHERE done_at IS NULL AND next_attempt <= ? ORDER BY next_attempt LIMIT ?`,
  )
    .bind(now, limit)
    .all<OutboxRow>();

  const rows = results ?? [];
  let delivered = 0;
  const deliverFn = deliver ?? defaultDeliver;

  for (const row of rows) {
    try {
      await deliverFn(row, env);
      await env.SHOP_DB.prepare(`UPDATE outbox SET done_at = ?, last_error = NULL WHERE id = ?`)
        .bind(nowISO(), row.id)
        .run();
      delivered++;
    } catch (e) {
      const attempts = row.attempts + 1;
      const wait = BACKOFF_MINUTES[Math.min(attempts - 1, BACKOFF_MINUTES.length - 1)] ?? 1440;
      const next = new Date(Date.now() + wait * 60000).toISOString();
      // The message is truncated because it is shown in the panel and a
      // provider's HTML error page is not useful at full length.
      const message = String(e instanceof Error ? e.message : e).slice(0, 500);
      await env.SHOP_DB.prepare(
        `UPDATE outbox SET attempts = ?, next_attempt = ?, last_error = ? WHERE id = ?`,
      )
        .bind(attempts, next, message, row.id)
        .run();
    }
  }
  return delivered;
}

/** Wires the queue to the modules that know how to send each kind. Imported
 *  lazily so a test can drain with its own deliverer and never touch the
 *  network. */
async function defaultDeliver(row: OutboxRow, env: Env): Promise<void> {
  const payload = JSON.parse(row.payload_json) as Record<string, unknown>;
  switch (row.kind) {
    case "email": {
      const { deliverEmail } = await import("./_mail");
      return deliverEmail(env, row.target, payload);
    }
    case "webhook": {
      const { deliverWebhook } = await import("./_hooks");
      return deliverWebhook(env, row.target, payload);
    }
    case "tracking": {
      const { deliverTracking } = await import("./_tracking");
      return deliverTracking(env, row.target, payload);
    }
    default:
      throw new Error(`unknown outbox kind ${row.kind}`);
  }
}

/** Fire-and-forget draining, for handlers that have a waitUntil to spend.
 *  Swallows everything: the caller's response is not the queue's business. */
export function drainInBackground(env: Env, ctx: { waitUntil(p: Promise<unknown>): void }, limit = 3): void {
  ctx.waitUntil(drainOutbox(env, limit).catch(() => 0));
}

export async function retryOutbox(env: Env, id: string): Promise<boolean> {
  const res = await env.SHOP_DB.prepare(
    `UPDATE outbox SET next_attempt = ?, attempts = 0 WHERE id = ? AND done_at IS NULL`,
  )
    .bind(nowISO(), id)
    .run();
  return (res.meta?.changes ?? 0) > 0;
}
