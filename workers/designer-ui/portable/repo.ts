// The repository for one request: the {PREFIX}_GIT_* configuration and a
// provider bound to it. One provider per request — it counts its own
// subrequests against the plan's budget and applies the plan's limits
// ({PREFIX}_PLAN, plan.ts). Shared by the panel workers (GO-185): the prefix
// comes from the template's src/http.ts.

import { repoConfigFrom } from "./repo-config";
import type { RepoConfig } from "./repo-config";
import { githubProvider } from "./repo-github";
import type { GitHubOptions } from "./repo-github";
import type { RepoProvider } from "./repo-provider";
import { planOf } from "./plan";
import type { ActivePlan } from "./plan";
import { PREFIX } from "../src/http";
import type { Env } from "../src/http";
import { OpError } from "./ops";
import type { Deps } from "./ops";

// Repository: plan.subrequests is the budget an operation reading lazily
// (titles, links, the other side of a conflict) keeps within.
export type Repository = { config: RepoConfig; provider: RepoProvider & { used(): number }; plan: ActivePlan };

// repositoryOf: the repository, or the problem naming the variable (never its value).
export function repositoryOf(env: Env, deps: Deps, opts: GitHubOptions = {}): { ok: true; repo: Repository } | { ok: false; problem: string } {
  const p = planOf(env);
  if (!p.ok) return p;
  const r = repoConfigFrom(env as Record<string, unknown>, PREFIX);
  if (!r.ok) return r;
  const plan = p.active;
  const provider = githubProvider(r.config, { fetch: deps.fetch, now: deps.now, maxSubrequests: plan.subrequests, limits: plan.limits, ...opts });
  return { ok: true, repo: { config: r.config, provider, plan } };
}

// requireRepository: for an operation that cannot work without it — 503
// repo_not_configured otherwise.
export function requireRepository(env: Env, deps: Deps, opts: GitHubOptions = {}): Repository {
  const r = repositoryOf(env, deps, opts);
  if (!r.ok) throw new OpError("repo_not_configured", 503, r.problem);
  return r.repo;
}
