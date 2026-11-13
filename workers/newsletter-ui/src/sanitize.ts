// An allowlist HTML sanitizer that needs no DOM (Workers have none).
//
// The safety argument does not rest on parsing HTML the way a browser does —
// a hand-written parser never will. It rests on the OUTPUT: the result is
// rebuilt from scratch out of a small set of tags and attributes, every text and
// attribute value is escaped, and every URL is decoded, parsed with the URL
// standard and checked by scheme. Whatever the parser misreads can only end up
// as escaped text or be dropped; it cannot become markup.
//
// Rules:
//   - kept tags: ALLOWED below, each with its own attributes; nothing else
//     (no style, class, id, on*…);
//   - dropped WITH their content: script, style, iframe, form, svg, math… (DROP)
//     — when the closing tag is missing, everything after the opening tag goes;
//   - dropped alone: void elements that load or submit (input, meta, link…);
//   - any other tag is unwrapped: the tag goes, its text stays;
//   - links: https: and mailto: only; images: https: only; anything else (a
//     relative URL, javascript:, data:, vbscript:…) unwraps the link and drops
//     the image;
//   - no link inside a link, nesting capped at MAX_DEPTH, comments dropped.

import { decodeEntities, escapeHtml } from "./render-entities";

export type HtmlNode = { type: "text"; text: string } | HtmlElement;
export interface HtmlElement {
  type: "el";
  tag: string;
  attrs: [string, string][];
  children: HtmlNode[];
}

const ALLOWED: Record<string, string[]> = {
  p: [], br: [], hr: [], h1: [], h2: [], h3: [], h4: [], h5: [], h6: [],
  strong: [], b: [], em: [], i: [], u: [], s: [], del: [], sup: [], sub: [], small: [],
  span: [], div: [], blockquote: [], pre: [], code: [],
  ul: [], ol: ["start"], li: [],
  a: ["href", "title"],
  img: ["src", "alt", "title", "width", "height"],
  table: [], thead: [], tbody: [], tr: [], th: ["colspan", "rowspan"], td: ["colspan", "rowspan"],
};
export const VOID = new Set(["br", "hr", "img"]);
const DROP = new Set([
  "script", "style", "iframe", "object", "applet", "form", "button", "select", "textarea", "option",
  "svg", "math", "template", "noscript", "noembed", "noframes", "frameset", "title", "xmp", "plaintext",
  "head", "video", "audio", "canvas", "dialog", "marquee",
]);
const DROP_VOID = new Set(["input", "meta", "link", "base", "embed", "frame", "source", "track", "param", "keygen", "isindex"]);
const MAX_DEPTH = 32;
const NUMERIC = new Set(["width", "height", "colspan", "rowspan", "start"]);

// safeUrl returns the URL to emit, or null. The URL standard strips tabs,
// newlines and leading/trailing control characters exactly as a browser does,
// so "java\tscript:" and " javascript:" are caught by the protocol check.
export function safeUrl(raw: string, schemes: string[]): string | null {
  let u: URL;
  try {
    u = new URL(decodeEntities(raw));
  } catch {
    return null; // relative or malformed: meaningless in a mail anyway
  }
  return schemes.includes(u.protocol) ? u.href : null;
}

function cleanAttrs(tag: string, raw: [string, string][]): [string, string][] | null {
  const allowed = ALLOWED[tag];
  const out: [string, string][] = [];
  for (const [name, value] of raw) {
    if (!allowed.includes(name) || out.some(([n]) => n === name)) continue;
    let v: string | null = decodeEntities(value);
    if (name === "href") v = safeUrl(value, ["https:", "mailto:"]);
    else if (name === "src") v = safeUrl(value, ["https:"]);
    else if (NUMERIC.has(name)) v = /^\d{1,4}$/.test(v) ? v : null;
    else v = v.slice(0, 500);
    if (v !== null) out.push([name, v]);
  }
  // A link without a usable target is just text; an image without one is nothing.
  if (tag === "a" && !out.some(([n]) => n === "href")) return null;
  if (tag === "img" && !out.some(([n]) => n === "src")) return null;
  return out;
}

// readTag reads the attributes of a tag whose name ended at `i`. Returns the
// index after ">", or -1 when the tag never closes (the rest is then dropped).
function readTag(s: string, i: number, attrs: [string, string][]): number {
  for (;;) {
    while (i < s.length && /[\s/]/.test(s[i])) i++;
    if (i >= s.length) return -1;
    if (s[i] === ">") return i + 1;
    const name = /[^\s/>][^\s/>=]*/y;
    name.lastIndex = i;
    const n = (name.exec(s) as RegExpExecArray)[0];
    i += n.length;
    while (i < s.length && /\s/.test(s[i])) i++;
    let value = "";
    if (s[i] === "=") {
      i++;
      while (i < s.length && /\s/.test(s[i])) i++;
      const q = s[i];
      if (q === '"' || q === "'") {
        const end = s.indexOf(q, i + 1);
        if (end < 0) return -1;
        value = s.slice(i + 1, end);
        i = end + 1;
      } else {
        const m = /[^\s>]*/y;
        m.lastIndex = i;
        value = (m.exec(s) as RegExpExecArray)[0];
        i += value.length;
      }
    }
    attrs.push([n.toLowerCase(), value]);
  }
}

// skipContent returns the index after the closing tag of a dropped element,
// or the end of the input when it has none.
function skipContent(s: string, i: number, tag: string): number {
  const close = new RegExp(`</${tag}(?=[\\s/>])`, "gi");
  close.lastIndex = i;
  const m = close.exec(s);
  if (!m) return s.length;
  const gt = s.indexOf(">", m.index);
  return gt < 0 ? s.length : gt + 1;
}

// sanitizeHtml parses untrusted HTML into a tree that holds only what the
// rules above allow. serialize turns it back into a string.
export function sanitizeHtml(s: string): HtmlNode[] {
  const root: HtmlElement = { type: "el", tag: "#root", attrs: [], children: [] };
  const stack: HtmlElement[] = [root];
  const text = (t: string) => t && stack[stack.length - 1].children.push({ type: "text", text: decodeEntities(t) });
  const tagName = /<(\/?)([A-Za-z][A-Za-z0-9-]*)/y;
  let i = 0;
  while (i < s.length) {
    const lt = s.indexOf("<", i);
    if (lt < 0) {
      text(s.slice(i));
      break;
    }
    text(s.slice(i, lt));
    i = lt;
    if (s.startsWith("<!--", i)) {
      const end = s.indexOf("-->", i + 4);
      i = end < 0 ? s.length : end + 3;
      continue;
    }
    if (s[i + 1] === "!" || s[i + 1] === "?") {
      const end = s.indexOf(">", i);
      i = end < 0 ? s.length : end + 1;
      continue;
    }
    tagName.lastIndex = i;
    const m = tagName.exec(s);
    if (!m) {
      text("<");
      i++;
      continue;
    }
    const attrs: [string, string][] = [];
    const end = readTag(s, i + m[0].length, attrs);
    if (end < 0) break; // an unterminated tag: nothing after it is trusted
    i = end;
    const tag = m[2].toLowerCase();
    if (m[1]) {
      // A closing tag closes the nearest open element of that name, and
      // everything opened inside it; a stray one is ignored.
      const at = stack.findLastIndex((e) => e.tag === tag);
      if (at > 0) stack.length = at;
      continue;
    }
    if (DROP.has(tag)) {
      i = skipContent(s, i, tag);
      continue;
    }
    if (DROP_VOID.has(tag) || !Object.hasOwn(ALLOWED, tag)) continue;
    if (tag === "a" && stack.some((e) => e.tag === "a")) continue;
    if (!VOID.has(tag) && stack.length > MAX_DEPTH) continue;
    const clean = cleanAttrs(tag, attrs);
    if (!clean) continue;
    const el: HtmlElement = { type: "el", tag, attrs: clean, children: [] };
    stack[stack.length - 1].children.push(el);
    if (!VOID.has(tag)) stack.push(el);
  }
  return root.children;
}

export function serialize(nodes: HtmlNode[]): string {
  let out = "";
  for (const n of nodes) {
    if (n.type === "text") {
      out += escapeHtml(n.text);
      continue;
    }
    const attrs = n.attrs.map(([k, v]) => ` ${k}="${escapeHtml(v)}"`).join("");
    out += VOID.has(n.tag) ? `<${n.tag}${attrs}>` : `<${n.tag}${attrs}>${serialize(n.children)}</${n.tag}>`;
  }
  return out;
}

// sanitize is sanitizeHtml + serialize, for callers that want a string.
export const sanitize = (html: string): string => serialize(sanitizeHtml(html));
