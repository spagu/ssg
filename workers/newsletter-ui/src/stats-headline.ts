// The headline numbers of campaigns (GO-129), computed for a set of campaigns
// at once with one query per figure — never one query per campaign or per
// person. Everything here is a count; no row of the result names anyone.
//
//   sends         campaign_sends by status: the ledger the engine writes
//   unsubscribes  attributed by TIME WINDOW (below), not an exact count
//   clicks        the sum of click_links counters (GO-129 click counting)
//
// Unsubscribe attribution. The unsubscribe link belongs to the `newsletter`
// template and carries only the subscriber's token, so nothing records which
// mail it was clicked in. An unsubscribe is therefore credited to the most
// recent campaign actually SENT to that subscriber (campaign_sends.sent_at)
// before their unsubscribed_at — if that send is at most `windowDays` old at
// that moment; otherwise to no campaign. Any unsubscribe counts (the link, an
// admin, a complaint); a person erased since is no longer in the table and no
// longer counted; someone who signed up again has no unsubscribed_at.

import type { Env } from "./lib";
import { intVar } from "./lib";

export const DEFAULT_UNSUBSCRIBE_WINDOW_DAYS = 14;

export const unsubscribeWindowDays = (env: Env): number => intVar(env.NEWSLETTER_UNSUBSCRIBE_WINDOW_DAYS, DEFAULT_UNSUBSCRIBE_WINDOW_DAYS);

export interface Headline {
  total: number; // recipients counted when the campaign started (campaigns.total_count)
  sent: number;
  failed: number;
  skipped: number; // unsubscribed or suppressed between start and their turn
  pending: number; // claimed or waiting for a retry
  uncertain: number; // in flight when a consumer died: maybe delivered; the admin decides
  unsubscribes: number; // attributed by time window
  clicks: number; // all links together; 0 when click counting never ran
}

// The ids travel as one JSON array bound once (json_each), whatever their
// number: D1 caps bound parameters per statement.
const IDS = "SELECT value FROM json_each(?)";

type Counts = Map<string, Record<string, number>>;

async function grouped(env: Env, sql: string, binds: unknown[]): Promise<Counts> {
  const { results } = await env.NEWSLETTER_DB.prepare(sql).bind(...binds).all<Record<string, unknown>>();
  return new Map(results.map((r) => [r.campaign_id as string, r as Record<string, number>]));
}

const SENDS = `SELECT campaign_id, SUM(status = 'sent') AS sent, SUM(status = 'failed') AS failed,
    SUM(status = 'skipped') AS skipped, SUM(status IN ('claimed', 'retry')) AS pending, SUM(status = 'uncertain') AS uncertain
  FROM campaign_sends WHERE campaign_id IN (${IDS}) GROUP BY campaign_id`;

// SQLite returns the bare column cs.campaign_id from the row that holds
// MAX(cs.sent_at), so the inner query is "each unsubscribed subscriber's
// latest send before they left, and which campaign it was". The outer query
// keeps those within the window — compared as ISO text against left_at minus
// the window, so the edge is exact to the millisecond — and counts them.
const UNSUBSCRIBES = `SELECT campaign_id, COUNT(*) AS unsubscribes FROM (
    SELECT cs.campaign_id AS campaign_id, MAX(cs.sent_at) AS last_sent, s.unsubscribed_at AS left_at
    FROM subscribers s JOIN campaign_sends cs ON cs.subscriber_id = s.id
    WHERE s.unsubscribed_at IS NOT NULL AND cs.status = 'sent' AND cs.sent_at < s.unsubscribed_at
      AND s.id IN (SELECT subscriber_id FROM campaign_sends WHERE campaign_id IN (${IDS}))
    GROUP BY s.id
  ) WHERE last_sent >= strftime('%Y-%m-%dT%H:%M:%fZ', left_at, ?) AND campaign_id IN (${IDS})
  GROUP BY campaign_id`;

const CLICKS = `SELECT campaign_id, SUM(clicks) AS clicks FROM click_links WHERE campaign_id IN (${IDS}) GROUP BY campaign_id`;

// attributedUnsubscribes returns campaign id → unsubscribes credited to it.
export async function attributedUnsubscribes(env: Env, ids: string[], windowDays: number): Promise<Map<string, number>> {
  const list = JSON.stringify(ids);
  const rows = await grouped(env, UNSUBSCRIBES, [list, `-${windowDays} days`, list]);
  return new Map([...rows].map(([id, r]) => [id, Number(r.unsubscribes)]));
}

// headlines returns the Headline of each campaign in `campaigns` (id →
// total_count), in one round of three queries.
export async function headlines(env: Env, campaigns: Map<string, number>, windowDays: number): Promise<Map<string, Headline>> {
  const ids = [...campaigns.keys()];
  const list = JSON.stringify(ids);
  const sends = await grouped(env, SENDS, [list]);
  const unsubscribes = await attributedUnsubscribes(env, ids, windowDays);
  const clicks = await grouped(env, CLICKS, [list]);
  const out = new Map<string, Headline>();
  for (const [id, total] of campaigns) {
    const s = sends.get(id) ?? {};
    const n = (k: string) => Number(s[k] ?? 0);
    out.set(id, {
      total,
      sent: n("sent"),
      failed: n("failed"),
      skipped: n("skipped"),
      pending: n("pending"),
      uncertain: n("uncertain"),
      unsubscribes: unsubscribes.get(id) ?? 0,
      clicks: Number(clicks.get(id)?.clicks ?? 0),
    });
  }
  return out;
}
