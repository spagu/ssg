// Revisions are the repository's history (README of the round, decision 6),
// for designer-ui:read:
//
//   GET {base}/api/revisions?path=&cursor=   → {items: [{commitId, message, authorName, authorEmail, date}], nextCursor}
//   GET {base}/api/revisions/{commitId}?path= → the file as it was at that commit
//
// `path` is a theme file or .ssg.designer.json (the history of the config
// file is listed too). A theme file comes back as in api/theme/files; the
// overlay comes back parsed and redacted ({settings}), never as text — a
// literal secret written into it by hand must not leave the worker; the config
// file's old text is never served (api/settings shows its values, redacted).
// Going back to a revision is an ordinary publication of the old values.

import { checkPath } from "../portable/path-policy";
import { validCommitId } from "../portable/repo-provider";
import type { HistoryPage } from "../portable/repo-provider";
import { CONFIG_FILES, OVERLAY_FILE, utf8 } from "./config-read";
import { OpError } from "./ops";
import type { Op } from "./ops";
import { validateOverlay } from "./overlay-validate";
import { PERM } from "./permissions";
import { redact } from "./redact";
import { ROOT } from "./schema";
import { siteOf } from "./site";
import type { Site } from "./site";
import { SETTINGS_POLICY, themePolicy } from "./theme";
import { readThemeFile, requireTheme } from "./theme-ops";
import type { FileBody } from "./theme-ops";

export interface RevisionsInput { path: string | null; cursor: string | null }
export interface RevisionInput { commitId: string; path: string | null }

// kindOf: which of the three a path is — checked by the policies, normalised.
async function kindOf(site: Site, raw: unknown): Promise<{ kind: "settings" | "config" | "theme"; path: string; dir?: string }> {
  const s = checkPath(raw, SETTINGS_POLICY, "read");
  if (s.ok) return { kind: s.path === OVERLAY_FILE ? "settings" : "config", path: s.path };
  const theme = await requireTheme(site);
  const t = checkPath(raw, themePolicy(theme.dir), "read");
  if (!t.ok) throw new OpError("path_not_allowed", 400, `path: not a theme or settings file (${t.reason})`);
  return { kind: "theme", path: t.path, dir: theme.dir };
}

export const revisionsOp: Op<RevisionsInput, HistoryPage> = {
  name: "revisions_list",
  permission: PERM.read,
  run: async (ctx, { path, cursor }) => {
    const site = await siteOf(ctx);
    const k = await kindOf(site, path);
    return site.repo.provider.history(k.path, cursor, site.state.head);
  },
};

export type Revision =
  | { commitId: string; path: string; kind: "settings"; blobId: string; settings: unknown; problems: string[] }
  | ({ commitId: string; kind: "theme" } & FileBody);

export const revisionReadOp: Op<RevisionInput, Revision> = {
  name: "revision_read",
  permission: PERM.read,
  run: async (ctx, { commitId, path }) => {
    if (!validCommitId(commitId)) throw new OpError("invalid_change", 400, "commitId must be a full commit id from api/revisions");
    const site = await siteOf(ctx);
    const k = await kindOf(site, path);
    if (k.kind === "config") throw new OpError("not_served", 400, `the text of ${CONFIG_FILES.join("/")} is not served; GET api/settings shows its values`);
    if (k.kind === "theme") return { commitId, kind: "theme", ...(await readThemeFile(site.repo.provider, commitId, k.dir!, k.path)) };
    const file = await site.repo.provider.readAt(commitId, k.path);
    let doc: unknown = null;
    try {
      doc = JSON.parse(utf8(file.bytes) ?? "");
    } catch { /* not JSON: validateOverlay reports it */ }
    const problems = validateOverlay(doc);
    return { commitId, path: k.path, kind: "settings", blobId: file.blobId, settings: redact(doc, ROOT), problems };
  },
};
