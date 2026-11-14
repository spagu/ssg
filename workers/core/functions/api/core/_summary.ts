// A module's summary (GO-148) as core reads and shows it. A leading
// underscore keeps this file out of the Pages route table.
//
// A summary is what a module says about itself, so it is read like a hostile
// document: through the same probe as a manifest (same host, no redirect, a
// timeout, a size cap, only the caller's credential), then by readSummary
// against the schema copy. Whatever goes wrong becomes a state of that one
// module — never an error of the whole answer:
//   ok           it answered a summary the reader accepts
//   unavailable  no answer in time, a network error, an error status, a
//                redirect, a URL off this host (reason says which)
//   denied       it refused the caller's credential (401/403)
//   invalid      not JSON, too large, or refused by the reader (detail)
// Texts are handed on as plain strings in the asked language. core never
// turns them into markup and neither may the page that shows them.

import { readSummary, SUMMARY_MAX_BYTES } from "./_module-manifest";
import type { ModuleSummary, Schema } from "./_module-manifest";
import schema from "./_module-summary-schema.json" with { type: "json" };
import type { Reader } from "./_probe";
import type { Answer } from "./_status";
import { textIn } from "./_view";

export type { ModuleSummary };

// summaryReader: the body may be pretty-printed, so twice the reader's cap is
// read; readSummary then holds the value itself to SUMMARY_MAX_BYTES.
export const summaryReader: Reader = {
  tag: "summary",
  max: SUMMARY_MAX_BYTES * 2,
  invalid: "invalid",
  read(raw) {
    const r = readSummary(raw, schema as Schema);
    return r.ok ? { ok: true, found: { value: r.summary } } : { ok: false, detail: r.errors[0] };
  },
};

export type SummaryState = "ok" | "unavailable" | "denied" | "invalid";
// DETAIL_MAX: a reader's finding can quote what the module sent; it is shown
// as text, and cut short so a module cannot fill the page with it.
export const DETAIL_MAX = 300;

export interface MetricView {
  key: string;
  label: string;
  value: number;
  unit: "percent" | "days" | null;
}
export interface SummaryView {
  state: SummaryState;
  reason?: string;
  httpStatus?: number;
  detail?: string;
  cached?: boolean;
  generatedAt?: string;
  metrics?: MetricView[];
  problems?: { code: string; text: string }[];
  series?: { label: string; points: { day: string; value: number }[] } | null;
  metricsIn?: string; // these numbers are shown by that module's summary instead
}

// summaryView turns one answer into what a caller is shown, in `lang`, and
// hands back the accepted summary itself (for the attention list).
export function summaryView(answer: Answer, lang: string): { view: SummaryView; summary: ModuleSummary | null } {
  const meta = {
    ...(answer.httpStatus === undefined ? {} : { httpStatus: answer.httpStatus }),
    ...(answer.cached ? { cached: true } : {}),
  };
  if (answer.status === "reachable") {
    const s = answer.value as ModuleSummary;
    return {
      summary: s,
      view: {
        state: "ok",
        ...meta,
        generatedAt: s.generatedAt,
        metrics: s.metrics.map((m) => ({ key: m.key, label: textIn(m.label, lang), value: m.value, unit: m.unit ?? null })),
        problems: s.problems.map((p) => ({ code: p.code, text: textIn(p.text, lang) })),
        series: s.series ? { label: textIn(s.series.label, lang), points: s.series.points.map((p) => ({ day: p.day, value: p.value })) } : null,
      },
    };
  }
  if (answer.status === "denied") return { summary: null, view: { state: "denied", ...meta } };
  if (answer.status === "invalid") {
    const detail = answer.detail === undefined ? {} : { detail: answer.detail.slice(0, DETAIL_MAX) };
    return { summary: null, view: { state: "invalid", reason: answer.reason, ...detail, ...meta } };
  }
  return { summary: null, view: { state: "unavailable", reason: answer.reason, ...meta } };
}
