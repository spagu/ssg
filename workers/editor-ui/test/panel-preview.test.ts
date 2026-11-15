// GO-173: the preview's renderer (public/js/models/preview.js,
// preview-inline.js). The rule that matters most: whatever HTML an article
// holds never becomes live DOM — the tree has text where the markup was,
// links carry only safe addresses, images load only from this site or this
// draft. Then the Markdown subset it draws, case by case.
import { test } from "node:test";
import assert from "node:assert/strict";
import { find, findAll, render, text } from "../public/js/core/vnode.js";
import { renderPreview } from "../public/js/models/preview.js";
import { callAt, imageSource, inline, safeHref } from "../public/js/models/preview-inline.js";
import { fakeDoc, fakeText, hasTag } from "./panel-dom.ts";

type Node = { tag: string; props: Record<string, unknown>; children: (Node | string)[] };
const t = (key: string, vars: Record<string, unknown> = {}) => `${key}${Object.keys(vars).length ? JSON.stringify(vars) : ""}`;
const ctx = { t, images: new Map([["/media/2026/11/cat-1a2b3c4d.png", "blob:local-cat"]]) };
const md = (s: string, extra = {}) => ({ tag: "div", props: {}, children: renderPreview(s, { ...ctx, ...extra }).filter(Boolean) }) as Node;
const tags = (n: Node) => findAll(n, (x: Node) => typeof x === "object").map((x: Node) => x.tag);
const SAFE_TAGS = new Set(["div", "p", "h1", "h2", "h3", "h4", "h5", "h6", "strong", "em", "del", "code", "pre", "a", "img", "ul", "ol", "li", "input", "blockquote", "hr", "br", "span", "table", "thead", "tbody", "tr", "th", "td", "dl", "dt", "dd"]);
const SAFE_PROPS = new Set(["class", "role", "href", "target", "rel", "src", "alt", "loading", "type", "disabled", "checked", "aria-label", "start", "style", "data-lang"]);

const HOSTILE = [
  "<script>alert(1)</script>",
  "<img src=x onerror=alert(1)>",
  "text <b onclick=alert(1)>bold</b> and <iframe src=//evil.example.com></iframe>",
  "[click](javascript:alert(1)) [tab](java\tscript:alert(1)) [data](data:text/html,<script>alert(1)</script>) [vb](VBScript:x) [proto](//evil.example.com/x)",
  "![x](javascript:alert(1)) ![y](https://evil.example.com/a.png) ![z](data:image/svg+xml,<svg onload=alert(1)>) ![w](relative.png)",
  "<svg><script>alert(1)</script></svg>",
  "- <style>body{display:none}</style>\n> <a href=javascript:x>q</a>",
  "| <img src=x> | b |\n| --- | --- |\n| <script> | c |",
  "{{< youtube id=\"<script>\" >}}",
];

test("hostile HTML never becomes live DOM: only safe tags and attributes, unsafe addresses dropped, markup kept as text", () => {
  for (const src of HOSTILE) {
    const tree = md(src);
    for (const n of findAll(tree, (x: Node) => typeof x === "object") as Node[]) {
      assert.ok(SAFE_TAGS.has(n.tag), `${src}: <${n.tag}>`);
      for (const [k, v] of Object.entries(n.props)) {
        assert.ok(SAFE_PROPS.has(k), `${src}: attribute ${k}`);
        if (k === "href") assert.match(String(v), /^(https?:|mailto:|\/[^/]|#|[^:]*$)/i, `${src}: href ${v}`);
        if (k === "src") assert.match(String(v), /^(\/[^/]|blob:local)/, `${src}: src ${v}`);
      }
    }
  }
  const s = md(HOSTILE[0]);
  assert.match(text(s), /<script>alert\(1\)<\/script>/, "the markup is shown as its text");
  // Rendered into a (fake) document: the script is one text node, never an element.
  const doc = fakeDoc();
  const el = render(s, doc as never) as never;
  assert.ok(fakeText(el).includes("<script>alert(1)</script>"));
  assert.equal(hasTag(el, "script"), false);
  assert.equal(find(md(HOSTILE[2]), (x: Node) => x.tag === "b" || x.tag === "iframe"), null);
  assert.equal(findAll(md(HOSTILE[3]), (x: Node) => x.tag === "a").length, 0, "no unsafe link survives as a link");
});

test("safeHref and imageSource: the address rules", () => {
  assert.deepEqual(["https://example.com", "http://x.example.com", "mailto:a@example.com", "/about/", "#top", "page.html", " https://example.com/a "].map(safeHref), ["https://example.com", "http://x.example.com", "mailto:a@example.com", "/about/", "#top", "page.html", "https://example.com/a"]);
  assert.deepEqual(["javascript:x", "JavaScript:x", "java\nscript:x", "data:x", "vbscript:x", "//evil.example.com", "file:///etc"].map(safeHref), [null, null, null, null, null, null, null]);
  assert.deepEqual(imageSource("/media/2026/11/cat-1a2b3c4d.png", ctx.images), { src: "blob:local-cat" });
  assert.deepEqual(imageSource("/media/x.png", null), { src: "/media/x.png" });
  assert.deepEqual(imageSource("//cdn.example.com/x.png", null), { away: "elsewhere" });
  assert.deepEqual(imageSource("https://cdn.example.com/x.png", null), { away: "elsewhere" });
  assert.deepEqual(imageSource("x.png", null), { away: "relative" });
});

test("blocks: headings (ATX and setext), paragraphs, rules, quotes, code, tables, raw HTML and calls framed", () => {
  const doc = md("# One\n\nTitle two\n---\n\nTitle one\n===\n\n### Three ###\n\n***\n\n> quoted\n> > deeper\n\n```js\nlet a = '<b>';\n```\n\n~~~\nunclosed");
  assert.deepEqual(tags(doc).filter((x) => x !== "div"), ["h1", "h2", "h1", "h3", "hr", "blockquote", "p", "blockquote", "p", "pre", "code", "pre", "code"]);
  assert.equal(find(doc, (x: Node) => x.tag === "pre")!.props["data-lang"], "js");
  assert.equal(text(find(doc, (x: Node) => x.tag === "code")!), "let a = '<b>';");
  const tbl = md("| a | b | c | d |\n| :-- | :-: | --: | --- |\n| 1 | x \\| y |\n\nhead | only\n--- | ---");
  const cells = findAll(tbl, (x: Node) => x.tag === "td") as Node[];
  assert.deepEqual(cells.map(text), ["1", "x | y", "", ""]);
  assert.deepEqual((findAll(tbl, (x: Node) => x.tag === "th") as Node[]).slice(0, 4).map((c) => (c.props.style as Record<string, string> | undefined)?.["text-align"] ?? null), ["left", "center", "right", null]);
  assert.equal(findAll(tbl, (x: Node) => x.tag === "table").length, 2, "a table without body rows is a table too");
  const raw = md("<div class=\"x\">\n<p>hi</p>\n</div>\n\nafter\n\n<!-- note -->\n\n{{< youtube id=\"abc\" >}}\n\n{{% note %}}\n\n{{ promo }}");
  const frames = findAll(raw, (x: Node) => x.props.class === "pv-frame") as Node[];
  assert.deepEqual(frames.map((f) => text(f.children[0] as Node)), ["preview.html", "preview.html", 'preview.component{"name":"youtube"}', 'preview.component{"name":"note"}', 'preview.shortcode{"name":"promo"}']);
  assert.match(text(frames[0]), /<p>hi<\/p>/);
  assert.equal(text(find(raw, (x: Node) => x.tag === "p")!), "after");
  assert.equal(md("", { title: "A title" }).children.length, 1);
  assert.equal(text(md("x", { title: "A title" }).children[0] as Node), "A title");
});

test("lists: bullets, numbers from their start, tasks, nesting, tight and loose, lazy lines", () => {
  const doc = md("- a\n- b\n  - nested\n- [x] done\n- [ ] open\n\n3. three\n4. four\n\n1) other\n\n* star\n\n  more of star\n\n* second");
  const lists = findAll(doc, (x: Node) => x.tag === "ul" || x.tag === "ol") as Node[];
  assert.deepEqual(lists.map((l) => l.tag), ["ul", "ul", "ol", "ol", "ul"]);
  assert.equal(lists[2].props.start, 3);
  assert.equal(lists[3].props.start, undefined, "a list starting at 1 has no start");
  const boxes = findAll(doc, (x: Node) => x.tag === "input") as Node[];
  assert.deepEqual(boxes.map((b) => [b.props.checked, b.props["aria-label"]]), [[true, "preview.taskDone"], [false, "preview.taskOpen"]]);
  assert.equal(findAll(lists[0], (x: Node) => x.tag === "p").length, 0, "a tight list has no paragraphs");
  assert.equal(findAll(lists[4], (x: Node) => x.tag === "p").length, 3, "a loose list keeps them");
  const lazy = md("- item\ncontinued\n\nafter");
  assert.equal(text(find(lazy, (x: Node) => x.tag === "li")!), "item\ncontinued");
  const mixed = md("- a\n\n1. b");
  assert.deepEqual(tags(mixed).filter((x) => x === "ul" || x === "ol"), ["ul", "ol"]);
  const empty = md("-\n- x");
  assert.equal(findAll(empty, (x: Node) => x.tag === "li").length, 2);
  const stop = md("- a\n# heading");
  assert.deepEqual(tags(stop).filter((x) => x !== "div"), ["ul", "li", "h1"]);
});

test("inline: emphasis, code, links, images, breaks, escapes, calls", () => {
  const out = (s: string) => ({ tag: "p", props: {}, children: inline(s, ctx) }) as Node;
  const n = out("**b** __b__ *i* _i_ ~~d~~ `c` `` a`b `` [l](/x \"t\") <https://example.com> ![a](/media/2026/11/cat-1a2b3c4d.png) \\*no\\* a  \nb\\\nc");
  assert.deepEqual(tags(n).slice(1), ["strong", "strong", "em", "em", "del", "code", "code", "a", "a", "img", "br", "br"]);
  assert.equal(text(find(n, (x: Node) => x.tag === "code" && text(x).includes("`"))!), "a`b");
  assert.match(text(n), /\*no\*/);
  assert.equal(find(n, (x: Node) => x.tag === "img")!.props.src, "blob:local-cat");
  const plain = out("snake_case_name 2 * 3 * 4 _ open ** not");
  assert.deepEqual(tags(plain).slice(1), [], "intraword underscores and spaced stars are text");
  assert.equal(text(out("[unclosed](x and [no paren]")), "[unclosed](x and [no paren]");
  assert.equal(find(out("[a [nested] b](/x)"), (x: Node) => x.tag === "a")!.props.href, "/x");
  assert.equal(find(out("[a](<with space.md>)"), (x: Node) => x.tag === "a")!.props.href, "with space.md");
  assert.equal(text(out("[\\]](/x)")), "]");
  assert.equal(text(out("**a\\*b**")), "a*b");
  assert.deepEqual(tags(out("{{< x >}} and {{y}}")).slice(1), ["span", "span", "code", "span", "span", "code"]);
  assert.equal(text(out("![away](https://cdn.example.com/a.png)")), "preview.image.elsewhere away");
  assert.equal(text(out("![](x.png)")), "preview.image.relative x.png");
  assert.deepEqual(callAt("{{< /x >}}"), { kind: "component", name: "/x", raw: "{{< /x >}}" });
  assert.equal(callAt("{{ }}"), null);
  assert.equal(callAt("text"), null);
  // Edges: a delimiter at either end, an opener with no closer, a backslash last.
  assert.deepEqual(tags(out("_x_ and *y*")).slice(1), ["em", "em"]);
  assert.equal(text(out("ends with *")), "ends with *");
  assert.equal(text(out("*open")), "*open");
  assert.equal(text(out("*a\\*b*")), "a*b");
  assert.equal(text(out("_a_b_")), "a_b");
  assert.deepEqual(tags(out("*a **b** c*")).slice(1), ["em", "strong"]);
  assert.equal(text(out("a\\")), "a\\");
  assert.equal(text(out("trailing\\")), "trailing\\");
});

test("blocks at the end of the text: a list ending in a blank line, a pipe on the last line", () => {
  assert.equal(findAll(md("- a\n"), (x: Node) => x.tag === "li").length, 1);
  assert.deepEqual(tags(md("a | b")).filter((x) => x !== "div"), ["p"]);
  assert.equal(findAll(md("| a\n---"), (x: Node) => x.tag === "table").length, 1, "a delimiter row without pipes under a piped header");
});
