// GET {base}/api/stats/subscribers?days=30|90|365 — aggregates for the panel's
// charts. Counts only: nothing here identifies a person.
//
//   totals      by status, and by language × status (as the `newsletter` admin)
//   series      one entry per UTC day, oldest first, ending today, with the
//               sign-ups (created_at) and unsubscribes (unsubscribed_at) of that day
//
// The series reads what the table holds now: an erased person no longer counts
// as a sign-up, and someone who unsubscribed and signed up again keeps their
// first created_at and has no unsubscribed_at.

import type { RouteContext } from "./router";
import { json } from "./lib";
import { STATUSES } from "./input";

export const STAT_DAYS = [30, 90, 365];

const zeroes = () => Object.fromEntries(STATUSES.map((s) => [s, 0])) as Record<string, number>;

export async function subscriberStats({ env, url }: RouteContext): Promise<Response> {
  const days = Number(url.searchParams.get("days") ?? "30");
  if (!STAT_DAYS.includes(days)) return json({ error: "invalid_days", detail: "days must be 30, 90 or 365" }, 400);
  const db = env.NEWSLETTER_DB;

  const { results: groups } = await db
    .prepare("SELECT status, COALESCE(language, '') AS language, COUNT(*) AS n FROM subscribers GROUP BY status, language")
    .all<{ status: string; language: string; n: number }>();
  const byStatus = zeroes();
  const byLanguage: Record<string, Record<string, number>> = {};
  let total = 0;
  for (const g of groups) {
    total += g.n;
    byStatus[g.status] = (byStatus[g.status] ?? 0) + g.n;
    const lang = g.language || "unknown";
    byLanguage[lang] ??= zeroes();
    byLanguage[lang][g.status] = (byLanguage[lang][g.status] ?? 0) + g.n;
  }

  // ISO timestamps sort as text, so the first 10 characters are the UTC day.
  const today = new Date();
  today.setUTCHours(0, 0, 0, 0);
  const first = new Date(today.getTime() - (days - 1) * 86_400_000).toISOString();
  const perDay = async (column: string) => {
    const { results } = await db
      .prepare(`SELECT substr(${column}, 1, 10) AS day, COUNT(*) AS n FROM subscribers WHERE ${column} >= ? GROUP BY day`)
      .bind(first)
      .all<{ day: string; n: number }>();
    return new Map(results.map((r) => [r.day, r.n]));
  };
  const signups = await perDay("created_at");
  const unsubscribes = await perDay("unsubscribed_at");

  const series = [];
  for (let i = 0; i < days; i++) {
    const d = new Date(Date.parse(first) + i * 86_400_000).toISOString().slice(0, 10);
    series.push({ day: d, signups: signups.get(d) ?? 0, unsubscribes: unsubscribes.get(d) ?? 0 });
  }
  return json({ days, total, byStatus, byLanguage, series });
}
