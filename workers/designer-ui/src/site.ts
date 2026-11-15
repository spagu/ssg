// What every repository operation starts from: the repository, the branch
// publications go to, and the configuration at its tip — plus the commit
// author for a publication (README of the round, decision 5).

import type { Author } from "../portable/repo-provider";
import { readConfig } from "./config-read";
import type { ConfigState } from "./config-read";
import type { Caller } from "./gate";
import type { OpContext } from "./ops";
import { requireRepository } from "./repo";
import type { Repository } from "./repo";

export interface Site { repo: Repository; branch: string; state: ConfigState }

// siteOf: 503 repo_not_configured without the DESIGNER_GIT_* set; errors of
// the repository host are thrown as RepoError for errorResponse to answer.
export async function siteOf(ctx: OpContext): Promise<Site> {
  const repo = requireRepository(ctx.env, ctx.deps);
  const branch = await repo.provider.branch();
  const state = await readConfig(repo.provider, `${repo.config.owner}/${repo.config.repo}`, branch);
  return { repo, branch, state };
}

// SERVICE_DOMAIN: a service account has no e-mail (SERVICE-TOKENS.md: email
// is ""), and a commit needs one: a reserved domain (RFC 2606) that can never
// reach anybody.
export const SERVICE_DOMAIN = "service.invalid";

// authorOf: the signed-in person — the token carries an e-mail and no name,
// so the e-mail is both; a service account is named by its id.
export function authorOf(caller: Caller): Author {
  if (caller.kind === "service") return { name: `service ${caller.user.id}`, email: `${caller.user.id}@${SERVICE_DOMAIN}` };
  return { name: caller.user.email, email: caller.user.email };
}
