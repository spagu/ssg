// What every module summary (GO-148) is built with: the shape of
// GET …/api/module/summary (workers/module-summary.schema.json, contract 1) and
// two helpers. A template that serves a summary carries a byte-identical copy
// of this file — a Pages Function deploys only its own functions/ tree, so it
// cannot import from outside it; workers/module-summary.test.ts fails while a
// copy differs. No imports, no I/O, no person's data: callers pass counts.

export const SUMMARY_CONTRACT = 1;
export const SUMMARY_LANGUAGES = ["en", "pl", "hi", "de", "fr"] as const;
export type Language = (typeof SUMMARY_LANGUAGES)[number];
export type Text = Record<Language, string>;
// Forms: one language's CLDR plural forms of a sentence with {count}.
export type Forms = { other: string } & Partial<Record<"zero" | "one" | "two" | "few" | "many", string>>;
export type CountedText = Record<Language, Forms>;
export type Severity = "action" | "warning" | "info";

export interface Metric {
  key: string; // camelCase; the same number under the same key in every module
  label: Text;
  value: number;
  unit?: "percent" | "days"; // absent for a plain count
}
export interface Attention {
  severity: Severity; // action: waits for a person; warning: going wrong; info: good to know
  text: Text;
  href: string; // a path inside the module's own panel, never a URL
}
export interface Problem {
  code: string; // snake_case
  text: Text;
}
export interface Series {
  label: Text;
  points: { day: string; value: number }[]; // oldest first, one per UTC day
}
export interface Summary {
  contract: number;
  generatedAt: string;
  metrics: Metric[];
  attention: Attention[];
  problems: Problem[];
  series?: Series;
}

// The contract's caps. A module never sends more than a reader accepts.
export const SUMMARY_LIMITS = { metrics: 4, attention: 10, problems: 10, points: 90 };
const RANK: Record<Severity, number> = { action: 0, warning: 1, info: 2 };

// counted writes a sentence about n in every language: the plural form
// Intl.PluralRules picks for that language (or "other"), with {count} written
// as that language writes numbers.
export function counted(phrase: CountedText, n: number): Text {
  const out = {} as Text;
  for (const lang of SUMMARY_LANGUAGES) {
    const forms = phrase[lang];
    const form = forms[new Intl.PluralRules(lang).select(n) as keyof Forms] ?? forms.other;
    out[lang] = form.replaceAll("{count}", new Intl.NumberFormat(lang).format(n));
  }
  return out;
}

// summaryOf assembles the answer: attention most serious first (stable within
// a severity), every list cut to the contract's caps.
export function summaryOf(
  parts: { metrics: Metric[]; attention?: Attention[]; problems?: Problem[]; series?: Series },
  now: Date = new Date(),
): Summary {
  const attention = [...(parts.attention ?? [])].sort((a, b) => RANK[a.severity] - RANK[b.severity]);
  const out: Summary = {
    contract: SUMMARY_CONTRACT,
    generatedAt: now.toISOString(),
    metrics: parts.metrics.slice(0, SUMMARY_LIMITS.metrics),
    attention: attention.slice(0, SUMMARY_LIMITS.attention),
    problems: (parts.problems ?? []).slice(0, SUMMARY_LIMITS.problems),
  };
  if (parts.series) out.series = { label: parts.series.label, points: parts.series.points.slice(-SUMMARY_LIMITS.points) };
  return out;
}
