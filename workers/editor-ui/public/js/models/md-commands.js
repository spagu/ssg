// What the editor's buttons do to the Markdown (GO-173): each command is a
// pure function (text, selection, …) → {text, start, end}. A button never
// guesses: bold on bold text takes the bold off again, a heading button on
// a heading of that level makes it a paragraph, a list button on a list
// takes the markers off. Placeholders ("bold text") come from the caller in
// the panel's language and are left selected, so typing replaces them.

import { lineEnd, lineStart, linesOf, listItem, replace } from "./md-text.js";

// toggleInline wraps the selection in `mark` (** bold, _ italic, ` code), or
// unwraps it when the marks are already there — around the selection or as
// its first and last characters.
export function toggleInline(text, sel, mark, placeholder) {
  const { start, end } = sel;
  const n = mark.length;
  const inner = text.slice(start, end);
  if (text.slice(start - n, start) === mark && text.slice(end, end + n) === mark) {
    return replace(text, start - n, end + n, inner, start - n, end - n);
  }
  if (inner.length >= 2 * n && inner.startsWith(mark) && inner.endsWith(mark)) {
    return replace(text, start, end, inner.slice(n, -n), start, end - 2 * n);
  }
  const words = inner || placeholder;
  return replace(text, start, end, mark + words + mark, start + n, start + n + words.length);
}

// heading makes the selection's first line a heading of `level` (2-4 on the
// buttons), or a paragraph when it already is one of that level.
export function heading(text, sel, level) {
  const ls = lineStart(text, sel.start);
  const le = lineEnd(text, ls);
  const line = text.slice(ls, le);
  const old = /^#{1,6}[ \t]+/.exec(line)?.[0] ?? "";
  const same = old.trim() === "#".repeat(level);
  const now = same ? "" : `${"#".repeat(level)} `;
  const delta = now.length - old.length;
  const at = (p) => (p >= ls + old.length ? p + delta : Math.min(p, ls + now.length));
  return { text: text.slice(0, ls) + now + text.slice(ls + old.length), start: at(sel.start), end: Math.max(at(sel.end), at(sel.start)) };
}

const PREFIX = { bullet: () => "- ", task: () => "- [ ] ", number: (i) => `${i + 1}. `, quote: () => "> " };
const HAS = { bullet: (l) => listItem(l) && !listItem(l).ordered && !listItem(l).task, task: (l) => !!listItem(l)?.task, number: (l) => !!listItem(l)?.ordered, quote: (l) => /^[ \t]*>/.test(l) };
// strip: the line without its list marker or quote mark (indent kept).
const strip = (line) => {
  const item = listItem(line);
  if (item) return item.indent + line.slice(item.prefix.length);
  return line.replace(/^([ \t]*)>[ \t]?/, "$1");
};

// linePrefix turns the selected lines into a bullet, task or numbered list
// or a quote — or, when every non-empty line already is one, back into
// plain lines. Empty lines inside a longer selection are left alone.
export function linePrefix(text, sel, kind) {
  const lines = linesOf(text, sel).map((l) => text.slice(l.start, l.end));
  const from = lineStart(text, sel.start);
  const to = from + lines.join("\n").length;
  const filled = lines.filter((l) => l.trim() !== "");
  const off = filled.length > 0 && filled.every((l) => HAS[kind](l));
  let i = 0;
  const out = lines.map((l) => {
    if (l.trim() === "" && lines.length > 1) return l;
    const bare = strip(l);
    if (off) return bare;
    const indent = /^[ \t]*/.exec(bare)[0];
    return indent + PREFIX[kind](i++) + bare.slice(indent.length);
  });
  const joined = out.join("\n");
  const collapsed = sel.start === sel.end;
  return replace(text, from, to, joined, collapsed ? from + out[0].length : from, from + joined.length);
}

// block puts `insert` on lines of its own, a blank line before and after
// (none at the very start or end of the text), in place of the selection.
// `pick` = [from, to] inside `insert` to select afterwards (default: after it).
export function block(text, sel, insert, pick = null) {
  const before = text.slice(0, sel.start);
  const after = text.slice(sel.end);
  const lead = before === "" || before.endsWith("\n\n") ? "" : before.endsWith("\n") ? "\n" : "\n\n";
  const tail = after === "" || after.startsWith("\n\n") ? "" : after.startsWith("\n") ? "\n" : "\n\n";
  const at = sel.start + lead.length;
  const [a, b] = pick ?? [insert.length, insert.length];
  return replace(text, sel.start, sel.end, lead + insert + tail, at + a, at + b);
}

// codeBlock fences the selected lines (or an empty block for typing into).
export function codeBlock(text, sel) {
  const words = text.slice(sel.start, sel.end).replace(/\n$/, "");
  return block(text, sel, "```\n" + words + "\n```", [4, 4 + words.length]);
}

// table inserts a two-column table; the first header is selected.
export function table(text, sel, [one, two, cell]) {
  const t = `| ${one} | ${two} |\n| --- | --- |\n| ${cell} | ${cell} |`;
  return block(text, sel, t, [2, 2 + one.length]);
}

export const rule = (text, sel) => block(text, sel, "---");

const URLISH = /^(?:https?:\/\/|mailto:|\/)\S*$/i;
// link: [selected words](https://) with the address selected; a selected
// address becomes [placeholder](address) with the words selected.
export function link(text, sel, placeholder) {
  const words = text.slice(sel.start, sel.end);
  if (words && URLISH.test(words.trim())) {
    const s = `[${placeholder}](${words.trim()})`;
    return replace(text, sel.start, sel.end, s, sel.start + 1, sel.start + 1 + placeholder.length);
  }
  const label = words || placeholder;
  const s = `[${label}](https://)`;
  const urlAt = sel.start + label.length + 3;
  return replace(text, sel.start, sel.end, s, urlAt, urlAt + "https://".length);
}

// escapeAlt: alt text that cannot end the ![…] early or break the line.
export const escapeAlt = (alt) => String(alt).replace(/\s+/g, " ").trim().replace(/[\\[\]]/g, "\\$&");

// image: ![alt](url) in a paragraph of its own; the cursor after it.
export const image = (text, sel, alt, url) => block(text, sel, `![${escapeAlt(alt)}](${url})`);
