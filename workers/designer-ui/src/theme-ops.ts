// Reading the theme: GET {base}/api/theme (the files) and
// GET {base}/api/theme/files/{path} (one file), for designer-ui:read — the
// permission table of the round gives reading the theme to `read`; writing it
// needs `theme` (theme-publish.ts).
//
// A theme that cannot be edited here is not an error of the list: it answers
// {available: false, reason, detail} with the way out, so the panel can show
// the settings tab anyway (ticket: "ustawienia działają normalnie").

import { checkPath } from "../portable/path-policy";
import { RepoError } from "../portable/repo-provider";
import type { RepoProvider } from "../portable/repo-provider";
import { utf8 } from "../portable/config-files";
import { effective } from "./config-read";
import { OpError } from "../portable/ops";
import type { Op, OpContext } from "../portable/ops";
import { PERM } from "./permissions";
import { siteOf } from "./site";
import type { Site } from "./site";
import { exportHint, isText, themeLocation, themePolicy } from "./theme";

export interface ThemeFile { path: string; blobId: string; size?: number; text: boolean }
export type ThemeList =
  | { available: true; head: string; branch: string; template: string; dir: string; files: ThemeFile[]; truncated: boolean }
  | { available: false; head: string; branch: string; template: string | null; dir: string | null; reason: string; detail: string };

// openTheme: the theme folder and its listing, or why it cannot be edited.
export async function openTheme(site: Site): Promise<ThemeList> {
  const { state, branch } = site;
  const loc = themeLocation(state, effective(state));
  const base = { head: state.head, branch };
  if (!loc.ok) return { available: false, ...base, template: loc.template, dir: null, reason: loc.reason, detail: loc.detail };
  let listing;
  try {
    listing = await site.repo.provider.tree(state.head, loc.dir);
  } catch (err) {
    if (!(err instanceof RepoError) || err.code !== "not_found") throw err;
    return { available: false, ...base, template: loc.template, dir: loc.dir, reason: "theme_not_in_repository", detail: exportHint(loc.dir, loc.template) };
  }
  const policy = themePolicy(loc.dir);
  const files = listing.entries
    .filter((e) => e.type === "file" && checkPath(e.path, policy, "read").ok)
    .map((e) => ({ path: e.path, blobId: e.blobId, size: e.size, text: isText(e.path) }))
    .sort((a, b) => (a.path < b.path ? -1 : 1));
  return { available: true, ...base, template: loc.template, dir: loc.dir, files, truncated: listing.truncated };
}

// requireTheme: the editable theme, or 409 with the reason and the way out.
export async function requireTheme(site: Site): Promise<Extract<ThemeList, { available: true }>> {
  const t = await openTheme(site);
  if (!t.available) throw new OpError(t.reason, 409, t.detail, { template: t.template, dir: t.dir });
  return t;
}

export const themeListOp: Op<void, ThemeList> = {
  name: "theme_list",
  permission: PERM.read,
  run: async (ctx) => openTheme(await siteOf(ctx)),
};

// FileBody: a file's bytes as UTF-8 text, or base64 for an image or a font.
export type FileBody = { path: string; blobId: string } & ({ encoding: "text"; text: string } | { encoding: "base64"; base64: string });
export type ThemeFileContent = FileBody & { head: string };

// readThemeFile reads one file the theme policy allows; a text file that is
// not UTF-8 comes back as base64 rather than mangled.
export async function readThemeFile(provider: RepoProvider, ref: string, dir: string, raw: unknown): Promise<FileBody> {
  const v = checkPath(raw, themePolicy(dir), "read");
  if (!v.ok) throw new OpError("path_not_allowed", 400, `not a theme file (${v.reason})`);
  const file = await provider.read(ref, v.path);
  const text = isText(v.path) ? utf8(file.bytes) : undefined;
  if (text !== undefined) return { path: v.path, blobId: file.blobId, encoding: "text", text };
  let bin = "";
  for (const b of file.bytes) bin += String.fromCharCode(b);
  return { path: v.path, blobId: file.blobId, encoding: "base64", base64: btoa(bin) };
}

export const themeReadOp: Op<{ path: string }, ThemeFileContent> = {
  name: "theme_read",
  permission: PERM.read,
  run: async (ctx: OpContext, { path }) => {
    const site = await siteOf(ctx);
    const t = await requireTheme(site);
    return { head: t.head, ...(await readThemeFile(site.repo.provider, t.head, t.dir, path)) };
  },
};
