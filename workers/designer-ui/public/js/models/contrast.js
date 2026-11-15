// WCAG 2.2 contrast, as pure functions: relative luminance, the contrast
// ratio of two colours, the thresholds, and the nearest colour to a given one
// that meets a ratio against a background. No DOM, no imports — the panel's
// Appearance view and its tests use it as is, and another panel can copy it.
//
// Colours are hex strings: "#rgb" or "#rrggbb" (case-insensitive). Anything
// else (a name, rgb(), a var() reference, alpha) is not a plain colour here:
// parseHex answers null and the callers say so instead of guessing.

// Thresholds of WCAG 2.2 success criteria 1.4.3 (AA), 1.4.6 (AAA) and 1.4.11
// (non-text: control edges, focus rings).
export const AA = 4.5;
export const AA_LARGE = 3;
export const AAA = 7;
export const NON_TEXT = 3;

// parseHex("#0f172a") → {r: 15, g: 23, b: 42}; null for anything else.
export function parseHex(s) {
  const m = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(String(s ?? "").trim());
  if (!m) return null;
  const h = m[1].length === 3 ? [...m[1]].map((c) => c + c).join("") : m[1];
  return { r: parseInt(h.slice(0, 2), 16), g: parseInt(h.slice(2, 4), 16), b: parseInt(h.slice(4, 6), 16) };
}

const two = (n) => Math.round(n).toString(16).padStart(2, "0");
// toHex({r, g, b}) → "#rrggbb" (channels rounded and clamped to 0–255).
export const toHex = ({ r, g, b }) => "#" + [r, g, b].map((c) => two(Math.min(255, Math.max(0, c)))).join("");

// Linear value of one sRGB channel (0–255), as WCAG 2.2 defines it.
const linear = (c) => {
  const s = c / 255;
  return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
};

// luminance: the relative luminance of a colour (0 black … 1 white), or null.
export function luminance(colour) {
  const c = typeof colour === "string" ? parseHex(colour) : colour;
  if (!c) return null;
  return 0.2126 * linear(c.r) + 0.7152 * linear(c.g) + 0.0722 * linear(c.b);
}

// ratio: the contrast ratio of two colours (1 … 21), or null when either is
// not a plain colour. Order does not matter.
export function ratio(a, b) {
  const la = luminance(a);
  const lb = luminance(b);
  if (la === null || lb === null) return null;
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

// rounded: a ratio as people read it — two decimals, truncated rather than
// rounded, so 4.496 never shows as a passing "4.50".
export const rounded = (r) => Math.floor(r * 100) / 100;

const mix = (c, to, t) => ({ r: c.r + (to.r - c.r) * t, g: c.g + (to.g - c.g) * t, b: c.b + (to.b - c.b) * t });
const BLACK = { r: 0, g: 0, b: 0 };
const WHITE = { r: 255, g: 255, b: 255 };

// towards finds the smallest step t (0…1) from `fg` towards `to` whose
// rounded hex colour meets `target` against `bg`; null when even `to` fails.
// Luminance changes monotonically along the way, so a bisection finds it.
function towards(fg, to, bg, target) {
  const meets = (t) => ratio(toHex(mix(fg, to, t)), bg) >= target;
  if (!meets(1)) return null;
  let lo = 0;
  let hi = 1;
  for (let i = 0; i < 24; i++) {
    const mid = (lo + hi) / 2;
    if (meets(mid)) hi = mid;
    else lo = mid;
  }
  return { t: hi, colour: toHex(mix(fg, to, hi)) };
}

// nearest: the colour closest to `fg` (same hue, mixed towards black or
// white — whichever needs the smaller step) that reaches `target` against
// `bg`. {colour, ratio} — `fg` itself when it already passes — or null when
// either colour is not plain or no mix can reach the target.
export function nearest(fg, bg, target = AA) {
  const c = parseHex(fg);
  if (!c || !parseHex(bg)) return null;
  const now = ratio(fg, bg);
  if (now >= target) return { colour: toHex(c), ratio: now };
  const options = [towards(c, BLACK, bg, target), towards(c, WHITE, bg, target)].filter(Boolean);
  if (!options.length) return null;
  const best = options.reduce((a, b) => (b.t < a.t ? b : a));
  return { colour: best.colour, ratio: ratio(best.colour, bg) };
}

// check: one pair judged — {ratio, passes, suggestion} where suggestion is the
// nearest passing foreground (null when it passes or none exists).
export function check(fg, bg, target = AA) {
  const r = ratio(fg, bg);
  if (r === null) return { ratio: null, passes: null, suggestion: null };
  const passes = r >= target;
  return { ratio: r, passes, suggestion: passes ? null : nearest(fg, bg, target) };
}
