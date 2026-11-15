// Which theme tokens are text on which background, and how each pair fares
// against WCAG 2.2 (1.4.3: 4.5:1 for text) in the light and the dark scheme.
// A theme states no pairs, so they are read from the token names — the
// conventions of the themes ssg ships and of the panels' own tokens:
//
//   text      a name whose part after "color-" (and a short theme prefix
//             such as "ad-") starts with fg, text, ink or
//             link (--color-fg-muted, --text, --link) — on every general
//             background: a name that starts with bg, background, surface or
//             canvas (--color-bg-subtle, --surface-sunk), inverse ones aside;
//   on-X      --on-primary on --primary, --on-primary-soft on --primary-soft;
//   X-fg      --color-accent-fg on every --color-accent-bg* (hover, active);
//   inverse   a text token naming "inverse" (--color-fg-onInverse) on the
//             backgrounds naming "inverse".
//
// Palette ramps (a name holding "palette") are raw material, never a pair.

import { AA, check } from "./contrast.js";
import { resolve } from "./tokens-css.js";

const WORDS = ["bg", "fg", "on", "ink", "text", "link", "background", "surface", "canvas"];
// strip: the name without "--", "color-" and a short theme prefix
// (--ad-text, --ps-fg: a segment of up to three letters that is no word above).
function strip(name) {
  const s = name.replace(/^--/, "").replace(/^color-/, "");
  const [first, ...rest] = s.split("-");
  return rest.length && first.length <= 3 && !WORDS.includes(first) ? rest.join("-").replace(/^color-/, "") : s;
}
const starts = (s, words) => words.some((w) => s === w || s.startsWith(w + "-"));
const inverse = (name) => /inverse/i.test(name);

const isGeneralBg = (n) => starts(strip(n), ["bg", "background", "surface", "canvas"]) && !inverse(n);
const isGeneralFg = (n) => starts(strip(n), ["fg", "text", "ink", "link"]) && !inverse(n);

// pairsOf: [{fg, bg}] among the names, in the names' order.
export function pairsOf(names) {
  const list = names.filter((n) => !/palette/i.test(n));
  const out = [];
  const add = (fg, bg) => fg !== bg && !out.some((p) => p.fg === fg && p.bg === bg) && out.push({ fg, bg });
  for (const fg of list) {
    const s = strip(fg);
    if (s.startsWith("on-")) {
      const target = list.find((n) => strip(n) === s.slice(3));
      if (target) add(fg, target);
    } else if (/-fg$/.test(s)) {
      const stem = s.slice(0, -3);
      for (const bg of list) if (strip(bg).startsWith(stem + "-bg")) add(fg, bg);
    } else if (inverse(fg) && /fg|text|on/i.test(s)) {
      for (const bg of list) if (inverse(bg) && /bg|background|surface/i.test(strip(bg))) add(fg, bg);
    } else if (isGeneralFg(fg)) {
      for (const bg of list) if (isGeneralBg(bg)) add(fg, bg);
    }
  }
  return out;
}

// evaluate: every pair in both schemes — {fg, bg, scheme, fgColour,
// bgColour, ratio, passes, suggestion} — skipping a pair whose colours are
// not plain in that scheme. `all` is tokens-css.js valueMap() of the text
// as it is now (with the panel's edits applied).
export function evaluate(pairs, all, target = AA) {
  const out = [];
  for (const scheme of ["light", "dark"]) {
    for (const { fg, bg } of pairs) {
      const fgColour = resolve(all, fg, scheme);
      const bgColour = resolve(all, bg, scheme);
      if (!fgColour || !bgColour) continue;
      out.push({ fg, bg, scheme, fgColour, bgColour, target, ...check(fgColour, bgColour, target) });
    }
  }
  return out;
}

// failingWith: the failing pairs that involve one of `names` (the tokens the
// panel changed) — the ones publishing asks to confirm. A pair the theme
// already failed before the panel touched it is shown, never blamed on the edit.
export const failingWith = (results, names) => results.filter((r) => !r.passes && (names.has(r.fg) || names.has(r.bg)));
