// The plain-text part of a campaign mail, generated from the SANITIZED tree —
// the same content the HTML part shows, so the two can never disagree about
// what was allowed through. Links become "text (url)", list items get "- " or
// "1. ", quotes get "> ", rules become "----", preformatted text keeps its
// spacing. An admin who wants different wording sets text_override instead.

import type { HtmlNode } from "./sanitize";

const BLOCK = new Set(["p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "table", "ul", "ol"]);

const block = (s: string): string => `\n\n${s.trim()}\n\n`;

function walk(nodes: HtmlNode[], pre: boolean): string {
  let out = "";
  for (const n of nodes) {
    if (n.type === "text") {
      out += pre ? n.text : n.text.replace(/\s+/g, " ");
      continue;
    }
    const inner = () => walk(n.children, pre || n.tag === "pre");
    const attr = (name: string) => n.attrs.find(([k]) => k === name)?.[1] ?? "";
    switch (n.tag) {
      case "br":
        out += "\n";
        break;
      case "hr":
        out += block("----");
        break;
      case "img":
        out += attr("alt");
        break;
      case "a": {
        const href = attr("href");
        const text = inner().trim();
        const bare = href.startsWith("mailto:") ? href.slice(7) : href;
        out += !text || text === href || text === bare ? href : `${text} (${href})`;
        break;
      }
      case "pre":
        out += `\n\n${inner().replace(/^\n+|\s+$/g, "")}\n\n`;
        break;
      case "blockquote":
        out += block(tidy(inner()).split("\n").map((l) => (l ? `> ${l}` : ">")).join("\n"));
        break;
      case "li": {
        const ordered = n.attrs.find(([k]) => k === "#n");
        out += `\n${ordered ? ordered[1] + "." : "-"} ${tidy(inner())}`;
        break;
      }
      case "ol": {
        let k = Number(attr("start") || 1);
        const items = n.children.map((c) => (c.type === "el" && c.tag === "li" ? { ...c, attrs: [...c.attrs, ["#n", String(k++)] as [string, string]] } : c));
        out += block(walk(items, pre));
        break;
      }
      case "tr":
        out += `\n${inner().trim()}`;
        break;
      case "td":
      case "th":
        out += `${inner().trim()}  `;
        break;
      default:
        out += BLOCK.has(n.tag) ? block(inner()) : inner();
    }
  }
  return out;
}

// tidy trims each line's trailing spaces and the spaces that whitespace
// collapsing leaves at the start of a line, and caps blank runs at one line.
function tidy(s: string): string {
  return s
    .replace(/[ \t]+\n/g, "\n")
    .replace(/\n {1}(?=\S)/g, "\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

export function htmlToText(nodes: HtmlNode[]): string {
  return tidy(walk(nodes, false));
}
