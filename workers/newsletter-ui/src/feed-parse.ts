// Reading the site's feed (GO-130): RSS 2.0 and Atom, without a DOM and
// without dependencies. Workers have no DOMParser; the feeds read here are
// the ones ssg writes (`feed: true` = Atom at /feed.xml, `feeds:` may add RSS),
// so a small scanner is enough.
//
// The scanner is linear on purpose: it walks the text with indexOf and never
// uses a regular expression that can backtrack over the whole body, so a
// hostile feed (thousands of unclosed <item> tags) costs one pass, not n².
// Entities: the five XML ones and numeric references are decoded; anything a
// DOCTYPE declares is refused outright (see parseFeed), so "billion laughs"
// expansion can never happen — there is nothing that expands.

export interface FeedPost {
  guid: string; // RSS <guid> / Atom <id>, else the link
  title: string;
  link: string; // absolute, resolved against the feed's URL
  summary: string; // plain text, HTML removed
  published: string | null; // ISO 8601, or null when absent or unreadable
  language: string | null; // the item's, else the channel's; null when unknown
}

export class FeedError extends Error {}

const ENTITIES: Record<string, string> = { amp: "&", lt: "<", gt: ">", quot: '"', apos: "'" };
const LANG = /^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})?$/;

export function decodeEntities(s: string): string {
  return s.replace(/&(#x[0-9a-f]{1,6}|#[0-9]{1,7}|[a-z]{2,4});/gi, (m, e: string) => {
    if (e[0] === "#") {
      const hex = e[1] === "x" || e[1] === "X";
      const code = Number.parseInt(e.slice(hex ? 2 : 1), hex ? 16 : 10);
      return code > 0 && code <= 0x10ffff ? String.fromCodePoint(code) : m;
    }
    return ENTITIES[e.toLowerCase()] ?? m;
  });
}

// plain turns feed markup into one line of text: CDATA unwrapped, entities
// decoded, tags (HTML escaped inside a summary) removed, the entities of
// that HTML decoded, whitespace collapsed. `[^<>]` keeps the tag pattern
// linear on a body full of unmatched "<".
export function plain(s: string): string {
  let out = s.replace(/<!\[CDATA\[([\s\S]*?)\]\]>/g, "$1");
  out = decodeEntities(out);
  out = out.replace(/<[^<>]*>/g, " ");
  return decodeEntities(out).replace(/\s+/g, " ").trim();
}

interface Element {
  attrs: string; // the opening tag's attribute text
  inner: string; // "" for a self-closing tag
  end: number; // index just past the closing tag
}

// element finds the next <name …>…</name> at or after `from`. A closing tag
// inside a CDATA section does not count. Returns null when there is no
// opening tag; throws FeedError when one is never closed.
export function element(text: string, name: string, from = 0): Element | null {
  const open = new RegExp(`<${name.replace(":", "\\:")}(?=[\\s/>])`, "g");
  open.lastIndex = from;
  const m = open.exec(text);
  if (!m) return null;
  const tagEnd = text.indexOf(">", m.index);
  if (tagEnd < 0) throw new FeedError(`unterminated <${name}> tag`);
  const attrs = text.slice(m.index + name.length + 1, tagEnd);
  if (attrs.endsWith("/")) return { attrs: attrs.slice(0, -1), inner: "", end: tagEnd + 1 };
  const closeTag = `</${name}>`;
  let i = tagEnd + 1;
  for (;;) {
    const close = text.indexOf(closeTag, i);
    if (close < 0) throw new FeedError(`<${name}> is never closed`);
    // Searched only up to `close`: an unbounded search would rescan the rest
    // of the body for every element.
    const cdata = text.slice(i, close).indexOf("<![CDATA[");
    if (cdata < 0) return { attrs, inner: text.slice(tagEnd + 1, close), end: close + closeTag.length };
    const cdataEnd = text.indexOf("]]>", i + cdata);
    if (cdataEnd < 0) throw new FeedError("a CDATA section is never closed");
    i = cdataEnd + 3;
  }
}

function elements(text: string, name: string): Element[] {
  const out: Element[] = [];
  for (let e = element(text, name); e; e = element(text, name, e.end)) out.push(e);
  return out;
}

const field = (block: string, name: string): string => plain(element(block, name)?.inner ?? "");

const attr = (attrs: string, name: string): string | null => {
  const m = new RegExp(`(?:^|\\s)${name}\\s*=\\s*(?:"([^"]*)"|'([^']*)')`).exec(attrs);
  return m ? decodeEntities(m[1] ?? m[2]) : null;
};

const language = (raw: string | null): string | null => (raw && LANG.test(raw.trim()) ? raw.trim() : null);

function isoDate(raw: string): string | null {
  const t = Date.parse(raw);
  return Number.isNaN(t) ? null : new Date(t).toISOString();
}

function absolute(href: string, base: string): string {
  try {
    return href ? new URL(href, base).href : "";
  } catch {
    return "";
  }
}

// atomLink prefers rel="alternate", which is also what a link without rel means.
function atomLink(block: string): string {
  for (const m of block.matchAll(/<link\b([^<>]*)>/g)) {
    const rel = attr(m[1], "rel") ?? "alternate";
    const href = attr(m[1], "href");
    if (href && rel === "alternate") return href;
  }
  return "";
}

function atom(root: Element, base: string): FeedPost[] {
  const feedLang = language(attr(root.attrs, "xml:lang"));
  return elements(root.inner, "entry").map((e) => {
    const link = absolute(atomLink(e.inner), base);
    return {
      guid: field(e.inner, "id") || link,
      title: field(e.inner, "title"),
      link,
      summary: field(e.inner, "summary") || field(e.inner, "content"),
      published: isoDate(field(e.inner, "published") || field(e.inner, "updated")),
      language: language(attr(e.attrs, "xml:lang")) ?? feedLang,
    };
  });
}

function rss(channel: Element, base: string): FeedPost[] {
  const channelLang = language(field(channel.inner, "language"));
  return elements(channel.inner, "item").map((e) => {
    const link = absolute(field(e.inner, "link"), base);
    return {
      guid: field(e.inner, "guid") || link,
      title: field(e.inner, "title"),
      link,
      summary: field(e.inner, "description") || field(e.inner, "content:encoded"),
      published: isoDate(field(e.inner, "pubDate") || field(e.inner, "dc:date")),
      language: language(field(e.inner, "dc:language")) ?? channelLang,
    };
  });
}

// parseFeed reads an RSS 2.0 or Atom document. `base` resolves relative links.
// Items without a guid and a link cannot be told apart and are dropped.
// Throws FeedError for anything it will not read.
export function parseFeed(text: string, base: string): FeedPost[] {
  if (/<!ENTITY/i.test(text)) throw new FeedError("the feed declares XML entities; they are not expanded, so it is not read");
  const feed = element(text, "feed");
  let posts: FeedPost[];
  if (feed) {
    posts = atom(feed, base);
  } else {
    const channel = element(text, "rss") && element(text, "channel");
    if (!channel) throw new FeedError("not an RSS 2.0 or Atom feed");
    posts = rss(channel, base);
  }
  return posts.filter((p) => p.guid);
}

// excerpt shortens text at a word boundary, with an ellipsis.
export function excerpt(s: string, max: number): string {
  if (s.length <= max) return s;
  const cut = s.slice(0, max - 1);
  const space = cut.lastIndexOf(" ");
  return (space > max / 2 ? cut.slice(0, space) : cut).trimEnd() + "…";
}
