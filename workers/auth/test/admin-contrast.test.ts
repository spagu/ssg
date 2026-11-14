// GO-138: the colour pairs the admin panel's CSS uses, computed from
// tokens.css in both themes (WCAG 2.2: text 4.5:1, control edges, focus ring
// and status borders 3:1). A token change that breaks a pair fails here.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const css = readFileSync(new URL("../public/auth/assets/tokens.css", import.meta.url), "utf8");
function tokens(block: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [, k, v] of block.matchAll(/--([\w-]+):\s*(#[0-9a-f]{6})/gi)) out[k] = v.toLowerCase();
  return out;
}
const light = tokens(css.slice(css.indexOf(":root {"), css.indexOf("@media")));
const dark = { ...light, ...tokens(css.slice(css.indexOf(':root[data-theme="dark"]'))) };

const lum = (hex: string) => {
  const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
};
export const ratio = (a: string, b: string) => {
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
  return (x + 0.05) / (y + 0.05);
};

// [foreground, background, minimum] — what admin.css and views.css draw
// (links fall back to --primary while core's theme has no --link).
const PAIRS: [string, string, number][] = [
  ["text", "bg", 4.5], ["text", "surface", 4.5], ["text", "surface-sunk", 4.5],
  ["text-muted", "surface", 4.5], ["text-muted", "bg", 4.5], ["text-muted", "surface-sunk", 4.5],
  ["primary", "surface", 4.5], ["primary", "bg", 4.5],
  ["on-primary", "primary", 4.5], ["on-primary-soft", "primary-soft", 4.5], ["on-danger", "danger", 4.5],
  ["control", "surface", 3], ["focus", "bg", 3], ["focus", "surface", 3],
  ["ok", "surface", 3], ["critical", "surface", 3], ["warning", "surface", 3], ["info", "surface", 3], ["danger", "surface", 3],
];

for (const [name, t] of [["light", light], ["dark", dark]] as const) {
  test(`admin panel contrast, ${name} theme`, () => {
    for (const [fg, bg, min] of PAIRS) {
      assert.ok(t[fg] && t[bg], `${fg}/${bg} defined`);
      const r = ratio(t[fg], t[bg]);
      assert.ok(r >= min, `${name}: --${fg} on --${bg} is ${r.toFixed(2)}:1, needs ${min}:1`);
    }
  });
}
