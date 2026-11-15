// GET {base}/api/media/references?path=&cursor= (editor-ui:read)
//   → {path, url, items: [{path}], scanned, total, complete, nextCursor}
//
// The articles that link an image — the warning before it is deleted. A
// link counts when it ends in the image's path below the media folder
// ("/media/2026/11/cat-1a2b3c4d.png", "../media/2026/11/cat-1a2b3c4d.png",
// an absolute URL ending so), query and fragment ignored. Only Markdown
// content files are searched: a template or a page outside the content
// folders that links the image is not found.
//
// Each article costs two requests to the repository host the first time
// (content-list.ts READ_COST); what it links is then cached by its version.
// The search stops before the request's subrequest budget runs out and says
// so: complete = false, nextCursor = where to go on (the items found so far
// are this page's; ask again with the cursor and add them up). `scanned` is
// how many articles this page looked at, `total` how many there are.

import { contentFiles, decodeCursor, encodeCursor, READ_COST } from "./content-list";
import { decodeText } from "./content-meta";
import { contentScope } from "./content-scope";
import { mediaPath, requireMedia, urlOf } from "./media-scope";
import type { Op } from "../portable/ops";
import { PERM } from "./permissions";

export interface ReferencesInput { path?: string | null; cursor?: string | null }
export interface References {
  path: string; url: string; items: { path: string }[]; scanned: number; total: number; complete: boolean; nextCursor: string | null;
}

// LINK: anything link-like ending in an image extension, stopped by white
// space, quotes, brackets, a query or a fragment.
const LINK = /[^\s"'()<>[\]?#]+\.(?:png|jpe?g|webp|gif|avif|svg)(?=[\s"'()<>[\]?#]|$)/gi;
export const MAX_LINK_CACHE = 5000;
const linkCache = new Map<string, string[]>();
export const clearLinkCache = (): void => linkCache.clear();
export const cachedLinks = (blobId: string): string[] | undefined => linkCache.get(blobId);

// linksOf: the image links of a text (none for a file that is not UTF-8).
export function linksOf(bytes: Uint8Array): string[] {
  return [...new Set((decodeText(bytes) ?? "").match(LINK) ?? [])];
}

// rememberLinks keeps what an article version links, the oldest entry going first.
export function rememberLinks(blobId: string, links: string[]): string[] {
  if (linkCache.size >= MAX_LINK_CACHE) linkCache.delete(linkCache.keys().next().value!);
  linkCache.set(blobId, links);
  return links;
}

export const mediaReferencesOp: Op<ReferencesInput, References> = {
  name: "media_references",
  permission: PERM.read,
  run: async (ctx, input) => {
    const s = await contentScope(ctx);
    const place = requireMedia(s.model);
    const path = mediaPath(input.path, place, "read");
    const rel = path.slice(place.dir.length + 1);
    const links = (l: string[]) => l.some((x) => x === rel || x.endsWith("/" + rel));
    const { files, truncated } = await contentFiles(s, s.model.contentRoots);
    const after = input.cursor ? decodeCursor(input.cursor) : null;
    const todo = files.filter((f) => after === null || f.path > after);
    const items: { path: string }[] = [];
    let scanned = 0;
    let last = after ?? ""; // the cursor of "" starts from the beginning
    for (const f of todo) {
      let l = cachedLinks(f.blobId);
      if (!l) {
        if (s.repo.provider.used() + READ_COST > s.repo.plan.subrequests - READ_COST) break;
        l = rememberLinks(f.blobId, linksOf((await s.repo.provider.read(s.model.head, f.path)).bytes));
      }
      scanned++;
      last = f.path;
      if (links(l)) items.push({ path: f.path });
    }
    // Cut short: go on after the last article looked at (or where this page began).
    const cut = scanned < todo.length;
    const nextCursor = cut ? encodeCursor(last) : null;
    return { path, url: urlOf(path, place), items, scanned, total: files.length, complete: !cut && !truncated, nextCursor };
  },
};
