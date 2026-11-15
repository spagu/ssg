// A theme's design tokens (css/tokens.css) as fields: the custom properties
// declared on :root for the light scheme and for the dark one, which of them
// are plain colours and font stacks, and the edit that changes one value in
// the file's text without touching anything else — comments, order, layout.
// Pure; the contract it relies on is in the README ("Theme tokens").
//
// Light: declarations in a rule whose selector is :root (or
// :root:not([data-theme="dark"])) outside a dark media query. Dark: in a rule
// inside @media (prefers-color-scheme: dark), or a rule for
// :root[data-theme="dark"]. A theme that states the dark values twice (for the
// system preference and for its own toggle) gets both copies changed at once.

import { parseHex } from "./contrast.js";

// blank replaces comments with spaces of the same length, so offsets found
// in the blanked text are offsets in the original.
const blank = (css) => css.replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, " "));

// schemeOf: "light", "dark" or null for a rule's selector inside `media`.
function schemeOf(selector, media) {
  const sel = selector.replace(/\s+/g, "");
  if (!sel.split(",").some((s) => s.startsWith(":root"))) return null;
  if (/data-theme=["']?dark/.test(sel) && !/:not\(\[data-theme=["']?dark/.test(sel)) return "dark";
  if (/prefers-color-scheme:\s*dark/.test(media)) return "dark";
  if (/data-theme=["']?light/.test(sel) && !/:not\(/.test(sel)) return "light";
  // Any other query (width, print) overrides a token for some screens; the
  // token itself is the value outside it.
  return /prefers-color-scheme:\s*light/.test(media) || !media ? "light" : null;
}

// declarations: every --name: value; of a light or dark :root rule, with the
// offsets of its value. A rule nested in anything but @media is skipped.
export function declarations(css) {
  const text = blank(css);
  const out = [];
  const stack = [];
  let from = 0;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (ch === "{") {
      stack.push(text.slice(from, i).trim());
      from = i + 1;
    } else if (ch === "}") {
      const selector = stack.pop() ?? "";
      const media = stack.filter((s) => s.startsWith("@media")).join(" ");
      const nested = stack.some((s) => !s.startsWith("@media"));
      const scheme = selector.startsWith("@") || nested ? null : schemeOf(selector, media);
      if (scheme) collect(text, css, from, i, scheme, out);
      from = i + 1;
    }
  }
  return out.sort((a, b) => a.start - b.start);
}

// collect reads the declarations of one rule body (text[from, to)).
function collect(text, css, from, to, scheme, out) {
  const body = text.slice(from, to);
  const re = /(--[A-Za-z0-9_-]+)\s*:([^;]*)(;|$)/g;
  for (let m; (m = re.exec(body)); ) {
    const lead = m[2].length - m[2].trimStart().length;
    const value = m[2].trim();
    const start = from + m.index + m[0].indexOf(":") + 1 + lead;
    out.push({ name: m[1], value: css.slice(start, start + value.length), start, end: start + value.length, scheme });
  }
}

const FONT = /font/i;
const NOT_FONT = /size|weight|height|spacing|style|feature|variation/i;

// kindOf: "colour" for a plain hex colour, "font" for a font stack, else null.
export function kindOf(name, value) {
  if (parseHex(value)) return "colour";
  if (FONT.test(name) && !NOT_FONT.test(name) && /[A-Za-z]/.test(value) && !/^var\(|^clamp\(|^calc\(/.test(value.trim())) return "font";
  return null;
}

// tokens: the fields — {name, kind, light, dark} for every colour and font
// token, in the order the file declares them. dark is null where the dark
// scheme does not restate the token (the light value applies there).
export function tokens(css) {
  const byName = new Map();
  for (const d of declarations(css)) {
    const t = byName.get(d.name) ?? { name: d.name, kind: null, light: null, dark: null };
    if (t[d.scheme] === null) t[d.scheme] = d.value;
    byName.set(d.name, t);
  }
  return [...byName.values()].map((t) => ({ ...t, kind: kindOf(t.name, t.light ?? t.dark) })).filter((t) => t.kind);
}

// resolve: a token's colour in a scheme, following var(--x, fallback) chains
// (a derived token is checked, not edited); null when it is not a colour.
export function resolve(all, name, scheme, seen = new Set()) {
  if (seen.has(name) || !all.has(name)) return null;
  seen.add(name);
  const t = all.get(name);
  return valueOf(all, scheme === "dark" ? t.dark ?? t.light : t.light ?? t.dark, scheme, seen);
}

function valueOf(all, v, scheme, seen) {
  if (parseHex(v)) return v.trim().toLowerCase();
  const m = /^var\(\s*(--[A-Za-z0-9_-]+)\s*(?:,\s*(.+))?\)$/.exec(v.trim());
  if (!m) return null;
  return resolve(all, m[1], scheme, seen) ?? (m[2] === undefined ? null : valueOf(all, m[2], scheme, seen));
}

// valueMap: every declared token by name ({light, dark}), colours or not —
// what resolve() reads.
export function valueMap(css) {
  const all = new Map();
  for (const d of declarations(css)) {
    const t = all.get(d.name) ?? { light: null, dark: null };
    if (t[d.scheme] === null) t[d.scheme] = d.value;
    all.set(d.name, t);
  }
  return all;
}

// setToken: the file's text with `name`'s value in `scheme` replaced by
// `value` (every copy of that scheme). A token the dark scheme does not
// restate cannot be changed for dark only: null, and the caller says so.
export function setToken(css, name, scheme, value) {
  const hits = declarations(css).filter((d) => d.name === name && d.scheme === scheme);
  if (!hits.length) return null;
  let out = css;
  for (const d of hits.reverse()) out = out.slice(0, d.start) + value + out.slice(d.end);
  return out;
}

// findTokensFile: the theme's tokens file among its file paths —
// <dir>/css/tokens.css first, else the shortest path ending in tokens.css.
export function findTokensFile(paths, dir) {
  if (paths.includes(`${dir}/css/tokens.css`)) return `${dir}/css/tokens.css`;
  const any = paths.filter((p) => /(^|\/)tokens\.css$/.test(p)).sort((a, b) => a.length - b.length || (a < b ? -1 : 1));
  return any[0] ?? null;
}
