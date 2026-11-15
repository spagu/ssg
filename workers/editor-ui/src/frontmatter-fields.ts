// The simple top-level keys of a YAML frontmatter block — read for the form,
// written back by replacing only the lines of the keys that changed
// (frontmatter.ts splits the file; yaml-scalar.ts reads one-line values).
//
// A key is simple when it is written as one of:
//   title: Hello            (a one-line scalar: plain, 'single' or "double")
//   tags: [a, b]            (a one-line list of plain or quoted items)
//   tags:                   (a list of "- item" lines, one indentation,
//     - a                    no comments or blank lines between the items)
// Anything else — a nested mapping, a block value (| >), an anchor, alias or
// tag, a key written twice, a quoted or complex key — is `fixed`: listed, kept
// byte for byte, and refused by name when a change asks for it.

import { inlineValue, stripComment, typed } from "./yaml-scalar";

export type FieldValue = string | number | boolean | null | string[];
export interface Refusal { key: string; reason: string }

type Style = "plain" | "single" | "double" | "empty" | "inline" | "dash";
interface Field { value: FieldValue; style: Style; indent?: string; gap?: string }
interface Entry { name: string | null; start: number; end: number }
interface KeyLine { name: string; colon: string; sep: string; value: string; comment: string }

// NAME: a key the form may write (the minimal reader's key shape).
const NAME = /^[A-Za-z_][A-Za-z0-9_.-]*$/;
const KEY = /^([A-Za-z_][A-Za-z0-9_.-]*)([ \t]*:)(?:([ \t]+)(.*))?$/;
const ITEM = /^([ \t]*)-([ \t]+)(.+)$/;

// linesOf splits text into lines that keep their line ends.
export const linesOf = (text: string): string[] => text.match(/(?:[^\n]*\n|[^\n]+$)/g) ?? [];
const content = (line: string): string => line.replace(/\r?\n$/, "");
const eolOfLine = (line: string): string => line.slice(content(line).length);
const isGap = (c: string): boolean => c.trim() === "" || c.startsWith("#");
const isCont = (c: string): boolean => /^[ \t]/.test(c) || c === "-" || /^-[ \t]/.test(c);

function keyLine(c: string): KeyLine | null {
  const m = KEY.exec(c);
  if (!m) return null;
  const rest = m[4] ?? "";
  const value = stripComment(rest);
  return { name: m[1], colon: m[2], sep: m[3] ?? "", value, comment: rest.slice(value.length) };
}

// scan: the top-level entries — a key line (or an opaque line) and the
// indented or "- " lines after it; blank and comment lines between entries
// belong to none.
function scan(lines: string[]): Entry[] {
  const out: Entry[] = [];
  for (let i = 0; i < lines.length; ) {
    if (isGap(content(lines[i]))) { i++; continue; }
    let last = i;
    for (let j = i + 1; j < lines.length; j++) {
      const c = content(lines[j]);
      if (isCont(c)) last = j;
      else if (!isGap(c)) break;
    }
    out.push({ name: keyLine(content(lines[i]))?.name ?? null, start: i, end: last });
    i = last + 1;
  }
  return out;
}

// read: the one-line value, or undefined when the reader refuses it (it
// throws only YamlUnsupported: a value it does not follow is not simple).
const read = (text: string, line: number): ReturnType<typeof inlineValue> | undefined => {
  try {
    return inlineValue(text, line);
  } catch {
    return undefined;
  }
};
const styleOf = (v: string): Style => (v === "" ? "empty" : v[0] === '"' ? "double" : v[0] === "'" ? "single" : "plain");
const scalarItem = (v: unknown): v is string | number => typeof v === "string" || typeof v === "number";

// classify: the entry's value when it is simple, else null.
function classify(lines: string[], e: Entry): Field | null {
  const k = keyLine(content(lines[e.start]))!;
  if (e.end === e.start) {
    const v = read(k.value, e.start + 1);
    if (Array.isArray(v)) return v.every(scalarItem) ? { value: v.map(String), style: "inline" } : null;
    if (v === undefined || (v !== null && typeof v === "object")) return null;
    if (typeof v === "string" && styleOf(k.value) === "plain" && /:([ \t]|$)/.test(k.value)) return null;
    return { value: v, style: styleOf(k.value) };
  }
  if (k.value !== "") return null;
  const items: string[] = [];
  let indent: string | undefined;
  let gap = " ";
  for (let j = e.start + 1; j <= e.end; j++) {
    const m = ITEM.exec(content(lines[j]));
    if (!m || (indent !== undefined && m[1] !== indent) || stripComment(m[3]) !== m[3].trimEnd()) return null;
    if (/^[^"'[{]*?:([ \t]|$)/.test(m[3])) return null; // "- key: value" is a mapping
    const v = read(m[3], j + 1);
    if (!scalarItem(v)) return null;
    items.push(String(v));
    indent = m[1];
    gap = m[2];
  }
  return { value: items, style: "dash", indent, gap };
}

// fieldsOf: the simple keys with their values, and the keys the form may not change.
export function fieldsOf(raw: string): { fields: Record<string, FieldValue>; fixed: string[] } {
  const lines = linesOf(raw);
  const entries = scan(lines);
  const fields: Record<string, FieldValue> = {};
  const fixed: string[] = [];
  for (const e of entries) {
    if (e.name === null || Object.hasOwn(fields, e.name) || fixed.includes(e.name)) continue;
    const dup = entries.filter((x) => x.name === e.name).length > 1;
    // "__proto__" would set the object's prototype instead of a field.
    const f = dup || e.name === "__proto__" ? null : classify(lines, e);
    if (f) fields[e.name] = f.value;
    else fixed.push(e.name);
  }
  return { fields, fixed };
}

const noLoneSurrogate = (s: string): boolean => !/\p{Cs}/u.test(s);
function isFieldValue(v: unknown): v is FieldValue {
  if (v === null || typeof v === "boolean") return true;
  if (typeof v === "number") return Number.isFinite(v);
  if (typeof v === "string") return noLoneSurrogate(v);
  return Array.isArray(v) && v.every((x) => typeof x === "string" && noLoneSurrogate(x));
}

const ESC: Record<string, string> = { "\\": "\\\\", '"': '\\"', "\n": "\\n", "\t": "\\t", "\r": "\\r" };
const CONTROL = /[\x00-\x1f\x7f\u0085\u2028\u2029\uFEFF]/;
const dq = (s: string): string =>
  '"' + s.replace(/[\\"\x00-\x1f\x7f\u0085\u2028\u2029\uFEFF]/g, (c) => ESC[c] ?? "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0")) + '"';

// plainSafe: the string reads back as itself, unquoted (in a [list] when inFlow).
function plainSafe(s: string, inFlow: boolean): boolean {
  return s !== "" && s === s.trim() && !CONTROL.test(s) && !/^[-?:,[\]{}#&*!|>'"%@`~]/.test(s) && !/:([ \t]|$)|[ \t]#/.test(s)
    && typeof typed(s) === "string" && !/^(?:y|n|yes|no|on|off)$/i.test(s) && !(inFlow && /[,[\]{}]/.test(s));
}

function scalarText(v: Exclude<FieldValue, string[]>, style: Style): string {
  if (v === null) return "";
  if (typeof v !== "string") return String(v);
  if (style === "single" && !CONTROL.test(v)) return `'${v.replaceAll("'", "''")}'`;
  return style !== "double" && plainSafe(v, false) ? v : dq(v);
}
const inline = (items: string[]): string => `[${items.map((s) => (plainSafe(s, true) ? s : dq(s))).join(", ")}]`;
const valueText = (v: FieldValue, style: Style): string => (Array.isArray(v) ? inline(v) : scalarText(v, style));

// keyText: the key line with a new value, its spacing and comment kept.
function keyText(k: KeyLine, text: string): string {
  let comment = k.comment;
  if (comment && !/^[ \t]/.test(comment)) comment = (text === "" ? k.sep || " " : " ") + comment;
  return k.name + k.colon + (text === "" ? "" : (k.sep || " ") + text) + comment;
}

function rewrite(lines: string[], e: Entry, f: Field, value: FieldValue, eol: string): string[] {
  const end = eolOfLine(lines[e.start]) || eol;
  if (Array.isArray(value) && f.style === "dash" && value.length) {
    return [lines[e.start], ...value.map((item) => `${f.indent}-${f.gap}${scalarText(item, "plain")}${end}`)];
  }
  return [keyText(keyLine(content(lines[e.start]))!, valueText(value, f.style)) + end];
}

// setFields writes `updates` into the block `raw`: changed keys get new lines,
// new keys go at its end, everything else stays byte for byte. Any refusal →
// raw unchanged and every refusal listed.
export function setFields(raw: string, updates: Record<string, unknown>, eol: string): { raw: string; refused: Refusal[] } {
  const lines = linesOf(raw);
  const entries = scan(lines);
  const refused: Refusal[] = [];
  const replaced = new Map<number, { end: number; lines: string[] }>();
  const added: string[] = [];
  for (const [key, value] of Object.entries(updates)) {
    const found = entries.filter((e) => e.name === key);
    const opaque = entries.some((e) => e.name === null && new RegExp(`^["']?${key.replace(/[.-]/g, "\\$&")}["']?[ \t]*:`).test(lines[e.start]));
    const f = found.length === 1 ? classify(lines, found[0]) : null;
    if (!NAME.test(key)) refused.push({ key, reason: "bad_key" });
    else if (!isFieldValue(value)) refused.push({ key, reason: "bad_value" });
    else if (found.length > 1) refused.push({ key, reason: "duplicate_key" });
    else if (opaque || (found.length === 1 && !f)) refused.push({ key, reason: "not_simple" });
    else if (!f) added.push(`${key}:${Array.isArray(value) || value !== null ? " " + valueText(value, "plain") : ""}${eol}`);
    else if (JSON.stringify(f.value) !== JSON.stringify(value)) replaced.set(found[0].start, { end: found[0].end, lines: rewrite(lines, found[0], f, value, eol) });
  }
  if (refused.length) return { raw, refused };
  const out: string[] = [];
  for (let i = 0; i < lines.length; i++) {
    const r = replaced.get(i);
    if (r) { out.push(...r.lines); i = r.end; } else out.push(lines[i]);
  }
  return { raw: out.join("") + added.join(""), refused };
}
