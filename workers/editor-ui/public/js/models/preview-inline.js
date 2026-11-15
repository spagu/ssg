// The preview's inline Markdown: emphasis, code, links, images, line breaks,
// component and shortcode calls — turned into vnodes (core/vnode.js), never
// into an HTML string. Whatever the text holds is a text node in the end, so
// "<script>" in an article shows as those characters. Two more rules keep a
// link or an image from doing anything a page of the panel must not:
//
//   href  only http(s), mailto, a fragment or a path on this site; anything
//         else (javascript:, data:, vbscript:…) is shown as plain text
//   src   an image pasted in this draft (its local blob address), or a path
//         on this site; an image from elsewhere is a labelled frame — the
//         panel loads nothing from another origin (CSP img-src 'self')
//
// The preview is approximate by design (README of the round, decision 10):
// the build is the truth. ctx = {t, images: Map(url → local address)}.

import { h } from "../core/vnode.js";

const SCHEME = /^([a-z][a-z0-9+.-]*):/i;

// safeHref: the address a preview link may carry, or null.
export function safeHref(url) {
  const bare = String(url).replace(/[\x00-\x20\x7f]/g, "");
  const m = SCHEME.exec(bare);
  if (!m) return bare.startsWith("//") ? null : String(url).trim();
  return /^(https?|mailto)$/i.test(m[1]) ? String(url).trim() : null;
}

// imageSource: {src} the preview may load, or {away: reason} when it may not.
export function imageSource(url, images) {
  const u = String(url).trim();
  if (images && images.has(u)) return { src: images.get(u) };
  if (u.startsWith("/") && !u.startsWith("//")) return { src: u };
  return { away: SCHEME.test(u.replace(/[\x00-\x20\x7f]/g, "")) || u.startsWith("//") ? "elsewhere" : "relative" };
}

// target: "[text](url "title")" from `at` (the "[") → {label, url, end} or null.
function target(src, at) {
  let depth = 0;
  let i = at;
  for (; i < src.length; i++) {
    if (src[i] === "\\") i++;
    else if (src[i] === "[") depth++;
    else if (src[i] === "]" && --depth === 0) break;
  }
  if (i >= src.length || src[i + 1] !== "(") return null;
  const m = /^\(\s*(<[^>\n]*>|[^\s()]*(?:\([^\s()]*\)[^\s()]*)*)(?:\s+"[^"]*")?\s*\)/.exec(src.slice(i + 1));
  if (!m) return null;
  return { label: src.slice(at + 1, i), url: m[1].replace(/^<|>$/g, ""), end: i + 1 + m[0].length };
}

// callAt: a component call {{< name … >}} / {{% name … %}} or a legacy
// shortcode {{name}} at the start of `s` → {kind, name, raw}, or null.
export function callAt(s) {
  let m = /^\{\{(?:<\s*(\/?[\w-]+)[^\n]*?>|%\s*(\/?[\w-]+)[^\n]*?%)\}\}/.exec(s);
  if (m) return { kind: "component", name: m[1] ?? m[2], raw: m[0] };
  m = /^\{\{\s*([\w-]+)[^{}\n]*\}\}/.exec(s);
  return m ? { kind: "shortcode", name: m[1], raw: m[0] } : null;
}

const frame = (ctx, kind, name, raw) =>
  h("span", { class: "pv-frame pv-inline" }, h("span", { class: "pv-label" }, ctx.t(`preview.${kind}`, { name })), " ", h("code", null, raw));

// imageNode: the image, or a frame saying why the preview does not show it.
export function imageNode(ctx, alt, url) {
  const s = imageSource(url, ctx.images);
  if (s.src) return h("img", { src: s.src, alt, loading: "lazy" });
  return h("span", { class: "pv-frame pv-inline" }, h("span", { class: "pv-label" }, ctx.t(`preview.image.${s.away}`)), " ", alt || url);
}

// EMPHASIS: [delimiter, tag], longest first.
const EMPHASIS = [["**", "strong"], ["__", "strong"], ["~~", "del"], ["*", "em"], ["_", "em"]];
const word = (c) => /[\p{L}\p{N}]/u.test(c ?? "");

// closer: where the emphasis opened at `at` with `d` closes, or -1.
function closer(src, at, d) {
  if (/\s/.test(src[at + d.length] ?? " ")) return -1;
  if (d[0] === "_" && word(src[at - 1])) return -1;
  for (let i = at + d.length + 1; i <= src.length - d.length; i++) {
    if (src[i] === "\\") { i++; continue; }
    // A single * or _ next to another one belongs to a ** or __ pair.
    const paired = d.length === 1 && (src[i + 1] === d || src[i - 1] === d);
    if (src.startsWith(d, i) && !/\s/.test(src[i - 1]) && !(d[0] === "_" && word(src[i + d.length])) && !paired) return i;
  }
  return -1;
}

// inline: vnodes for one block's text.
export function inline(src, ctx) {
  const out = [];
  let buf = "";
  const flush = () => (buf && out.push(buf), (buf = ""));
  const push = (node, end) => (flush(), out.push(node), end);
  for (let i = 0; i < src.length; ) {
    const c = src[i];
    const rest = src.slice(i);
    let m;
    if (c === "\\" && /[!-/:-@[-`{-~]/.test(src[i + 1] ?? "")) { buf += src[i + 1]; i += 2; continue; }
    if ((c === "\\" && src[i + 1] === "\n") || (m = /^ {2,}\n/.exec(rest))) { i = push(h("br"), i + (m ? m[0].length : 2)); continue; }
    if (c === "`" && (m = /^(`+)([\s\S]*?[^`])\1(?!`)/.exec(rest))) {
      const code = m[2].replace(/\n/g, " ");
      i = push(h("code", null, /^ .* $/.test(code) ? code.slice(1, -1) : code), i + m[0].length);
      continue;
    }
    if ((m = callAt(rest))) { i = push(frame(ctx, m.kind, m.name, m.raw), i + m.raw.length); continue; }
    if (c === "!" && src[i + 1] === "[" && (m = target(src, i + 1))) { i = push(imageNode(ctx, m.label, m.url), m.end); continue; }
    if (c === "[" && (m = target(src, i))) {
      const href = safeHref(m.url);
      const kids = inline(m.label, ctx);
      i = push(href === null ? h("span", { class: "pv-unsafe" }, kids) : h("a", { href, target: "_blank", rel: "noopener noreferrer" }, kids), m.end);
      continue;
    }
    if (c === "<" && (m = /^<((?:https?:\/\/|mailto:)[^\s<>]+)>/i.exec(rest))) { i = push(h("a", { href: m[1], target: "_blank", rel: "noopener noreferrer" }, m[1]), i + m[0].length); continue; }
    const em = EMPHASIS.find(([d]) => src.startsWith(d, i));
    const end = em ? closer(src, i, em[0]) : -1;
    if (end > 0) { i = push(h(em[1], null, inline(src.slice(i + em[0].length, end), ctx)), end + em[0].length); continue; }
    buf += c;
    i++;
  }
  flush();
  return out;
}
