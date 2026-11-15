// The media folder: where pasted images go and the address the site serves
// them at — the second half of editor-ui's path policy (README of the round,
// decision 7; content-scope.ts is the first). Images are files with an image
// extension inside the media folder; the portable policy adds its hard
// denials (.github, workers, dotfiles, …).
//
// Where the folder is and how ssg publishes it:
//   - `<content_dir>/<source>/media` (the site model's mediaDir, as in ssg's
//     MCP mediaRootsOf) — ssg copies it to the output's media/, served at
//     /media/ (internal/generator/generator.go, "Copy media from content
//     source");
//   - no `source` → `<static_dir>/images`: static_dir is copied verbatim to
//     the output's root, so a folder inside it is served at its path below it;
//   - EDITOR_MEDIA_DIR: inside static_dir (served at its path below it), or
//     ssg's content media folder, ending in "/media" (served at /media/);
//     anything else is a folder ssg does not publish — 503.
// The address is root-relative, what an article links ("/media/…"); a site
// with base_path gets the prefix from ssg at build time (base_path.go).

import { checkPath } from "../portable/path-policy";
import type { Access, PathPolicy, PathRefusal } from "../portable/path-policy";
import { OpError } from "./ops";
import type { SiteModel } from "./site-model";

// MEDIA_EXTENSIONS: what the library lists and a publication may delete. New
// images are only ever the canonical types (media-name.ts); an existing
// .jpeg or .svg can still be listed, linked and removed.
export const MEDIA_EXTENSIONS = [".png", ".jpg", ".jpeg", ".webp", ".gif", ".avif", ".svg"];
const TYPES: Record<string, string> = {
  png: "image/png", jpg: "image/jpeg", jpeg: "image/jpeg", webp: "image/webp", gif: "image/gif", avif: "image/avif", svg: "image/svg+xml",
};

export interface MediaPlace { dir: string; urlBase: string; policy: PathPolicy }

export const MEDIA_HINT =
  "set EDITOR_MEDIA_DIR in wrangler.toml [vars] to a folder inside static_dir (e.g. \"static/images\") or to " +
  "<content_dir>/<source>/media, then deploy";

// urlBaseOf: the address the folder's files are served under, or null.
function urlBaseOf(dir: string, model: SiteModel): string | null {
  const st = model.staticDir;
  if (st !== null && dir === st) return "/";
  if (st !== null && dir.startsWith(st + "/")) return `/${dir.slice(st.length + 1)}/`;
  if (!model.overrides.mediaDir || dir.endsWith("/media")) return "/media/";
  return null;
}

// mediaPlace: the media folder of the site, or null when there is none ssg publishes.
export function mediaPlace(model: SiteModel): MediaPlace | null {
  const dir = model.mediaDir ?? (model.staticDir === null ? null : `${model.staticDir}/images`);
  const urlBase = dir === null ? null : urlBaseOf(dir, model);
  if (dir === null || urlBase === null) return null;
  return { dir, urlBase, policy: { roots: [dir], extensions: MEDIA_EXTENSIONS } };
}

// requireMedia: for an operation on images — 503 media_not_configured otherwise.
export function requireMedia(model: SiteModel): MediaPlace {
  const place = mediaPlace(model);
  if (!place) {
    const detail = model.mediaDir === null ? "the site has no media folder" : `${model.mediaDir} is not a folder ssg publishes`;
    throw new OpError("media_not_configured", 503, detail, { hint: MEDIA_HINT });
  }
  return place;
}

const DETAIL: Partial<Record<PathRefusal, string>> = {
  outside_roots: "not inside the site's media folder",
  extension: "not an image file name",
  denied: "this path is never readable or writable from the editor",
};

// mediaPath: the normalised path when `raw` is an image in the media folder,
// else 400 invalid_path naming it.
export function mediaPath(raw: unknown, place: MediaPlace, access: Access): string {
  const v = checkPath(raw, place.policy, access);
  if (v.ok) return v.path;
  const path = typeof raw === "string" ? raw.slice(0, 400) : null;
  throw new OpError("invalid_path", 400, DETAIL[v.reason] ?? "not a valid repository path", { path, reason: v.reason, mediaDir: place.dir });
}

// urlOf: the address the site serves an image of the folder at.
export const urlOf = (path: string, place: MediaPlace): string => place.urlBase + path.slice(place.dir.length + 1);

// typeOf: the image type its extension stands for (listing only — no bytes read).
export const typeOf = (path: string): string => TYPES[path.slice(path.lastIndexOf(".") + 1).toLowerCase()];
