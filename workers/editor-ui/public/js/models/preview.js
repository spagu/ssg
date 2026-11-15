// The preview beside the editor: Markdown blocks turned into vnodes — the
// common subset ssg renders (headings, paragraphs, lists with task boxes,
// quotes, fenced code, tables, rules) plus labelled frames for what the
// preview cannot draw: a component or shortcode call (the build fills it in)
// and a block of raw HTML, which is shown as its text and never becomes
// live markup. Inline text goes through preview-inline.js. Approximate on
// purpose — the caption under the preview says so; the build is the truth.
//
// renderPreview(markdown, {t, images, title}) → [vnode]

import { h } from "../core/vnode.js";
import { callAt, inline } from "./preview-inline.js";
import { callArgs } from "./widget-call.js";

const FENCE = /^ {0,3}(`{3,}|~{3,})[ \t]*([^\s`]*)/;
const HEADING = /^ {0,3}(#{1,6})(?:[ \t]+|$)(.*?)(?:[ \t]+#+)?[ \t]*$/;
const RULE = /^ {0,3}([-*_])(?:[ \t]*\1){2,}[ \t]*$/;
const QUOTE = /^ {0,3}>[ \t]?/;
const ITEM = /^( {0,3})([-*+]|\d{1,9}[.)])([ \t]+|$)/;
const HTML = /^ {0,3}<(?:[A-Za-z][\w-]*[\s/>]|[A-Za-z][\w-]*$|!--|\/[A-Za-z])/;
const DELIM = /^\s*\|?\s*:?-+:?\s*(?:\|\s*:?-+:?\s*)*\|?\s*$/;
const blank = (l) => l.trim() === "";
const opens = (l) => FENCE.test(l) || HEADING.test(l) || RULE.test(l) || QUOTE.test(l) || HTML.test(l) || /^ {0,3}(?:[-*+]|1[.)])[ \t]+\S/.test(l);

// labelled: a frame with what it is; a component call lists its values
// (GO-174) — the template is never run here, the build renders it.
const labelled = (ctx, key, vars, body) => {
  const args = key === "preview.component" ? callArgs(body) : [];
  return h("div", { class: "pv-frame", role: "note" }, h("span", { class: "pv-label" }, ctx.t(key, vars)),
    args.length ? h("dl", { class: "pv-args" }, args.map(([k, v]) => [h("dt", null, k), h("dd", null, v)])) : h("pre", null, h("code", null, body)));
};

// cells: a table row's cells (outer pipes dropped, \| kept as a pipe).
const cells = (line) => line.trim().replace(/^\|/, "").replace(/(?<!\\)\|$/, "").split(/(?<!\\)\|/).map((c) => c.trim().replace(/\\\|/g, "|"));
const ALIGN = (d) => (/^:-+:$/.test(d) ? "center" : /-:$/.test(d) ? "right" : /^:/.test(d) ? "left" : null);

function tableNode(rows, delim, ctx) {
  const align = cells(delim).map(ALIGN);
  const cell = (tag, text, i) => h(tag, align[i] ? { style: { "text-align": align[i] } } : null, inline(text, ctx));
  const head = cells(rows[0]);
  return h("div", { class: "table-scroll" }, h("table", { class: "pv-table" },
    h("thead", null, h("tr", null, head.map((c, i) => cell("th", c, i)))),
    rows.length > 1 ? h("tbody", null, rows.slice(1).map((r) => h("tr", null, head.map((_, i) => cell("td", cells(r)[i] ?? "", i))))) : null));
}

// listNode: the list starting at lines[i] → {node, next}.
function listNode(lines, i, ctx) {
  const first = ITEM.exec(lines[i]);
  const ordered = /\d/.test(first[2]);
  const items = [];
  let tight = true;
  // same: the line is an item of this list (same kind and delimiter).
  const same = (line) => {
    const m = ITEM.exec(line);
    return !!m && /\d/.test(m[2]) === ordered && m[2].slice(-1) === first[2].slice(-1);
  };
  while (i < lines.length && same(lines[i])) {
    const m = ITEM.exec(lines[i]);
    const width = m[1].length + m[2].length + (m[3].length || 1);
    const body = [lines[i].slice(Math.min(width, lines[i].length))];
    i++;
    while (i < lines.length) {
      const l = lines[i];
      if (blank(l)) {
        // A blank line inside an item (more of it follows, indented) or
        // between two items makes the list loose; anything else ends it.
        const next = lines[i + 1] ?? "";
        if (blank(next) || !(next.startsWith(" ".repeat(width)) || same(next))) break;
        tight = false;
        i++;
        if (!next.startsWith(" ".repeat(width))) break;
        body.push("");
        continue;
      }
      if (l.startsWith(" ".repeat(width))) body.push(l.slice(width));
      else if (!ITEM.test(l) && !opens(l) && !blank(body[body.length - 1])) body.push(l);
      else break;
      i++;
    }
    items.push(body);
  }
  const start = ordered ? Number.parseInt(first[2], 10) : 1;
  const li = (body) => {
    const task = /^\[([ xX])\][ \t]+/.exec(body[0]);
    const text = task ? [body[0].slice(task[0].length), ...body.slice(1)] : body;
    const kids = blocks(text, ctx);
    const flat = tight ? kids.flatMap((k) => (k.tag === "p" ? k.children : [k])) : kids;
    return h("li", task ? { class: "task" } : null, task ? h("input", { type: "checkbox", disabled: true, checked: task[1] !== " ", "aria-label": ctx.t(task[1] === " " ? "preview.taskOpen" : "preview.taskDone") }) : null, task ? " " : null, flat);
  };
  return { node: h(ordered ? "ol" : "ul", start !== 1 ? { start } : null, items.map(li)), next: i };
}

// blocks: the vnodes of a run of lines.
export function blocks(lines, ctx) {
  const out = [];
  for (let i = 0; i < lines.length; ) {
    const line = lines[i];
    let m;
    if (blank(line)) { i++; continue; }
    if ((m = FENCE.exec(line))) {
      const close = new RegExp(`^ {0,3}${m[1][0]}{${m[1].length},}[ \\t]*$`);
      let j = i + 1;
      while (j < lines.length && !close.test(lines[j])) j++;
      out.push(h("pre", { class: "pv-code", "data-lang": m[2] || null }, h("code", null, lines.slice(i + 1, j).join("\n"))));
      i = j + 1;
    } else if ((m = HEADING.exec(line))) {
      out.push(h(`h${m[1].length}`, null, inline(m[2], ctx)));
      i++;
    } else if (RULE.test(line)) {
      out.push(h("hr"));
      i++;
    } else if (QUOTE.test(line)) {
      const inner = [];
      while (i < lines.length && QUOTE.test(lines[i])) inner.push(lines[i++].replace(QUOTE, ""));
      out.push(h("blockquote", null, blocks(inner, ctx)));
    } else if (ITEM.test(line)) {
      const list = listNode(lines, i, ctx);
      out.push(list.node);
      i = list.next;
    } else if ((m = callAt(line.trim())) && m.raw === line.trim()) {
      out.push(labelled(ctx, `preview.${m.kind}`, { name: m.name }, m.raw));
      i++;
    } else if (HTML.test(line)) {
      const raw = [];
      while (i < lines.length && !blank(lines[i])) raw.push(lines[i++]);
      out.push(labelled(ctx, "preview.html", {}, raw.join("\n")));
    } else if (line.includes("|") && DELIM.test(lines[i + 1] ?? "") && (lines[i + 1].includes("|") || line.trim().startsWith("|"))) {
      const rows = [line];
      const delim = lines[i + 1];
      i += 2;
      while (i < lines.length && !blank(lines[i]) && lines[i].includes("|")) rows.push(lines[i++]);
      out.push(tableNode(rows, delim, ctx));
    } else {
      const para = [line];
      i++;
      while (i < lines.length && !blank(lines[i]) && !opens(lines[i]) && !/^ {0,3}(=+|-+)[ \t]*$/.test(lines[i])) para.push(lines[i++]);
      const setext = /^ {0,3}(=+|-+)[ \t]*$/.exec(lines[i] ?? "");
      if (setext) i++;
      out.push(h(setext ? (setext[1][0] === "=" ? "h1" : "h2") : "p", null, inline(para.join("\n").trim(), ctx)));
    }
  }
  return out;
}

export function renderPreview(markdown, ctx) {
  const lines = String(markdown).replace(/\r\n?/g, "\n").split("\n");
  return [ctx.title ? h("h1", { class: "pv-title" }, ctx.title) : null, ...blocks(lines, ctx)];
}
