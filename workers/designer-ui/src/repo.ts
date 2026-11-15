// The repository for one request: the DESIGNER_GIT_* configuration and a
// provider bound to it (portable/, copied from workers/editor-ui/portable/).
// One provider per request — it counts its own subrequests against the plan.

import { repoConfigFrom } from "../portable/repo-config";
import type { RepoConfig } from "../portable/repo-config";
import { githubProvider } from "../portable/repo-github";
import type { GitHubOptions } from "../portable/repo-github";
import type { RepoProvider } from "../portable/repo-provider";
import { PREFIX } from "./http";
import type { Env } from "./http";
import { OpError } from "./ops";
import type { Deps } from "./ops";

// SUBREQUESTS: Workers Free allows 50 subrequests per invocation; a few stay
// free for the `auth` key fetch (DESIGNER_AUTH_JWKS_URL) and the panel's assets.
export const SUBREQUESTS = 45;

export type Repository = { config: RepoConfig; provider: RepoProvider & { used(): number } };

// repositoryOf: the repository, or the problem naming the variable (never its value).
export function repositoryOf(env: Env, deps: Deps, opts: GitHubOptions = {}): { ok: true; repo: Repository } | { ok: false; problem: string } {
  const r = repoConfigFrom(env as Record<string, unknown>, PREFIX);
  if (!r.ok) return r;
  const provider = githubProvider(r.config, { fetch: deps.fetch, now: deps.now, maxSubrequests: SUBREQUESTS, ...opts });
  return { ok: true, repo: { config: r.config, provider } };
}

// requireRepository: for an operation that cannot work without it — 503
// repo_not_configured otherwise.
export function requireRepository(env: Env, deps: Deps, opts: GitHubOptions = {}): Repository {
  const r = repositoryOf(env, deps, opts);
  if (!r.ok) throw new OpError("repo_not_configured", 503, r.problem);
  return r.repo;
}
