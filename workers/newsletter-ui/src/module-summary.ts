// GET {base}/api/module/summary — the module summary (GO-148): a few numbers,
// what waits for the admin and what is wrong with the configuration, in the
// shape of workers/module-summary.schema.json. Behind admin auth like every
// /api/ route; answers without the subscribers table (it says so as a problem).
//
// Aggregates only, from at most three statements however large the tables:
// the status counts and the sign-up series the dashboard uses
// (subscriber-stats.ts), and one SELECT of scalar counts built from the
// engine's and the confirmation sender's own SQL. The `newsletter` template's
// summary shows the same confirmed and pending numbers under the same keys.

import { ELIGIBLE } from "./confirm";
import { UNCERTAIN_COUNT } from "./engine-status";
import { json } from "./lib";
import type { Env } from "./lib";
import type { RouteContext } from "./router";
import { subscribersReady } from "./schema";
import { settingsOf } from "./settings";
import { dayAt, firstDay, perDay, statusTotals } from "./subscriber-stats";
import { counted, summaryOf } from "./summary-kit";
import type { Attention, Metric, Problem, Summary, Text } from "./summary-kit";
import { DEAD_LETTERS, IN_PROGRESS, NEVER_MAILED, OWN_PROBLEMS, PANEL_LABELS, PANEL_PROBLEMS, PAUSED_BY_ENGINE, UNCERTAIN } from "./summary-texts";

// SERIES_DAYS: the sign-up series covers the dashboard's shortest range.
export const SERIES_DAYS = 30;

// Panel views each attention item opens (hash routes of public/js/core/router.js).
export const LINKS = {
  paused: "/#/campaigns?status=paused",
  dashboard: "/#/", // lists uncertain sends per campaign and the dead letters
  pending: "/#/subscribers?status=pending",
};

// COUNTS: one scalar subquery per number; the subscribers one only when that
// table exists. `?` is ELIGIBLE's id bound, "" for every row.
const COUNTS: [string, string][] = [
  ["inProgress", "SELECT COUNT(*) FROM campaigns WHERE status IN ('sending', 'paused')"],
  ["pausedByEngine", "SELECT COUNT(*) FROM campaigns c JOIN send_plans p ON p.campaign_id = c.id WHERE c.status = 'paused' AND p.error IS NOT NULL"],
  ["uncertain", UNCERTAIN_COUNT],
  ["deadLetters", "SELECT COUNT(*) FROM send_dead_letters"],
  ["suppressed", "SELECT COUNT(*) FROM suppressions"],
];
const NEVER_MAILED_COUNT: [string, string] = ["neverMailed", `SELECT COUNT(*) FROM subscribers s WHERE ${ELIGIBLE}`];

const withDetail = (text: Text, detail: string): Text =>
  Object.fromEntries(Object.entries(text).map(([l, s]) => [l, s.replace("{detail}", detail)])) as Text;

// configProblems: the dashboard's configuration warnings (models/dashboard.js)
// from the same settings logic, never a secret's value.
export function configProblems(env: Env, subscribers: boolean): Problem[] {
  const s = settingsOf(env);
  const out: Problem[] = [];
  if (!subscribers) out.push({ code: "no_subscribers_table", text: OWN_PROBLEMS.no_subscribers_table });
  if (!s.transport) out.push({ code: "no_transport", text: PANEL_PROBLEMS.no_transport });
  if (s.transportProblem) out.push({ code: "transport_unusable", text: withDetail(OWN_PROBLEMS.transport_unusable, s.transportProblem) });
  if (!s.siteUrlValid) out.push({ code: "no_site_url", text: PANEL_PROBLEMS.no_site_url });
  if (!s.sender.email) out.push({ code: "no_sender", text: PANEL_PROBLEMS.no_sender });
  if (!s.engine.queue) out.push({ code: "no_queue", text: PANEL_PROBLEMS.no_queue });
  return out;
}

// newsletterUiSummary computes the answer; moduleSummary serves it.
export async function newsletterUiSummary(env: Env): Promise<Summary> {
  const db = env.NEWSLETTER_DB;
  const subscribers = await subscribersReady(env);
  const counts = subscribers ? [...COUNTS, NEVER_MAILED_COUNT] : COUNTS;
  const select = db.prepare(`SELECT ${counts.map(([name, sql]) => `(${sql}) AS ${name}`).join(", ")}`);
  const n = (await (subscribers ? select.bind("") : select).first<Record<string, number>>()) as Record<string, number>;

  const metrics: Metric[] = [];
  const attention: Attention[] = [];
  const attend = (severity: Attention["severity"], count: number, text: Parameters<typeof counted>[0], href: string) => {
    if (count > 0) attention.push({ severity, text: counted(text, count), href });
  };
  attend("action", n.pausedByEngine, PAUSED_BY_ENGINE, LINKS.paused);
  attend("action", n.uncertain, UNCERTAIN, LINKS.dashboard);
  attend("warning", n.deadLetters, DEAD_LETTERS, LINKS.dashboard);
  let series: { label: Text; points: { day: string; value: number }[] } | undefined;
  if (subscribers) {
    const { byStatus } = await statusTotals(db);
    metrics.push({ key: "confirmed", label: PANEL_LABELS.confirmed, value: byStatus.confirmed }, { key: "pending", label: PANEL_LABELS.pending, value: byStatus.pending });
    attend("action", n.neverMailed, NEVER_MAILED, LINKS.pending);
    const first = firstDay(SERIES_DAYS);
    const signups = await perDay(db, "created_at", first);
    const points = Array.from({ length: SERIES_DAYS }, (_, i) => dayAt(first, i)).map((day) => ({ day, value: signups.get(day) ?? 0 }));
    series = { label: PANEL_LABELS.signups, points };
  }
  metrics.push({ key: "campaignsInProgress", label: IN_PROGRESS, value: n.inProgress }, { key: "suppressed", label: PANEL_LABELS.suppressed, value: n.suppressed });
  return summaryOf({ metrics, attention, problems: configProblems(env, subscribers), series });
}

export async function moduleSummary({ env }: RouteContext): Promise<Response> {
  return json(await newsletterUiSummary(env));
}
