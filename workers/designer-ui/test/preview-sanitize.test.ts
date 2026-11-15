// The preview's sanitizer and tokenizer (GO-188) against hostile HTML: no
// <script>, no on* attribute and no javascript:/vbscript: URL may survive,
// however the page spells them; what makes the page look like itself stays.
import { test } from "node:test";
import assert from "node:assert/strict";
import { tokenize } from "../src/html-tokens.ts";
import { normalized, safeAttr, sanitize } from "../src/preview-sanitize.ts";

const PAGE = { baseHref: "https://example.com/blog/post/", scheme: null, css: ":root:root:root:root{--x:#fff}" } as const;

// HOSTILE: one vector per line, the usual families - script elements,
// handlers, script URLs in every attribute that takes one, entity and
// whitespace obfuscation, parser differentials (svg/style, comments, raw-text
// elements, unclosed tags), meta refresh, frames, base hijack.
const HOSTILE = [
  "<script>alert(1)</script>",
  "<SCRIPT SRC=//example.org/x.js></SCRIPT>",
  "<script/src=data:,alert(1)></script>",
  "<scr<script>ipt>alert(1)</script>",
  "<<script>alert(1)//<</script>",
  "<script>document.write('<\\/script><img src=x onerror=alert(1)>')</script>",
  "<img src=x onerror=alert(1)>",
  "<img src=x OnErRoR=alert(1)>",
  "<img/src=x/onerror=alert(1)>",
  "<img src=x onerror\t=\nalert(1)>",
  "<img src=\"x\"onerror=\"alert(1)\">",
  "<body onload=alert(1)>",
  "<svg onload=alert(1)>",
  "<svg><script>alert(1)</script></svg>",
  "<svg><style><img src=x onerror=alert(1)></style></svg>",
  "<svg><a xlink:href=\"javascript:alert(1)\"><text>x</text></a></svg>",
  "<svg><animate attributeName=href values=\"javascript:alert(1)\"/></svg>",
  "<svg><set attributeName=onmouseover to=alert(1)/></svg>",
  "<math><mi xlink:href=\"javascript:alert(1)\">x</mi></math>",
  "<a href=\"javascript:alert(1)\">x</a>",
  "<a href=\" JaVaScRiPt:alert(1)\">x</a>",
  "<a href=\"jav&#x09;ascript:alert(1)\">x</a>",
  "<a href=\"jav&#9;ascript:alert(1)\">x</a>",
  "<a href=\"&#106;&#97;&#118;&#97;&#115;&#99;&#114;&#105;&#112;&#116;&#58;alert(1)\">x</a>",
  "<a href=\"&#x6A&#x61&#x76&#x61&#x73&#x63&#x72&#x69&#x70&#x74&#x3A;alert(1)\">x</a>",
  "<a href=\"javascript&colon;alert(1)\">x</a>",
  "<a href=\"java&NewLine;script:alert(1)\">x</a>",
  "<a href=\"java\u0000script:alert(1)\">x</a>",
  "<a href=\"\u200bjavascript:alert(1)\">x</a>",
  "<a href=vbscript:msgbox(1)>x</a>",
  "<a href=\"data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==\">x</a>",
  "<form action=\"javascript:alert(1)\"><button formaction=javascript:alert(1)>x</button></form>",
  "<iframe src=\"javascript:alert(1)\"></iframe>",
  "<iframe srcdoc=\"<script>alert(1)</script>\"></iframe>",
  "<div srcdoc=\"&lt;script&gt;alert(1)&lt;/script&gt;\">x</div>",
  "<object data=\"javascript:alert(1)\"><param name=x value=y></object>",
  "<embed src=\"javascript:alert(1)\">",
  "<meta http-equiv=\"refresh\" content=\"0;url=javascript:alert(1)\">",
  "<base href=\"javascript:alert(1)//\">",
  "<link rel=import href=\"https://example.org/x.html\">",
  "<link rel=stylesheet href=\"javascript:alert(1)\">",
  "<div style=\"background:url(javascript:alert(1))\">x</div>",
  "<div style=\"background:url(&#106;avascript:alert(1))\">x</div>",
  "<style>body{background:url(\"javascript:alert(1)\")}</style>",
  "<noscript><p title=\"</noscript><img src=x onerror=alert(1)>\"></noscript>",
  "<template><img src=x onerror=alert(1)></template>",
  "<xmp><img src=x onerror=alert(1)></xmp>",
  "<noembed><img src=x onerror=alert(1)></noembed>",
  "<plaintext><img src=x onerror=alert(1)>",
  "<!--<img src=x onerror=alert(1)>-->",
  "<!--><img src=x onerror=alert(1)>-->",
  "<![CDATA[<img src=x onerror=alert(1)>]]>",
  "<?xml <img src=x onerror=alert(1)> ?>",
  "<title><img src=x onerror=alert(1)></title>",
  "<textarea><script>alert(1)</script></textarea>",
  "<p title=\"a>b\" onclick=alert(1)>x</p>",
  "<p title='<script>alert(1)</script>'>x</p>",
  "<input onfocus=alert(1) autofocus>",
  "<details open ontoggle=alert(1)>",
  "<video><source onerror=alert(1)></video>",
  "<a ping=\"https://example.org/\" href=\"/x\">x</a>",
  "<img src=x onerror=alert(1)",
  "<img src=\"x\" onerror=\"alert(1)",
];

// survivors: every script element, handler or script URL left in `html`,
// read the way a browser would read the sanitizer's markup.
function survivors(html: string): string[] {
  const found: string[] = [];
  if (/<\s*\/?\s*script/i.test(html)) found.push("script tag");
  for (const t of tokenize(html)) {
    if (t.kind !== "start") continue;
    if (t.name === "script") found.push("script element");
    for (const a of t.attrs) {
      if (a.name.startsWith("on")) found.push(`handler ${a.name}`);
      if (/(java|vb)script:/.test(normalized(a.value))) found.push(`script URL in ${a.name}`);
      if (a.name === "srcdoc") found.push("srcdoc");
    }
    if (t.name === "style" && /(java|vb)script:|<img/i.test(normalized(t.raw ?? ""))) found.push(`script inside ${t.name}`);
  }
  if (/javascript:|vbscript:/i.test(html)) found.push("script URL text");
  return found;
}

test("hostile HTML corpus: no <script>, on* attribute or script URL survives", () => {
  for (const vector of HOSTILE) {
    const out = sanitize(`<!doctype html><html><head><title>t</title></head><body>${vector}</body></html>`, PAGE);
    assert.deepEqual(survivors(out), [], `${vector}\n-> ${out}`);
    const bare = sanitize(vector, { ...PAGE, scheme: "dark" }); // no <html>, no <head>
    assert.deepEqual(survivors(bare), [], `${vector} (bare)\n-> ${bare}`);
  }
});

test("the look stays: classes, inline styles, stylesheets, images, svg, forms", () => {
  const src =
    '<!DOCTYPE html><html lang="en" data-theme="light" class="no-js"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">' +
    '<base href="https://example.org/"><link rel="stylesheet" href="css/style.css"><link rel="preload" as="font" href="/f.woff2"><link rel="icon" href="/i.svg"><link rel="prefetch" href="/next/">' +
    "<style>.a > .b { color: red }</style><title>A &amp; B</title></head>" +
    '<body class="home"><p style="color: #333" data-x=1>Fish &amp; chips &lt;3</p><img src="data:image/png;base64,AAAA" srcset="a.png 1x, b.png 2x" alt="a">' +
    '<svg viewBox="0 0 10 10"><title>i</title><path d="M0 0h10"/></svg><a href="/menu/?a=1&b=2" target="_blank">menu</a><form action="/search"><input name=q></form><br/></body></html>';
  const out = sanitize(src, { ...PAGE, scheme: "dark" });
  assert.match(out, /^<!doctype html><html lang="en" class="no-js" data-theme="dark"><head><base href="https:\/\/example.com\/blog\/post\/"><style>:root:root:root:root\{--x:#fff\}<\/style><meta charset="utf-8">/);
  assert.doesNotMatch(out, /example\.org/, "the page's own <base> is gone");
  assert.match(out, /<link rel="stylesheet" href="css\/style.css"><link rel="preload" as="font" href="\/f.woff2"><link rel="icon" href="\/i.svg"><style>.a > .b \{ color: red \}<\/style>/);
  assert.doesNotMatch(out, /prefetch/);
  assert.match(out, /<title>A &amp; B<\/title>/);
  assert.match(out, /<p style="color: #333" data-x="1">Fish &amp; chips &lt;3<\/p>/);
  assert.match(out, /<img src="data:image\/png;base64,AAAA" srcset="a.png 1x, b.png 2x" alt="a">/);
  assert.match(out, /<svg viewbox="0 0 10 10"><title>i<\/title><path d="M0 0h10" \/><\/svg>/);
  assert.match(out, /<a href="\/menu\/\?a=1&b=2" target="_blank">menu<\/a><form action="\/search"><input name="q"><\/form><br \/>/);
});

test("injection: after <head>, before the first element when there is none, once", () => {
  const css = PAGE.css;
  assert.equal(sanitize("<p>x</p>", PAGE), `<base href="https://example.com/blog/post/"><style>${css}</style><p>x</p>`);
  assert.equal(sanitize("text only", PAGE), `text only<base href="https://example.com/blog/post/"><style>${css}</style>`);
  assert.equal(sanitize("<html><body>x", PAGE), `<html><base href="https://example.com/blog/post/"><style>${css}</style><body>x`);
  assert.equal(sanitize("<p>x", { ...PAGE, scheme: "light" }).startsWith('<html data-theme="light"><base'), true);
  assert.equal(sanitize('<html data-theme="dark"><head></head>', PAGE), '<html data-theme="dark"><head><base href="https://example.com/blog/post/"><style>:root:root:root:root{--x:#fff}</style></head>');
  assert.match(sanitize("<p>x</p>", { ...PAGE, baseHref: 'https://example.com/"><script>' }), /<base href="https:\/\/example.com\/&quot;&gt;&lt;script&gt;">/);
});

test("dropped with their content, counted; dropped tags keep what they enclose", () => {
  assert.match(sanitize("<template>a<template>b</template>c</template>d", PAGE), /^d<base /);
  assert.match(sanitize("<p><object>fallback<object>x</object></object>after", PAGE), /<\/style><p>after$/);
  assert.match(sanitize("<p><frameset><frame src=x></frameset><embed src=a.swf>kept", PAGE), /<\/style><p>kept$/);
  assert.match(sanitize("<p>a</p></embed></base><//p>", PAGE), /<p>a<\/p>$/);
  assert.match(sanitize("<style>a{b:c}</style>", PAGE), /<style>a\{b:c\}<\/style>$/);
  assert.match(sanitize("<style>a::after{content:'<'}</style>", PAGE), /<style><\/style>$/);
  assert.match(sanitize("<meta name=description content=x><meta http-equiv=x-ua-compatible content=IE>", PAGE), /<meta name="description" content="x">$/);
  assert.match(sanitize("<p><link href=x.css>", PAGE), /<\/style><p>$/, "a link without rel goes");
  assert.match(sanitize("<p x\u0001=1 y:z=2 =a>q</p>", PAGE), /<p y:z="2">q<\/p>$/, "names a browser would not keep go");
  assert.match(sanitize("<p><\u00c4>x</\u00c4>", PAGE), /<\/style><p>&lt;\u00c4&gt;x$/, "not a tag name: text, as in a browser");
});

test("tokenizer: quoted values hold >, first duplicate wins, self-closing, comments, bogus markup", () => {
  assert.deepEqual(tokenize('<a title=">" href=\'/x\' title=dup b c=d/ >'), [{ kind: "start", name: "a", attrs: [{ name: "title", value: ">" }, { name: "href", value: "/x" }, { name: "b", value: "" }, { name: "c", value: "d/" }], selfClosing: false }]);
  assert.deepEqual(tokenize("<br/><br / >"), [{ kind: "start", name: "br", attrs: [], selfClosing: true }, { kind: "start", name: "br", attrs: [], selfClosing: false }]);
  assert.deepEqual(tokenize("a<!---->b<!-->c<!--->d<!-- x --!>e<!-- open"), ["a", "b", "c", "d", "e"].map((text) => ({ kind: "text", text })));
  assert.deepEqual(tokenize("</>x</3>y< z"), [{ kind: "text", text: "x" }, { kind: "text", text: "y" }, { kind: "text", text: "<" }, { kind: "text", text: " z" }]);
  assert.deepEqual(tokenize("</p class=x>"), [{ kind: "end", name: "p" }]);
  assert.deepEqual(tokenize("</p"), []);
  assert.deepEqual(tokenize("<a b='x"), [], "a tag cut off by the end is dropped");
  assert.deepEqual(tokenize('<a b="x'), []);
  assert.deepEqual(tokenize("<a b = x>"), [{ kind: "start", name: "a", attrs: [{ name: "b", value: "x" }], selfClosing: false }]);
  assert.deepEqual(tokenize("<a b="), []);
  assert.deepEqual(tokenize("<!DOCTYPE html>"), [{ kind: "doctype" }]);
  assert.deepEqual(tokenize("<!doctype html"), [{ kind: "doctype" }], "cut off: a browser still reads a doctype");
  assert.deepEqual(tokenize("<script>a</script >b"), [{ kind: "start", name: "script", attrs: [], selfClosing: false, raw: "a" }, { kind: "end", name: "script" }, { kind: "text", text: "b" }]);
  assert.deepEqual(tokenize("<title>a</titlex></title>"), [{ kind: "start", name: "title", attrs: [], selfClosing: false, raw: "a</titlex>" }, { kind: "end", name: "title" }]);
  assert.deepEqual(tokenize("<style>never closed"), [{ kind: "start", name: "style", attrs: [], selfClosing: false, raw: "never closed" }]);
});

test("safeAttr and normalized: the reading that decides what goes", () => {
  assert.equal(safeAttr({ name: "href", value: "/ok" }), true);
  assert.equal(safeAttr({ name: "src", value: "data:image/png;base64,AA" }), true);
  assert.equal(safeAttr({ name: "href", value: " data:text/html,x" }), false);
  assert.equal(safeAttr({ name: "ping", value: "/p" }), false);
  assert.equal(normalized("&#0;&#x110000;&amp&lt;&unknown;&Tab;A"), "\ufffd\ufffd&<&unknown;a");
  assert.equal(normalized("&fjlig;&colon"), "fj:", "stricter than a browser (no legacy colon): drops more, never less");
});
