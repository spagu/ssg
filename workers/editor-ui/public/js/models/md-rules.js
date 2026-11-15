// The editor follows the Markdown being typed (GO-173): each rule is a pure
// function (text, selection) → {text, start, end}, or null when the rule
// does not apply and the browser should do what it always does. The view
// calls them from the <textarea>'s key and paste events; the tests run them
// on a table of cases without a browser.
//
//   continueOnEnter   Enter in a list item starts the next item (the next
//                     number, an empty task box); in a quote, the next quote
//                     line. Enter on an empty item or quote line ends it
//                     (with a blank line, so what follows is a new paragraph).
//   indent            Tab / Shift+Tab on list items moves them one level in
//                     or out. Anywhere else Tab is left alone, so it still
//                     moves the focus (no keyboard trap, WCAG 2.1.2).
//   wrapSelection     typing * _ ` ~ " ( [ { over a selection wraps it.
//   pasteLink         pasting an address over selected words makes a link.

import { QUOTE, inFence, lineEnd, lineStart, linesOf, listItem, replace } from "./md-text.js";

export function continueOnEnter(text, sel) {
  if (sel.start !== sel.end || inFence(text, sel.start)) return null;
  const pos = sel.start;
  const ls = lineStart(text, pos);
  const le = lineEnd(text, pos);
  const line = text.slice(ls, le);
  const item = listItem(line);
  if (item && pos >= ls + item.prefix.length) {
    // An empty item ends the list: its marker goes, and a blank line follows
    // so the next paragraph is not read as part of the last item.
    if (line.slice(item.prefix.length).trim() === "") return replace(text, ls, le, "\n", ls + 1);
    const marker = item.ordered ? `${Number.parseInt(item.marker, 10) + 1}${item.marker.slice(-1)}` : item.marker;
    const next = `\n${item.indent}${marker}${item.gap}${item.task ? "[ ] " : ""}`;
    return replace(text, pos, pos, next, pos + next.length);
  }
  const quote = QUOTE.exec(line);
  if (quote && pos >= ls + quote[1].length) {
    if (line.slice(quote[1].length).trim() === "") return replace(text, ls, le, "\n", ls + 1);
    const next = `\n${quote[1]}`;
    return replace(text, pos, pos, next, pos + next.length);
  }
  return null;
}

// parentIndent: the indent of the nearest list item above `at` whose indent
// is less than `indent` (the item an outdented line rejoins), else "".
function parentIndent(text, at, indent) {
  for (let ls = at; ls > 0; ) {
    ls = lineStart(text, ls - 1);
    const item = listItem(text.slice(ls, lineEnd(text, ls)));
    if (item && item.indent.length < indent.length) return item.indent;
  }
  return "";
}

// stepIn: how far an item moves in — the text column of the item above it
// at the same level (CommonMark nests a list under its parent's text), or
// its own marker width when there is none.
function stepIn(text, at, item) {
  if (at > 0) {
    const ls = lineStart(text, at - 1);
    const above = listItem(text.slice(ls, lineEnd(text, ls)));
    if (above && above.indent === item.indent) return above.width;
  }
  return item.width;
}

export function indent(text, sel, outdent = false) {
  const lines = linesOf(text, sel).map((l) => ({ ...l, item: listItem(text.slice(l.start, l.end)) }));
  if (!lines.some((l) => l.item) || lines.some((l) => !l.item && l.end > l.start)) return null;
  let out = text;
  let shift = 0;
  let start = sel.start;
  let end = sel.end;
  for (const l of lines) {
    if (!l.item) continue;
    const at = l.start + shift;
    const was = l.item.indent;
    const now = outdent ? parentIndent(out, at, was) : was + " ".repeat(stepIn(out, at, l.item));
    if (now === was) continue;
    out = out.slice(0, at) + now + out.slice(at + was.length);
    const delta = now.length - was.length;
    if (sel.start > l.start) start = Math.max(at, start + delta);
    end = Math.max(at, end + delta);
    shift += delta;
  }
  return out === text ? null : { text: out, start, end };
}

const PAIRS = { "*": "*", _: "_", "`": "`", "~": "~", '"': '"', "(": ")", "[": "]", "{": "}" };

export function wrapSelection(text, sel, ch) {
  if (sel.start === sel.end || !Object.hasOwn(PAIRS, ch)) return null;
  return replace(text, sel.start, sel.end, ch + text.slice(sel.start, sel.end) + PAIRS[ch], sel.start + 1, sel.end + 1);
}

const URL_ONLY = /^(?:https?:\/\/|mailto:)[^\s<>()]+$/i;

export function pasteLink(text, sel, pasted) {
  const url = String(pasted).trim();
  const words = text.slice(sel.start, sel.end);
  if (sel.start === sel.end || !URL_ONLY.test(url) || URL_ONLY.test(words.trim()) || words.includes("\n")) return null;
  const link = `[${words}](${url})`;
  return replace(text, sel.start, sel.end, link, sel.start + link.length);
}
