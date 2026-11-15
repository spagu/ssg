// A small HTML tokenizer for the preview (GO-188): it splits a page into
// text, start tags (name + attributes), end tags and a doctype, the way the
// HTML tokenizer of a browser finds where each tag ends — quoted attribute
// values may hold ">", comments and the content of script-like elements are
// skipped whole. It builds no tree and never needs to: the sanitizer
// (preview-sanitize.ts) writes every token back out itself, escaped, so what
// the browser parses is the sanitizer's own markup, not the page's.

export interface Attr { name: string; value: string } // value as written (character references not decoded)

export type Token =
  | { kind: "text"; text: string } // as written; the sanitizer escapes it
  | { kind: "start"; name: string; attrs: Attr[]; selfClosing: boolean; raw?: string } // raw: content of a raw-text element
  | { kind: "end"; name: string }
  | { kind: "doctype" };

// RAW_TEXT: elements whose content is text up to their own end tag, as a
// browser with scripting on reads them (the site's visitors do). PLAINTEXT
// never ends.
const RAW_TEXT = new Set(["script", "style", "xmp", "iframe", "noembed", "noframes", "noscript", "textarea", "title"]);

const SPACE = /[\t\n\f\r ]/;
const isLetter = (c: string | undefined): boolean => c !== undefined && /[A-Za-z]/.test(c);

// tokens yields the page's tokens one at a time. A tag cut off by the end of
// the input is dropped, as a browser drops it. One at a time because a page
// at the size limit made of tiny tags is millions of tokens: held in one
// array they outgrow a Worker's 128 MB (SR-21).
export function* tokens(html: string): Generator<Token> {
  let i = 0;
  while (i < html.length) {
    const lt = html.indexOf("<", i);
    if (lt < 0) {
      yield { kind: "text", text: html.slice(i) };
      return;
    }
    if (lt > i) yield { kind: "text", text: html.slice(i, lt) };
    const step: Token[] = [];
    i = markup(html, lt, step);
    yield* step;
  }
}

// tokenize: every token at once, for a short input.
export const tokenize = (html: string): Token[] => [...tokens(html)];

// markup reads what starts at html[at] === "<" and returns where it ends.
function markup(html: string, at: number, out: Token[]): number {
  const next = html[at + 1];
  if (html.startsWith("<!--", at)) return comment(html, at + 4);
  if (next === "!" || next === "?") {
    const end = until(html, at, ">");
    if (/^<!doctype/i.test(html.slice(at, at + 9))) out.push({ kind: "doctype" });
    return end;
  }
  if (next === "/") {
    if (isLetter(html[at + 2])) {
      const tag = readTag(html, at + 2);
      if (tag) out.push({ kind: "end", name: tag.name });
      return tag ? tag.end : html.length;
    }
    return until(html, at, ">"); // "</>" and "</3": nothing, as a browser reads them
  }
  if (!isLetter(next)) {
    out.push({ kind: "text", text: "<" });
    return at + 1;
  }
  const tag = readTag(html, at + 1);
  if (!tag) return html.length;
  const token: Token = { kind: "start", name: tag.name, attrs: tag.attrs, selfClosing: tag.selfClosing };
  out.push(token);
  if (tag.name === "plaintext") {
    token.raw = html.slice(tag.end);
    return html.length;
  }
  if (!RAW_TEXT.has(tag.name)) return tag.end;
  const close = closingTag(html, tag.end, tag.name);
  token.raw = html.slice(tag.end, close);
  return close;
}

// comment ends after "-->" (or "--!>"); "<!-->" and "<!--->" end at once.
function comment(html: string, from: number): number {
  if (html.startsWith(">", from)) return from + 1;
  if (html.startsWith("->", from)) return from + 2;
  const re = /--!?>/g;
  re.lastIndex = from;
  const m = re.exec(html);
  return m ? m.index + m[0].length : html.length;
}

const until = (html: string, from: number, ch: string): number => {
  const k = html.indexOf(ch, from);
  return k < 0 ? html.length : k + 1;
};

// closingTag: where "</name" followed by a space, "/" or ">" starts, or the
// end of the input (name is one of RAW_TEXT, never input).
function closingTag(html: string, from: number, name: string): number {
  const re = new RegExp(`</${name}[\\t\\n\\f\\r />]`, "gi");
  re.lastIndex = from;
  const m = re.exec(html);
  return m ? m.index : html.length;
}

// readTag reads a tag name and its attributes from html[from] (just after
// "<" or "</") up to its ">"; null when the input ends first.
function readTag(html: string, from: number): { name: string; attrs: Attr[]; selfClosing: boolean; end: number } | null {
  let i = from;
  while (i < html.length && !SPACE.test(html[i]) && html[i] !== "/" && html[i] !== ">") i++;
  const name = html.slice(from, i).toLowerCase();
  const attrs: Attr[] = [];
  const seen = new Set<string>();
  let selfClosing = false;
  while (i < html.length) {
    const c = html[i];
    if (c === ">") return { name, attrs, selfClosing, end: i + 1 };
    if (SPACE.test(c)) {
      i++;
      continue;
    }
    if (c === "/") {
      selfClosing = html[i + 1] === ">";
      i++;
      continue;
    }
    selfClosing = false;
    const a = readAttr(html, i);
    if (!seen.has(a.name)) attrs.push({ name: a.name, value: a.value }); // the first of two wins, as in a browser
    seen.add(a.name);
    i = a.end;
  }
  return null;
}

// readAttr reads one attribute starting at html[from] (not a space, "/" or ">").
function readAttr(html: string, from: number): { name: string; value: string; end: number } {
  let i = from + 1; // the first character is part of the name even when it is "="
  while (i < html.length && !SPACE.test(html[i]) && !"/>=".includes(html[i])) i++;
  const name = html.slice(from, i).toLowerCase();
  let j = i;
  while (j < html.length && SPACE.test(html[j])) j++;
  if (html[j] !== "=") return { name, value: "", end: i };
  j++;
  while (j < html.length && SPACE.test(html[j])) j++;
  const q = html[j];
  if (q === '"' || q === "'") {
    const close = html.indexOf(q, j + 1);
    if (close < 0) return { name, value: html.slice(j + 1), end: html.length };
    return { name, value: html.slice(j + 1, close), end: close + 1 };
  }
  let k = j;
  while (k < html.length && !SPACE.test(html[k]) && html[k] !== ">") k++;
  return { name, value: html.slice(j, k), end: k };
}
