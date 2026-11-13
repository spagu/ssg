// GET {base}/api/stats/suppressions?days=30|90|365 — the suppression list in
// numbers, for the panel's chart. Counts only.
//
//   total, byReason   suppressions now, by reason
//   softCounting      addresses with soft bounces below the limit (not blocked)
//   series            one entry per UTC day, oldest first, ending today: the
//                     suppressions created that day, by reason
//
// Like the subscriber stats, the series reads what the table holds now: a
// lifted suppression no longer counts on the day it was created.

import { json } from "./lib";
import type { RouteContext } from "./router";
import { STAT_DAYS } from "./subscriber-stats";
import { REASONS } from "./suppression-admin";

const zeroes = () => Object.fromEntries(REASONS.map((r) => [r, 0])) as Record<string, number>;

export async function suppressionStats({ env, url }: RouteContext): Promise<Response> {
  const days = Number(url.searchParams.get("days") ?? "30");
  if (!STAT_DAYS.includes(days)) return json({ error: "invalid_days", detail: "days must be 30, 90 or 365" }, 400);
  const db = env.NEWSLETTER_DB;

  const { results: groups } = await db.prepare("SELECT reason, COUNT(*) AS n FROM suppressions GROUP BY reason").all<{ reason: string; n: number }>();
  const byReason = zeroes();
  let total = 0;
  for (const g of groups) {
    total += g.n;
    byReason[g.reason] = (byReason[g.reason] ?? 0) + g.n;
  }
  const soft = (await db.prepare("SELECT COUNT(*) AS n FROM soft_bounces").first<{ n: number }>()) as { n: number };

  const today = new Date();
  today.setUTCHours(0, 0, 0, 0);
  const first = new Date(today.getTime() - (days - 1) * 86_400_000).toISOString();
  const { results: perDay } = await db
    .prepare("SELECT substr(created_at, 1, 10) AS day, reason, COUNT(*) AS n FROM suppressions WHERE created_at >= ? GROUP BY day, reason")
    .bind(first)
    .all<{ day: string; reason: string; n: number }>();
  const byDay = new Map<string, Record<string, number>>();
  for (const r of perDay) {
    const d = byDay.get(r.day) ?? zeroes();
    d[r.reason] = (d[r.reason] ?? 0) + r.n;
    byDay.set(r.day, d);
  }
  const series = [];
  for (let i = 0; i < days; i++) {
    const day = new Date(Date.parse(first) + i * 86_400_000).toISOString().slice(0, 10);
    series.push({ day, ...(byDay.get(day) ?? zeroes()) });
  }
  return json({ days, total, byReason, softCounting: Number(soft.n), series });
}
