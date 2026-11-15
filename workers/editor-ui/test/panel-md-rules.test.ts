// GO-173: the Markdown-following rules of the editor (public/js/models/
// md-rules.js, md-text.js) as a table of cases. In a case, "|" is the
// cursor and "<|" … "|>" a selection; null means the rule leaves the key to
// the browser.
import { test } from "node:test";
import assert from "node:assert/strict";
import { continueOnEnter, indent, pasteLink, wrapSelection } from "../public/js/models/md-rules.js";
import { inFence, lineEnd, lineStart, linesOf, listItem } from "../public/js/models/md-text.js";
import { at, show } from "./panel-sel.ts";

type Sel = { text: string; start: number; end: number };
const run = (fn: (text: string, sel: Sel) => Sel | null, input: string) => {
  const s = at(input);
  return show(fn(s.text, s));
};

const ENTER: [string, string | null][] = [
  ["- one|", "- one\n- |"],
  ["* one|", "* one\n* |"],
  ["  + nested|", "  + nested\n  + |"],
  ["1. first|", "1. first\n2. |"],
  ["9) ninth|", "9) ninth\n10) |"],
  ["- [x] done|", "- [x] done\n- [ ] |"],
  ["- [ ] open|", "- [ ] open\n- [ ] |"],
  ["- spl|it", "- spl\n- |it"],
  ["-   wide|", "-   wide\n-   |"],
  ["- one\n- |", "- one\n\n|"],
  ["1. one\n2. |", "1. one\n\n|"],
  ["- [ ] |", "\n|"],
  ["> said|", "> said\n> |"],
  ["> > deep|", "> > deep\n> > |"],
  ["> said\n> |", "> said\n\n|"],
  ["plain|", null],
  ["|- one", null],
  ["- <|one|>", null],
  ["```\n- in code|", null],
  ["```\n- code\n```\n- after|", "```\n- code\n```\n- after\n- |"],
  ["~~~~\n```\n- still code|", null],
  ["-not a list|", null],
];

test("Enter continues lists and quotes; an empty item or quote line ends them; code and selections are left alone", () => {
  for (const [input, want] of ENTER) assert.equal(run(continueOnEnter, input), want, JSON.stringify(input));
});

const TAB: [string, boolean, string | null][] = [
  ["- one\n- tw|o", false, "- one\n  - tw|o"],
  ["1. one\n2. tw|o", false, "1. one\n   2. tw|o"],
  ["10. one\n11. tw|o", false, "10. one\n    11. tw|o"],
  ["- on|ly", false, "  - on|ly"],
  ["- one\n  - tw|o", true, "- one\n- tw|o"],
  ["- one\n  - two\n    - thr|ee", true, "- one\n  - two\n  - thr|ee"],
  ["- to|p", true, null],
  ["plain| text", false, null],
  ["plain| text", true, null],
  ["- one\n<|- two\n- three|>", false, "- one\n<|  - two\n  - three|>"],
  ["- one\n- t<|wo\n- thr|>ee", false, "- one\n  - t<|wo\n  - thr|>ee"],
  ["- one\n<|- two\n\n- three|>", false, "- one\n<|  - two\n\n  - three|>"],
  ["<|- one\nplain|>", false, null],
  ["- one\n<|- two\n|>after", false, "- one\n<|  - two\n|>after"],
];

test("Tab and Shift+Tab move list items a level; anywhere else Tab is the browser's (no keyboard trap)", () => {
  for (const [input, out, want] of TAB) assert.equal(run((t, s) => indent(t, s, out), input), want, JSON.stringify([input, out]));
  assert.equal(run((t, s) => indent(t, s), "- one\n- tw|o"), "- one\n  - tw|o", "outdent defaults to false");
});

test("a character typed over a selection wraps it; nothing else is wrapped", () => {
  const cases: [string, string, string | null][] = [
    ["a <|word|> b", "*", "a *<|word|>* b"],
    ["a <|word|> b", "_", "a _<|word|>_ b"],
    ["<|x|>", "`", "`<|x|>`"],
    ["<|x|>", "(", "(<|x|>)"],
    ["<|x|>", "[", "[<|x|>]"],
    ["<|x|>", "{", "{<|x|>}"],
    ["<|x|>", '"', '"<|x|>"'],
    ["<|x|>", "~", "~<|x|>~"],
    ["<|x|>", "'", null],
    ["<|x|>", "a", null],
    ["x|", "*", null],
    ["<|x|>", "toString", null],
  ];
  for (const [input, ch, want] of cases) assert.equal(run((t, s) => wrapSelection(t, s, ch), input), want, JSON.stringify([input, ch]));
});

test("an address pasted over selected words makes a link; otherwise the paste is the browser's", () => {
  const cases: [string, string, string | null][] = [
    ["see <|the docs|> now", "https://example.com/docs", "see [the docs](https://example.com/docs)| now"],
    ["<|mail me|>", " mailto:a@example.com ", "[mail me](mailto:a@example.com)|"],
    ["<|x|>", "http://example.com", "[x](http://example.com)|"],
    ["here|", "https://example.com", null],
    ["<|x|>", "not a url", null],
    ["<|x|>", "https://example.com/a b", null],
    ["<|x|>", "javascript:alert(1)", null],
    ["<|https://example.com/old|>", "https://example.com/new", null],
    ["<|two\nlines|>", "https://example.com", null],
  ];
  for (const [input, pasted, want] of cases) assert.equal(run((t, s) => pasteLink(t, s, pasted), input), want, JSON.stringify([input, pasted]));
});

test("md-text: lines, the lines a selection touches, fences, list items", () => {
  assert.deepEqual([lineStart("ab\ncd", 4), lineEnd("ab\ncd", 4), lineEnd("ab\ncd", 1), lineStart("ab", 0)], [3, 5, 2, 0]);
  assert.deepEqual(linesOf("", { start: 0, end: 0 }), [{ start: 0, end: 0 }]);
  assert.deepEqual(linesOf("a\nb\nc", { start: 0, end: 5 }), [{ start: 0, end: 1 }, { start: 2, end: 3 }, { start: 4, end: 5 }]);
  assert.deepEqual(linesOf("a\nb\nc", { start: 0, end: 2 }), [{ start: 0, end: 1 }], "a selection ending at a line start leaves that line out");
  assert.deepEqual(linesOf("a\n", { start: 2, end: 2 }), [{ start: 2, end: 2 }]);
  assert.equal(inFence("```\ncode\n```\ntext", 14), false);
  assert.equal(inFence("```js\ncode", 7), true);
  assert.equal(inFence("````\n```\nx", 9), true, "a shorter fence does not close a longer one");
  assert.equal(inFence("~~~\n```\nx", 8), true, "a different fence does not close");
  assert.deepEqual(listItem("  1) [x] task"), { indent: "  ", marker: "1)", gap: " ", task: "[x] ", prefix: "  1) [x] ", width: 3, ordered: true });
  assert.equal(listItem("text"), null);
});
