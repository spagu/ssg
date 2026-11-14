// GO-146: every colour the panel draws comes from css/tokens.css, so a site
// theme (/core/theme.css, loaded right after it) reaches charts, buttons,
// links and focus rings — no colour written anywhere else can escape it.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

const PUBLIC = new URL("../public/", import.meta.url).pathname;
const files = (dir: string): string[] => readdirSync(dir).flatMap((f) => (statSync(join(dir, f)).isDirectory() ? files(join(dir, f)) : [join(dir, f)]));
const css = files(join(PUBLIC, "css")).filter((f) => f.endsWith(".css") && !f.endsWith("tokens.css"));
// The modules that draw (charts, views, the shell): no colour in them either.
const js = files(join(PUBLIC, "js")).filter((f) => !f.includes("/strings/"));
const tokens = readFileSync(join(PUBLIC, "css", "tokens.css"), "utf8");

// A colour literal: hex, a colour function, or a CSS named colour.
// transparent, currentColor and the forced-colors system colours (Canvas,
// CanvasText, Highlight) are not a palette and stay allowed.
const NAMED = "white|black|red|green|blue|gray|grey|silver|maroon|navy|teal|olive|purple|fuchsia|lime|aqua|orange|yellow|pink|brown|gold|indigo|violet";
const COLOUR = new RegExp(`#[0-9a-f]{3,8}\\b|\\b(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch|color)\\(|:[^;{}]*\\b(?:${NAMED})\\b`, "i");
// Comments and font names are words, not colours.
const strip = (text: string) => text.replace(/\/\*[\s\S]*?\*\//g, "").replace(/font-family:[^;]*;/g, "");

test("no colour literal in the panel's stylesheets outside tokens.css", () => {
  const found = css.flatMap((f) => strip(readFileSync(f, "utf8")).split("\n").filter((l) => COLOUR.test(l)).map((l) => `${f}: ${l.trim()}`));
  assert.deepEqual(found, []);
});

test("no colour literal in the panel's scripts (charts colour by class → token)", () => {
  const found = js.flatMap((f) => readFileSync(f, "utf8").split("\n").filter((l) => /["'`](?:#[0-9a-f]{3,8}|(?:rgba?|hsla?)\()/i.test(l)).map((l) => `${f}: ${l.trim()}`));
  assert.deepEqual(found, []);
});

// GO-152: links are drawn in --link, never --primary, so a site theme gives
// them its accent colour; without a theme --link equals --primary in both
// schemes, so the panel looks exactly as before.
test("links are drawn in --link, equal to --primary until a theme sets it", () => {
  const rules = css.flatMap((f) => [...strip(readFileSync(f, "utf8")).matchAll(/^(a(?::[\w-]+)?|\.skip)\s*\{([^}]*)\}/gm)].map((m) => [m[1], m[2]]));
  const coloured = rules.filter(([, body]) => /(?:^|[;\s])color:/.test(body));
  assert.ok(coloured.length >= 2, "the link rule and the skip link");
  for (const [sel, body] of coloured) assert.match(body, /(?:^|[;\s])color: var\(--link\)/, sel);
  for (const sel of [":root", ':root[data-theme="dark"]']) {
    const at = tokens.indexOf(sel + " {");
    const block = tokens.slice(at, tokens.indexOf("}", at));
    assert.equal(block.match(/--link:\s*(#[0-9a-f]{6})/)?.[1], block.match(/--primary:\s*(#[0-9a-f]{6})/)?.[1], sel);
  }
});

test("every custom property the stylesheets read is a token, or one set at run time", () => {
  const defined = new Set([...tokens.matchAll(/(--[\w-]+):/g)].map((m) => m[1]));
  // Set by the panel itself: per-entity chart colour and tile colour (CSS),
  // the measured sticky cover and the shared bar's size (js/main.js).
  for (const own of ["--k", "--tile", "--sticky-cover", "--admin-nav-height", "--admin-nav-sticky"]) defined.add(own);
  const used = new Set(css.flatMap((f) => [...readFileSync(f, "utf8").matchAll(/var\((--[\w-]+)/g)].map((m) => m[1])));
  assert.deepEqual([...used].filter((v) => !defined.has(v)), []);
});

test("the shared bar's variables fall back to the layout's old values (no core → no change)", () => {
  const layout = readFileSync(join(PUBLIC, "css", "layout.css"), "utf8");
  const uses = [...layout.matchAll(/var\(--admin-nav-(?:height|sticky)(, [^)]*)?\)/g)].map((m) => m[1]);
  assert.ok(uses.length >= 4);
  assert.deepEqual([...new Set(uses)], [", 0px"]);
});

test("the colour pattern itself", () => {
  for (const bad of ["color: #fff;", "fill: rgb(0 0 0);", "background: white;", "border-color: hsl(1 2% 3%);"]) assert.match(bad, COLOUR, bad);
  for (const ok of ["color: var(--text);", "border: 1px solid currentColor;", "background: transparent;", "fill: CanvasText;", "white-space: nowrap;"]) assert.doesNotMatch(ok, COLOUR, ok);
});
