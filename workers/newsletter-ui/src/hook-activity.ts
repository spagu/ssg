// Webhook health (#460): proof that verified webhooks arrive, `delivered`
// events included, which otherwise leave no row anywhere. Counts only, per
// UTC hour, provider and kind, in hook_activity; never an address. Rows older
// than ACTIVITY_RETENTION_MS are deleted with each write, so the table holds at
// most 8 days × 24 hours × providers × kinds rows.
//
// Read by GET /api/health and GET /api/settings as `webhookActivity`.

import type { Env } from "./lib";
import { transportOf } from "./mail/_transport";

export const ACTIVITY_RETENTION_MS = 8 * 86_400_000; // the 7-day window plus a day
export const ACTIVITY_KINDS = ["delivered", "hard", "soft", "complaint", "other"] as const;
// No warning until a campaign has been sending this long: webhooks lag sends.
export const WEBHOOK_GRACE_MS = 30 * 60_000;
// The transports whose bounces and complaints arrive only by webhook.
const WEBHOOK_TRANSPORTS: ReadonlySet<string> = new Set(["resend", "mailchannels"]);

const HOUR_MS = 3_600_000;
const hourOf = (ms: number): string => new Date(ms).toISOString().slice(0, 13); // "2026-11-26T14"

// recordActivity counts one verified request's events: one upsert per kind.
// `kinds` holds one entry per event (a classified kind, or "other").
export async function recordActivity(env: Env, provider: string, kinds: string[]): Promise<void> {
  const db = env.NEWSLETTER_DB;
  const now = Date.now();
  const at = new Date(now).toISOString();
  const tally = new Map<string, number>();
  for (const k of kinds) tally.set(k, (tally.get(k) ?? 0) + 1);
  const writes = [...tally].map(([kind, n]) =>
    db
      .prepare(
        `INSERT INTO hook_activity (hour, provider, kind, count, last_at) VALUES (?, ?, ?, ?, ?)
         ON CONFLICT(hour, provider, kind) DO UPDATE SET count = hook_activity.count + excluded.count, last_at = excluded.last_at`,
      )
      .bind(hourOf(now), provider, kind, n, at),
  );
  writes.push(db.prepare("DELETE FROM hook_activity WHERE hour < ?").bind(hourOf(now - ACTIVITY_RETENTION_MS)));
  await db.batch(writes);
}

type Counts = Record<(typeof ACTIVITY_KINDS)[number], number>;
const zero = (): Counts => ({ delivered: 0, hard: 0, soft: 0, complaint: 0, other: 0 });

export interface WebhookActivity {
  lastReceivedAt: string | null;
  last24h: Counts; // whole UTC hours: this one and the 23 before it
  last7d: Counts; // this hour and the 167 before it
  reportsByWebhook: boolean; // the configured transport reports bounces only by webhook
  lastCampaignStartedAt: string | null;
  warning: "no_webhook_since_campaign" | null;
}

// webhookActivity sums hook_activity (two statements, whatever the traffic).
// The warning: the transport reports by webhook, a campaign started more than
// WEBHOOK_GRACE_MS ago, and no verified webhook has arrived since it started.
export async function webhookActivity(env: Env, now = Date.now()): Promise<WebhookActivity> {
  const db = env.NEWSLETTER_DB;
  const { results } = await db
    .prepare(
      `SELECT kind, SUM(CASE WHEN hour >= ?1 THEN count ELSE 0 END) AS day, SUM(CASE WHEN hour >= ?2 THEN count ELSE 0 END) AS week,
         MAX(last_at) AS last FROM hook_activity GROUP BY kind`,
    )
    .bind(hourOf(now - 23 * HOUR_MS), hourOf(now - 167 * HOUR_MS))
    .all<{ kind: string; day: number; week: number; last: string }>();
  const last24h = zero();
  const last7d = zero();
  let lastReceivedAt: string | null = null;
  for (const r of results) {
    const kind = r.kind as keyof Counts; // recordActivity writes ACTIVITY_KINDS only
    last24h[kind] += Number(r.day);
    last7d[kind] += Number(r.week);
    if (lastReceivedAt === null || r.last > lastReceivedAt) lastReceivedAt = r.last;
  }
  // An aggregate always returns its one row.
  const started = (await db.prepare("SELECT MAX(started_at) AS at FROM campaigns").first<{ at: string | null }>()) as { at: string | null };
  const lastCampaignStartedAt = started.at;
  const reportsByWebhook = WEBHOOK_TRANSPORTS.has(transportOf(env) ?? "");
  const silent =
    reportsByWebhook &&
    lastCampaignStartedAt !== null &&
    now - Date.parse(lastCampaignStartedAt) > WEBHOOK_GRACE_MS &&
    (lastReceivedAt === null || lastReceivedAt < lastCampaignStartedAt);
  return { lastReceivedAt, last24h, last7d, reportsByWebhook, lastCampaignStartedAt, warning: silent ? "no_webhook_since_campaign" : null };
}
