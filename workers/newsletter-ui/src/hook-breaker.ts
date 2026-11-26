// The bounce breaker for hard bounces reported by webhook (#460). An HTTP
// provider accepts nearly every message and reports unknown mailboxes minutes
// later, so the send-time breaker (engine-breaker.ts) never sees them. Hard
// bounces attributed to a campaign (bounce-attribution.ts) that arrive while
// it is `sending` are counted in hook_breaker, and the campaign pauses when
//
//   trips(hard, sent - sent_base, NEWSLETTER_BOUNCE_BREAKER_MIN / _PERCENT)
//
// where hard counts the webhook hard bounces since the last trip and sent the
// campaign's `sent` rows now. A trip pauses the campaign through the same
// state change and pause reason as a send-time trip, and starts a new count
// (hard 0, sent_base = sent): a resume is not paused again by the bounces that
// caused the first pause. A report for a campaign that is not sending (paused,
// sent, cancelled) changes the counts only (campaign_bounces), never this.
//
// Unlike a send-time trip, the addresses ARE suppressed: each one is the
// provider's own report about one mailbox, not a relay refusing everyone.

import type { Bounce } from "./bounce-store";
import { setPauseReason } from "./engine-batches";
import { breakerLimits, trips } from "./engine-breaker";
import { transition } from "./campaign-state";
import type { Env } from "./lib";

// webhookHard groups a report's new hard bounces by attributed campaign;
// the engine's own refusals (source smtp) are the send-time breaker's.
export function webhookHard(fresh: Bounce[]): Map<string, number> {
  const out = new Map<string, number>();
  for (const b of fresh) {
    if (b.kind !== "hard" || !b.campaignId || b.source === "smtp") continue;
    out.set(b.campaignId, (out.get(b.campaignId) ?? 0) + 1);
  }
  return out;
}

export const hookBreakerReason = (hard: number, sent: number): string =>
  `bounces: the provider reported ${hard} hard bounces by webhook for the last ${sent} messages sent; ` +
  "those addresses are suppressed — check where the list came from, then resume to send the rest";

// noteWebhookHard counts n hard bounces for a sending campaign and pauses it
// when the breaker trips. Returns whether this call paused it.
export async function noteWebhookHard(env: Env, campaignId: string, n: number): Promise<boolean> {
  const db = env.NEWSLETTER_DB;
  const at = new Date().toISOString();
  await db
    .prepare(
      `INSERT INTO hook_breaker (campaign_id, hard, sent_base, updated_at)
       SELECT ?1, ?2, 0, ?3 WHERE EXISTS (SELECT 1 FROM campaigns WHERE id = ?1 AND status = 'sending')
       ON CONFLICT(campaign_id) DO UPDATE SET hard = hook_breaker.hard + excluded.hard, updated_at = excluded.updated_at`,
    )
    .bind(campaignId, n, at)
    .run();
  const row = await db
    .prepare(
      `SELECT b.hard, b.sent_base, (SELECT COUNT(*) FROM campaign_sends WHERE campaign_id = ?1 AND status = 'sent') AS sent
       FROM hook_breaker b JOIN campaigns c ON c.id = b.campaign_id WHERE b.campaign_id = ?1 AND c.status = 'sending'`,
    )
    .bind(campaignId)
    .first<{ hard: number; sent_base: number; sent: number }>();
  if (!row) return false;
  const hard = Number(row.hard);
  const sent = Number(row.sent);
  const since = Math.max(0, sent - Number(row.sent_base));
  if (!trips(hard, since, breakerLimits(env))) return false;
  if (!(await transition(env, campaignId, "sending", "paused"))) return false;
  const reason = hookBreakerReason(hard, since);
  console.error("newsletter-ui: campaign paused by the webhook bounce breaker", campaignId, reason);
  await setPauseReason(env, campaignId, reason);
  await db.prepare("UPDATE hook_breaker SET hard = 0, sent_base = ?, updated_at = ? WHERE campaign_id = ?").bind(sent, at, campaignId).run();
  return true;
}

// feedBreaker runs noteWebhookHard for every campaign a report attributed
// hard bounces to.
export async function feedBreaker(env: Env, fresh: Bounce[]): Promise<void> {
  for (const [campaignId, n] of webhookHard(fresh)) await noteWebhookHard(env, campaignId, n);
}
