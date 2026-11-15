// GET {base}/api/module/summary — the module summary (docs/WORKER-MODULES.md,
// workers/module-summary.schema.json): how many settings the panel has set,
// how many theme files it can edit, which branch it publishes to, and what is
// wrong with the repository, the configuration or the settings file.
//
// Requests to the repository host: the branch head, the configuration files
// only when the head moved (config-read.ts caches them), and the theme
// folder's listing. No person's data and no setting's value ever: counts and
// fixed sentences only.

import { RepoError } from "../portable/repo-provider";
import { siteOf } from "./site";
import type { Op, OpContext } from "../portable/ops";
import { validateOverlay } from "./overlay-validate";
import { PERM } from "./permissions";
import { repositoryOf } from "../portable/repo";
import { overlayLeaves } from "./settings-diff";
import { summaryOf } from "./summary-kit";
import type { Attention, Metric, Problem, Summary, Text } from "./summary-kit";
import { BRANCH, LABELS, PROBLEMS } from "./summary-texts";
import { openTheme } from "./theme-ops";

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

export async function designerSummary(ctx: OpContext): Promise<Summary> {
  const now = new Date((ctx.deps.now ?? Date.now)());
  if (!repositoryOf(ctx.env, ctx.deps).ok) return summaryOf({ metrics: [], problems: [problem("repo_not_configured")] }, now);
  const metrics: Metric[] = [];
  const attention: Attention[] = [];
  const problems: Problem[] = [];
  try {
    const site = await siteOf(ctx);
    const { state } = site;
    attention.push({ severity: "info", text: fill(BRANCH, site.branch), href: PANEL });
    if (state.fileProblem || state.format !== "yaml") problems.push(problem("config_unreadable"));
    else if (Object.keys(state.unread).length) problems.push(problem("config_partly_read"));
    if (state.overlayProblem || (state.overlay && validateOverlay(state.overlay).length)) problems.push(problem("overlay_invalid"));
    metrics.push({ key: "overlaySettings", label: LABELS.overlaySettings, value: overlayLeaves(state.overlay ?? {}).length });
    const theme = await openTheme(site);
    if (theme.available) metrics.push({ key: "themeFiles", label: LABELS.themeFiles, value: theme.files.length });
    else problems.push(problem("theme_not_editable"));
  } catch (err) {
    if (!(err instanceof RepoError)) throw err;
    problems.push(repoProblem(err));
  }
  return summaryOf({ metrics, attention, problems }, now);
}

export const summaryOp: Op<void, Summary> = {
  name: "module_summary",
  permission: PERM.read,
  run: (ctx) => designerSummary(ctx),
};
