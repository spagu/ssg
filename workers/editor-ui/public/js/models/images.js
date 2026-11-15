// Pasted images before they are published (GO-171, decision 3): they wait in
// the browser beside the draft and go to the repository in the publication's
// one commit, so an abandoned draft leaves no stray file. This model decides
// which waiting images a publication carries (only those the text still
// links to), whether one more fits the worker's limits (api/site `limits`),
// and how small an image too large has to become to fit. Pure.

// usedImages: the waiting images whose address the text links to.
export const usedImages = (body, images) => images.filter((img) => body.includes(`](${img.url})`) || body.includes(`](${img.url} `));

// fits: why one more image of `size` bytes does not fit beside the waiting
// `sizes` — "file" (too large on its own), "total" (the publication's
// images together), "count" (files per publication, the article included) —
// or null when it fits.
export function fits(size, sizes, limits) {
  if (size > limits.maxFileBytes) return "file";
  if (sizes.length + 2 > limits.maxFilesPerChange) return "count";
  if (sizes.reduce((a, b) => a + b, size) > limits.maxChangeBytes) return "total";
  return null;
}

// MARGIN: how far under the limit a downscaled image aims (encoders vary).
export const MARGIN = 0.85;

// scaleFor: the factor (0 < f < 1) a width and height are multiplied by so
// an image of `bytes` lands under `max` — bytes grow with the area, so the
// square root; null when it already fits.
export function scaleFor(bytes, max) {
  if (bytes <= max) return null;
  return Math.max(0.05, Math.sqrt((max * MARGIN) / bytes));
}

// sizeAfter: the dimensions at a scale, never below 1 pixel.
export const sizeAfter = (width, height, scale) => ({ width: Math.max(1, Math.round(width * scale)), height: Math.max(1, Math.round(height * scale)) });

// toBase64: bytes as base64, in chunks (a large image overflows the
// argument list of String.fromCharCode in one go).
export function toBase64(bytes) {
  let bin = "";
  for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(bin);
}

// imageChange: the publication's change for one waiting image.
export const imageChange = (img, bytes) => ({ op: "put", path: img.path, version: null, base64: toBase64(bytes) });

// formatBytes: "840 kB", "2.4 MB" in the panel's language.
export function formatBytes(n, locale) {
  const [unit, value] = n >= 1_000_000 ? ["megabyte", n / 1_000_000] : n >= 1000 ? ["kilobyte", n / 1000] : ["byte", n];
  return new Intl.NumberFormat(locale, { style: "unit", unit, unitDisplay: "short", maximumFractionDigits: unit === "megabyte" ? 1 : 0 }).format(value);
}

// libraryLabel: an image of the media library as a person reads it — its
// file name (the folder is the same for all of them).
export const libraryLabel = (path) => String(path).split("/").pop();

// matchesImage: the image's path holds the search text (any case).
export const matchesImage = (item, q) => item.path.toLowerCase().includes(String(q).trim().toLowerCase());
