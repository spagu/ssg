// A small Markdown renderer for campaign bodies: no dependencies, a deliberately
// short list of features, and its output always goes through the sanitizer
// (render.ts) like HTML bodies do — a bug here can make a mail look wrong but
// cannot let markup through.
//
// Supported (and nothing else):
//   blocks   # … ###### headings (ATX, a space after the #s)
//            paragraphs; a line ending in two spaces or "\" is a hard break
//            > blockquotes (every quoted line starts with ">"; may hold any block)
//            - * + bullet lists and 1. 1) numbered lists — one level; an indented
//            line continues the item above, a blank line between items is allowed
//            ``` or ~~~ fenced code (the info string is ignored)
//            --- *** ___ horizontal rules
//   inline   **strong** __strong__ *em* _em_ ~~strike~~ `code`
//            [text](https://… "title")  ![alt](https://… "title")
//            <https://…> <mailto:…> and bare https:// URLs become links
//            \ escapes any punctuation character
// Not supported: setext headings ("Title\n====="; a "---" line is a rule),
// indented code blocks, nested lists, tables, reference links, footnotes.
// Raw HTML is shown as text — write the body as HTML to use HTML.

import { escapeHtml } from "./render-entities";

const FENCE = /^ {0,3}(`{3,}|~{3,})/;
const HEADING = /^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$/;
const RULE = /^ {0,3}([-*_])(?:[ \t]*\1){2,}[ \t]*$/;
const QUOTE = /^ {0,3}> ?/;
const ITEM = /^ {0,3}(?:([-*+])|(\d{1,9})[.)])[ \t]+(.*)$/;

const isBlockStart = (line: string): boolean =>
  FENCE.test(line) || HEADING.test(line) || RULE.test(line) || QUOTE.test(line) || ITEM.test(line);

export function markdownToHtml(src: string): string {
  return blocks(src.replace(/\r\n?/g, "\n").replace(/\0/g, "").split("\n")).join("\n");
}

function blocks(lines: string[]): string[] {
  const out: string[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) {
      i++;
      continue;
    }
    const fence = FENCE.exec(line);
    if (fence) {
      const close = new RegExp(`^ {0,3}${fence[1][0] === "`" ? "`" : "~"}{${fence[1].length},}[ \\t]*$`);
      const code: string[] = [];
      for (i++; i < lines.length && !close.test(lines[i]); i++) code.push(lines[i]);
      i++; // the closing fence (or the end of the body)
      out.push(`<pre><code>${escapeHtml(code.join("\n"))}</code></pre>`);
      continue;
    }
    const heading = HEADING.exec(line);
    if (heading) {
      const n = heading[1].length;
      out.push(`<h${n}>${inline(heading[2] ?? "")}</h${n}>`);
      i++;
      continue;
    }
    if (RULE.test(line)) {
      out.push("<hr>");
      i++;
      continue;
    }
    if (QUOTE.test(line)) {
      const inner: string[] = [];
      for (; i < lines.length && QUOTE.test(lines[i]); i++) inner.push(lines[i].replace(QUOTE, ""));
      out.push(`<blockquote>${blocks(inner).join("\n")}</blockquote>`);
      continue;
    }
    if (ITEM.test(line)) {
      i = list(lines, i, out);
      continue;
    }
    const para: string[] = [];
    for (; i < lines.length && lines[i].trim() && !isBlockStart(lines[i]); i++) para.push(lines[i]);
    out.push(`<p>${paragraph(para)}</p>`);
  }
  return out;
}

// list consumes one list starting at lines[i] and returns the next index.
function list(lines: string[], i: number, out: string[]): number {
  const first = ITEM.exec(lines[i]) as RegExpExecArray;
  const ordered = first[2] !== undefined;
  const items: string[][] = [];
  while (i < lines.length) {
    const m = ITEM.exec(lines[i]);
    if (m && (m[2] !== undefined) === ordered) {
      items.push([m[3]]);
      i++;
    } else if (lines[i].trim() && /^[ \t]/.test(lines[i]) && !m) {
      items[items.length - 1].push(lines[i].trim()); // continuation
      i++;
    } else if (!lines[i].trim() && i + 1 < lines.length && ITEM.test(lines[i + 1])) {
      i++; // a blank line between two items
    } else {
      break;
    }
  }
  const start = ordered && first[2] !== "1" ? ` start="${Number(first[2])}"` : "";
  const tag = ordered ? "ol" : "ul";
  out.push(`<${tag}${start}>${items.map((it) => `<li>${paragraph(it)}</li>`).join("")}</${tag}>`);
  return i;
}

// paragraph renders lines of inline text, honouring hard breaks.
function paragraph(lines: string[]): string {
  const segments: string[] = [];
  let cur: string[] = [];
  lines.forEach((line, n) => {
    const hard = n < lines.length - 1 && /( {2,}|\\)$/.test(line);
    cur.push(hard ? line.replace(/( {2,}|\\)$/, "") : line.trim());
    if (hard) {
      segments.push(cur.join("\n"));
      cur = [];
    }
  });
  segments.push(cur.join("\n"));
  return segments.map((s) => inline(s.trim())).join("<br>\n");
}

const ATTR_TITLE = (t: string | undefined) => (t ? ` title="${escapeHtml(t)}"` : "");
const LINK = /(!?)\[([^\]]*)\]\(\s*<?([^\s<>()]+)>?(?:\s+"([^"]*)")?\s*\)/g;

// inline renders emphasis, code, links and images. Pieces that must not be
// touched again (code, URLs) are set aside as \0n\0 placeholders and put back
// at the end, so "_" inside a URL never turns into emphasis.
export function inline(src: string): string {
  const held: string[] = [];
  const hold = (html: string) => `\0${held.push(html) - 1}\0`;
  let s = src.replace(/\\([!-/:-@[-`{-~])/g, (_, c: string) => hold(escapeHtml(c)));
  s = s.replace(/(`+)([^`]|[^`][\s\S]*?[^`])\1(?!`)/g, (_, _t, code: string) => hold(`<code>${escapeHtml(code.trim())}</code>`));
  s = s.replace(LINK, (_, bang: string, text: string, url: string, title?: string) =>
    bang
      ? hold(`<img src="${escapeHtml(url)}" alt="${escapeHtml(text)}"${ATTR_TITLE(title)}>`)
      : hold(`<a href="${escapeHtml(url)}"${ATTR_TITLE(title)}>${emphasis(escapeHtml(text))}</a>`),
  );
  s = s.replace(/<((?:https|mailto):[^\s<>]+)>/gi, (_, url: string) => hold(`<a href="${escapeHtml(url)}">${escapeHtml(url)}</a>`));
  s = s.replace(/\bhttps:\/\/[^\s<>\0]*[^\s<>\0.,;:!?'")\]]/gi, (url) => hold(`<a href="${escapeHtml(url)}">${escapeHtml(url)}</a>`));
  s = emphasis(escapeHtml(s));
  // Placeholders can nest (a code span inside link text): restore until none.
  for (let n = 0; n < 4 && s.includes("\0"); n++) s = s.replace(/\0(\d+)\0/g, (_, k: string) => held[Number(k)]);
  return s;
}

function emphasis(s: string): string {
  return s
    .replace(/(\*\*|__)(?=\S)([\s\S]*?\S)\1/g, "<strong>$2</strong>")
    .replace(/~~(?=\S)([\s\S]*?\S)~~/g, "<del>$1</del>")
    .replace(/\*(?=\S)([\s\S]*?\S)\*/g, "<em>$1</em>")
    .replace(/(^|[^\p{L}\p{N}_])_(?=\S)([\s\S]*?\S)_(?![\p{L}\p{N}_])/gu, "$1<em>$2</em>");
}
