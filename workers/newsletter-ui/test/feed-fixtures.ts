// Fixtures for the GO-130 tests: an Atom and an RSS feed shaped like the ones
// ssg writes, and a scripted fetch for the feed URL.

export const ATOM = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xml:lang="pl">
  <title>Example</title>
  <link href="https://example.com/"/>
  <entry>
    <title type="html">Fish &amp;amp; chips &lt;em&gt;now&lt;/em&gt;</title>
    <link rel="self" href="/self"/>
    <link href="/posts/fish/"/>
    <id>tag:example.com,2026:fish</id>
    <published>2026-11-10T08:00:00Z</published>
    <summary type="html">&lt;p&gt;Crispy &amp;amp; hot.&lt;/p&gt;</summary>
  </entry>
  <entry xml:lang="en-GB">
    <title><![CDATA[Tea </entry> & <b>biscuits</b>]]></title>
    <link rel='alternate' href='https://example.com/posts/tea/'/>
    <updated>2026-11-09T08:00:00+01:00</updated>
    <content type="html"><![CDATA[<p>Milk first?</p>]]></content>
  </entry>
  <entry xml:lang="not a language">
    <title>No id, no link</title>
  </entry>
  <entry>
    <id>urn:x:bare</id>
    <updated>yesterday-ish</updated>
  </entry>
</feed>`;

export const RSS = `<?xml version="1.0"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel>
  <title>Example</title>
  <language>de</language>
  <atom:link href="https://example.com/rss.xml" rel="self"/>
  <item>
    <title>Br&#252;cke &#x26; Tunnel</title>
    <link>https://example.com/posts/bridge/</link>
    <guid isPermaLink="false">post-1</guid>
    <pubDate>Tue, 10 Nov 2026 08:00:00 GMT</pubDate>
    <description><![CDATA[<p>A <a href="/x">bridge</a> &amp; more.</p>]]></description>
    <dc:language>fr</dc:language>
  </item>
  <item>
    <title>Only a link</title>
    <link>/posts/link-only/</link>
    <content:encoded>&lt;p&gt;Encoded body&lt;/p&gt;</content:encoded>
  </item>
  <item><description>no guid and no link</description></item>
</channel>
</rss>`;

export interface FakeAnswer {
  status?: number;
  body?: string | Uint8Array | ReadableStream<Uint8Array>;
  headers?: Record<string, string>;
}

// feedFetch replaces globalThis.fetch: `answers` maps a URL to its answer (a
// function may throw, as a network error does). Unknown URLs answer 404.
// Every request is recorded with its init, so a test can check redirect: "manual".
export function feedFetch(answers: Record<string, FakeAnswer | (() => FakeAnswer)>) {
  const original = globalThis.fetch;
  const calls: { url: string; init: RequestInit }[] = [];
  globalThis.fetch = (async (url: string, init: RequestInit) => {
    calls.push({ url: String(url), init });
    const a = answers[String(url)];
    const answer = typeof a === "function" ? a() : (a ?? { status: 404, body: "nope", headers: { "content-type": "text/html" } });
    return new Response(answer.body ?? null, {
      status: answer.status ?? 200,
      headers: answer.headers ?? { "content-type": "application/atom+xml; charset=utf-8" },
    });
  }) as typeof fetch;
  return { calls, restore: () => void (globalThis.fetch = original) };
}
