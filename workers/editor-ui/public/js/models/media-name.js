// The panel's mirror of the worker's image naming (src/media-name.ts, which
// imports only src/slug.ts): a pasted image gets its canonical path here,
// before it is published, so the Markdown can link to it at once and the
// preview can show it. The worker checks every image of a publication
// against its own copy of these rules and refuses a wrong name rather than
// renaming it — test/panel-media-name.test.ts runs both on the same cases so
// the two never disagree.
//
//   <mediaDir>/<yyyy>/<mm>/<slug>-<hash8>.<ext>
//
// Pure: bytes in, strings out; sha256Hex takes the browser's SubtleCrypto.

// IMAGE_TYPES: the extension each accepted type is written with, and its type.
export const IMAGE_TYPES = Object.freeze({ png: "image/png", jpg: "image/jpeg", webp: "image/webp", gif: "image/gif", avif: "image/avif" });
export const MAX_MEDIA_SLUG = 50;
export const MEDIA_FALLBACK = "image";
export const HASH_LENGTH = 8;

// Go lower-cases rune by rune; only these non-ASCII runes become a-z there.
const GO_LOWER_TO_ASCII = { "\u0130": "i", "\u212a": "k" };

// slugify is ssg's models.Slugify (the worker's src/slug.ts slugify): a-z and
// 0-9, every run of anything else one hyphen, hyphens trimmed.
export function slugify(s) {
  let out = "";
  let dash = false;
  for (const ch of String(s)) {
    const c = /^[A-Z]$/.test(ch) ? ch.toLowerCase() : (GO_LOWER_TO_ASCII[ch] ?? ch);
    if (/^[a-z0-9]$/.test(c)) {
      out += c;
      dash = false;
    } else if (!dash) {
      out += "-";
      dash = true;
    }
  }
  return out.replace(/^-+|-+$/g, "");
}

// mediaSlug: the slug of a file name or an alt text: folder and last
// extension dropped, slugify, cut at a hyphen to MAX_MEDIA_SLUG.
export function mediaSlug(name) {
  let s = slugify(String(name).replace(/^.*[\\/]/, "").replace(/\.[A-Za-z0-9]{1,5}$/, ""));
  if (s.length > MAX_MEDIA_SLUG) {
    s = s.slice(0, MAX_MEDIA_SLUG);
    const cut = s.lastIndexOf("-");
    s = cut > 0 ? s.slice(0, cut) : s;
  }
  return s || MEDIA_FALLBACK;
}

const ascii = (b, at, n) => String.fromCharCode(...b.subarray(at, at + n));
const PNG = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];

function isAvif(b) {
  if (ascii(b, 4, 4) !== "ftyp") return false;
  const size = ((b[0] << 24) | (b[1] << 16) | (b[2] << 8) | b[3]) >>> 0;
  const end = Math.min(size, b.length, 64);
  for (let at = 8; at + 4 <= end; at += at === 8 ? 8 : 4) if (/^avi[fs]$/.test(ascii(b, at, 4))) return true;
  return false;
}

// sniffImage: the type the bytes are, by their signature; null otherwise
// (SVG and HTML included — the worker refuses both).
export function sniffImage(b) {
  if (PNG.every((x, i) => b[i] === x)) return "png";
  if (b[0] === 0xff && b[1] === 0xd8 && b[2] === 0xff) return "jpg";
  if (/^GIF8[79]a$/.test(ascii(b, 0, 6))) return "gif";
  if (ascii(b, 0, 4) === "RIFF" && ascii(b, 8, 4) === "WEBP") return "webp";
  return isAvif(b) ? "avif" : null;
}

// sha256Hex: SHA-256 of the bytes, lower-case hex.
export async function sha256Hex(bytes, subtle = globalThis.crypto.subtle) {
  const d = new Uint8Array(await subtle.digest("SHA-256", bytes));
  return [...d].map((x) => x.toString(16).padStart(2, "0")).join("");
}

// monthOf: "yyyy/mm" of a time in milliseconds, UTC.
export function monthOf(ms) {
  const d = new Date(ms);
  return `${d.getUTCFullYear()}/${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

// mediaPathFor: the canonical path of an image (see the top of the file).
export function mediaPathFor(mediaDir, ms, name, hash, ext) {
  return `${mediaDir}/${monthOf(ms)}/${mediaSlug(name)}-${hash.slice(0, HASH_LENGTH)}.${ext}`;
}

// urlOf: the image's address on the site (the worker's media-scope.ts urlOf):
// the media folder's URL base and the path below the folder.
export const urlOf = (path, media) => media.urlBase + path.slice(media.dir.length + 1);
