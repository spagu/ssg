// Images inside a publication (POST {base}/api/changes): the change
//   {op: "put", path, version: null, base64}
// adds one, and {op: "delete", path, version} removes one like any file.
// change-input.ts checks the permission (editor-ui:media for an image put,
// editor-ui:delete for any delete) before anything is read; this file checks
// the image itself, before the commit:
//   - sizes and count against the limits the repository layer enforces, from
//     the base64 length alone — nothing is decoded for a publication that
//     cannot pass (413 too_large with the limits, so a panel can offer to
//     shrink the image);
//   - the bytes are a PNG, JPEG, WebP, GIF or AVIF by their signature
//     (media-name.ts) — 400 invalid_image otherwise, whatever the name says;
//   - the path is canonical for those bytes: in the media folder, its type's
//     extension, the first 8 hex of their SHA-256 (400 invalid_image with
//     `expected`, the path the bytes call for);
//   - an image already on the branch with the same bytes is kept, not
//     written again — pasting the same file twice is no conflict.

import { checkPath } from "../portable/path-policy";
import { PLAN_LIMITS, contentBytes } from "../portable/repo-provider";
import type { Change, ChangeLimits } from "../portable/repo-provider";
import type { Planned } from "./change-input";
import { contentFiles } from "./content-list";
import { contentPath } from "./content-scope";
import type { ContentScope } from "./content-scope";
import { looksLikeMarkup, parseMediaName, sha256Hex, sniffImage } from "./media-name";
import { mediaPath, mediaPlace, requireMedia } from "./media-scope";
import type { MediaPlace } from "./media-scope";
import { OpError } from "./ops";

// LIMITS: the provider's own (repo.ts builds it without limits, so the Free
// plan's — portable/repo-provider.ts PLAN_LIMITS, measured in GO-168).
export const LIMITS: ChangeLimits = PLAN_LIMITS.free;

export type ImageOutcome = { change: Extract<Change, { op: "put" }> } | { kept: { path: string; version: string } };

const tooLarge = (detail: string) => new OpError("too_large", 413, detail, { limits: LIMITS });

// checkImageLimits: the publication's file count, and every image's size and
// their total from the base64 length (3 bytes per 4 characters).
export function checkImageLimits(planned: Planned[]): void {
  if (planned.length > LIMITS.maxFilesPerChange) throw tooLarge(`at most ${LIMITS.maxFilesPerChange} files per publication`);
  let total = 0;
  for (const p of planned) {
    if (p.op !== "put" || !("base64" in p.content)) continue;
    const n = Math.floor((p.content.base64.length * 3) / 4);
    if (n > LIMITS.maxFileBytes) throw tooLarge(`an image may have at most ${LIMITS.maxFileBytes} bytes`);
    total += n;
  }
  if (total > LIMITS.maxChangeBytes) throw tooLarge(`the images of one publication may have at most ${LIMITS.maxChangeBytes} bytes`);
}

// decodeBase64: the bytes (contentBytes refuses malformed base64 first, 400).
// Uint8Array.fromBase64 is native; the loop is for a runtime without it.
export function decodeBase64(b64: string): Uint8Array {
  contentBytes({ base64: b64 });
  const native = (Uint8Array as unknown as { fromBase64?: (s: string) => Uint8Array }).fromBase64;
  if (native) return native(b64);
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

// gitBlobId: the id git gives these bytes as a file — compared with the
// listing's to know the image on the branch is this one.
export async function gitBlobId(bytes: Uint8Array): Promise<string> {
  const head = new TextEncoder().encode(`blob ${bytes.length}\0`);
  const all = new Uint8Array(head.length + bytes.length);
  all.set(head);
  all.set(bytes, head.length);
  const d = new Uint8Array(await crypto.subtle.digest("SHA-1", all));
  return [...d].map((x) => x.toString(16).padStart(2, "0")).join("");
}

const refused = (path: string, reason: string, detail: string, extra: Record<string, unknown> = {}) =>
  new OpError("invalid_image", 400, detail, { path, reason, ...extra });

// mediaFiles: the images in the media folder at the head (listing cached per head).
export const mediaFiles = (s: ContentScope, place: MediaPlace) => contentFiles({ ...s, policy: place.policy }, [place.dir]);

// imageChange: the change for one image put, or the file already there.
export async function imageChange(s: ContentScope, raw: unknown, base64: string): Promise<ImageOutcome> {
  const place = requireMedia(s.model);
  const path = mediaPath(raw, place, "write");
  const name = parseMediaName(path.slice(place.dir.length + 1));
  if (!name) {
    throw refused(path, "not_canonical", "an image is named <yyyy>/<mm>/<slug>-<8 hex of its SHA-256>.<png|jpg|webp|gif|avif> inside the media folder");
  }
  const bytes = decodeBase64(base64);
  const ext = sniffImage(bytes);
  if (!ext) {
    if (looksLikeMarkup(bytes)) throw refused(path, "markup", "HTML, SVG and XML are refused: served from the site's host they could run script");
    throw refused(path, "not_an_image", "the bytes are not a PNG, JPEG, WebP, GIF or AVIF image");
  }
  const hash = (await sha256Hex(bytes)).slice(0, name.hash.length);
  if (ext !== name.ext || hash !== name.hash) {
    const expected = `${place.dir}/${name.month}/${name.slug}-${hash}.${ext}`;
    throw refused(path, ext !== name.ext ? "type_mismatch" : "hash_mismatch", "the name does not match the bytes", { expected, type: ext });
  }
  const there = (await mediaFiles(s, place)).files.find((f) => f.path === path);
  if (there && there.blobId === (await gitBlobId(bytes))) return { kept: { path, version: there.blobId } };
  return { change: { op: "put", path, content: { base64 }, baseBlobId: null } };
}

// deletablePath: an image of the media folder, or else a content file
// (content-scope.ts refuses everything else).
export function deletablePath(raw: unknown, s: ContentScope): string {
  const place = mediaPlace(s.model);
  const v = place ? checkPath(raw, place.policy, "write") : null;
  return v?.ok ? v.path : contentPath(raw, s, "write");
}
