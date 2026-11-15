// GET {base}/api/content — the site's Markdown files, a page at a time:
//   ?q=        part of the path, any case
//   ?section=  a folder inside the content folders
//   ?cursor=   the nextCursor of the previous page
//   ?limit=    1–100, default 50
// → {items: [{path, version, size, section, type, language, title, date,
//    draft, loaded}], nextCursor, pending, truncated}
//
// Requests to the repository host: the branch head, two per content folder
// for its listing (cached per head), and two per article whose title is not
// cached yet (content-meta.ts). The listing stops reading titles before the
// request's subrequest budget (repo.ts) runs out: such items come back with
// `loaded: false` and `pending` counts them — asking again fills them in from
// the cache. The last-change time of each file is not listed: it would cost
// one more request per file (the revisions view shows it, GO-172).

import { RepoError } from "../portable/repo-provider";
import type { TreeEntry } from "../portable/repo-provider";
import { checkPath } from "../portable/path-policy";
import { contentFolder } from "./content-scope";
import type { ContentScope } from "./content-scope";
import { cachedMeta, metaOfBytes, remember } from "./content-meta";
import type { Meta } from "./content-meta";
import { OpError } from "../portable/ops";
import type { Op } from "../portable/ops";
import { PERM } from "./permissions";
import { languageOf, sectionOf } from "./content-place";
import { listSources } from "./source";

export interface ListInput { q?: string | null; section?: string | null; cursor?: string | null; limit?: string | null }
// origin: present when the site has an MDDB source too (GO-175) — "git" for
// a repository file, "mddb" for an MDDB article; absent = a repository file.
export interface ListItem {
  path: string; version: string; size: number | null; section: string; type: "page" | "post" | null; language: string | null;
  title: string | null; date: string | null; draft: boolean | null; loaded: boolean; origin?: "git" | "mddb";
}
export interface ListPage { items: ListItem[]; nextCursor: string | null; pending: number; truncated: boolean }

export const DEFAULT_LIMIT = 50;
export const MAX_LIMIT = 100;
// READ_COST: a file read is its directory's listing plus its blob.
export const READ_COST = 2;
// MAX_LISTINGS: folder listings kept per isolate (keyed by head and folder).
export const MAX_LISTINGS = 50;
const listings = new Map<string, { entries: TreeEntry[]; truncated: boolean }>();
export const clearListingCache = (): void => listings.clear();

// encodeCursor / decodeCursor: the last path of a page, base64url of UTF-8.
export const encodeCursor = (path: string): string =>
  btoa(String.fromCharCode(...new TextEncoder().encode(path))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
export function decodeCursor(raw: string): string {
  try {
    const bin = atob(raw.replace(/-/g, "+").replace(/_/g, "/"));
    return new TextDecoder("utf-8", { fatal: true, ignoreBOM: false }).decode(Uint8Array.from(bin, (c) => c.charCodeAt(0)));
  } catch {
    throw new OpError("bad_cursor", 400, "the cursor is not one this list gave");
  }
}

export function limitOf(raw: string | null | undefined): number {
  const n = Number(raw ?? DEFAULT_LIMIT);
  return Number.isInteger(n) ? Math.min(Math.max(n, 1), MAX_LIMIT) : DEFAULT_LIMIT;
}

// folderListing: one content folder at the head (empty when it does not exist).
async function folderListing(s: ContentScope, folder: string): Promise<{ entries: TreeEntry[]; truncated: boolean }> {
  const key = `${s.repo.config.owner}/${s.repo.config.repo}@${s.model.head}:${folder}`;
  let hit = listings.get(key);
  if (!hit) {
    try {
      hit = await s.repo.provider.tree(s.model.head, folder);
    } catch (err) {
      if (!(err instanceof RepoError) || err.code !== "not_found") throw err;
      hit = { entries: [], truncated: false };
    }
    if (listings.size >= MAX_LISTINGS) listings.delete(listings.keys().next().value!);
    listings.set(key, hit);
  }
  return hit;
}

// contentFiles: every content file under `folders`, by path, each once.
export async function contentFiles(s: ContentScope, folders: string[]): Promise<{ files: TreeEntry[]; truncated: boolean }> {
  const files = new Map<string, TreeEntry>();
  let truncated = false;
  for (const folder of folders) {
    const l = await folderListing(s, folder);
    truncated ||= l.truncated;
    for (const e of l.entries) if (e.type === "file" && checkPath(e.path, s.policy, "read").ok) files.set(e.path, e);
  }
  return { files: [...files.values()].sort((a, b) => (a.path < b.path ? -1 : 1)) /* paths are unique */, truncated };
}

// metaFor: the cached meta, or a read within the budget, or null (pending).
async function metaFor(s: ContentScope, e: TreeEntry): Promise<Meta | null> {
  const hit = cachedMeta(e.blobId);
  if (hit) return hit;
  if (s.repo.provider.used() + READ_COST > s.repo.plan.subrequests - READ_COST) return null;
  return remember(e.blobId, metaOfBytes((await s.repo.provider.read(s.model.head, e.path)).bytes));
}

export async function listContent(s: ContentScope, input: ListInput): Promise<ListPage> {
  const folders = input.section ? [contentFolder(input.section, s)] : s.model.contentRoots;
  const { files, truncated } = await contentFiles(s, folders);
  const q = (input.q ?? "").trim().toLowerCase();
  const after = input.cursor ? decodeCursor(input.cursor) : null;
  const matching = files.filter((f) => (!q || f.path.toLowerCase().includes(q)) && (after === null || f.path > after));
  const limit = limitOf(input.limit);
  const page = matching.slice(0, limit);
  const items: ListItem[] = [];
  for (const e of page) {
    const meta = await metaFor(s, e);
    const place = sectionOf(e.path, s.model);
    items.push({
      path: e.path, version: e.blobId, size: e.size ?? null, section: place.section, type: place.type,
      language: languageOf(e.path, meta?.lang ?? null, s.model),
      title: meta?.title ?? null, date: meta?.date ?? null, draft: meta ? meta.draft : null, loaded: meta !== null,
    });
  }
  const nextCursor = matching.length > limit ? encodeCursor(page[page.length - 1].path) : null;
  return { items, nextCursor, pending: items.filter((i) => !i.loaded).length, truncated };
}

export const contentListOp: Op<ListInput, ListPage> = {
  name: "content_list",
  permission: PERM.read,
  run: (ctx, input) => listSources(ctx, input),
};
