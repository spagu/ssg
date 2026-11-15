// Frontmatter without a YAML parser (README of the round, decision 9). A
// Markdown file is split into its frontmatter block and its body exactly as
// ssg splits it (internal/parser/markdown.go: blank lines may come first, the
// first other line must be "---", the block ends at the next line that is
// "---" once trimmed), and joining the parts gives back the same string —
// byte for byte, CRLF and a leading BOM included.
//
// The form reads and writes only *simple* top-level keys (frontmatter-fields.ts):
// a one-line scalar, a one-line [list], or a list of "- item" lines. Every
// other line — comments, nested keys, block values — passes through untouched,
// and a key the editor cannot change safely is refused by name, never guessed.
//
// Two things ssg itself does not read, which the editor still keeps intact:
// TOML frontmatter between "+++" fences (ssg renders it as text; the editor
// leaves the block alone and lets the body be edited) and a block after a
// byte-order mark (ssg sees the BOM first, so the file has no frontmatter for
// it; `bom` tells the panel to say so).

import { fieldsOf, linesOf, setFields } from "./frontmatter-fields";
import type { FieldValue, Refusal } from "./frontmatter-fields";

export type { FieldValue, Refusal } from "./frontmatter-fields";

export type Format = "yaml" | "toml" | "none" | "unclosed";

// Doc: text === bom + head + raw + close + body, always.
export interface Doc {
  format: Format;
  bom: boolean;
  head: string; // leading blank lines and the opening fence line, with its line end
  raw: string; // the lines between the fences, with their line ends
  close: string; // the closing fence line, with its line end (none at the end of the file)
  body: string;
}

const BOM = "\uFEFF";
// Go's strings.TrimSpace trims Unicode White_Space — not U+FEFF, which
// JavaScript's trim() would also remove.
const GO_SPACE = /^[\t\n\v\f\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\t\n\v\f\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/g;
const goTrim = (s: string): string => s.replace(GO_SPACE, "");

// splitDocument separates the frontmatter block from the body.
export function splitDocument(text: string): Doc {
  const bom = text.startsWith(BOM);
  const rest = bom ? text.slice(1) : text;
  const none: Doc = { format: "none", bom, head: "", raw: "", close: "", body: rest };
  const lines = linesOf(rest);
  let i = 0;
  while (i < lines.length && goTrim(lines[i]) === "" && lines[i].endsWith("\n")) i++;
  const fence = goTrim(lines[i] ?? "");
  if (fence !== "---" && fence !== "+++") return none;
  for (let j = i + 1; j < lines.length; j++) {
    if (goTrim(lines[j]) !== fence) continue;
    return {
      format: fence === "---" ? "yaml" : "toml",
      bom,
      head: lines.slice(0, i + 1).join(""),
      raw: lines.slice(i + 1, j).join(""),
      close: lines[j],
      body: lines.slice(j + 1).join(""),
    };
  }
  return { ...none, format: "unclosed" };
}

export const joinDocument = (d: Doc): string => (d.bom ? BOM : "") + d.head + d.raw + d.close + d.body;

// eolOf: the line end the file uses (its opening fence's; LF when unknown).
const eolOf = (d: Doc): string => (d.head.endsWith("\r\n") ? "\r\n" : "\n");

// Fields: what the form shows. fixed = top-level keys it may not change.
export interface Fields { fields: Record<string, FieldValue>; fixed: string[] }

// readFields: the simple keys of a document (none unless the block is YAML).
export function readFields(d: Doc): Fields {
  return d.format === "yaml" ? fieldsOf(d.raw) : { fields: {}, fixed: [] };
}

// Edit: what a publication may change in an existing file. `fields` and
// `frontmatterRaw` exclude each other; what is not given stays as it was.
export interface Edit { fields?: Record<string, unknown>; frontmatterRaw?: string; body?: string }

const NOT_YAML: Record<Exclude<Format, "yaml">, string> = {
  toml: "toml_frontmatter",
  none: "no_frontmatter",
  unclosed: "unclosed_frontmatter",
};

// applyEdit returns the edited text, or the refusals (then nothing is changed).
export function applyEdit(text: string, edit: Edit): { text: string; refused: Refusal[] } {
  const d = splitDocument(text);
  const refused: Refusal[] = [];
  const keys = Object.keys(edit.fields ?? {});
  if (edit.fields && edit.frontmatterRaw !== undefined) {
    refused.push({ key: "frontmatterRaw", reason: "fields_and_raw" });
  } else if (edit.frontmatterRaw !== undefined) {
    if (d.format === "yaml" || d.format === "toml") d.raw = withEol(edit.frontmatterRaw, eolOf(d));
    else refused.push({ key: "frontmatterRaw", reason: NOT_YAML[d.format] });
  } else if (keys.length) {
    if (d.format !== "yaml") refused.push(...keys.map((key) => ({ key, reason: NOT_YAML[d.format as Exclude<Format, "yaml">] })));
    else {
      const r = setFields(d.raw, edit.fields!, eolOf(d));
      refused.push(...r.refused);
      d.raw = r.raw;
    }
  }
  if (edit.body !== undefined) d.body = edit.body;
  return refused.length ? { text, refused } : { text: joinDocument(d), refused };
}

// withEol: a replaced block must end with a line end, or the closing fence
// would land on its last line.
const withEol = (raw: string, eol: string): string => (raw === "" || raw.endsWith("\n") ? raw : raw + eol);

// NEW_FILE: what a new file starts from when it is written from the form —
// an empty frontmatter block; applyEdit fills in the fields and the body.
export const NEW_FILE = "---\n---\n";
