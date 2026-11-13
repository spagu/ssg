// GO-130: fetching the feed — where it may come from, redirects, limits,
// content type and charset. Every refusal is a code, never a throw.
import { test } from "node:test";
import assert from "node:assert/strict";
import { DEFAULT_FEED_PATH, fetchFeed, feedSource } from "../src/feed-fetch.ts";
import { ATOM, feedFetch } from "./feed-fixtures.ts";

const SITE = { NEWSLETTER_SITE_URL: "https://example.com" };
const FEED = "https://example.com/feed.xml";
const env = (o: Record<string, string | undefined> = {}) => ({ ...SITE, ...o }) as never;
const atom = { body: ATOM };

async function fetched(e: never, answers: Parameters<typeof feedFetch>[0], ...rest: [number?, number?]) {
  const f = feedFetch(answers);
  try {
    return { r: await fetchFeed(e, ...rest), calls: f.calls };
  } finally {
    f.restore();
  }
}

test("source: default /feed.xml on the site, a path, an absolute URL, refusals", () => {
  assert.equal(DEFAULT_FEED_PATH, "/feed.xml");
  const s = feedSource(env()) as { url: URL; hosts: Set<string> };
  assert.equal(s.url.href, FEED);
  assert.deepEqual([...s.hosts], ["example.com"]);
  assert.equal((feedSource(env({ NEWSLETTER_FEED_URL: " /pl/feed.xml " })) as { url: URL }).url.href, "https://example.com/pl/feed.xml");
  const other = feedSource(env({ NEWSLETTER_FEED_URL: "https://blog.example.net/rss.xml" })) as { hosts: Set<string> };
  assert.deepEqual([...other.hosts].sort(), ["blog.example.net", "example.com"]);
  const alone = feedSource({ NEWSLETTER_FEED_URL: "https://blog.example.net/rss.xml" } as never) as { hosts: Set<string> };
  assert.deepEqual([...alone.hosts], ["blog.example.net"]);
  assert.deepEqual(feedSource({} as never), {
    error: "no_feed_url",
    detail: "set NEWSLETTER_SITE_URL (or an absolute https:// NEWSLETTER_FEED_URL)",
    status: 503,
  });
  assert.equal((feedSource({ NEWSLETTER_SITE_URL: "http://example.com" } as never) as { error: string }).error, "no_feed_url");
  assert.equal((feedSource(env({ NEWSLETTER_FEED_URL: "https://[" })) as { detail: string }).detail, "NEWSLETTER_FEED_URL is not a URL");
  assert.equal((feedSource(env({ NEWSLETTER_FEED_URL: "http://example.com/feed.xml" })) as { error: string }).error, "feed_not_https");
});

test("a good feed: manual redirects, an accept header, the final URL", async () => {
  const { r, calls } = await fetched(env(), { [FEED]: atom });
  assert.deepEqual(r, { ok: true, text: ATOM, url: FEED });
  assert.equal(calls[0].init.redirect, "manual");
  assert.match(String((calls[0].init.headers as Record<string, string>).accept), /atom\+xml/);
  assert.ok(calls[0].init.signal instanceof AbortSignal);
});

test("redirects: same host followed, another host / http / no location / loops refused", async () => {
  const hop = (location?: string) => ({ status: 301, body: "", headers: location ? { location } : {} });
  const same = await fetched(env(), { [FEED]: hop("/blog/feed.xml"), "https://example.com/blog/feed.xml": atom });
  assert.deepEqual([same.r.ok, (same.r as { url: string }).url], [true, "https://example.com/blog/feed.xml"]);
  const configured = await fetched(env({ NEWSLETTER_FEED_URL: "https://feeds.example.net/x" }), {
    "https://feeds.example.net/x": hop("https://example.com/feed.xml"),
    [FEED]: atom,
  });
  assert.equal(configured.r.ok, true, "the site and the configured feed host are both allowed");
  const cases: [Parameters<typeof feedFetch>[0], RegExp][] = [
    [{ [FEED]: hop("https://evil.example.org/feed.xml") }, /another host \(evil\.example\.org\)/],
    [{ [FEED]: hop("https://example.com:8443/feed.xml") }, /another host \(example\.com:8443\)/],
    [{ [FEED]: hop("http://example.com/feed.xml") }, /non-https/],
    [{ [FEED]: hop() }, /without a location/],
    [{ [FEED]: hop(FEED) }, /too many redirects/],
  ];
  for (const [answers, detail] of cases) {
    const { r, calls } = await fetched(env(), answers);
    assert.equal(r.ok, false);
    const f = r as { error: string; detail: string; status: number };
    assert.deepEqual([f.error, f.status], ["feed_redirect", 502]);
    assert.match(f.detail, detail);
    assert.ok(calls.every((c) => c.url.startsWith("https://example.com/")), "never fetched another host");
  }
});

test("unavailable: an error status, a network error, a timeout", async () => {
  const fail = async (answers: Parameters<typeof feedFetch>[0], timeout?: number) => (await fetched(env(), answers, timeout)).r as { error: string; detail: string };
  assert.deepEqual(await fail({}), { ok: false, error: "feed_unavailable", detail: "the feed answered 404", status: 502 });
  const down = await fail({ [FEED]: () => { throw new TypeError("connection refused"); } });
  assert.match(down.detail, /fetch failed: TypeError: connection refused/);
  const slow = await fail({ [FEED]: () => { throw new DOMException("t", "TimeoutError"); } }, 2000);
  assert.equal(slow.detail, "no answer within 2 s");
  const aborted = await fail({ [FEED]: () => { throw new DOMException("t", "AbortError"); } });
  assert.equal(aborted.detail, "no answer within 10 s");
});

test("content type: XML types accepted, HTML / text / none refused", async () => {
  for (const type of ["application/rss+xml", "application/xml", "text/xml; charset=utf-8", "application/atom+xml"]) {
    assert.equal((await fetched(env(), { [FEED]: { body: ATOM, headers: { "content-type": type } } })).r.ok, true, type);
  }
  for (const [headers, shown] of [[{ "content-type": "text/html; charset=utf-8" }, "text/html; charset=utf-8"], [{}, "no content type"]] as const) {
    const { r } = await fetched(env(), { [FEED]: { body: new TextEncoder().encode(ATOM), headers } });
    assert.deepEqual([(r as { error: string }).error, (r as { detail: string }).detail], ["feed_content_type", `expected an XML feed, got "${shown}"`]);
  }
});

test("size: a declared length over the cap, a stream over the cap, a stream that breaks", async () => {
  const big = await fetched(env(), { [FEED]: { body: ATOM, headers: { "content-type": "application/xml", "content-length": "99999" } } }, undefined, 100);
  assert.deepEqual([(big.r as { error: string }).error, (big.r as { detail: string }).detail], ["feed_too_large", "the feed is larger than 100 bytes"]);
  let pulled = 0;
  const endless = () => ({
    body: new ReadableStream<Uint8Array>({
      pull(c) {
        pulled++;
        c.enqueue(new Uint8Array(64).fill(0x20));
      },
    }),
    headers: { "content-type": "application/xml" },
  });
  const huge = await fetched(env(), { [FEED]: endless }, undefined, 1000);
  assert.equal((huge.r as { error: string }).error, "feed_too_large");
  assert.ok(pulled < 40, "stopped reading at the cap");
  const broken = () => ({
    body: new ReadableStream<Uint8Array>({
      start(c) {
        c.enqueue(new Uint8Array([0x3c]));
        c.error(new Error("reset by peer"));
      },
    }),
    headers: { "content-type": "application/xml" },
  });
  const r = (await fetched(env(), { [FEED]: broken })).r as { error: string; detail: string };
  assert.deepEqual([r.error, r.detail], ["feed_unavailable", "reading the feed failed: Error: reset by peer"]);
});

test("charset: header, XML declaration, BOM; undecodable and unknown refused", async () => {
  const latin1 = (s: string) => Uint8Array.from([...s].map((ch) => ch.charCodeAt(0)));
  const xml = `<?xml version="1.0" encoding="ISO-8859-1"?><rss><channel><item><guid>g</guid><title>Brücke</title></item></channel></rss>`;
  const declared = await fetched(env(), { [FEED]: { body: latin1(xml), headers: { "content-type": "application/rss+xml" } } });
  assert.match((declared.r as { text: string }).text, /Brücke/);
  const header = await fetched(env(), { [FEED]: { body: latin1("<rss>é</rss>"), headers: { "content-type": 'text/xml; charset="windows-1252"' } } });
  assert.equal((header.r as { text: string }).text, "<rss>é</rss>");
  const bom = new Uint8Array([0xef, 0xbb, 0xbf, ...new TextEncoder().encode(`<?xml version="1.0" encoding="UTF-8"?><rss>ü</rss>`)]);
  const withBom = await fetched(env(), { [FEED]: { body: bom, headers: { "content-type": "application/xml" } } });
  assert.match((withBom.r as { text: string }).text, /^<\?xml.*<rss>ü<\/rss>$/);
  const bad = await fetched(env(), { [FEED]: { body: new Uint8Array([0x3c, 0xff, 0xfe, 0x3e]), headers: { "content-type": "application/xml" } } });
  assert.deepEqual([(bad.r as { error: string }).error, (bad.r as { detail: string }).detail], ["invalid_feed", "the feed is not valid utf-8"]);
  const unknown = await fetched(env(), { [FEED]: { body: "<rss/>", headers: { "content-type": "application/xml; charset=x-klingon" } } });
  assert.deepEqual([(unknown.r as { error: string }).error, (unknown.r as { status: number }).status], ["unsupported_charset", 502]);
});

test("configuration refusals come back as failures without a fetch", async () => {
  const { r, calls } = await fetched({} as never, {});
  assert.deepEqual([(r as { error: string }).error, (r as { status: number }).status, calls.length], ["no_feed_url", 503, 0]);
});
