// The Markdown renderer: every documented feature, the documented
// non-features, and that hostile Markdown ends up inert after sanitizing.
import { test } from "node:test";
import assert from "node:assert/strict";
import { inline, markdownToHtml } from "../src/markdown.ts";
import { sanitize } from "../src/sanitize.ts";

const md = (s: string) => sanitize(markdownToHtml(s));

test("blocks: headings, paragraphs, rules, quotes, fenced code", () => {
  assert.equal(md("# One\n###### Six ##\n#nohash"), "<h1>One</h1>\n<h6>Six</h6>\n<p>#nohash</p>");
  assert.equal(md("#"), "<h1></h1>");
  assert.equal(md("first\nsecond\n\nthird"), "<p>first\nsecond</p>\n<p>third</p>");
  assert.equal(md("hard  \nbreak\\\nagain"), "<p>hard<br>\nbreak<br>\nagain</p>");
  assert.equal(md("a\r\n\r\n---\r\n* * *\n___"), "<p>a</p>\n<hr>\n<hr>\n<hr>");
  assert.equal(md("> quoted *x*\n> # inner\n\nafter"), "<blockquote><p>quoted <em>x</em></p>\n<h1>inner</h1></blockquote>\n<p>after</p>");
  assert.equal(md("```js\n<b>&</b>\n  indented\n```\nafter"), "<pre><code>&lt;b&gt;&amp;&lt;/b&gt;\n  indented</code></pre>\n<p>after</p>");
  assert.equal(md("~~~~\ncode\n~~~\nstill\n~~~~"), "<pre><code>code\n~~~\nstill</code></pre>");
  assert.equal(md("```\nunclosed"), "<pre><code>unclosed</code></pre>");
  assert.equal(md("para\n# heading right after"), "<p>para</p>\n<h1>heading right after</h1>");
});

test("lists: bullets, numbers, start, continuation, blank lines, end", () => {
  assert.equal(md("- a\n* b\n+ c"), "<ul><li>a</li><li>b</li><li>c</li></ul>");
  assert.equal(md("1. a\n2) b"), "<ol><li>a</li><li>b</li></ol>");
  assert.equal(md("3. a\n4. b"), '<ol start="3"><li>a</li><li>b</li></ol>');
  assert.equal(md("- a\n  more\n\n- b\n\nafter"), "<ul><li>a\nmore</li><li>b</li></ul>\n<p>after</p>");
  assert.equal(md("- a\n1. b"), "<ul><li>a</li></ul>\n<ol><li>b</li></ol>");
  assert.equal(md("- a\n\n"), "<ul><li>a</li></ul>");
  assert.equal(md("- a\nnot indented"), "<ul><li>a</li></ul>\n<p>not indented</p>");
});

test("inline: emphasis, code, links, images, autolinks, escapes", () => {
  assert.equal(inline("**s** __s__ *e* _e_ ~~d~~"), "<strong>s</strong> <strong>s</strong> <em>e</em> <em>e</em> <del>d</del>");
  assert.equal(inline("snake_case_word and 2 * 3 * 4"), "snake_case_word and 2 * 3 * 4");
  assert.equal(inline("`a_b *c*` ``x ` y``"), "<code>a_b *c*</code> <code>x ` y</code>");
  assert.equal(inline('[t *x*](https://example.com/a_b "T")'), '<a href="https://example.com/a_b" title="T">t <em>x</em></a>');
  assert.equal(inline("[`c`](https://example.com)"), '<a href="https://example.com"><code>c</code></a>');
  assert.equal(inline('![alt "q"](https://example.com/i.png)'), '<img src="https://example.com/i.png" alt="alt &quot;q&quot;">');
  assert.equal(inline("<https://example.com/x> <mailto:a@example.com>"), '<a href="https://example.com/x">https://example.com/x</a> <a href="mailto:a@example.com">mailto:a@example.com</a>');
  assert.equal(inline("see https://example.com/a_b."), 'see <a href="https://example.com/a_b">https://example.com/a_b</a>.');
  assert.equal(inline("\\*not em\\* \\_ \\\\"), "*not em* _ \\");
  assert.equal(inline("<b>raw</b> & \"q\""), "&lt;b&gt;raw&lt;/b&gt; &amp; &quot;q&quot;");
  assert.equal(md("nul\0char"), "<p>nulchar</p>");
});

test("hostile Markdown is inert after the sanitizer", () => {
  assert.equal(md("[x](javascript:alert)"), "<p>x</p>");
  assert.equal(md("![x](javascript:alert)"), "<p></p>");
  assert.equal(md("[x](http://example.com)"), "<p>x</p>");
  assert.equal(md("![x](data:image/png;base64,AAAA)"), "<p></p>");
  assert.equal(md("<script>alert(1)</script>"), "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>");
  // A quote in the title stays inside the (escaped) title attribute.
  assert.equal(md('[x](https://example.com "a\\" onmouseover=\\"alert(1)")'), '<p><a href="https://example.com/" title="a&quot; onmouseover=&quot;alert(1)">x</a></p>');
  assert.equal(md("<img src=x onerror=alert(1)>"), "<p>&lt;img src=x onerror=alert(1)&gt;</p>");
});
