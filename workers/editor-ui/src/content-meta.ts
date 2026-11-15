// What the list shows of an article besides its path — title, date, draft,
// language — read from the file's frontmatter, as ssg reads it:
//   - title: `title:`, else the first "# heading" of the body (ssg's fallback);
//   - draft: a file with YAML frontmatter is published only with
//     `status: publish` (docs/CONTENT.md); a file without one is published;
//     an unclosed block is refused by ssg's build (draft unknown: null);
//   - date and lang: `date:` and `lang:` as written.
//
// A blob id names its bytes, so what was read from it is cached per isolate
// for good: the list reads each version of a file at most once.

import { readFields, splitDocument } from "./frontmatter";
import type { Format } from "./frontmatter";

export interface Meta { title: string | null; date: string | null; draft: boolean | null; lang: string | null; format: Format }

// META_BYTES: the head of a file the list decodes — frontmatter and the first
// heading live there; decoding a whole 2 MB article for a title costs CPU the
// Free plan does not have.
export const META_BYTES = 16 * 1024;
export const MAX_CACHED = 5000;

const cache = new Map<string, Meta>();
export const clearMetaCache = (): void => cache.clear();
export const cachedMeta = (blobId: string): Meta | undefined => cache.get(blobId);

// remember keeps a blob's meta; the oldest entry goes when the cache is full.
export function remember(blobId: string, meta: Meta): Meta {
  if (!cache.has(blobId) && cache.size >= MAX_CACHED) cache.delete(cache.keys().next().value!);
  cache.set(blobId, meta);
  return meta;
}

// decodeText: the bytes as UTF-8 text, a leading BOM kept; null when they are
// not UTF-8 (the editor does not open such a file).
export function decodeText(bytes: Uint8Array): string | null {
  try {
    return new TextDecoder("utf-8", { fatal: true, ignoreBOM: true }).decode(bytes);
  } catch {
    return null;
  }
}

const text = (v: unknown): string | null => (typeof v === "string" || typeof v === "number" ? String(v) : null);

// firstHeading: the first ATX "# heading" outside fenced code.
function firstHeading(body: string): string | null {
  let fence = "";
  for (const line of body.split(/\r?\n/)) {
    const m = /^\s*(```|~~~)/.exec(line);
    if (m) fence = fence === "" ? m[1] : fence === m[1] ? "" : fence;
    else if (!fence) {
      const h = /^#[ \t]+(.+?)[ \t#]*$/.exec(line);
      if (h) return h[1];
    }
  }
  return null;
}

// metaOf reads the list's fields from a file's text (or its head).
export function metaOf(source: string): Meta {
  const d = splitDocument(source);
  const { fields } = readFields(d);
  return {
    title: text(fields.title) ?? (d.format === "unclosed" ? null : firstHeading(d.body)),
    date: text(fields.date),
    draft: d.format === "yaml" ? fields.status !== "publish" : d.format === "unclosed" ? null : false,
    lang: text(fields.lang),
    format: d.format,
  };
}

// metaOfBytes: the meta of a file's bytes — only its head decoded (a cut
// multi-byte character at the end is replaced, never refused).
export const metaOfBytes = (bytes: Uint8Array): Meta =>
  metaOf(new TextDecoder("utf-8", { fatal: false, ignoreBOM: true }).decode(bytes.subarray(0, META_BYTES)));
