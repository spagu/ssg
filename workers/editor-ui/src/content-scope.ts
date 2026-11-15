// What every content operation starts from: the repository, the branch, the
// site model at its head, and the path policy built from the model — the one
// place that decides which files are content (README of the round, decision
// 7). Content is a `.md` file inside one of the site's content folders; the
// portable policy adds its hard denials (.github, workers, dotfiles, …).

import { checkPath, checkPrefix } from "../portable/path-policy";
import type { Access, PathPolicy, PathRefusal } from "../portable/path-policy";
import { OpError } from "../portable/ops";
import type { OpContext } from "../portable/ops";
import { requireRepository } from "../portable/repo";
import type { Repository } from "../portable/repo";
import { requireSiteModel } from "./site-model";
import type { SiteModel } from "./site-model";

export interface ContentScope {
  repo: Repository;
  branch: string;
  model: SiteModel; // model.head is the commit every read of the request uses
  policy: PathPolicy;
}

// CONTENT_EXTENSIONS: Markdown only (ssg's MCP also takes .markdown; the
// editor's ticket keeps to .md).
export const CONTENT_EXTENSIONS = [".md"];
const IMAGE = /\.(?:png|jpe?g|gif|webp|avif|svg)$/i;

export const contentPolicy = (model: SiteModel): PathPolicy => ({ roots: model.contentRoots, extensions: CONTENT_EXTENSIONS });

// contentScope opens the repository and reads the site model (503 when either
// is not usable — repo.ts, site-model.ts say what to set).
export async function contentScope(ctx: OpContext): Promise<ContentScope> {
  const repo = requireRepository(ctx.env, ctx.deps);
  const branch = await repo.provider.branch();
  const model = await requireSiteModel(repo.provider, repo.config, branch, ctx.env);
  return { repo, branch, model, policy: contentPolicy(model) };
}

const DETAIL: Partial<Record<PathRefusal, string>> = {
  outside_roots: "not inside one of the site's content folders",
  extension: "only Markdown (.md) files are content",
  denied: "this path is never readable or writable from the editor",
  dot_segment: "a name starting with a dot is not content",
  traversal: "\"..\" is not allowed in a path",
};

// pathError: 400 invalid_path naming the path, the policy's reason and a
// sentence for a person. An image gets its own reason: images are not
// published as content files.
export function pathError(raw: unknown, reason: PathRefusal): OpError {
  const path = typeof raw === "string" ? raw.slice(0, 400) : null;
  if (reason === "extension" && path !== null && IMAGE.test(path)) {
    return new OpError("invalid_path", 400, "images are not content files; they are added through the media library", { path, reason: "media" });
  }
  return new OpError("invalid_path", 400, DETAIL[reason] ?? "not a valid repository path", { path, reason });
}

// contentPath: the normalised path when `raw` is content the caller may
// access this way, else throws pathError.
export function contentPath(raw: unknown, scope: ContentScope, access: Access): string {
  const v = checkPath(raw, scope.policy, access);
  if (!v.ok) throw pathError(raw, v.reason);
  return v.path;
}

// contentFolder: a folder inside the content folders (for listing and new files).
export function contentFolder(raw: unknown, scope: ContentScope): string {
  const v = checkPrefix(raw, scope.policy);
  if (!v.ok) throw pathError(raw, v.reason);
  return v.path;
}
