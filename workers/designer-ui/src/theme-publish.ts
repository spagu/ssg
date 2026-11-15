// POST {base}/api/theme/publish — one commit with the theme files a person
// changed, for designer-ui:theme (sensitive: a template is code the build runs
// and HTML every visitor gets).
//
//   {message, changes: [{op: "put", path, content: {text} | {base64}, baseBlobId: string | null}
//                       | {op: "delete", path, baseBlobId} | {op: "move", from, to, baseBlobId}]}
//   → {commitId, branch, blobIds}
//
// Every path passes the theme policy for writing (theme.ts): inside the theme
// folder, an extension on the list — so neither .ssg.designer.json (whose
// keys must be checked one by one, settings-publish.ts) nor .ssg.yaml can be
// written this way. A file changed on the branch since it was read is a 409
// with the conflicting paths (README, decision 4); the provider never forces.

import type { Change } from "../portable/repo-provider";
import { OpError } from "./ops";
import type { Op } from "./ops";
import { PERM } from "./permissions";
import { authorOf, siteOf } from "./site";
import type { Site } from "./site";
import { allowedPath, themePolicy } from "./theme";
import { requireTheme } from "./theme-ops";

export interface ThemePublishInput { message?: unknown; changes?: unknown }
export interface PublishResult { commitId: string; branch: string; blobIds: Record<string, string | null> }

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

// themeChanges: the changes with every path checked and normalised. Shapes
// beyond the paths (content, baseBlobId) are the provider's to validate.
export function themeChanges(raw: unknown, dir: string): Change[] {
  if (!Array.isArray(raw) || raw.length === 0) throw new OpError("invalid_change", 400, "changes must be a non-empty list");
  const policy = themePolicy(dir);
  return raw.map((c, i): Change => {
    if (!isObject(c)) throw new OpError("invalid_change", 400, `changes[${i}] is not an object`);
    if (c.op === "put") return { op: "put", path: allowedPath(c.path, policy, `changes[${i}]`), content: c.content as never, baseBlobId: c.baseBlobId as never };
    if (c.op === "delete") return { op: "delete", path: allowedPath(c.path, policy, `changes[${i}]`), baseBlobId: c.baseBlobId as never };
    if (c.op === "move") return { op: "move", from: allowedPath(c.from, policy, `changes[${i}]`), to: allowedPath(c.to, policy, `changes[${i}]`), baseBlobId: c.baseBlobId as never, ...(c.content === undefined ? {} : { content: c.content as never }) };
    throw new OpError("invalid_change", 400, `changes[${i}].op must be put, delete or move`);
  });
}

// themePart: the theme's changes, every path checked, nothing written.
export async function themePart(site: Site, input: ThemePublishInput | undefined): Promise<Change[]> {
  const theme = await requireTheme(site);
  return themeChanges(input?.changes, theme.dir);
}

export const themePublishOp: Op<ThemePublishInput, PublishResult> = {
  name: "theme_publish",
  permission: PERM.theme,
  run: async (ctx, input) => {
    const site = await siteOf(ctx);
    const changes = await themePart(site, input);
    const r = await site.repo.provider.commit({ branch: site.branch, message: input?.message as string, author: authorOf(ctx.caller), changes });
    return { commitId: r.commitId, branch: site.branch, blobIds: r.blobIds };
  },
};
