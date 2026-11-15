// The preview's page, made inert (GO-188). The preview is served from the
// panel's own origin — the origin of the `auth` session — so nothing of the
// page may run there. Three layers, each enough on its own for scripts:
//   1. this file: every token is written back out by us, escaped; no script
//      element, no on* attribute, no javascript:/vbscript: URL, no meta
//      refresh, no frame, no <base> but ours survives;
//   2. the response's CSP (preview-entry.ts): script-src 'none' and `sandbox`;
//   3. the panel's <iframe sandbox> without allow-scripts / allow-same-origin.
// What stays is what makes the page look like itself: its elements, classes,
// inline styles, stylesheets, images and fonts.

import { tokens } from "./html-tokens";
import type { Attr, Token } from "./html-tokens";

// DROP_WITH_CONTENT: never shown, content included. The raw-text ones arrive
// with their content in the token; template and object are counted.
const DROP_WITH_CONTENT = new Set(["script", "noscript", "iframe", "noembed", "noframes", "xmp", "plaintext", "template", "object", "applet"]);
// DROP_TAG: the tag goes, what it encloses stays.
const DROP_TAG = new Set(["embed", "frame", "frameset", "base", "portal", "fencedframe", "param"]);
const NAME = /^[a-z][a-z0-9-]*$/;
const ATTR_NAME = /^[a-z_:][a-z0-9_:.-]*$/;
const URL_ATTRS = new Set(["href", "xlink:href", "action", "formaction"]); // where a data: URL would be a document
const KEEP_REL = /(^|\s)(stylesheet|preload|icon)(\s|$)/i;

export interface PreviewPage {
  baseHref: string; // the page's own address: relative URLs resolve as on the site
  scheme: "light" | "dark" | null; // forced on <html data-theme>, null = the page's own
  css: string; // the token overrides (preview-tokens.ts), already validated
}

// sanitize returns the page as inert markup with the preview's base and
// stylesheet injected at the top of <head>.
export function sanitize(html: string, page: PreviewPage): string {
  const out = writer();
  let injected = false;
  let skip: string | null = null;
  let depth = 0;
  let htmlSeen = false;
  const inject = (): void => {
    if (injected) return;
    injected = true;
    out.push(`<base href="${escapeAttr(page.baseHref)}"><style>${page.css}</style>`);
  };
  for (const t of tokens(html)) {
    if (skip) {
      if (t.kind === "start" && t.name === skip) depth++;
      if (t.kind === "end" && t.name === skip && --depth === 0) skip = null;
      continue;
    }
    if (t.kind === "doctype") {
      out.push("<!doctype html>");
    } else if (t.kind === "text") {
      out.push(escapeText(t.text));
    } else if (t.kind === "end") {
      if (NAME.test(t.name) && !DROP_TAG.has(t.name) && !DROP_WITH_CONTENT.has(t.name)) out.push(`</${t.name}>`);
    } else if (DROP_WITH_CONTENT.has(t.name)) {
      if (t.raw === undefined) [skip, depth] = [t.name, 1];
    } else if (NAME.test(t.name) && !DROP_TAG.has(t.name) && kept(t)) {
      // A page that leaves out <html> still gets the forced scheme.
      if (!htmlSeen && t.name !== "html" && page.scheme) out.push(`<html data-theme="${page.scheme}">`);
      htmlSeen = true;
      if (t.name !== "html" && t.name !== "head") inject();
      out.push(startTag(t, page.scheme));
      if (t.name === "head") inject();
      if (t.raw !== undefined) out.push(rawContent(t.name, t.raw));
    }
  }
  inject(); // a page with no element at all still gets the base
  return out.done();
}

// writer collects the page in chunks of joined pieces: a page of tiny tokens
// would otherwise hold millions of small strings until the end (SR-21).
function writer(): { push: (s: string) => void; done: () => string } {
  const chunks: string[] = [];
  let parts: string[] = [];
  return {
    push(s) {
      parts.push(s);
      if (parts.length < 4096) return;
      chunks.push(parts.join(""));
      parts = [];
    },
    done: () => chunks.join("") + parts.join(""),
  };
}

// kept: false for the elements dropped by what they say, not by name — a
// meta that refreshes or navigates, a link that is not a stylesheet, a
// preload or an icon.
function kept(t: Extract<Token, { kind: "start" }>): boolean {
  if (t.name === "meta") return !t.attrs.some((a) => a.name === "http-equiv");
  if (t.name === "link") return KEEP_REL.test(t.attrs.find((a) => a.name === "rel")?.value ?? "");
  return true;
}

// startTag writes the tag back with only safe attributes; <html> gets the
// forced scheme instead of its own data-theme.
function startTag(t: Extract<Token, { kind: "start" }>, scheme: PreviewPage["scheme"]): string {
  let attrs = t.attrs.filter(safeAttr);
  if (t.name === "html" && scheme) attrs = [...attrs.filter((a) => a.name !== "data-theme"), { name: "data-theme", value: scheme }];
  const written = attrs.map((a) => ` ${a.name}="${escapeAttr(a.value)}"`).join("");
  return `<${t.name}${written}${t.selfClosing ? " /" : ""}>`;
}

// safeAttr: no event handler, no inline document, no script URL — anywhere,
// since srcset, style and SVG animation values carry URLs too.
export function safeAttr(a: Attr): boolean {
  if (!ATTR_NAME.test(a.name) || a.name.startsWith("on") || a.name === "srcdoc" || a.name === "ping") return false;
  const v = normalized(a.value);
  if (/(java|vb|live)script:/.test(v)) return false;
  return !(URL_ATTRS.has(a.name) && v.startsWith("data:"));
}

// rawContent: a style element's CSS as written unless it holds "<" (inside
// <svg> a browser reads a style's content as markup, so "<" could start a
// tag) or a script URL; title and textarea text escaped.
function rawContent(name: string, raw: string): string {
  if (name !== "style") return escapeText(raw);
  return raw.includes("<") || /(java|vb|live)script:/.test(normalized(raw)) ? "" : raw;
}

// normalized: a value as a URL parser would read its scheme — character
// references that can produce ASCII decoded, then every space, control and
// invisible character removed, lowercased. It only decides what to drop: a
// stricter reading drops more, never less.
export function normalized(value: string): string {
  const decoded = value.replace(/&(#[xX][0-9a-fA-F]+|#[0-9]+|[A-Za-z][A-Za-z0-9]*);?/g, (m, ref: string) => decodeRef(ref) ?? m);
  return decoded.replace(/[\u0000-\u0020\u007f-\u00a0\u00ad\u200b-\u200f\u2028-\u202e\u2060-\u2064\ufeff]/g, "").toLowerCase();
}

// ASCII: the named character references whose value is ASCII (WHATWG list),
// the only ones that can spell a scheme. Others are left as written.
const ASCII: Record<string, string> = {
  Tab: "\t", NewLine: "\n", excl: "!", quot: '"', QUOT: '"', num: "#", dollar: "$", percnt: "%", amp: "&", AMP: "&", apos: "'",
  lpar: "(", rpar: ")", ast: "*", midast: "*", plus: "+", comma: ",", period: ".", sol: "/", colon: ":", semi: ";", lt: "<", LT: "<",
  equals: "=", gt: ">", GT: ">", quest: "?", commat: "@", lsqb: "[", lbrack: "[", bsol: "\\", rsqb: "]", rbrack: "]", Hat: "^",
  lowbar: "_", UnderBar: "_", grave: "`", DiacriticalGrave: "`", lcub: "{", lbrace: "{", verbar: "|", vert: "|", VerticalLine: "|",
  rcub: "}", rbrace: "}", fjlig: "fj",
};

function decodeRef(ref: string): string | null {
  if (ref[0] !== "#") return Object.hasOwn(ASCII, ref) ? ASCII[ref] : null;
  const code = ref[1] === "x" || ref[1] === "X" ? parseInt(ref.slice(2), 16) : parseInt(ref.slice(1), 10);
  return code > 0 && code <= 0x10ffff ? String.fromCodePoint(code) : "\ufffd";
}

const escapeText = (s: string): string => s.replace(/</g, "&lt;").replace(/>/g, "&gt;");
const escapeAttr = (s: string): string => s.replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
