// GET {base}/api/module/summary — the module summary (docs/WORKER-MODULES.md,
// workers/module-summary.schema.json): how long since the content changed,
// how many content folders the editor sees, which branch it publishes to, and
// what is wrong with the repository or the site's configuration.
//
// Requests to the repository host: the branch head, the configuration only
// when the head moved (site-model.ts caches it), and one one-entry history
// page per content folder (at most MAX_HISTORY). No person's data: the
// history's author names and messages are never read into the answer.

import { RepoError } from "../portable/repo-provider";
import type { Op } from "../portable/ops";
import { PERM } from "./permissions";
import { repositoryOf } from "../portable/repo";
import { loadSiteModel } from "./site-model";
import type { SiteModel } from "./site-model";
import { summaryOf } from "./summary-kit";
import type { Attention, Metric, Problem, Summary, Text } from "./summary-kit";
import { BRANCH, LABELS, PROBLEMS } from "./summary-texts";

export const MAX_HISTORY = 3;
const DAY_MS = 86_400_000;
// PANEL: where the attention items link (the panel's start view).
export const PANEL = "/#/";

const problem = (code: keyof typeof PROBLEMS): Problem => ({ code, text: PROBLEMS[code] });
const fill = (text: Text, branch: string): Text =>
  Object.fromEntries(Object.entries(text).map(([l, s]) => [l, s.replace("{branch}", branch)])) as Text;

// repoProblem names a repository host failure without its details.
function repoProblem(err: RepoError): Problem {
  if (err.code === "token_rejected") return problem("repo_token_rejected");
  if (err.code === "not_found") return problem("repo_not_found");
  return problem("repo_unreachable");
}

// lastChange: the newest commit date touching a content folder, or null.
async function lastChange(model: SiteModel, history: (path: string) => Promise<string | null>): Promise<number | null> {
  let newest: number | null = null;
  for (const root of model.contentRoots.slice(0, MAX_HISTORY)) {
    const at = Date.parse((await history(root)) ?? "");
    if (Number.isFinite(at) && (newest === null || at > newest)) newest = at;
  }
  return newest;
}

export async function editorSummary(ctx: Parameters<Op["run"]>[0]): Promise<Summary> {
  const now = (ctx.deps.now ?? Date.now)();
  const r = repositoryOf(ctx.env, ctx.deps, { historyPageSize: 1 });
  if (!r.ok) return summaryOf({ metrics: [], problems: [problem("repo_not_configured")] }, new Date(now));
  const { provider, config } = r.repo;
  const metrics: Metric[] = [];
  const attention: Attention[] = [];
  const problems: Problem[] = [];
  try {
    const branch = await provider.branch();
    attention.push({ severity: "info", text: fill(BRANCH, branch), href: PANEL });
    const state = await loadSiteModel(provider, config, branch, ctx.env);
    if (!state.ok) {
      problems.push(problem(state.error));
    } else {
      const { model } = state;
      if (model.overrides.contentDirs && model.configFile === null) problems.push(problem("site_config_overridden"));
      else if (model.problems.length) problems.push(problem("site_config_partial"));
      metrics.push({ key: "contentFolders", label: LABELS.contentFolders, value: model.contentRoots.length });
      const newest = await lastChange(model, async (path) => (await provider.history(path, null, model.head)).items[0]?.date ?? null);
      if (newest !== null) metrics.unshift({ key: "daysSinceContentChange", label: LABELS.daysSinceContentChange, value: Math.max(0, Math.floor((now - newest) / DAY_MS)), unit: "days" });
    }
  } catch (err) {
    if (!(err instanceof RepoError)) throw err;
    problems.push(repoProblem(err));
  }
  return summaryOf({ metrics, attention, problems }, new Date(now));
}

export const summaryOp: Op<void, Summary> = {
  name: "module_summary",
  permission: PERM.read,
  run: (ctx) => editorSummary(ctx),
};
