// What an uploaded file is, decided by its first bytes only (#446): JPEG, PNG
// or WebP, nothing else. The content-type header and any file name are never
// consulted — both are whatever the client says. SVG is refused on purpose: it
// is a document that can carry script, and mail clients render it unevenly.
//
// The header parsers read the dimensions (null when the header does not give
// them) and whether the file carries camera metadata that may hold a location
// (JPEG APP1 = Exif or XMP, PNG eXIf, WebP VP8X with its EXIF or XMP flag).
// The file is never changed: the panel warns, the author decides.

export interface ImageInfo {
  type: "image/jpeg" | "image/png" | "image/webp";
  ext: "jpg" | "png" | "webp";
  width: number | null;
  height: number | null;
  metadata: boolean;
}

const PNG_SIGNATURE = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];

const ascii = (b: Uint8Array, at: number, n: number): string => String.fromCharCode(...b.subarray(at, at + n));
const be16 = (b: Uint8Array, at: number): number => (b[at] << 8) | b[at + 1];
const be32 = (b: Uint8Array, at: number): number => ((b[at] << 24) | (b[at + 1] << 16) | (b[at + 2] << 8) | b[at + 3]) >>> 0;
const le24 = (b: Uint8Array, at: number): number => b[at] | (b[at + 1] << 8) | (b[at + 2] << 16);
// size: a positive dimension, or null (a zero or missing value says nothing).
const size = (n: number): number | null => (n > 0 ? n : null);

// sniffImage returns what the bytes are, or null for anything but the three.
export function sniffImage(b: Uint8Array): ImageInfo | null {
  if (b.length >= 3 && b[0] === 0xff && b[1] === 0xd8 && b[2] === 0xff) return { type: "image/jpeg", ext: "jpg", ...jpeg(b) };
  if (b.length >= 8 && PNG_SIGNATURE.every((v, i) => b[i] === v)) return { type: "image/png", ext: "png", ...png(b) };
  if (b.length >= 12 && ascii(b, 0, 4) === "RIFF" && ascii(b, 8, 4) === "WEBP") return { type: "image/webp", ext: "webp", ...webp(b) };
  return null;
}

type Header = Pick<ImageInfo, "width" | "height" | "metadata">;

// jpeg walks the marker segments up to the start of the scan. A SOFn segment
// (C0–CF except C4 DHT, C8 JPG, CC DAC) holds height then width.
function jpeg(b: Uint8Array): Header {
  const out: Header = { width: null, height: null, metadata: false };
  let i = 2;
  while (i + 4 <= b.length) {
    if (b[i] !== 0xff) break; // not a marker: a damaged file, stop reading
    const marker = b[i + 1];
    if (marker === 0xff) {
      i++; // fill byte
      continue;
    }
    if (marker === 0xd8 || marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) {
      i += 2; // markers without a length
      continue;
    }
    if (marker === 0xd9 || marker === 0xda) break; // end of image, start of scan
    const len = be16(b, i + 2);
    if (len < 2) break;
    if (marker === 0xe1) out.metadata = true;
    if (marker >= 0xc0 && marker <= 0xcf && marker !== 0xc4 && marker !== 0xc8 && marker !== 0xcc && i + 9 <= b.length) {
      out.height = size(be16(b, i + 5));
      out.width = size(be16(b, i + 7));
    }
    i += 2 + len;
  }
  return out;
}

// png: IHDR is the first chunk (width, height at 16 and 20); eXIf may come
// anywhere before the image data, so chunks are walked until IDAT.
function png(b: Uint8Array): Header {
  const ihdr = b.length >= 24 && ascii(b, 12, 4) === "IHDR";
  const out: Header = { width: ihdr ? size(be32(b, 16)) : null, height: ihdr ? size(be32(b, 20)) : null, metadata: false };
  let i = 8;
  while (i + 8 <= b.length) {
    const name = ascii(b, i + 4, 4);
    if (name === "IDAT" || name === "IEND") break;
    if (name === "eXIf") out.metadata = true;
    i += 12 + be32(b, i); // length, name, data, CRC
  }
  return out;
}

// webp: the first chunk decides. VP8 (lossy): a key frame's start code, then
// 14-bit width and height. VP8L (lossless): signature 0x2F, then width-1 and
// height-1 in 14 bits each. VP8X (extended): flags, then 24-bit width-1 and
// height-1; flag 0x08 = EXIF chunk present, 0x04 = XMP.
function webp(b: Uint8Array): Header {
  const out: Header = { width: null, height: null, metadata: false };
  const chunk = b.length >= 16 ? ascii(b, 12, 4) : "";
  if (chunk === "VP8 " && b.length >= 30 && b[23] === 0x9d && b[24] === 0x01 && b[25] === 0x2a) {
    out.width = size(((b[27] << 8) | b[26]) & 0x3fff);
    out.height = size(((b[29] << 8) | b[28]) & 0x3fff);
  } else if (chunk === "VP8L" && b.length >= 25 && b[20] === 0x2f) {
    const bits = (b[21] | (b[22] << 8) | (b[23] << 16) | (b[24] << 24)) >>> 0;
    out.width = (bits & 0x3fff) + 1;
    out.height = ((bits >>> 14) & 0x3fff) + 1;
  } else if (chunk === "VP8X" && b.length >= 30) {
    out.metadata = (b[20] & 0x0c) !== 0;
    out.width = le24(b, 24) + 1;
    out.height = le24(b, 27) + 1;
  }
  return out;
}
