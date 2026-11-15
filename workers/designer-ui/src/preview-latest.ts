// "The latest post" for the preview (GO-188), found the way a feed reader
// finds it: the home page names its feed (<link rel="alternate">), and a feed
// lists its newest entry first. Only the PATH of the feed's and the entry's
// address is used, always on the panel's own origin: a site whose feeds say
// https://example.com while it is served on www.example.com still previews,
// and no address in a page can send a fetch to another host. Every fetch goes
// through fetchPage (no credentials, same origin, size limit).

import type { FetchFn } from "../portable/repo-github-http";
import { tokens } from "./html-tokens";
import { MAX_PAGE, PreviewError, fetchPage, sitePath } from "./preview-fetch";

const FEED_TYPE = /^application\/(rss\+xml|atom\+xml|feed\+json)$/i;

// feedHref: the first feed the page announces, or null.
export function feedHref(html: string): string | null {
  for (const t of tokens(html)) {
    if (t.kind !== "start" || t.name !== "link") continue;
    const attr = (n: string): string => t.attrs.find((a) => a.name === n)?.value.trim() ?? "";
    if (/(^|\s)alternate(\s|$)/i.test(attr("rel")) && FEED_TYPE.test(attr("type")) && attr("href")) return attr("href");
  }
  return null;
}

// firstEntry: the address of a feed's first entry (JSON Feed items[0].url,
// RSS <item><link>, Atom <entry><link href>), or null. Read with the page
// tokenizer, one pass: the patterns it replaced went quadratic on a feed of
// unclosed <item> or <link (SR-23).
export function firstEntry(feed: string): string | null {
  if (feed.trimStart().startsWith("{")) {
    try {
      const url = (JSON.parse(feed) as { items?: { url?: unknown }[] }).items?.[0]?.url;
      return typeof url === "string" ? url : null;
    } catch {
      return null;
    }
  }
  let entry: string | null = null; // the first entry's element, once inside it
  let inLink = false; // inside an RSS <link>, whose text is the address
  for (const t of tokens(feed)) {
    if (entry === null) {
      if (t.kind === "start" && (t.name === "item" || t.name === "entry")) entry = t.name;
    } else if (t.kind === "start" && t.name === "link") {
      const href = t.attrs.find((a) => a.name === "href")?.value.trim();
      if (href) return href.replace(/&amp;/g, "&");
      inLink = true;
    } else if (inLink && t.kind === "text" && t.text.trim()) {
      return t.text.trim().replace(/&amp;/g, "&");
    } else if (t.kind === "end" && t.name === entry) {
      return null;
    } else if (t.kind === "end") {
      inLink = false;
    }
  }
  return null;
}

// pathOf: the path and query of `ref` read against `from`, checked as any
// path the caller sends; no_latest_post when it is none.
function pathOf(ref: string | null, from: string, base: string): string {
  try {
    const u = new URL(ref ?? "", from);
    if (ref && /^https?:$/.test(u.protocol)) return sitePath(u.pathname + u.search, base);
  } catch {
    // an address that does not parse, or a path the preview refuses
  }
  throw new PreviewError("no_latest_post", 404);
}

// latestPath: the path (and query) of the newest post on `origin`; the home
// page and the feed are read within the same maxBytes as the page.
export async function latestPath(doFetch: FetchFn, origin: string, base: string, maxBytes = MAX_PAGE): Promise<string> {
  const home = await fetchPage(doFetch, origin, "/", maxBytes);
  const feed = await fetchPage(doFetch, origin, pathOf(feedHref(home.text), home.url, base), maxBytes, "feed");
  return pathOf(firstEntry(feed.text), feed.url, base);
}
