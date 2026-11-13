// GO-128: WCAG 2.2 contrast of every colour pair the panel uses, computed
// from css/tokens.css itself (both themes) — a token change that breaks a
// ratio fails here, not in someone's eyes.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const css = readFileSync(new URL("../public/css/tokens.css", import.meta.url), "utf8");

function block(selector: string): Record<string, string> {
  const at = css.indexOf(selector + " {");
  assert.ok(at >= 0, selector);
  const body = css.slice(at, css.indexOf("}", at));
  return Object.fromEntries([...body.matchAll(/--([\w-]+):\s*(#[0-9a-f]{6})/gi)].map((m) => [m[1], m[2].toLowerCase()]));
}

const LIGHT = block(":root");
const DARK = block(':root[data-theme="dark"]');
// The media-query copy of the dark theme must equal the data-theme copy.
const media = css.slice(css.indexOf("@media (prefers-color-scheme: dark)"), css.indexOf(':root[data-theme="dark"]'));
const DARK_MEDIA = Object.fromEntries([...media.matchAll(/--([\w-]+):\s*(#[0-9a-f]{6})/gi)].map((m) => [m[1], m[2].toLowerCase()]));

const lum = (hex: string) => {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};
export const ratio = (a: string, b: string) => {
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
  return (x + 0.05) / (y + 0.05);
};

// [foreground, background] — text needs 4.5:1.
const TEXT: [string, string][] = [
  ["text", "bg"], ["text", "surface"], ["text", "surface-sunk"],
  ["text-muted", "bg"], ["text-muted", "surface"], ["text-muted", "surface-sunk"],
  ["primary", "surface"], ["primary", "bg"], ["on-primary", "primary"],
  ["on-primary-soft", "primary-soft"], ["surface", "on-primary-soft"],
  ["on-danger", "danger"], ["danger", "surface"], ["critical", "surface"], ["critical", "bg"],
  ["ok", "surface"], ["surface", "text"],
];
// Boundaries and marks that must be seen: 3:1 (WCAG 1.4.11), focus too (2.4.13).
const UI: [string, string][] = [
  ["control", "surface"], ["control", "bg"], ["focus", "surface"], ["focus", "bg"], ["focus", "surface-sunk"],
  ["axis", "surface"], ["warning", "surface"], ["critical", "surface"], ["ok", "surface"], ["primary", "surface"],
  ["c1", "surface"], ["c2", "surface"], ["c3", "surface"], ["c4", "surface"], ["c5", "surface"],
];

for (const [name, theme] of [["light", LIGHT], ["dark", DARK]] as const) {
  test(`contrast ${name}: text pairs ≥ 4.5:1, UI and chart marks ≥ 3:1`, () => {
    const bad: string[] = [];
    for (const [fg, bg] of TEXT) if (ratio(theme[fg], theme[bg]) < 4.5) bad.push(`${fg} on ${bg}: ${ratio(theme[fg], theme[bg]).toFixed(2)}`);
    for (const [fg, bg] of UI) if (ratio(theme[fg], theme[bg]) < 3) bad.push(`${fg} on ${bg}: ${ratio(theme[fg], theme[bg]).toFixed(2)}`);
    assert.deepEqual(bad, []);
  });
}

test("contrast: both dark definitions agree, and every token exists in both themes", () => {
  assert.deepEqual(DARK_MEDIA, DARK);
  assert.deepEqual(Object.keys(DARK).sort(), Object.keys(LIGHT).sort());
});

test("contrast: the ratio itself", () => {
  assert.equal(ratio("#000000", "#ffffff").toFixed(1), "21.0");
  assert.equal(ratio("#ffffff", "#ffffff"), 1);
});
