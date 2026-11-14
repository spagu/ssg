// Gathering every visible module with its summary — the work behind
// /api/core/overview and /api/core/info. A leading underscore keeps this file
// out of the Pages route table.
//
// One deadline covers the whole gathering: the manifests of modules the build
// could not describe are asked first, the summaries second, both within
// CORE_STATUS_DEADLINE_MS of the start (_status.ts `ask`).
//
// COST. A summary is a few D1 statements that count whole tables
// (docs/WORKER-MODULES.md, "What a summary costs on D1"). core asks for them
// only when a person opens or refreshes the home page — never on a timer — and
// keeps each answer for CORE_STATUS_CACHE_SECONDS per caller.

import type { Env } from "./_lib";
import type { Registry } from "./_registry";
import type { Caller } from "./_signin";
import { ask } from "./_status";
import type { Job } from "./_status";
import { summaryReader, summaryView } from "./_summary";
import type { ModuleSummary, SummaryView } from "./_summary";
import { visibleModules } from "./_visible";
import type { Visible } from "./_visible";

export interface Gathered extends Visible {
  summary: SummaryView | null; // null: the module declares no summary
  accepted: ModuleSummary | null; // the summary as the module sent it, when accepted
}

// SUPERSETS: a module whose summary repeats another's numbers. Both
// `newsletter` and `newsletter-ui` count the same subscribers under the same
// keys; when both answer, the numbers are shown once, by newsletter-ui (it
// also has the campaigns, the series and a panel to link to). The hidden
// module's problems stay: they are about its own configuration.
export const SUPERSETS: Record<string, string> = { newsletter: "newsletter-ui" };

export async function gather(request: Request, env: Env, caller: Caller, lang: string, registry: Registry, started = Date.now()): Promise<Gathered[]> {
  const visible = await visibleModules(request, env, caller, lang, registry, started);
  const jobs: Job[] = [];
  for (const v of visible) {
    const path = v.manifest?.endpoints.summary;
    if (path && v.entry.id !== "core") jobs.push({ id: v.entry.id, base: v.entry.base, path, reader: summaryReader });
  }
  const answers = await ask(request, env, jobs, started);
  const out: Gathered[] = visible.map((v) => {
    const answer = answers.get(v.entry.id);
    if (!answer) return { ...v, summary: null, accepted: null };
    const { view, summary } = summaryView(answer, lang);
    return { ...v, summary: view, accepted: summary };
  });
  for (const [shadowed, by] of Object.entries(SUPERSETS)) {
    const a = out.find((g) => g.entry.id === shadowed);
    const b = out.find((g) => g.entry.id === by);
    if (a?.summary?.state === "ok" && b?.summary?.state === "ok") {
      a.summary = { ...a.summary, metrics: [], series: null, metricsIn: by };
    }
  }
  return out;
}
