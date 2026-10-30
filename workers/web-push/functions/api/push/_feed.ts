// Reading the site's feed. A leading underscore keeps this file out of the
// Pages route table — it is imported, never served.
//
// Workers have no DOMParser, and the feeds here are the ones ssg writes —
// Atom (`feed: true`), RSS or JSON Feed (`feeds:` with `format:`) — so a few
// patterns read them. Items come back in feed order, which for ssg is newest
// first.

export interface FeedItem {
  id: string;
  title: string;
  url: string;
  summary: string;
}

const ENTITIES: Record<string, string> = { amp: "&", lt: "<", gt: ">", quot: '"', apos: "'" };

function decodeEntities(s: string): string {
  return s.replace(/&(#x[0-9a-f]+|#[0-9]+|[a-z]+);/gi, (m, e: string) => {
    if (e[0] === "#") {
      const code = e[1] === "x" || e[1] === "X" ? Number.parseInt(e.slice(2), 16) : Number.parseInt(e.slice(1), 10);
      return code > 0 && code <= 0x10ffff ? String.fromCodePoint(code) : m;
    }
    return ENTITIES[e.toLowerCase()] ?? m;
  });
}

// plain turns feed markup into one line of text: CDATA unwrapped, entities
// decoded, tags (escaped HTML in a summary) removed, whitespace collapsed.
function plain(s: string): string {
  let out = s.replace(/<!\[CDATA\[([\s\S]*?)\]\]>/g, "$1");
  out = decodeEntities(out);
  out = out.replace(/<[^>]*>/g, " ");
  return out.replace(/\s+/g, " ").trim();
}

function tag(block: string, name: string): string {
  const m = block.match(new RegExp(`<${name}(?:\\s[^>]*)?>([\\s\\S]*?)</${name}>`, "i"));
  return m ? m[1] : "";
}

// atomLink prefers rel="alternate", which is also what a link without rel
// means.
function atomLink(block: string): string {
  for (const m of block.matchAll(/<link\b([^>]*)\/?>/gi)) {
    const attrs = m[1];
    const rel = attrs.match(/\brel="([^"]*)"/i)?.[1] ?? "alternate";
    const href = attrs.match(/\bhref="([^"]*)"/i)?.[1];
    if (href && rel === "alternate") return decodeEntities(href);
  }
  return "";
}

function parseXML(text: string): FeedItem[] {
  const atom = [...text.matchAll(/<entry\b[^>]*>([\s\S]*?)<\/entry>/gi)].map((m) => {
    const b = m[1];
    const url = atomLink(b);
    return { id: plain(tag(b, "id")) || url, title: plain(tag(b, "title")), url, summary: plain(tag(b, "summary") || tag(b, "content")) };
  });
  if (atom.length) return atom;
  return [...text.matchAll(/<item\b[^>]*>([\s\S]*?)<\/item>/gi)].map((m) => {
    const b = m[1];
    const url = plain(tag(b, "link"));
    return { id: plain(tag(b, "guid")) || url, title: plain(tag(b, "title")), url, summary: plain(tag(b, "description")) };
  });
}

function parseJSONFeed(text: string): FeedItem[] {
  const feed = JSON.parse(text) as { items?: Record<string, unknown>[] };
  const str = (v: unknown) => (typeof v === "string" ? v : "");
  return (feed.items ?? []).map((it) => {
    const url = str(it.url);
    const summary = str(it.summary) || str(it.content_text) || plain(str(it.content_html));
    return { id: str(it.id) || url, title: plain(str(it.title)), url, summary: plain(summary) };
  });
}

// parseFeed reads a feed body; an item with no id and no link cannot be told
// apart from the next one and is dropped.
export function parseFeed(text: string, contentType = ""): FeedItem[] {
  const isJSON = /json/i.test(contentType) || text.trimStart().startsWith("{");
  const items = isJSON ? parseJSONFeed(text) : parseXML(text);
  return items.filter((it) => it.id);
}

// newItems are the items above `lastId`, newest first, at most `max`. When
// `lastId` is no longer in the feed — the post was deleted, or the feed only
// keeps the latest few — nothing is new: guessing would re-announce old posts.
export function newItems(items: FeedItem[], lastId: string, max: number): FeedItem[] {
  const at = items.findIndex((it) => it.id === lastId);
  return at < 0 ? [] : items.slice(0, Math.min(at, max));
}

// excerpt shortens a summary for a notification body at a word boundary.
export function excerpt(s: string, max = 140): string {
  if (s.length <= max) return s;
  const cut = s.slice(0, max - 1);
  const space = cut.lastIndexOf(" ");
  return (space > max / 2 ? cut.slice(0, space) : cut).trimEnd() + "…";
}
