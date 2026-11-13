// renderMessage: variables, both body formats, the text part, the footer in
// every language, the layout and the RFC 8058 headers.
import { test } from "node:test";
import assert from "node:assert/strict";
import { FOOTER, footerLanguage } from "../src/i18n.ts";
import { renderMessage, siteOf, unknownVariables, unsubscribeUrl } from "../src/render.ts";
import type { CampaignContent } from "../src/render.ts";
import { htmlToText } from "../src/render-text.ts";
import { sanitizeHtml } from "../src/sanitize.ts";

const SITE = { url: "https://example.com", host: "example.com" };
const TOKEN = "A".repeat(43);
const UNSUB = `https://example.com/api/newsletter/unsubscribe?token=${TOKEN}`;
const content = (over: Partial<CampaignContent> = {}): CampaignContent => ({ subject: "Hello", body: "Body", bodyFormat: "markdown", textOverride: null, ...over });
const to = (language: string | null = "en") => ({ email: "reader@example.com", language, token: TOKEN });

test("variables: known ones substituted everywhere, unknown ones listed", () => {
  const r = renderMessage(
    content({ subject: "News ({{ language }})", body: "[Leave]({{unsubscribe_url}}) · {{site_url}}/{{language}}/" }),
    to("pt-BR"),
    SITE,
  );
  assert.equal(r.subject, "News (pt-BR)");
  assert.match(r.html, new RegExp(`<a href="${UNSUB.replace(/[?.]/g, "\\$&")}">Leave</a>`));
  assert.match(r.text, /Leave \(https:\/\/example\.com\/api\/newsletter\/unsubscribe\?token=A{43}\)/);
  assert.match(r.text, /https:\/\/example\.com\/pt-BR\//);
  // A stored language that is not a code falls back to the footer language.
  assert.equal(renderMessage(content({ subject: "{{language}}" }), to("<x>"), SITE).subject, "en");
  assert.equal(renderMessage(content({ subject: "{{language}}" }), to(null), SITE).subject, "en");
  assert.deepEqual(unknownVariables("{{name}} {{ site_url }}", null, "{{first_name}}{{name}}{{}}"), ["name", "first_name", ""]);
  assert.deepEqual(unknownVariables("{ {x} } {{unsubscribe_url}}"), []);
  assert.throws(() => renderMessage(content({ body: "{{nope}}" }), to(), SITE), /unknown variable \{\{nope\}\}/);
});

test("HTML bodies are sanitized; variables in them are escaped", () => {
  const r = renderMessage(
    content({ bodyFormat: "html", body: '<p onclick="x()">Hi <script>alert(1)</script><a href="{{unsubscribe_url}}">out</a></p>' }),
    to(),
    SITE,
  );
  assert.match(r.html, new RegExp(`<p>Hi <a href="${UNSUB.replace(/[?.]/g, "\\$&")}">out</a></p>`));
  assert.doesNotMatch(r.html, /script|onclick/);
});

test("text part: generated from the sanitized content, or text_override", () => {
  const r = renderMessage(content({ body: "# Title\n\nSee [the post](https://example.com/p) or https://example.com/q.\n\n- a\n- b\n\n3. c\n\n> q1\n> q2\n\n---\n\n![logo](https://example.com/l.png)" }), to(), SITE);
  const body = r.text.split("\n\n-- \n")[0];
  assert.equal(body, "Title\n\nSee the post (https://example.com/p) or https://example.com/q.\n\n- a\n- b\n\n3. c\n\n> q1 q2\n\n----\n\nlogo");
  const o = renderMessage(content({ textOverride: "  Plain {{site_url}}  " }), to(), SITE);
  assert.ok(o.text.startsWith("Plain https://example.com\n\n-- \n"));
});

test("htmlToText: tables, pre, mailto, br, nested quotes, empty links", () => {
  const t = (html: string) => htmlToText(sanitizeHtml(html));
  assert.equal(t("<table><tr><th>A</th><td>1</td></tr><tr><td>B</td><td>2</td></tr></table>"), "A  1\nB  2");
  assert.equal(t("<pre>  keep\n    this</pre><p>x</p>"), "keep\n    this\n\nx");
  assert.equal(t('<a href="mailto:a@example.com">a@example.com</a> <a href="https://example.com/">https://example.com/</a>'), "mailto:a@example.com https://example.com/");
  assert.equal(t('<a href="https://example.com/"><img src="https://example.com/i.png"></a>'), "https://example.com/");
  assert.equal(t("one<br>two"), "one\ntwo");
  assert.equal(t("<blockquote><p>a</p><p>b</p></blockquote>"), "> a\n>\n> b");
  assert.equal(t('<ol start="9"><li>x</li><li>y</li></ol>'), "9. x\n10. y");
  assert.equal(t("<ol><li>x</li><b>loose</b><li>y</li></ol>"), "1. xloose\n2. y");
  assert.equal(t("<div>  spaced   out  </div><span>inline</span>"), "spaced out\n\ninline");
});

test("footer: always in both parts, in all five languages, plus fallback", () => {
  assert.deepEqual(Object.keys(FOOTER).sort(), ["de", "en", "fr", "hi", "pl"]);
  for (const [lang, strings] of Object.entries(FOOTER)) {
    // Even when the body already links to the unsubscribe URL itself.
    const r = renderMessage(content({ body: "[bye]({{unsubscribe_url}})" }), to(lang), SITE);
    const reason = strings.reason.replace("{site}", "example.com");
    assert.ok(r.html.includes(`${reason}<br>\n<a href="${UNSUB}">${strings.unsubscribe}</a>`), lang);
    assert.ok(r.text.endsWith(`\n\n-- \n${reason}\n${strings.unsubscribe}: ${UNSUB}\n`), lang);
    assert.equal(r.html.split(UNSUB).length - 1, 2, `${lang}: body link + footer link`);
    assert.match(r.html, new RegExp(`<html lang="${lang}" dir="ltr">`));
  }
  assert.equal(footerLanguage("de-AT"), "de");
  assert.equal(footerLanguage("es"), "en");
  assert.equal(footerLanguage(null), "en");
  assert.equal(footerLanguage("constructor"), "en");
  assert.match(renderMessage(content(), to("es"), SITE).text, /Unsubscribe: https/);
});

test("headers: one-click unsubscribe, list id, bulk", () => {
  const r = renderMessage(content(), to(), SITE);
  assert.deepEqual(r.headers, {
    "List-Unsubscribe": `<${UNSUB}>`,
    "List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
    "List-Id": "<newsletter.example.com>",
    Precedence: "bulk",
  });
  assert.equal(unsubscribeUrl(SITE, "a+b/c"), "https://example.com/api/newsletter/unsubscribe?token=a%2Bb%2Fc");
});

test("layout: lang, title escaped, no remote resources, AA contrast", () => {
  const r = renderMessage(content({ subject: "<Tom & Jerry>" }), to(), SITE);
  assert.match(r.html, /^<!doctype html>\n<html lang="en"/);
  assert.match(r.html, /<title>&lt;Tom &amp; Jerry&gt;<\/title>/);
  assert.match(r.html, /<meta name="color-scheme" content="light dark">/);
  // Only the content may reference other origins; the frame itself loads nothing.
  assert.doesNotMatch(r.html.replace(/https:\/\/example\.com\/api\/newsletter\/unsubscribe\?token=A{43}/g, ""), /https?:|url\(|@import|<link|<script/i);
  const lum = (hex: string) => {
    const [r1, g, b] = [1, 3, 5].map((i) => Number.parseInt(hex.slice(i, i + 2), 16) / 255).map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
    return 0.2126 * r1 + 0.7152 * g + 0.0722 * b;
  };
  const ratio = (a: string, b: string) => (Math.max(lum(a), lum(b)) + 0.05) / (Math.min(lum(a), lum(b)) + 0.05);
  for (const [fg, bg] of [["#202124", "#ffffff"], ["#5f6368", "#ffffff"], ["#1967d2", "#ffffff"], ["#e8eaed", "#202124"], ["#bdc1c6", "#202124"], ["#8ab4f8", "#202124"]]) {
    assert.ok(r.html.includes(fg) && r.html.includes(bg), `${fg} on ${bg} is used`);
    assert.ok(ratio(fg, bg) >= 4.5, `${fg} on ${bg}: ${ratio(fg, bg).toFixed(2)}`);
  }
});

test("siteOf: https origin only", () => {
  assert.deepEqual(siteOf({ NEWSLETTER_SITE_URL: " https://example.com/blog/ " }), SITE);
  assert.equal(siteOf({ NEWSLETTER_SITE_URL: "http://example.com" }), null);
  assert.equal(siteOf({ NEWSLETTER_SITE_URL: "example.com" }), null);
  assert.equal(siteOf({}), null);
});
