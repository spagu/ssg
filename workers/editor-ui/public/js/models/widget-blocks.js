// The built-in blocks of the Insert list (GO-174), as pure text commands
// (text, selection, …) → {text, start, end}, like md-commands.js. Each writes
// what ssg renders: goldmark tables and footnotes (always on), fenced code,
// ```mermaid with `mermaid: true`, $$ display math with `math: true`, and
// [embed]…[/embed], which ssg turns into a YouTube player — only for a
// YouTube address, so another address is refused here rather than left as
// text on the page.

import { block } from "./md-commands.js";
import { replace } from "./md-text.js";

export const MAX_ROWS = 20;
export const MAX_COLS = 10;
// LANG: a fence's info word — no spaces or backticks.
export const LANG = /^[A-Za-z0-9_+#.-]{0,32}$/;
const YOUTUBE = /^\s*(?:https?:\/\/)?(?:www\.)?(?:youtube\.com\/watch\?v=|youtu\.be\/)([a-zA-Z0-9_-]+)\s*$/;

const clamp = (n, max) => Math.min(max, Math.max(1, Math.trunc(Number(n)) || 1));

// tableText: a header row of "Column 1…n", then `rows` rows of cells.
export function tableText(rows, cols, { column, cell }) {
  const c = clamp(cols, MAX_COLS);
  const line = (cells) => `| ${cells.join(" | ")} |`;
  const head = Array.from({ length: c }, (_, i) => column(i + 1));
  const body = Array.from({ length: clamp(rows, MAX_ROWS) }, () => line(Array(c).fill(cell)));
  return [line(head), line(Array(c).fill("---")), ...body].join("\n");
}

// table: the table on lines of its own, the first header selected.
export function table(text, sel, rows, cols, words) {
  const t = tableText(rows, cols, words);
  return block(text, sel, t, [2, 2 + words.column(1).length]);
}

// fenced: ```info, the selection (or `sample`, selected) inside, ```.
export function fenced(text, sel, info, sample) {
  const inner = text.slice(sel.start, sel.end).replace(/\n$/, "") || sample;
  const head = "```" + info + "\n";
  return block(text, sel, head + inner + "\n```", [head.length, head.length + inner.length]);
}

// math: display math between $$ lines, the formula selected.
export function math(text, sel, sample) {
  const inner = text.slice(sel.start, sel.end).trim() || sample;
  return block(text, sel, `$$\n${inner}\n$$`, [3, 3 + inner.length]);
}

// nextFootnote: one more than the highest numbered footnote in the text.
export function nextFootnote(text) {
  const used = [...text.matchAll(/\[\^(\d{1,6})\]/g)].map((m) => Number(m[1]));
  return used.length ? Math.max(...used) + 1 : 1;
}

// footnote: the mark [^n] at the caret, its note at the end of the article.
export function footnote(text, sel, note) {
  const n = nextFootnote(text);
  const mark = `[^${n}]`;
  const withMark = text.slice(0, sel.end) + mark + text.slice(sel.end);
  // withMark ends in the mark or in text after it — never empty.
  const sep = withMark.endsWith("\n\n") ? "" : withMark.endsWith("\n") ? "\n" : "\n\n";
  const line = `[^${n}]: ${String(note).replace(/\s+/g, " ").trim()}`;
  const at = sel.end + mark.length;
  return replace(withMark, withMark.length, withMark.length, sep + line, at);
}

// embedUrl: the address as ssg's [embed] reads it, or null for one it would
// not turn into a player.
export function embedUrl(url) {
  const m = YOUTUBE.exec(String(url));
  return m ? `https://www.youtube.com/watch?v=${m[1]}` : null;
}

// embed: [embed]address[/embed] in a paragraph of its own.
export const embed = (text, sel, url) => block(text, sel, `[embed]${url}[/embed]`);

// component: a component call on a line of its own.
export const component = (text, sel, call) => block(text, sel, call);

// shortcode: {{name}} at the caret, in place of the selection.
export function shortcode(text, sel, name) {
  const s = `{{${name}}}`;
  return replace(text, sel.start, sel.end, s, sel.start + s.length);
}
