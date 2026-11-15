// GO-179: WCAG 2.2 contrast maths (public/js/models/contrast.js) against a
// table of reference results, and the nearest passing colour it suggests.
import { test } from "node:test";
import assert from "node:assert/strict";
import { AA, AAA, AA_LARGE, NON_TEXT, check, luminance, nearest, parseHex, ratio, rounded, toHex } from "../public/js/models/contrast.js";

// [foreground, background, ratio to two decimals (truncated)] — computed by
// the WCAG 2.2 formula; the well-known ones (black/white 21, #777 on white
// 4.47) double as a check on the formula itself.
const TABLE: [string, string, number][] = [
  ["#000000", "#ffffff", 21],
  ["#ffffff", "#000000", 21],
  ["#777777", "#ffffff", 4.47],
  ["#767676", "#ffffff", 4.54],
  ["#0f172a", "#ffffff", 17.85],
  ["#64748b", "#ffffff", 4.75],
  ["#64748b", "#f2f3f6", 4.28],
  ["#0050a6", "#ffffff", 7.78],
  ["#99bfeb", "#0f172a", 9.35],
  ["#00695c", "#eceff1", 5.72],
  ["#80cbc4", "#121212", 10.04],
  ["#fff", "#fff", 1],
  ["#f00", "#00f", 2.14],
];

for (const [fg, bg, want] of TABLE) {
  test(`contrast ${fg} on ${bg} is ${want}:1`, () => {
    assert.equal(rounded(ratio(fg, bg)!), want);
  });
}

test("thresholds are WCAG 2.2's", () => {
  assert.deepEqual([AA, AA_LARGE, AAA, NON_TEXT], [4.5, 3, 7, 3]);
});

test("parseHex reads #rgb and #rrggbb only", () => {
  assert.deepEqual(parseHex("#0F172A"), { r: 15, g: 23, b: 42 });
  assert.deepEqual(parseHex(" #abc "), { r: 170, g: 187, b: 204 });
  for (const bad of ["red", "rgb(0 0 0)", "#abcd", "#12345678", "var(--x)", "", null, undefined]) assert.equal(parseHex(bad), null, String(bad));
  assert.equal(toHex({ r: 300, g: -4, b: 15.6 }), "#ff0010");
});

test("luminance and ratio refuse what is not a plain colour", () => {
  assert.equal(luminance("#ffffff"), 1);
  assert.equal(luminance({ r: 0, g: 0, b: 0 }), 0);
  assert.equal(luminance("tomato"), null);
  assert.equal(ratio("#fff", "tomato"), null);
  assert.equal(ratio("tomato", "#fff"), null);
});

test("rounded truncates: 4.496 never reads as a passing 4.50", () => {
  assert.equal(rounded(4.496), 4.49);
  assert.equal(rounded(4.5), 4.5);
});

test("nearest: a passing colour is returned as it is", () => {
  assert.deepEqual(nearest("#000", "#fff"), { colour: "#000000", ratio: 21 });
});

test("nearest: the smallest step that passes, in the direction that needs less", () => {
  const n = nearest("#64748b", "#f2f3f6")!;
  assert.ok(n.ratio >= AA, String(n.ratio));
  assert.ok(ratio(n.colour, "#f2f3f6")! >= AA);
  // darker, and only just: one step lighter of each channel would fail
  assert.ok(luminance(n.colour)! < luminance("#64748b")!);
  assert.ok(n.ratio < 4.6, `${n.colour} moved further than needed (${n.ratio})`);
  // on a dark background the way out is lighter
  const d = nearest("#334155", "#0f172a")!;
  assert.ok(luminance(d.colour)! > luminance("#334155")!);
  assert.ok(d.ratio >= AA);
});

test("nearest: when both directions reach the target, the shorter one wins", () => {
  // Mid greys against a mid grey at 3:1 — black and white both get there.
  const lighter = nearest("#707070", "#666666", AA_LARGE)!;
  assert.equal(lighter.colour, "#bcbcbc");
  assert.ok(luminance(lighter.colour)! > luminance("#707070")!);
  const darker = nearest("#808080", "#888888", AA_LARGE)!;
  assert.equal(darker.colour, "#3e3e3e");
  assert.ok(luminance(darker.colour)! < luminance("#808080")!);
});

test("nearest: AAA target and the impossible", () => {
  const n = nearest("#888888", "#ffffff", AAA)!;
  assert.ok(n.ratio >= 7);
  // mid grey on mid grey cannot reach 21:1 in either direction
  assert.equal(nearest("#777777", "#777777", 21), null);
  assert.equal(nearest("tomato", "#fff"), null);
  assert.equal(nearest("#fff", "tomato"), null);
});

test("check: passes, fails with a suggestion, or cannot tell", () => {
  assert.deepEqual(check("#000", "#fff"), { ratio: 21, passes: true, suggestion: null });
  const f = check("#777777", "#ffffff");
  assert.equal(f.passes, false);
  assert.ok(f.suggestion && f.suggestion.ratio >= AA);
  assert.deepEqual(check("tomato", "#fff"), { ratio: null, passes: null, suggestion: null });
  // AA large (3:1): #777 on white passes there
  assert.equal(check("#777777", "#ffffff", AA_LARGE).passes, true);
});
