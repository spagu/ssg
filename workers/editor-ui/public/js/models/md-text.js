// Small pieces of Markdown text the editing rules share: where a line starts
// and ends, which lines a selection covers, whether a position is inside a
// fenced code block, and how a list item or a quote line begins. Pure: a
// text and positions in, values out. A selection is {start, end} (offsets
// into the text, start <= end), as a <textarea> has them.

// LIST: indent, marker (- * + or 1. 1)), the spaces after it, a task box.
export const LIST = /^([ \t]*)([-*+]|\d{1,9}[.)])([ \t]+)(\[[ xX]\][ \t]+)?/;
// QUOTE: one or more ">" with their spaces, after an optional indent.
export const QUOTE = /^([ \t]*(?:>[ \t]?)+)/;
const FENCE = /^[ ]{0,3}(`{3,}|~{3,})/;

export const lineStart = (text, i) => text.lastIndexOf("\n", i - 1) + 1;
export function lineEnd(text, i) {
  const n = text.indexOf("\n", i);
  return n < 0 ? text.length : n;
}

// linesOf: the start and end of every line the selection touches. A
// selection that ends at the very start of a line does not touch that line.
export function linesOf(text, sel) {
  const last = sel.end > sel.start && sel.end === lineStart(text, sel.end) ? sel.end - 1 : sel.end;
  const first = lineStart(text, sel.start);
  const out = [{ start: first, end: lineEnd(text, first) }];
  while (out[out.length - 1].end < last) {
    const at = out[out.length - 1].end + 1;
    out.push({ start: at, end: lineEnd(text, at) });
  }
  return out;
}

// inFence: whether the line holding `pos` is inside a fenced code block
// (between an opening ``` or ~~~ line and its closing one).
export function inFence(text, pos) {
  let open = null;
  const upto = lineStart(text, pos);
  for (let at = 0; at < upto; at = lineEnd(text, at) + 1) {
    const m = FENCE.exec(text.slice(at, lineEnd(text, at)));
    if (!m) continue;
    if (open === null) open = m[1];
    else if (m[1][0] === open[0] && m[1].length >= open.length) open = null;
  }
  return open !== null;
}

// listItem: the parts of a list line, or null. width = marker + spaces, the
// column the item's text starts at relative to its indent.
export function listItem(line) {
  const m = LIST.exec(line);
  if (!m) return null;
  return { indent: m[1], marker: m[2], gap: m[3], task: m[4] ?? "", prefix: m[0], width: m[2].length + m[3].length, ordered: /\d/.test(m[2]) };
}

// replace: the text with [from, to) replaced, and the new selection.
export const replace = (text, from, to, insert, start, end = start) => ({ text: text.slice(0, from) + insert + text.slice(to), start, end });
