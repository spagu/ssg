// Fetching the site's feed (GO-130) with the guards a worker holding admin
// credentials needs before it reads a URL from configuration:
//   - https: only;
//   - the host must be the site's (NEWSLETTER_SITE_URL) or exactly the host of
//     NEWSLETTER_FEED_URL — nothing else, also not after a redirect: redirects
//     are followed by hand (redirect: "manual"), each hop checked, at most 3;
//   - a time limit and a size limit (read as a stream, cut off at the cap);
//   - an XML content type (an HTML error page is not a feed);
//   - a charset the runtime can decode (from the header or the XML
//     declaration), decoded strictly.
// Every refusal is a FeedFailure with a code the API passes on; nothing here
// throws for a bad feed.

import type { Env } from "./lib";
import { siteOf } from "./render";

// ssg's `feed: true` writes the site's Atom feed here (docs/CONFIGURATION.md).
export const DEFAULT_FEED_PATH = "/feed.xml";
export const FEED_MAX_BYTES = 2 * 1024 * 1024; // a feed of full posts stays well under this
export const FEED_TIMEOUT_MS = 10_000;
const MAX_REDIRECTS = 3;

export type FeedFailure = { error: string; detail: string; status: number };
export type Fetched = { ok: true; text: string; url: string } | ({ ok: false } & FeedFailure);

const fail = (error: string, detail: string, status = 502): Fetched => ({ ok: false, error, detail, status });

// feedSource resolves where the feed is and which hosts may serve it.
// NEWSLETTER_FEED_URL may be absolute ("https://example.com/pl/feed.xml") or a
// path on the site ("/pl/feed.xml"); unset = DEFAULT_FEED_PATH on the site.
export function feedSource(env: Env): { url: URL; hosts: Set<string> } | FeedFailure {
  const site = siteOf(env);
  const raw = (env.NEWSLETTER_FEED_URL ?? "").trim() || DEFAULT_FEED_PATH;
  let url: URL;
  try {
    url = site ? new URL(raw, site.url) : new URL(raw);
  } catch {
    const detail = site ? "NEWSLETTER_FEED_URL is not a URL" : "set NEWSLETTER_SITE_URL (or an absolute https:// NEWSLETTER_FEED_URL)";
    return { error: "no_feed_url", detail, status: 503 };
  }
  if (url.protocol !== "https:") return { error: "feed_not_https", detail: "the feed URL must use https:", status: 503 };
  const hosts = new Set([url.host]);
  if (site) hosts.add(new URL(site.url).host);
  return { url, hosts };
}

// charsetOf: the content-type parameter wins, then the XML declaration, then UTF-8.
function charsetOf(contentType: string, head: Uint8Array): string {
  const fromHeader = /charset\s*=\s*"?([^";\s]+)/i.exec(contentType)?.[1];
  if (fromHeader) return fromHeader;
  // The declaration is ASCII in every charset a feed uses; reading the bytes
  // as Latin-1 needs no decoder (Workers do not promise legacy decoders).
  const decl = String.fromCharCode(...head.subarray(0, 200)).replace(/^\xEF\xBB\xBF/, "");
  return /^<\?xml[^>]*\bencoding\s*=\s*["']([^"']+)["']/.exec(decl)?.[1] ?? "utf-8";
}

// readCapped reads at most `max` bytes, cancelling the stream past that.
async function readCapped(res: Response, max: number): Promise<Uint8Array | null> {
  const declared = Number(res.headers.get("content-length"));
  if (declared > max) {
    await res.body?.cancel();
    return null;
  }
  const chunks: Uint8Array[] = [];
  let size = 0;
  const reader = (res.body as ReadableStream<Uint8Array>).getReader();
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    size += value.byteLength;
    if (size > max) {
      await reader.cancel();
      return null;
    }
    chunks.push(value);
  }
  const out = new Uint8Array(size);
  let at = 0;
  for (const c of chunks) {
    out.set(c, at);
    at += c.byteLength;
  }
  return out;
}

// follow GETs `url`, following redirects by hand: each hop must stay https:
// and on an allowed host. Returns the first non-redirect answer.
async function follow(url: URL, hosts: Set<string>, signal: AbortSignal, timeoutMs: number): Promise<{ res: Response; url: URL } | Fetched> {
  for (let hop = 0; hop <= MAX_REDIRECTS; hop++) {
    let res: Response;
    try {
      res = await fetch(url.href, {
        redirect: "manual",
        signal,
        headers: { accept: "application/atom+xml, application/rss+xml, application/xml;q=0.9, text/xml;q=0.9" },
      });
    } catch (e) {
      const timedOut = e instanceof Error && (e.name === "TimeoutError" || e.name === "AbortError");
      return fail("feed_unavailable", timedOut ? `no answer within ${timeoutMs / 1000} s` : `fetch failed: ${e}`);
    }
    if (res.status < 300 || res.status >= 400) return { res, url };
    await res.body?.cancel();
    const next = res.headers.get("location");
    if (!next) return fail("feed_redirect", "a redirect without a location");
    url = new URL(next, url);
    if (url.protocol !== "https:") return fail("feed_redirect", "the feed redirects to a non-https URL");
    if (!hosts.has(url.host)) return fail("feed_redirect", `the feed redirects to another host (${url.host})`);
  }
  return fail("feed_redirect", "too many redirects");
}

// fetchFeed GETs the feed under every rule above.
export async function fetchFeed(env: Env, timeoutMs = FEED_TIMEOUT_MS, maxBytes = FEED_MAX_BYTES): Promise<Fetched> {
  const source = feedSource(env);
  if (!("url" in source)) return { ok: false, ...source };
  const got = await follow(source.url, source.hosts, AbortSignal.timeout(timeoutMs), timeoutMs);
  if ("ok" in got) return got;
  const { res, url } = got;
  if (!res.ok) {
    await res.body?.cancel();
    return fail("feed_unavailable", `the feed answered ${res.status}`);
  }
  const type = res.headers.get("content-type") ?? "";
  if (!/\bxml\b|rss|atom/i.test(type)) {
    await res.body?.cancel();
    return fail("feed_content_type", `expected an XML feed, got ${JSON.stringify(type || "no content type")}`);
  }
  let bytes: Uint8Array | null;
  try {
    bytes = await readCapped(res, maxBytes);
  } catch (e) {
    return fail("feed_unavailable", `reading the feed failed: ${e}`);
  }
  if (!bytes) return fail("feed_too_large", `the feed is larger than ${maxBytes} bytes`);
  const charset = charsetOf(type, bytes);
  let decoder: TextDecoder;
  try {
    decoder = new TextDecoder(charset, { fatal: true, ignoreBOM: false });
  } catch {
    return fail("unsupported_charset", `the feed's charset ${JSON.stringify(charset)} cannot be decoded here; serve it as UTF-8`);
  }
  try {
    return { ok: true, text: decoder.decode(bytes), url: url.href };
  } catch {
    return fail("invalid_feed", `the feed is not valid ${charset}`);
  }
}
