// Feed parsing. Run from the repository root:
//   node --import ./workers/web-push/test/resolve.mjs --test 'workers/web-push/test/*.test.ts'
import { test } from "node:test";
import assert from "node:assert/strict";
import { excerpt, newItems, parseFeed } from "../functions/api/push/_feed.ts";

const ATOM = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Blog</title>
  <link href="https://example.com/feed.xml" rel="self"/>
  <entry>
    <title type="html">Fish &amp; chips</title>
    <link rel="self" href="https://example.com/x.xml"/>
    <link href="https://example.com/fish/"/>
    <id>https://example.com/fish/</id>
    <summary type="html">&lt;p&gt;Crispy &#8212; hot&lt;/p&gt; &#x263A; &bogus; &#0;</summary>
  </entry>
  <entry>
    <title><![CDATA[Older <post>]]></title>
    <link rel="alternate" href="/older/"/>
    <content type="html">Body</content>
  </entry>
  <entry><title>No id or link</title></entry>
</feed>`;

test("parseFeed reads Atom", () => {
  const items = parseFeed(ATOM, "application/atom+xml");
  assert.deepEqual(items, [
    { id: "https://example.com/fish/", title: "Fish & chips", url: "https://example.com/fish/", summary: "Crispy — hot ☺ &bogus; &#0;" },
    { id: "/older/", title: "Older", url: "/older/", summary: "Body" },
  ]);
});

test("parseFeed reads RSS", () => {
  const rss = `<rss><channel><item><title>A</title><link>https://example.com/a/</link><guid>a-1</guid>
    <description>&lt;b&gt;Hi&lt;/b&gt;</description></item><item><title>B</title><link>https://example.com/b/</link></item></channel></rss>`;
  assert.deepEqual(parseFeed(rss), [
    { id: "a-1", title: "A", url: "https://example.com/a/", summary: "Hi" },
    { id: "https://example.com/b/", title: "B", url: "https://example.com/b/", summary: "" },
  ]);
});

test("parseFeed reads JSON Feed, by type or by shape", () => {
  const feed = JSON.stringify({
    version: "https://jsonfeed.org/version/1.1",
    items: [
      { id: "1", title: "One", url: "https://example.com/1/", summary: "S" },
      { id: "2", title: "Two", url: "https://example.com/2/", content_text: "T" },
      { title: "Three", url: "https://example.com/3/", content_html: "<p>H &amp; H</p>" },
      { title: 4 },
    ],
  });
  const want = [
    { id: "1", title: "One", url: "https://example.com/1/", summary: "S" },
    { id: "2", title: "Two", url: "https://example.com/2/", summary: "T" },
    { id: "https://example.com/3/", title: "Three", url: "https://example.com/3/", summary: "H & H" },
  ];
  assert.deepEqual(parseFeed(feed, "application/feed+json"), want);
  assert.deepEqual(parseFeed("  " + feed), want);
  assert.deepEqual(parseFeed("{}"), []);
});

test("newItems takes what is above the last id, capped", () => {
  const items = ["5", "4", "3", "2", "1"].map((id) => ({ id, title: id, url: "/" + id, summary: "" }));
  assert.deepEqual(newItems(items, "2", 10).map((i) => i.id), ["5", "4", "3"]);
  assert.deepEqual(newItems(items, "2", 2).map((i) => i.id), ["5", "4"]);
  assert.deepEqual(newItems(items, "5", 3), []);
  assert.deepEqual(newItems(items, "gone", 3), []);
});

test("excerpt cuts at a word", () => {
  assert.equal(excerpt("short"), "short");
  assert.equal(excerpt("aaaa bbbb cccc", 10), "aaaa bbbb…");
  assert.equal(excerpt("aaaa bbbbbbbbbbbb", 10), "aaaa bbbb…");
  assert.equal(excerpt("abcdefghijklmnop", 10), "abcdefghi…");
});
