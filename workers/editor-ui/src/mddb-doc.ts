// An MDDB document as the editor shows it, and back. The editor works on
// Markdown with a frontmatter block; MDDB keeps `meta` (string lists, flat by
// design) and `contentMd`. ssg builds a page from a document by reading its
// meta keys as frontmatter keys of the same names (internal/mddb/converter.go
// ToPage, after client.go toDocument flattens a one-value list to its value),
// so the mapping is exactly that and nothing more:
//
//   meta {title: ["Hello"], tags: ["a", "b"]} + contentMd "Body\n"
//   ⇄ "---\ntags: [a, b]\ntitle: Hello\n---\nBody\n"
//
// A one-value list is written as a scalar (as ssg reads it) except for the
// keys ssg reads as lists (tags, categories, aliases), which stay lists so the
// form shows them as such. Keys in sorted order. A meta key that is not a
// plain YAML key is kept as it is on every write and listed in `fixed` — the
// form cannot show it, a publication never loses it. Comments typed into the
// block are not kept: MDDB has no place for them.
// test/mddb-samples.json holds the shared samples, checked against ssg's
// converter (the report of GO-175 says how).

import { fieldsOf, setFields } from "./frontmatter-fields";
import type { FieldValue, Refusal } from "./frontmatter-fields";
import { splitDocument } from "./frontmatter";
import { setOwn } from "./mddb-client";
import type { Meta } from "./mddb-client";

export const LIST_KEYS: ReadonlySet<string> = new Set(["tags", "categories", "aliases"]);
const NAME = /^[A-Za-z_][A-Za-z0-9_.-]*$/;
const writable = (key: string, values: string[]): boolean => NAME.test(key) && key !== "__proto__" && !values.some((v) => /\p{Cs}/u.test(v));
const byName = (a: string, b: string): number => (a < b ? -1 : 1); // object keys are unique

export interface DocText { text: string; hidden: Meta }

// docText: the article text of a document, and the meta keys it leaves out.
export function docText(meta: Meta, contentMd: string): DocText {
  const fields: Record<string, FieldValue> = {};
  const hidden: Meta = {};
  for (const key of Object.keys(meta).sort(byName)) {
    const values = meta[key];
    if (!values.length) continue; // ssg's toDocument drops an empty list
    if (!writable(key, values)) setOwn(hidden, key, values);
    else fields[key] = values.length === 1 && !LIST_KEYS.has(key) ? values[0] : values;
  }
  return { text: "---\n" + setFields("", fields, "\n").raw + "---\n" + contentMd, hidden };
}

// textDoc: the meta and content of an article text, the hidden keys merged
// back; or what the block holds that MDDB's flat meta cannot.
export function textDoc(text: string, hidden: Meta): { ok: true; meta: Meta; contentMd: string } | { ok: false; refused: Refusal[] } {
  const d = splitDocument(text);
  if (d.format === "none") return { ok: true, meta: { ...hidden }, contentMd: d.body };
  if (d.format !== "yaml") return { ok: false, refused: [{ key: "frontmatter", reason: d.format === "toml" ? "toml_frontmatter" : "unclosed_frontmatter" }] };
  const { fields, fixed } = fieldsOf(d.raw);
  if (fixed.length) return { ok: false, refused: fixed.map((key) => ({ key, reason: "not_flat" })) };
  const meta: Meta = {};
  for (const [key, v] of Object.entries(fields)) {
    if (v === null || (Array.isArray(v) && !v.length)) continue;
    meta[key] = Array.isArray(v) ? v : [String(v)];
  }
  return { ok: true, meta: { ...meta, ...hidden }, contentMd: d.body };
}

// one: a meta value as ssg's converter sees a scalar — only a one-value list.
export const one = (meta: Meta, key: string): string | null => (meta[key]?.length === 1 ? meta[key][0] : null);

// goDate: the instant (ms) of a date ssg's parseDate accepts — RFC 3339,
// "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02" — else null.
export function goDate(s: string): number | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})(?:([T ])(\d{2}):(\d{2}):(\d{2})(\.\d+)?(Z|[+-]\d{2}:\d{2})?)?$/.exec(s);
  if (!m) return null;
  const [y, mo, d, sep, h = "0", mi = "0", se = "0", frac, zone] = m.slice(1);
  // Only RFC 3339 has a zone; Go accepts a fraction after the seconds in any layout.
  if (zone && sep !== "T") return null;
  const t = Date.UTC(+y, +mo - 1, +d, +h, +mi, +se);
  const back = new Date(t);
  if (back.getUTCMonth() !== +mo - 1 || back.getUTCDate() !== +d || +h > 23 || +mi > 59 || +se > 59) return null;
  const offset = !zone || zone === "Z" ? 0 : (zone[0] === "-" ? -1 : 1) * (+zone.slice(1, 3) * 60 + +zone.slice(4, 6));
  if (Math.abs(offset) >= 24 * 60) return null;
  return t - offset * 60_000 + (frac ? Math.floor(Number("0" + frac) * 1000) : 0);
}

export const iso = (ms: number): string => new Date(ms).toISOString().replace(/\.000Z$/, "Z");

export interface DocSummary { title: string | null; slug: string; status: string; type: "page" | "post" | null; date: string | null; draft: boolean }

// summaryOf: what the list shows of a document, read as ssg's ToPage reads
// it: the title, the slug (meta slug, else the key), the status (default
// "publish" — anything else is left out of the build, ToPages), the type (the
// source's, else the document's), the date as written when ssg can read it,
// else the time the document was added.
export function summaryOf(doc: { key: string; meta: Meta; addedAt: number }, sourceType: "page" | "post" | null): DocSummary {
  const status = one(doc.meta, "status") ?? "publish";
  const own = one(doc.meta, "type");
  const written = one(doc.meta, "date");
  const date = written !== null && goDate(written) !== null ? written : doc.addedAt ? iso(doc.addedAt * 1000) : null;
  return {
    title: one(doc.meta, "title"), slug: one(doc.meta, "slug") ?? doc.key, status,
    type: sourceType ?? (own === "page" || own === "post" ? own : null), date, draft: status !== "publish",
  };
}

// matchesFilter: the document is one the source reads — every filter key
// holds one of its values (MDDB's filterMeta: AND between keys, OR between
// values), and without a source type the document is a page or a post
// (internal/generator/mddb_sources.go reads only those).
export function matchesFilter(meta: Meta, filter: Meta, sourceType: string | null): boolean {
  const all = sourceType ? filter : { ...filter, type: ["page", "post"] };
  return Object.entries(all).every(([k, want]) => (meta[k] ?? []).some((v) => want.includes(v)));
}
