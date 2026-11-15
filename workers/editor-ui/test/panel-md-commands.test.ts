// GO-173: what the editor's buttons do (public/js/models/md-commands.js), the
// one history behind Undo/Redo (history.js) and the shortcut table
// (shortcuts.js). Cases use the notation of panel-md-rules.test.ts.
import { test } from "node:test";
import assert from "node:assert/strict";
import * as cmd from "../public/js/models/md-commands.js";
import { LIMIT, WINDOW_MS, createHistory } from "../public/js/models/history.js";
import { HELP_ONLY, SHORTCUTS, commandFor, isMac, keysOf } from "../public/js/models/shortcuts.js";
import { at, show } from "./panel-sel.ts";

type Sel = { text: string; start: number; end: number };
const run = (fn: (text: string, sel: Sel) => Sel, input: string) => {
  const s = at(input);
  return show(fn(s.text, s));
};

test("bold, italic and code wrap the selection, or a selected placeholder, and come off again", () => {
  const bold = (t: string, s: Sel) => cmd.toggleInline(t, s, "**", "bold text");
  const cases: [typeof bold, string, string][] = [
    [bold, "a <|word|> b", "a **<|word|>** b"],
    [bold, "a **<|word|>** b", "a <|word|> b"],
    [bold, "a <|**word**|> b", "a <|word|> b"],
    [bold, "a | b", "a **<|bold text|>** b"],
    [bold, "<|**|>", "**<|**|>**"],
    [(t, s) => cmd.toggleInline(t, s, "_", "it"), "x <|y|>", "x _<|y|>_"],
    [(t, s) => cmd.toggleInline(t, s, "`", "c"), "`<|y|>`", "<|y|>"],
  ];
  for (const [fn, input, want] of cases) assert.equal(run(fn, input), want, input);
});

test("heading buttons set, change and take off a heading on the selection's line", () => {
  const cases: [string, number, string][] = [
    ["Tit|le", 2, "## Tit|le"],
    ["## Tit|le", 3, "### Tit|le"],
    ["### Tit|le", 3, "Tit|le"],
    ["# |Title", 2, "## |Title"],
    ["a\nTi<|tle|>\nb", 4, "a\n#### Ti<|tle|>\nb"],
    ["|## Title", 2, "|Title"],
    ["<|## Ti|>tle", 2, "<|Ti|>tle"],
    ["|", 2, "## |"],
  ];
  for (const [input, level, want] of cases) assert.equal(run((t, s) => cmd.heading(t, s, level), input), want, input);
});

test("list and quote buttons mark the selected lines, switch kinds, and unmark", () => {
  const cases: [string, string, string][] = [
    ["one|", "bullet", "- one|"],
    ["<|one\ntwo|>", "bullet", "<|- one\n- two|>"],
    ["<|one\n\ntwo|>", "number", "<|1. one\n\n2. two|>"],
    ["<|- one\n- two|>", "bullet", "<|one\ntwo|>"],
    ["<|- one\n- two|>", "number", "<|1. one\n2. two|>"],
    ["<|1. one\n2. two|>", "number", "<|one\ntwo|>"],
    ["<|- [ ] one\n- [x] two|>", "task", "<|one\ntwo|>"],
    ["<|one|>", "task", "<|- [ ] one|>"],
    ["- [ ] one|", "bullet", "- one|"],
    ["  indented|", "quote", "  > indented|"],
    ["> quoted|", "quote", "quoted|"],
    ["|", "bullet", "- |"],
  ];
  for (const [input, kind, want] of cases) assert.equal(run((t, s) => cmd.linePrefix(t, s, kind), input), want, `${kind}: ${input}`);
});

test("blocks sit on lines of their own with blank lines around: code block, table, rule, image", () => {
  assert.equal(run((t, s) => cmd.codeBlock(t, s), "a\n<|x = 1|>\nb"), "a\n\n```\n<|x = 1|>\n```\n\nb");
  assert.equal(run((t, s) => cmd.codeBlock(t, s), "|"), "```\n|\n```");
  assert.equal(run((t, s) => cmd.codeBlock(t, s), "<|line\n|>"), "```\n<|line|>\n```");
  assert.equal(run((t, s) => cmd.table(t, s, ["A", "B", "."]), "text|"), "text\n\n| <|A|> | B |\n| --- | --- |\n| . | . |");
  assert.equal(run((t, s) => cmd.rule(t, s), "a\n\n|\n\nb"), "a\n\n---|\n\nb");
  assert.equal(run((t, s) => cmd.rule(t, s), "a\n|b"), "a\n\n---|\n\nb");
  assert.equal(run((t, s) => cmd.image(t, s, "A [cat]\non a mat", "/media/c.png"), "x|"), "x\n\n![A \\[cat\\] on a mat](/media/c.png)|");
  assert.equal(cmd.escapeAlt("  back\\slash  "), "back\\\\slash");
});

test("link: the selection becomes the text with the address selected; a selected address becomes the target", () => {
  assert.equal(run((t, s) => cmd.link(t, s, "text"), "see <|docs|>"), "see [docs](<|https://|>)");
  assert.equal(run((t, s) => cmd.link(t, s, "text"), "|"), "[text](<|https://|>)");
  assert.equal(run((t, s) => cmd.link(t, s, "text"), "<|https://example.com/a|>"), "[<|text|>](https://example.com/a)");
  assert.equal(run((t, s) => cmd.link(t, s, "text"), "<|/about/|>"), "[<|text|>](/about/)");
});

test("history: commands are steps, typing is one step per burst, undo and redo walk them", () => {
  const s = (text: string) => ({ text, start: text.length, end: text.length });
  const h = createHistory(s(""));
  assert.equal(h.undo(), null);
  assert.equal(h.canUndo(), false);
  h.record(s("a"), 1000, true);
  h.record(s("ab"), 1000 + WINDOW_MS - 1, true);
  h.record(s("abc"), 1000 + 3 * WINDOW_MS, true);
  h.record(s("**abc**"), 1000 + 3 * WINDOW_MS + 1);
  assert.deepEqual([h.undo(), h.undo(), h.undo(), h.undo()], [s("abc"), s("ab"), s(""), null]);
  assert.equal(h.canRedo(), true);
  assert.deepEqual(h.redo(), s("ab"));
  h.record(s("abX"), 1000 + 3 * WINDOW_MS + 2, true);
  assert.equal(h.redo(), null, "a new change drops what could be redone");
  assert.deepEqual(h.undo(), s("ab"), "typing right after a redo did not merge into the state it brought back");
  const small = createHistory(s(""), 3);
  for (const t of ["a", "b", "c", "d"]) small.record(s(t), 0);
  assert.deepEqual([small.undo(), small.undo(), small.undo()], [s("c"), s("b"), null], "the oldest step falls off at the limit");
  assert.ok(LIMIT >= 100);
});

test("shortcuts: Mod is Ctrl or Command; AltGr never; Shift and Alt must match; names for people", () => {
  const key = (o: Record<string, unknown>) => ({ key: "", code: "", ctrlKey: false, metaKey: false, shiftKey: false, altKey: false, altGraph: false, ...o });
  assert.equal(commandFor(key({ key: "b", ctrlKey: true }), false), "bold");
  assert.equal(commandFor(key({ key: "B", metaKey: true }), true), "bold");
  assert.equal(commandFor(key({ key: "b", metaKey: true }), false), null);
  assert.equal(commandFor(key({ key: "b", ctrlKey: true, metaKey: true }), true), null);
  assert.equal(commandFor(key({ key: "b" }), false), null);
  assert.equal(commandFor(key({ key: "Z", ctrlKey: true, shiftKey: true }), false), "redo");
  assert.equal(commandFor(key({ key: "z", ctrlKey: true }), false), "undo");
  assert.equal(commandFor(key({ key: "y", ctrlKey: true }), false), "redo");
  assert.equal(commandFor(key({ key: "@", code: "Digit2", ctrlKey: true, altKey: true }), false), "h2");
  assert.equal(commandFor(key({ key: "²", code: "Digit2", ctrlKey: true, altKey: true, altGraph: true }), false), null, "AltGr types a character");
  assert.equal(commandFor(key({ key: "*", code: "Digit8", ctrlKey: true, shiftKey: true }), false), "bullet");
  assert.equal(commandFor(key({ key: ">", code: "Period", ctrlKey: true, shiftKey: true }), false), "quote");
  assert.equal(commandFor(key({ key: "q", ctrlKey: true }), false), null);
  assert.equal(commandFor({ code: "x", ctrlKey: true } as never, false), null, "no key at all");
  assert.deepEqual([isMac("MacIntel"), isMac("iPhone"), isMac("Win32"), isMac(undefined)], [true, true, false, false]);
  assert.deepEqual([keysOf("bold", false), keysOf("h2", false), keysOf("bullet", false), keysOf("quote", false), keysOf("codeBlock", false)], ["Ctrl+B", "Ctrl+Alt+2", "Ctrl+Shift+8", "Ctrl+Shift+.", "Ctrl+Alt+C"]);
  assert.deepEqual([keysOf("bold", true), keysOf("h2", true), keysOf("bullet", true)], ["⌘B", "⌥⌘2", "⇧⌘8"]);
  assert.equal(keysOf("redo", false), "Ctrl+Shift+Z", "the first shortcut of a command is the one shown");
  assert.equal(keysOf("table", false), "", "no shortcut");
  assert.ok(SHORTCUTS.length > 10 && HELP_ONLY.every(([k, keys]) => k && keys));
});
