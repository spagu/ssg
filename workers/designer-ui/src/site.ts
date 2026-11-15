// What every repository operation starts from: the repository, the branch
// publications go to, and the configuration at its tip. The commit author of
// a publication is portable/author.ts (README of the round, decision 5).

import { readConfig } from "./config-read";
import type { ConfigState } from "./config-read";
import type { OpContext } from "../portable/ops";
import { requireRepository } from "../portable/repo";
import type { Repository } from "../portable/repo";

export interface Site { repo: Repository; branch: string; state: ConfigState }

// siteOf: 503 repo_not_configured without the DESIGNER_GIT_* set; errors of
// the repository host are thrown as RepoError for errorResponse to answer.
export async function siteOf(ctx: OpContext): Promise<Site> {
  const repo = requireRepository(ctx.env, ctx.deps);
  const branch = await repo.provider.branch();
  const state = await readConfig(repo.provider, `${repo.config.owner}/${repo.config.repo}`, branch);
  return { repo, branch, state };
}
