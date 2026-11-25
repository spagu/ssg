// Campaign images (#446), the panel's side: what it decides without a DOM.
// The checks here are courtesy — they spare an upload that cannot succeed —
// and the worker stays the authority: it sniffs the bytes, caps the size and
// answers with a code that assetErrorKey turns into a sentence.

export const ACCEPTED_TYPES = ["image/jpeg", "image/png", "image/webp"];
// A browser may leave File.type empty (an unusual extension, some file
// pickers): the name then decides, and the worker checks the bytes anyway.
const ACCEPTED_NAME = /\.(jpe?g|png|webp)$/i;
// The page size of the image library (GET /api/assets?limit=).
export const LIBRARY_PAGE = 24;

// assetsState: what the editor offers, from GET /api/settings' `assets`.
//   off      the store is not bound: nothing is shown
//   problem  bound but unusable: say why instead of offering a dead button
//   ready    upload and the library are offered
export function assetsState(assets) {
  if (!assets || !assets.enabled) return "off";
  return assets.problem ? "problem" : "ready";
}

// checkFile: null when `file` may be sent, else {key, vars} naming why not.
export function checkFile(file, maxBytes, locale) {
  if (!file || !file.size) return { key: "asset.check.empty", vars: {} };
  const typed = ACCEPTED_TYPES.includes(file.type);
  if (!typed && (file.type || !ACCEPTED_NAME.test(file.name || ""))) return { key: "asset.check.type", vars: {} };
  if (maxBytes && file.size > maxBytes) return { key: "asset.check.tooLarge", vars: { size: byteSize(file.size, locale), max: byteSize(maxBytes, locale) } };
  return null;
}

// cleanAlt: the alt text as it will be stored — one line, single spaces, no
// control characters. Empty means there is none (and the insert is refused).
export const cleanAlt = (text) =>
  String(text ?? "")
    // eslint-disable-next-line no-control-regex
    .replace(/[\u0000-\u001f\u007f]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();

// markdownAlt escapes what would end or change `![…]`: brackets close it,
// a backslash would escape the next character, a backtick opens a code span
// inside the alt attribute. Newlines are already gone (cleanAlt).
export const markdownAlt = (text) => cleanAlt(text).replace(/[\\[\]`]/g, (c) => "\\" + c);

// htmlAttr escapes a value for a double-quoted HTML attribute.
export const htmlAttr = (text) =>
  String(text).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);

// markdownUrl keeps a URL inside `(…)`: the worker's keys are hex, but an
// own NEWSLETTER_ASSETS_URL may hold a space or a parenthesis.
const markdownUrl = (url) => String(url).replace(/[\s()<>]/g, (c) => "%" + c.charCodeAt(0).toString(16).toUpperCase().padStart(2, "0"));

// imageSnippet: what is inserted into the body, in the body's format.
export function imageSnippet(format, alt, url) {
  if (format === "html") return `<img src="${htmlAttr(url)}" alt="${htmlAttr(cleanAlt(alt))}">`;
  return `![${markdownAlt(alt)}](${markdownUrl(url)})`;
}

// insertAt: the body with `snippet` put in place of the selection, on a line
// of its own when the cursor sits inside other text; `cursor` is where the
// caret goes afterwards (after the snippet).
export function insertAt(body, start, end, snippet) {
  const before = body.slice(0, start);
  const after = body.slice(end);
  const lead = before && !before.endsWith("\n") ? "\n\n" : "";
  const tail = after && !after.startsWith("\n") ? "\n\n" : "";
  const text = before + lead + snippet + tail + after;
  return { text, cursor: before.length + lead.length + snippet.length };
}

// ASSET_CODES: the worker's error codes on the asset routes, each with a
// sentence under asset.error.<code>. Others fall back to the panel's words.
export const ASSET_CODES = ["empty_body", "too_large", "unsupported_type", "assets_disabled", "assets_misconfigured", "asset_in_use", "not_found"];

export const assetErrorKey = (err) => (err && ASSET_CODES.includes(err.code) ? `asset.error.${err.code}` : null);

// byteSize: 2097152 → "2 MB" in the panel's language (1 kB = 1000 bytes is
// what file managers show today; the worker counts plain bytes).
export function byteSize(bytes, locale) {
  const n = Number(bytes) || 0;
  const [value, unit] = n >= 1e6 ? [n / 1e6, "megabyte"] : n >= 1e3 ? [n / 1e3, "kilobyte"] : [n, "byte"];
  return new Intl.NumberFormat(locale, { style: "unit", unit, unitDisplay: "short", maximumFractionDigits: unit === "byte" ? 0 : 1 }).format(value);
}

// uploadNotes: what the author should know after an upload, as {key, vars}
// for each, and the notice level they add up to.
export function uploadNotes(r, locale) {
  const notes = [{ key: "asset.uploaded", vars: {} }];
  let level = "ok";
  if (r.resize?.error) {
    notes.push({ key: "asset.note.resizeFailed", vars: { max: r.resize.maxWidth } });
    level = "warning";
  } else if (r.resize?.resized) {
    notes.push({ key: "asset.note.resized", vars: { width: r.width, height: r.height, size: byteSize(r.bytes, locale), was: r.resize.original.width } });
  }
  if (r.metadata) {
    notes.push({ key: "asset.note.metadata", vars: {} });
    level = "warning";
  }
  return { notes, level };
}

// dimensions: "1200 × 800", or null when the worker could not read them.
export const dimensions = (a) => (a.width && a.height ? `${a.width} × ${a.height}` : null);

// thumbSize scales an image to fit a box, keeping its proportions, for the
// <img width height> that reserves its space before it loads.
export function thumbSize(a, box = 120) {
  if (!a.width || !a.height) return { width: box, height: box };
  const scale = Math.min(1, box / Math.max(a.width, a.height));
  return { width: Math.max(1, Math.round(a.width * scale)), height: Math.max(1, Math.round(a.height * scale)) };
}
