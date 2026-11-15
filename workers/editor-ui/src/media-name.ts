// What an image is and what it is called — pure functions, no repository, no
// request. The panel (GO-173) mirrors this file to name a pasted image before
// it is published; the worker checks every image of a publication against it,
// so a panel that gets a name wrong is refused, never silently renamed.
//
// The type is decided by the file's first bytes (its signature), never by its
// name or the type a client claims: PNG, JPEG, WebP, GIF and AVIF only. SVG,
// HTML and anything else are refused — an SVG served from the site's host can
// run script.
//
// The canonical path of an image inside the media folder:
//
//   <mediaDir>/<yyyy>/<mm>/<slug>-<hash8>.<ext>
//
//   yyyy/mm  the month (UTC) the image was added — the panel's choice, any
//            real month; the worker does not insist on the current one (a
//            draft can be published days after the paste)
//   slug     mediaSlug(original name): a–z, 0–9 and single hyphens, ≤ 50
//   hash8    the first 8 hex characters of SHA-256 of the bytes
//   ext      the extension of the type the bytes are (EXTENSIONS)
//
// The same bytes pasted twice get the same hash, so the same path in the
// same month — and the publication keeps the file already there.

import { slugify } from "./slug";

// IMAGE_TYPES: the extension each accepted type is written with, and its type.
export const IMAGE_TYPES = { png: "image/png", jpg: "image/jpeg", webp: "image/webp", gif: "image/gif", avif: "image/avif" } as const;
export type ImageExt = keyof typeof IMAGE_TYPES;

export const MAX_MEDIA_SLUG = 50;
export const MEDIA_FALLBACK = "image";
export const HASH_LENGTH = 8;

const ascii = (b: Uint8Array, at: number, n: number): string => String.fromCharCode(...b.subarray(at, at + n));
const PNG = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];

// isAvif: an ISO-BMFF "ftyp" box whose major or a compatible brand is AVIF
// (avif: still image, avis: image sequence). Only the box's first 64 bytes
// are looked at.
function isAvif(b: Uint8Array): boolean {
  if (ascii(b, 4, 4) !== "ftyp") return false;
  const size = ((b[0] << 24) | (b[1] << 16) | (b[2] << 8) | b[3]) >>> 0;
  const end = Math.min(size, b.length, 64);
  // major brand at 8, minor version at 12, compatible brands from 16
  for (let at = 8; at + 4 <= end; at += at === 8 ? 8 : 4) if (/^avi[fs]$/.test(ascii(b, at, 4))) return true;
  return false;
}

// sniffImage: the type the bytes are, by their signature; null for anything else.
export function sniffImage(b: Uint8Array): ImageExt | null {
  if (PNG.every((x, i) => b[i] === x)) return "png";
  if (b[0] === 0xff && b[1] === 0xd8 && b[2] === 0xff) return "jpg";
  if (/^GIF8[79]a$/.test(ascii(b, 0, 6))) return "gif";
  if (ascii(b, 0, 4) === "RIFF" && ascii(b, 8, 4) === "WEBP") return "webp";
  return isAvif(b) ? "avif" : null;
}

// looksLikeMarkup: the bytes start (after a BOM and white space) with "<" —
// HTML, SVG or XML. Only used to word the refusal; the check is sniffImage.
export function looksLikeMarkup(b: Uint8Array): boolean {
  const head = new TextDecoder().decode(b.subarray(0, 256)).trimStart(); // TextDecoder drops a BOM
  return head.startsWith("<");
}

// extOfType: an extension or a MIME type a client names ("png", "jpeg",
// "image/jpeg") → the canonical extension; null for SVG and anything else.
// A hint only: the bytes decide when the image is published.
export function extOfType(type: string): ImageExt | null {
  const t = type.trim().toLowerCase().replace(/^image\//, "").replace(/^\./, "");
  if (t === "jpeg" || t === "pjpeg") return "jpg";
  return Object.hasOwn(IMAGE_TYPES, t) ? (t as ImageExt) : null;
}

// mediaSlug: the slug of an original file name (or of the alt text): the
// folder and the last extension dropped, ssg's slugify (slug.ts), cut at a
// hyphen to MAX_MEDIA_SLUG, MEDIA_FALLBACK when nothing is left.
export function mediaSlug(name: string): string {
  let s = slugify(name.replace(/^.*[\\/]/, "").replace(/\.[A-Za-z0-9]{1,5}$/, ""));
  if (s.length > MAX_MEDIA_SLUG) {
    s = s.slice(0, MAX_MEDIA_SLUG);
    const cut = s.lastIndexOf("-");
    s = cut > 0 ? s.slice(0, cut) : s;
  }
  return s || MEDIA_FALLBACK;
}

// sha256Hex: SHA-256 of the bytes, lower-case hex (hash8 is its first 8).
export async function sha256Hex(bytes: Uint8Array): Promise<string> {
  const d = new Uint8Array(await crypto.subtle.digest("SHA-256", bytes as Uint8Array<ArrayBuffer>));
  return [...d].map((x) => x.toString(16).padStart(2, "0")).join("");
}

// monthOf: "yyyy/mm" of a time in milliseconds, UTC.
export function monthOf(ms: number): string {
  const d = new Date(ms);
  return `${d.getUTCFullYear()}/${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

// mediaPathFor: the canonical path of an image (see the top of the file).
export function mediaPathFor(mediaDir: string, ms: number, name: string, hash: string, ext: ImageExt): string {
  return `${mediaDir}/${monthOf(ms)}/${mediaSlug(name)}-${hash.slice(0, HASH_LENGTH)}.${ext}`;
}

export interface MediaName { month: string; slug: string; hash: string; ext: ImageExt }
const CANONICAL = /^([12]\d{3}\/(?:0[1-9]|1[0-2]))\/([a-z0-9]+(?:-[a-z0-9]+)*)-([0-9a-f]{8})\.(png|jpg|webp|gif|avif)$/;

// parseMediaName: the parts of a canonical path relative to the media folder
// ("2026/11/cat-1a2b3c4d.png"), or null when it is not canonical.
export function parseMediaName(rel: string): MediaName | null {
  const m = CANONICAL.exec(rel);
  if (!m || m[2].length > MAX_MEDIA_SLUG) return null;
  return { month: m[1], slug: m[2], hash: m[3], ext: m[4] as ImageExt };
}
