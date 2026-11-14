// The Appearance view's decisions (GO-152), as plain functions over
// /api/core/info's `theme` — the build computed every colour and ratio; this
// only sorts, labels and explains them. No DOM here, no colour maths.

// ROUTE: the view lives at /core/#appearance (route.js decides).
export const ROUTE = "#appearance";
export { routeOf } from "./route.js";

// The neutral surfaces the pairs are checked on: never themed, so the build
// does not send them. Equal to tokens.css (test/home-static.test.ts).
export const NEUTRAL = {
  light: { surface: "#ffffff", bg: "#efebe9", "surface-sunk": "#efebe9" },
  dark: { surface: "#1e1e1e", bg: "#121212", "surface-sunk": "#121212" },
};
export const SCHEMES = ["light", "dark"];

// Every token a pair can name; a name outside the list is shown as itself.
export const TOKENS = [
  "primary", "on-primary", "primary-soft", "on-primary-soft", "link", "focus", "c1", "c2", "c3", "c4", "c5",
  "nav-bg", "on-nav", "nav-muted", "nav-hover", "nav-control", "nav-current", "surface", "bg", "surface-sunk",
];
// Text tokens: a background adjusted against one of them moved so that the
// text stays readable on it ("under"), not so that it stands out.
const UNDER = new Set(["on-nav", "on-primary", "on-primary-soft", "nav-muted"]);

const HEX = /^#[0-9a-f]{6}$/;
export const colour = (v) => (typeof v === "string" && HEX.test(v) ? v : null);

export const tokenLabel = (t, name) => (TOKENS.includes(name) ? t(`theme.token.${name}`) : name);
const schemeLabel = (t, s) => t(`theme.scheme.${["light", "dark"].includes(s) ? s : "both"}`);

// adjustmentText: one adjustment in words —
// "Light scheme: for links and primary buttons on cards and fields, #776c04
// is used instead of #fff59d, so it stands out enough against …".
export function adjustmentText(t, a) {
  const key = a.reason === "distinct" ? "theme.adjust.distinct" : UNDER.has(a.against) ? "theme.adjust.under" : "theme.adjust.contrast";
  return t(key, { scheme: schemeLabel(t, a.scheme), role: tokenLabel(t, a.token), against: tokenLabel(t, a.against), used: a.used, requested: a.requested });
}

// hexOf: a token's colour in a scheme — the theme's, else the neutral one.
export const hexOf = (theme, scheme, name) => colour(theme[scheme]?.[name]) ?? NEUTRAL[scheme]?.[name] ?? null;

// rows: the contrast pairs of one scheme, ready to lay out. A pair whose
// colours are not both known is left out (it cannot be drawn honestly).
export function rows(theme, scheme) {
  return theme.checks
    .filter((c) => c.scheme === scheme)
    .map((c) => ({ ...c, fgHex: hexOf(theme, scheme, c.fg), bgHex: hexOf(theme, scheme, c.bg), pass: c.ratio >= c.minimum, kind: c.minimum >= 4.5 ? "text" : "ui" }))
    .filter((r) => r.fgHex && r.bgHex);
}

// brandColours: main and accent with whether the build derived it.
export function brandColours(theme) {
  return ["main", "accent"].map((role) => ({ role, hex: colour(theme[role]), derived: theme.derived.includes(role) })).filter((c) => c.hex);
}

// logos: where the view previews the logo — on a light page (logo), on a
// dark page and in the bar (the dark-background logo when there is one).
export function logos(theme) {
  const light = theme.logo ?? theme.logoDark;
  const dark = theme.logoDark ?? theme.logo;
  if (!light) return [];
  return [
    { where: "light", src: light },
    { where: "dark", src: dark },
    { where: "bar", src: dark },
  ];
}

// failing: how many pairs miss their minimum (only a token override can).
export const failing = (theme) => theme.checks.filter((c) => c.ratio < c.minimum).length;
