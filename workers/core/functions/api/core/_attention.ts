// Things that need a person, from every module at once, and the
// configuration problems they report. A leading underscore keeps this file out
// of the Pages route table.

import type { ModuleSummary } from "./_summary";
import { joinHref } from "./_target";
import { textIn } from "./_view";

// ATTENTION_MAX: a home page lists the most serious first and says how many
// more there are; ten per module are allowed, so a cap keeps the list a list.
export const ATTENTION_MAX = 20;
export const SEVERITY_RANK = { action: 0, warning: 1, info: 2 } as const;
export type Severity = keyof typeof SEVERITY_RANK;

export interface Source {
  id: string;
  title: string;
  base: string;
  summary: ModuleSummary | null;
}
export interface AttentionView {
  module: string;
  title: string; // the module's title, in the asked language
  severity: Severity;
  text: string;
  href: string | null; // absolute path on this host; null when the join refused it
}

// mergeAttention lists every module's attention items, most serious first,
// then by module id (a module's own order kept within one severity), at most
// `max`. `total` is how many there were before the cap.
export function mergeAttention(sources: Source[], lang: string, max = ATTENTION_MAX): { items: AttentionView[]; total: number } {
  const all: AttentionView[] = [];
  for (const s of sources) {
    for (const a of s.summary?.attention ?? []) {
      all.push({ module: s.id, title: s.title, severity: a.severity, text: textIn(a.text, lang), href: joinHref(s.base, a.href) });
    }
  }
  // Array.prototype.sort is stable, so a module's own order survives.
  all.sort((a, b) => SEVERITY_RANK[a.severity] - SEVERITY_RANK[b.severity] || (a.module < b.module ? -1 : a.module > b.module ? 1 : 0));
  return { items: all.slice(0, max), total: all.length };
}

export interface ProblemView {
  module: string;
  title: string;
  code: string;
  text: string;
}

// problemsOf: every configuration problem every module reports, by module.
export function problemsOf(sources: Source[], lang: string): ProblemView[] {
  return sources.flatMap((s) => (s.summary?.problems ?? []).map((p) => ({ module: s.id, title: s.title, code: p.code, text: textIn(p.text, lang) })));
}
