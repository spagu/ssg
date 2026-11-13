// The sanitizer against real bypass attempts. Every hostile input must come out
// without executable markup: no script/style/iframe/form/object, no on*
// attribute, no javascript:/data:/vbscript: URL, no http: link or image.
import { test } from "node:test";
import assert from "node:assert/strict";
import { safeUrl, sanitize, sanitizeHtml } from "../src/sanitize.ts";
import { decodeEntities, escapeHtml } from "../src/render-entities.ts";

// assertInert fails when the output could run anything in a mail client or a
// browser, whatever the input was.
function assertInert(out: string, input: string) {
  const tags = [...out.matchAll(/<\/?([a-z0-9]+)/gi)].map((m) => m[1].toLowerCase());
  const allowed = new Set("p br hr h1 h2 h3 h4 h5 h6 strong b em i u s del sup sub small span div blockquote pre code ul ol li a img table thead tbody tr th td".split(" "));
  for (const t of tags) assert.ok(allowed.has(t), `tag <${t}> from ${JSON.stringify(input)} → ${out}`);
  for (const attr of out.matchAll(/<[a-z0-9]+((?:\s+[a-z]+="[^"]*")*)\s*>/gi)) {
    for (const [, name, value] of attr[1].matchAll(/\s+([a-z]+)="([^"]*)"/g)) {
      assert.ok(["href", "src", "title", "alt", "width", "height", "colspan", "rowspan", "start"].includes(name), `attr ${name} in ${out}`);
      if (name === "href") assert.match(value, /^(https:\/\/|mailto:)/, out);
      if (name === "src") assert.match(value, /^https:\/\//, out);
    }
  }
  // Nothing outside a tag can be markup: every "<" left is an escaped one or a tag above.
  assert.doesNotMatch(out.replace(/<\/?[a-z0-9]+(\s+[a-z]+="[^"]*")*\s*>/gi, ""), /[<>]/, out);
}

const HOSTILE = [
  "<script>alert(1)</script>",
  "<SCRIPT SRC=https://example.com/x.js></SCRIPT>",
  "<ScRiPt>alert(1)</sCrIpT>",
  "<script>alert(1)",
  "<scr<script>ipt>alert(1)</script>",
  "<<script>script>alert(1)<</script>/script>",
  "<script/xss src=https://example.com/x.js></script>",
  '<img src=x onerror=alert(1)>',
  '<img src="https://example.com/a.png" onerror="alert(1)">',
  "<img src=https://example.com/a.png ONERROR=alert(1)>",
  '<img src="https://example.com/a.png"onerror="alert(1)">',
  "<img/src=https://example.com/a.png/onerror=alert(1)>",
  '<img src="javascript:alert(1)">',
  '<img src="data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=">',
  '<img src="http://example.com/a.png">',
  '<a href="javascript:alert(1)">x</a>',
  '<a href="JaVaScRiPt:alert(1)">x</a>',
  '<a href=" javascript:alert(1)">x</a>',
  '<a href="java\tscript:alert(1)">x</a>',
  '<a href="java&#x09;script:alert(1)">x</a>',
  '<a href="&#106;&#97;&#118;&#97;&#115;&#99;&#114;&#105;&#112;&#116;&#58;alert(1)">x</a>',
  '<a href="&#0000106&#0000097vascript:alert(1)">x</a>',
  '<a href="javascript&colon;alert(1)">x</a>',
  '<a href="vbscript:msgbox(1)">x</a>',
  '<a href="data:text/html,<script>alert(1)</script>">x</a>',
  '<a href="http://example.com">plain http</a>',
  '<a href="//example.com">protocol-relative</a>',
  '<a href="/relative">relative</a>',
  '<a href=javascript:alert(1)>unquoted</a>',
  "<a href='javascript:alert(1)'>single</a>",
  '<a href="https://example.com" onclick="alert(1)" onmouseover=alert(1)>ok</a>',
  '<a href="https://example.com" style="background:url(javascript:alert(1))">ok</a>',
  '<div style="x:expression(alert(1))">css</div>',
  "<iframe src=https://example.com></iframe>",
  "<iframe srcdoc='<script>alert(1)</script>'>",
  '<form action="https://example.com"><input name=p><button>go</button></form>',
  "<object data=https://example.com/x.swf></object>",
  "<embed src=https://example.com/x.swf>",
  "<svg onload=alert(1)><script>alert(1)</script></svg>",
  "<svg><a xlink:href=javascript:alert(1)><text>x</text></a></svg>",
  "<math><mtext><table><mglyph><style><img src=x onerror=alert(1)>",
  "<style>@import 'https://example.com/x.css';</style>",
  '<meta http-equiv="refresh" content="0;url=javascript:alert(1)">',
  '<link rel=stylesheet href=https://example.com/x.css>',
  '<base href="https://example.com/">',
  "<!--<script>alert(1)</script>-->",
  "<!-- unterminated comment <script>alert(1)</script>",
  "<![CDATA[<script>alert(1)</script>]]>",
  "<?xml version=1?><script>alert(1)</script>",
  "<textarea><script>alert(1)</script></textarea>",
  "<title><script>alert(1)</script></title>",
  "<noscript><p title=\"</noscript><img src=x onerror=alert(1)>\"></noscript>",
  '<p title="a"onmouseover="alert(1)">x</p>',
  '<p title="unterminated>x',
  "<p <script>alert(1)</script>>",
  "<a href=\"https://example.com\"><a href=\"javascript:alert(1)\">nested</a></a>",
  "<img src=`javascript:alert(1)`>",
  '<a href="https://example.com/&quot;onmouseover=&quot;alert(1)">quoted entity</a>',
  "<template><script>alert(1)</script></template>",
  "<details open ontoggle=alert(1)>x</details>",
  "<video><source onerror=alert(1)></video>",
  "<body onload=alert(1)>",
  "<marquee onstart=alert(1)>x</marquee>",
  "<isindex type=image src=1 onerror=alert(1)>",
  "<plaintext><script>alert(1)",
  "<x onclick=alert(1)>custom</x>",
  "<p>" + "<div>".repeat(200) + "deep" + "</div>".repeat(200) + "</p>",
];

test("hostile corpus: nothing executable survives", () => {
  for (const input of HOSTILE) {
    const out = sanitize(input);
    assertInert(out, input);
    assert.doesNotMatch(out, /javascript:|vbscript:|data:|<script|<style|<iframe|<form|<object|<svg|<math/i, `${input} → ${out}`);
    // An event handler would be an attribute; inside a quoted value (a URL
    // path like ".../onerror=alert(1)") the text is inert.
    assert.doesNotMatch(out.replace(/"[^"]*"/g, '""'), /\son[a-z]+=/i, `${input} → ${out}`);
  }
});

test("exact results: removed, unwrapped or escaped — never passed through", () => {
  const cases: [string, string][] = [
    ["<script>alert(1)</script>ok", "ok"],
    ["<script>alert(1)", ""],
    ["before<style>p{}</style>after", "beforeafter"],
    ['<a href="javascript:alert(1)">x</a>', "x"],
    ['<a href="https://example.com" onclick="alert(1)">x</a>', '<a href="https://example.com/">x</a>'],
    ['<a href="mailto:hello@example.com">mail</a>', '<a href="mailto:hello@example.com">mail</a>'],
    ['<img src="https://example.com/a.png" alt="A &amp; B" width="600" height="x">', '<img src="https://example.com/a.png" alt="A &amp; B" width="600">'],
    ['<img src="http://example.com/a.png">', ""],
    ["<scr<script>ipt>alert(1)</script>", "ipt&gt;alert(1)"],
    ["<div><span>kept</span></div>", "<div><span>kept</span></div>"],
    ["<font color=red>unwrapped</font>", "unwrapped"],
    ["<p>unclosed <b>bold", "<p>unclosed <b>bold</b></p>"],
    ["<b><i>mis</b>nested</i>", "<b><i>mis</i></b>nested"],
    ["stray </p> end", "stray  end"],
    ["a < b > c & d", "a &lt; b &gt; c &amp; d"],
    ["&lt;script&gt;", "&lt;script&gt;"],
    ["<input>text after a void", "text after a void"],
    ['<p title="unterminated>x', ""],
    ["ok<p ", "ok"],
    ["ok<a href= ", "ok"],
    ["ok<script>x</script", "ok"],
    ["ok<script>x</script ", "ok"],
    ["<!-- c -->x<!doctype html>y<?pi?>z", "xyz"],
    ["<br/><hr/>", "<br><hr>"],
    ['<td colspan="2" rowspan="x">c</td>', '<td colspan="2">c</td>'],
    ['<ol start="3" type="a"><li>x</li></ol>', '<ol start="3"><li>x</li></ol>'],
    ['<a href="https://example.com" href="javascript:alert(1)" title="t">dup</a>', '<a href="https://example.com/" title="t">dup</a>'],
    ['<a title="no href">text</a>', "text"],
    ['<a href="https://example.com"><a href="https://example.org">inner</a>tail</a>', '<a href="https://example.com/">inner</a>tail'],
    ["<P>Upper</P>", "<p>Upper</p>"],
    ["x <3 y", "x &lt;3 y"],
    ["<a href=https://example.com/a?b=1&amp;c=2>q</a>", '<a href="https://example.com/a?b=1&amp;c=2">q</a>'],
  ];
  for (const [input, want] of cases) assert.equal(sanitize(input), want, input);
});

test("nesting is capped: deep input is unwrapped, not recursed into", () => {
  const tree = sanitizeHtml("<div>".repeat(200) + "deep");
  let depth = 0;
  for (let n = tree[0]; n && n.type === "el"; n = n.children[0]) depth++;
  assert.equal(depth, 32);
  assert.match(sanitize("<div>".repeat(200) + "deep"), /deep/);
});

test("long titles are cut, numeric attributes bounded", () => {
  assert.equal(sanitize(`<a href="https://example.com" title="${"t".repeat(600)}">x</a>`), `<a href="https://example.com/" title="${"t".repeat(500)}">x</a>`);
  assert.equal(sanitize('<img src="https://example.com/a.png" width="99999">'), '<img src="https://example.com/a.png">');
});

test("safeUrl: decoded, parsed, scheme-checked", () => {
  assert.equal(safeUrl("https://example.com/a b", ["https:"]), "https://example.com/a%20b");
  assert.equal(safeUrl("&#104;ttps://example.com", ["https:"]), "https://example.com/");
  assert.equal(safeUrl("mailto:a@example.com", ["https:"]), null);
  assert.equal(safeUrl("not a url", ["https:"]), null);
  assert.equal(safeUrl("\u0000javascript:alert(1)", ["https:", "mailto:"]), null);
});

test("entities: numeric with and without ;, named, invalid code points", () => {
  assert.equal(decodeEntities("&#106;&#x61;&#X76;&#97"), "java");
  assert.equal(decodeEntities("&AMP;&Colon;&nbsp;&copy;&bogus;&#0;&#xD800;&#x110000;"), "&: ©&bogus;���");
  assert.equal(decodeEntities("AT&T"), "AT&T");
  assert.equal(escapeHtml(`<a href="x" title='y'>&</a>`), "&lt;a href=&quot;x&quot; title=&#39;y&#39;&gt;&amp;&lt;/a&gt;");
});
