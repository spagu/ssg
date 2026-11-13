// Campaign statistics (GO-129), counts only:
//   GET {base}/api/campaigns/:id/stats   one campaign, in full
//   GET {base}/api/stats/campaigns       the latest started campaigns' headline
//                                        numbers, for the dashboard's bar chart
// What is counted and what is not: README "Statistics and privacy".

import { BOUNCES } from "./stats-bounces";
import { trackClicks } from "./click-track";
import { headlines, unsubscribeWindowDays } from "./stats-headline";
import type { Headline } from "./stats-headline";
import { json } from "./lib";
import type { RouteContext } from "./router";

interface CampaignTimes {
  id: string;
  subject: string;
  status: string;
  total_count: number;
  scheduled_at: string | null;
  started_at: string | null;
  finished_at: string | null;
}

const COLUMNS = "id, subject, status, total_count, scheduled_at, started_at, finished_at";

// durationSeconds is finished minus started, or null while either is missing.
const durationSeconds = (c: CampaignTimes): number | null =>
  c.started_at && c.finished_at ? Math.round((Date.parse(c.finished_at) - Date.parse(c.started_at)) / 1000) : null;

const timing = (c: CampaignTimes) => ({
  scheduledAt: c.scheduled_at,
  startedAt: c.started_at,
  finishedAt: c.finished_at,
  durationSeconds: durationSeconds(c),
});

// GET /api/campaigns/:id/stats → {id, subject, status, recipients: {total,
// sent, failed, skipped, pending, uncertain}, bounces, unsubscribes: {count, windowDays,
// attribution}, clicks: {tracking, total, links: [{linkId, url, clicks}]},
// timing: {scheduledAt, startedAt, finishedAt, durationSeconds}}, or 404.
export async function campaignStats({ env, params }: RouteContext): Promise<Response> {
  const db = env.NEWSLETTER_DB;
  const c = await db.prepare(`SELECT ${COLUMNS} FROM campaigns WHERE id = ?`).bind(params.id).first<CampaignTimes>();
  if (!c) return json({ error: "not_found" }, 404);
  const windowDays = unsubscribeWindowDays(env);
  const h = (await headlines(env, new Map([[c.id, Number(c.total_count)]]), windowDays)).get(c.id) as Headline;
  const { results: links } = await db
    .prepare("SELECT link_id, url, clicks FROM click_links WHERE campaign_id = ? ORDER BY clicks DESC, url")
    .bind(c.id)
    .all<{ link_id: string; url: string; clicks: number }>();
  return json({
    id: c.id,
    subject: c.subject,
    status: c.status,
    recipients: { total: h.total, sent: h.sent, failed: h.failed, skipped: h.skipped, pending: h.pending, uncertain: h.uncertain },
    bounces: BOUNCES,
    unsubscribes: { count: h.unsubscribes, windowDays, attribution: "time-window" },
    clicks: {
      tracking: trackClicks(env),
      total: h.clicks,
      links: links.map((l) => ({ linkId: l.link_id, url: l.url, clicks: Number(l.clicks) })),
    },
    timing: timing(c),
  });
}

export const DEFAULT_LIST = 10;
export const MAX_LIST = 50;

// GET /api/stats/campaigns?limit=10&cursor=… → {items: [{id, subject, status,
// total, sent, failed, skipped, pending, uncertain, unsubscribes, clicks, startedAt,
// finishedAt, durationSeconds}], nextCursor, windowDays}. Newest start first;
// drafts and scheduled campaigns have nothing to show and are left out.
export async function campaignsStats({ env, url }: RouteContext): Promise<Response> {
  const raw = url.searchParams.get("limit");
  const limit = raw === null ? DEFAULT_LIST : Number(raw);
  if (!Number.isInteger(limit) || limit < 1 || limit > MAX_LIST) {
    return json({ error: "invalid_limit", detail: `limit must be an integer from 1 to ${MAX_LIST}` }, 400);
  }
  const cursor = parseCursor(url.searchParams.get("cursor"));
  if (cursor === undefined) return json({ error: "invalid_cursor" }, 400);
  const page = cursor ? "AND (started_at, id) < (?, ?)" : "";
  const { results } = await env.NEWSLETTER_DB.prepare(
    `SELECT ${COLUMNS} FROM campaigns WHERE started_at IS NOT NULL ${page} ORDER BY started_at DESC, id DESC LIMIT ?`,
  )
    .bind(...(cursor ?? []), limit + 1)
    .all<CampaignTimes>();
  const rows = results.slice(0, limit);
  const windowDays = unsubscribeWindowDays(env);
  const numbers = await headlines(env, new Map(rows.map((c) => [c.id, Number(c.total_count)])), windowDays);
  const items = rows.map((c) => {
    const t = timing(c);
    return { id: c.id, subject: c.subject, status: c.status, ...numbers.get(c.id), startedAt: t.startedAt, finishedAt: t.finishedAt, durationSeconds: t.durationSeconds };
  });
  const last = rows[rows.length - 1];
  const nextCursor = results.length > limit ? `${last.started_at}~${last.id}` : null;
  return json({ items, nextCursor, windowDays });
}

// A cursor is "{started_at}~{id}" of the last item shown. null = first page,
// undefined = not a cursor this endpoint wrote.
function parseCursor(raw: string | null): [string, string] | null | undefined {
  if (raw === null) return null;
  const at = raw.indexOf("~");
  if (at < 1 || at === raw.length - 1 || Number.isNaN(Date.parse(raw.slice(0, at)))) return undefined;
  return [raw.slice(0, at), raw.slice(at + 1)];
}
