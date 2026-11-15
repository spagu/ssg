// The Insert list's models (GO-174): component calls written from a form —
// exactly the shared samples ssg's own parser reads back (Go test
// internal/components/editor_samples_test.go) — refusals by name, the
// built-in blocks as text commands, the media library's labels, and the
// preview listing a call's values in its frame.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { findAll, text } from "../public/js/core/vnode.js";
import { NUMBER, callArgs, componentCall, defaultText, filterWidgets, initialValues, quoted } from "../public/js/models/widget-call.js";
import * as wb from "../public/js/models/widget-blocks.js";
import { libraryLabel, matchesImage } from "../public/js/models/images.js";
import { renderPreview } from "../public/js/models/preview.js";

type Comp = { name: string; props: { name: string; type: string; required?: boolean; default?: unknown; enum?: string[] }[] };
const samples = JSON.parse(readFileSync(new URL("./widget-samples.json", import.meta.url), "utf8")) as {
  components: Comp[];
  samples: { component: string; values: Record<string, unknown>; call: string }[];
  refused: { component: string; values: Record<string, unknown>; errors: Record<string, string> }[];
};
const comp = (name: string) => samples.components.find((c) => c.name === name)!;
const at = (text: string, start: number, end = start) => ({ text, start, end });

test("the panel writes exactly the shared samples ssg parses back", () => {
  assert.ok(samples.samples.length >= 8);
  for (const s of samples.samples) assert.deepEqual(componentCall(comp(s.component), s.values), { text: s.call }, s.call);
});

test("values the syntax cannot carry are refused by name, not altered", () => {
  for (const r of samples.refused) assert.deepEqual(componentCall(comp(r.component), r.values), { errors: r.errors }, JSON.stringify(r.values));
});

test("form values start at the defaults; quoting; numbers", () => {
  assert.deepEqual(initialValues(comp("card")), { caption: "", count: "3", featured: false, kind: "note", tags: "", title: "" });
  assert.deepEqual(initialValues(comp("lights")), { on: true });
  assert.equal(defaultText({ default: ["a", "b"] }), "a, b");
  assert.equal(defaultText({ default: null }), "");
  assert.deepEqual([quoted("a"), quoted('a"b'), quoted("a'b\"")], ['"a"', "'a\"b'", null]);
  for (const ok of ["3", "-2", "+1", "1.5", ".5", "1.", "1e3", "2E-2"]) assert.match(ok, NUMBER);
  for (const bad of ["", "1,5", "0x10", "Inf", "NaN", "1_000", "e3"]) assert.doesNotMatch(bad, NUMBER);
});

test("callArgs reads a call as ssg does; anything else has none", () => {
  assert.deepEqual(callArgs("{{< card title='It \"is\"' count=3 tags=\"a, b\" empty=\"\" >}}"), [["title", 'It "is"'], ["count", "3"], ["tags", "a, b"], ["empty", ""]]);
  assert.deepEqual(callArgs("{{< plain >}}"), []);
  assert.deepEqual(callArgs("{{% legacy x=1 %}}"), []);
});

test("filterWidgets: every word, any case, in name, title or description", () => {
  const items = [{ name: "youtube", title: "youtube", description: "A video" }, { name: "table", title: "Table" }, { name: "note" }];
  const label = (i: { title?: string; description?: string }) => `${i.title ?? ""} ${i.description ?? ""}`;
  assert.equal(filterWidgets(items, "", label).length, 3);
  assert.deepEqual(filterWidgets(items, "VIDEO you", label).map((i: { name: string }) => i.name), ["youtube"]);
  assert.deepEqual(filterWidgets(items, "zzz", label), []);
});

const words = { column: (n: number) => `C${n}`, cell: "." };
test("table: header, delimiter, rows; sizes kept in range; the first header selected", () => {
  assert.equal(wb.tableText(1, 2, words), "| C1 | C2 |\n| --- | --- |\n| . | . |");
  assert.equal(wb.tableText(0, "x", words).split("\n").length, 3);
  assert.equal(wb.tableText(99, 99, words).split("\n").length, wb.MAX_ROWS + 2);
  assert.equal(wb.tableText(1, 99, words).split("\n")[0].split("|").length - 2, wb.MAX_COLS);
  const r = wb.table("Intro", at("Intro", 5), 1, 1, words);
  assert.equal(r.text, "Intro\n\n| C1 |\n| --- |\n| . |");
  assert.equal(r.text.slice(r.start, r.end), "C1");
});

test("fenced code and math: the selection inside, else the sample selected", () => {
  const code = wb.fenced("", at("", 0), "go", "code");
  assert.deepEqual([code.text, code.text.slice(code.start, code.end)], ["```go\ncode\n```", "code"]);
  const sel = wb.fenced("x = 1\n", at("x = 1\n", 0, 6), "", "code");
  assert.equal(sel.text, "```\nx = 1\n```");
  const m = wb.math("a", at("a", 1), "E");
  assert.deepEqual([m.text, m.text.slice(m.start, m.end)], ["a\n\n$$\nE\n$$", "E"]);
  assert.equal(wb.math(" y ", at(" y ", 0, 3), "E").text, "$$\ny\n$$");
  assert.match("c++", wb.LANG);
  assert.doesNotMatch("a b", wb.LANG);
});

test("footnotes: the next free number, the mark at the caret, the note at the end", () => {
  assert.equal(wb.nextFootnote("no notes"), 1);
  assert.equal(wb.nextFootnote("a[^2] b[^7]\n\n[^2]: x"), 8);
  const r = wb.footnote("One. Two.", at("One. Two.", 4), " a\n note ");
  assert.equal(r.text, "One.[^1] Two.\n\n[^1]: a note");
  assert.equal(r.start, 8);
  assert.equal(wb.footnote("End\n", at("End\n", 3), "n").text, "End[^1]\n\n[^1]: n");
  assert.equal(wb.footnote("End\n\n", at("End\n\n", 5), "n").text, "End\n\n[^1]\n\n[^1]: n");
  assert.equal(wb.footnote("", at("", 0), "n").text, "[^1]\n\n[^1]: n");
  assert.equal(wb.footnote("a\n\n", at("a\n\n", 1), "n").text, "a[^1]\n\n[^1]: n");
});

test("embed: only addresses ssg turns into a player, written canonically", () => {
  assert.equal(wb.embedUrl(" https://www.youtube.com/watch?v=abc_D-1 "), "https://www.youtube.com/watch?v=abc_D-1");
  assert.equal(wb.embedUrl("youtu.be/xyz"), "https://www.youtube.com/watch?v=xyz");
  for (const bad of ["https://video.example.com/x", "https://www.youtube.com/watch?v=abc&t=3", ""]) assert.equal(wb.embedUrl(bad), null);
  assert.equal(wb.embed("", at("", 0), "https://www.youtube.com/watch?v=a").text, "[embed]https://www.youtube.com/watch?v=a[/embed]");
});

test("components on a line of their own; shortcodes at the caret", () => {
  assert.equal(wb.component("ab", at("ab", 1), "{{< x >}}").text, "a\n\n{{< x >}}\n\nb");
  const s = wb.shortcode("say  now", at("say  now", 4), "note");
  assert.deepEqual([s.text, s.start], ["say {{note}} now", 12]);
});

test("media library: file names, search by path", () => {
  assert.equal(libraryLabel("content/blog/media/2026/11/cat-1234abcd.png"), "cat-1234abcd.png");
  assert.ok(matchesImage({ path: "media/2026/Cat.png" }, " cat "));
  assert.ok(!matchesImage({ path: "media/dog.png" }, "cat"));
});

test("the preview frames a component call with its values, never running it", () => {
  const t = (k: string, v: Record<string, unknown> = {}) => `${k}${JSON.stringify(v)}`;
  const tree = renderPreview('{{< card title="Hi <b>" count=3 >}}\n\n{{< plain >}}', { t, images: new Map() });
  const dl = findAll({ tag: "root", props: {}, children: tree.filter(Boolean) }, (n: { tag?: string }) => n.tag === "dl");
  assert.equal(dl.length, 1);
  assert.equal(text(dl[0]), "titleHi <b>count3");
  assert.equal(findAll({ tag: "root", props: {}, children: tree.filter(Boolean) }, (n: { tag?: string }) => n.tag === "pre").length, 1, "a call without values shows its text");
});
