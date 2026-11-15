// The media library (editor-ui:read):
//
// GET {base}/api/media?cursor=&limit=
//   → {dir, urlBase, limits, items: [{path, url, version, size, type}], nextCursor, truncated}
//   Images in the media folder, newest path first (canonical names start with
//   yyyy/mm). `url` is the root-relative address the site serves the image at
//   once deployed — what an article links and what a thumbnail loads; `type`
//   comes from the extension (no bytes are read). `limits` are the sizes a
//   publication may carry (media-change.ts), for the panel to warn early.
//   Costs the branch head and one cached listing of the folder.
//
// GET {base}/api/media/new-path?name=&type=&hash=
//   → {path, url, name}
//   The canonical path for an image added now (media-name.ts mediaPathFor):
//   name = the original file name or the alt text, type = an extension or a
//   MIME type (a hint: the bytes decide at publication), hash = hex SHA-256
//   of the bytes (at least its first 8 characters). The panel may compute the
//   same itself; the publication checks it either way.

import { decodeCursor, encodeCursor } from "./content-list";
import { contentScope } from "./content-scope";
import { LIMITS, mediaFiles } from "./media-change";
import { HASH_LENGTH, extOfType, mediaPathFor } from "./media-name";
import { requireMedia, typeOf, urlOf } from "./media-scope";
import { OpError } from "./ops";
import type { Op } from "./ops";
import { PERM } from "./permissions";
import type { ChangeLimits } from "../portable/repo-provider";

export interface MediaItem { path: string; url: string; version: string; size: number | null; type: string }
export interface MediaPage { dir: string; urlBase: string; limits: ChangeLimits; items: MediaItem[]; nextCursor: string | null; truncated: boolean }
export interface MediaListInput { cursor?: string | null; limit?: string | null }

export const DEFAULT_MEDIA_LIMIT = 60;
export const MAX_MEDIA_LIMIT = 200;

function limitOf(raw: string | null | undefined): number {
  const n = Number(raw ?? DEFAULT_MEDIA_LIMIT);
  return Number.isInteger(n) ? Math.min(Math.max(n, 1), MAX_MEDIA_LIMIT) : DEFAULT_MEDIA_LIMIT;
}

export const mediaListOp: Op<MediaListInput, MediaPage> = {
  name: "media_list",
  permission: PERM.read,
  run: async (ctx, input) => {
    const s = await contentScope(ctx);
    const place = requireMedia(s.model);
    const { files, truncated } = await mediaFiles(s, place);
    const before = input.cursor ? decodeCursor(input.cursor) : null;
    const newest = files.filter((f) => before === null || f.path < before).reverse();
    const limit = limitOf(input.limit);
    const page = newest.slice(0, limit);
    const items = page.map((f) => ({ path: f.path, url: urlOf(f.path, place), version: f.blobId, size: f.size ?? null, type: typeOf(f.path) }));
    const nextCursor = newest.length > limit ? encodeCursor(page[page.length - 1].path) : null;
    return { dir: place.dir, urlBase: place.urlBase, limits: LIMITS, items, nextCursor, truncated };
  },
};

export interface NewPathInput { name?: string | null; type?: string | null; hash?: string | null }

export const mediaNewPathOp: Op<NewPathInput, { path: string; url: string; name: string }> = {
  name: "media_new_path",
  permission: PERM.read,
  run: async (ctx, input) => {
    const ext = extOfType(input.type ?? "");
    if (!ext) throw new OpError("unsupported_type", 400, "type must be png, jpg, webp, gif or avif (SVG is refused)");
    const hash = (input.hash ?? "").toLowerCase();
    if (!/^[0-9a-f]{8,64}$/.test(hash)) throw new OpError("bad_hash", 400, `hash must be the hex SHA-256 of the bytes (at least ${HASH_LENGTH} characters)`);
    const place = requireMedia((await contentScope(ctx)).model);
    const path = mediaPathFor(place.dir, (ctx.deps.now ?? Date.now)(), input.name ?? "", hash, ext);
    return { path, url: urlOf(path, place), name: path.slice(path.lastIndexOf("/") + 1) };
  },
};
