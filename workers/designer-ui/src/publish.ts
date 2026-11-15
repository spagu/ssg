// POST {base}/api/publish — settings and theme files in ONE commit (README of
// the round, decision 3: one publication = one atomic commit). The panel's
// publish dialog sends everything edited here; the two single-part routes
// stay for scripts and MCP.
//
//   {message, settings?: {baseBlobId, changes}, theme?: {changes}}
//   → {commitId, branch, settings?: {blobId, changed}, theme?: {blobIds}}
//
// Every check runs before anything is written, and a refusal refuses the
// whole publication:
//   1. at least one part;
//   2. permissions: designer-ui:settings for a settings part and
//      designer-ui:theme for a theme part — a mixed publication needs both;
//   3. each part exactly as its own route checks it (settingsPart, themePart:
//      conflicts, schema, secrets, designer-ui:advanced per key, path policy);
//   4. one commit with the overlay and the theme files; a file changed on the
//      branch since it was read is a 409 naming it, from either part.

import { OpError, requirePermission } from "../portable/ops";
import type { Op } from "../portable/ops";
import { PERM } from "./permissions";
import { settingsPart } from "./settings-publish";
import type { SettingsPublishInput } from "./settings-publish";
import { authorOf } from "../portable/author";
import { siteOf } from "./site";
import { themePart } from "./theme-publish";
import type { ThemePublishInput } from "./theme-publish";

export interface PublishInput { message?: unknown; settings?: SettingsPublishInput; theme?: ThemePublishInput }
export interface CombinedResult {
  commitId: string;
  branch: string;
  settings?: { blobId: string; changed: string[] };
  theme?: { blobIds: Record<string, string | null> };
}

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

export const publishOp: Op<PublishInput, CombinedResult> = {
  name: "publish",
  // Reading is the floor; each part's own permission is checked in run().
  permission: PERM.read,
  run: async (ctx, input) => {
    const settings = input?.settings;
    const theme = input?.theme;
    if (settings !== undefined && !isObject(settings)) throw new OpError("invalid_change", 400, "settings must be {baseBlobId, changes}");
    if (theme !== undefined && !isObject(theme)) throw new OpError("invalid_change", 400, "theme must be {changes}");
    if (!settings && !theme) throw new OpError("invalid_change", 400, "a publication needs settings, theme, or both");
    if (settings) requirePermission(ctx.caller, PERM.settings);
    if (theme) requirePermission(ctx.caller, PERM.theme);
    const site = await siteOf(ctx);
    const s = settings ? settingsPart(ctx, site, settings) : null;
    const files = theme ? await themePart(site, theme) : [];
    const changes = s ? [s.change, ...files] : files;
    const r = await site.repo.provider.commit({ branch: site.branch, message: input?.message as string, author: authorOf(ctx.caller), changes });
    const out: CombinedResult = { commitId: r.commitId, branch: site.branch };
    if (s) out.settings = { blobId: r.blobIds[s.path]!, changed: s.changed };
    if (theme) out.theme = { blobIds: Object.fromEntries(Object.entries(r.blobIds).filter(([p]) => p !== s?.path)) };
    return out;
  },
};
