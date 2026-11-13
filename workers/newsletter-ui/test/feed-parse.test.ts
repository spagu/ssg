// GO-130: the feed parser — RSS 2.0 and Atom as ssg writes them, entities,
// CDATA, missing fields, and hostile input that must be refused or read in
// one linear pass.
import { test } from "node:test";
import assert from "node:assert/strict";
import { FeedError, decodeEntities, element, excerpt, parseFeed, plain } from "../src/feed-parse.ts";
import { ATOM, RSS } from "./feed-fixtures.ts";

const BASE = "https://example.com/feed.xml";

test("Atom: fields, links, html titles, CDATA, languages, dates, items without id", () => {
  const posts = parseFeed(ATOM, BASE);
  assert.equal(posts.length, 3);
  assert.deepEqual(posts[0], {
    guid: "tag:example.com,2026:fish",
    title: "Fish & chips now",
    link: "https://example.com/posts/fish/",
    summary: "Crispy & hot.",
    published: "2026-11-10T08:00:00.000Z",
    language: "pl",
  });
  assert.deepEqual(posts[1], {
    guid: "https://example.com/posts/tea/",
    title: "Tea & biscuits",
    link: "https://example.com/posts/tea/",
    summary: "Milk first?",
    published: "2026-11-09T07:00:00.000Z",
    language: "en-GB",
  });
  assert.deepEqual(posts[2], { guid: "urn:x:bare", title: "", link: "", summary: "", published: null, language: "pl" });
});

test("RSS 2.0: numeric entities, CDATA, dc:language, content:encoded, relative links, channel language", () => {
  const posts = parseFeed(RSS, BASE);
  assert.equal(posts.length, 2);
  assert.deepEqual(posts[0], {
    guid: "post-1",
    title: "Brücke & Tunnel",
    link: "https://example.com/posts/bridge/",
    summary: "A bridge & more.",
    published: "2026-11-10T08:00:00.000Z",
    language: "fr",
  });
  assert.deepEqual(posts[1], {
    guid: "https://example.com/posts/link-only/",
    title: "Only a link",
    link: "https://example.com/posts/link-only/",
    summary: "Encoded body",
    published: null,
    language: "de",
  });
});

test("RSS without a language and with an unparseable link", () => {
  const xml = `<rss><channel><item><guid>g</guid><link>http://[bad</link><title>T</title></item></channel></rss>`;
  assert.deepEqual(parseFeed(xml, BASE), [{ guid: "g", title: "T", link: "", summary: "", published: null, language: null }]);
});

test("refused: not a feed, unclosed elements, CDATA never closed, entity declarations", () => {
  const cases: [string, RegExp][] = [
    ["<html><body>Not found</body></html>", /not an RSS 2.0 or Atom feed/],
    ["", /not an RSS/],
    ["<rss><channel><item><title>x</title></channel></rss>", /<item> is never closed/],
    ["<feed><entry><title>x", /never closed/],
    ["<feed><entry><title><![CDATA[x</title></entry></feed>", /CDATA section is never closed/],
    ["<rss version=\"2.0\"", /unterminated <rss> tag/],
    [
      `<?xml version="1.0"?><!DOCTYPE lolz [<!ENTITY lol "lol"><!ENTITY lol2 "&lol;&lol;&lol;&lol;&lol;">]><rss><channel><item><title>&lol2;</title></item></channel></rss>`,
      /declares XML entities/,
    ],
  ];
  for (const [xml, message] of cases) {
    assert.throws(() => parseFeed(xml, BASE), (e: unknown) => e instanceof FeedError && message.test(e.message), xml.slice(0, 40));
  }
});

test("hostile bodies are read in one linear pass", () => {
  const n = 200_000;
  const started = Date.now();
  // Thousands of opening tags that never close: refused at the first.
  assert.throws(() => parseFeed("<rss><channel>" + "<item>".repeat(n), BASE), FeedError);
  // Many items without a CDATA anywhere: no rescans of the remaining body.
  const many = "<rss><channel>" + "<item><guid>g</guid></item>".repeat(n / 10) + "</channel></rss>";
  assert.equal(parseFeed(many, BASE).length, n / 10);
  // Unmatched "<" and "<link" inside text.
  assert.equal(plain("&lt;".repeat(n) + "x"), "<".repeat(n) + "x");
  const links = "<feed><entry><id>i</id>" + "<link ".repeat(n) + "</entry></feed>";
  assert.equal(parseFeed(links, BASE)[0].link, "");
  assert.ok(Date.now() - started < 3000, `took ${Date.now() - started} ms`);
});

test("helpers: entities, element, excerpt", () => {
  assert.equal(decodeEntities("&#0; &#x110000; &nbsp; &QUOT; &#65;"), '&#0; &#x110000; &nbsp; " A');
  assert.deepEqual(element("<a x='1'/><a>b</a>", "a"), { attrs: " x='1'", inner: "", end: 10 });
  assert.equal(element("<ab>x</ab>", "a"), null);
  assert.equal(element("<a>1</a><a>2</a>", "a", 8)?.inner, "2");
  assert.equal(excerpt("short", 10), "short");
  assert.equal(excerpt("one two three four five", 12), "one two…");
  assert.equal(excerpt("abcdefghijklmnop", 8), "abcdefg…");
});
