// The registry's `theme` (GO-152): the site's look for the admin panels, as
// the build computed it from admin_theme: in .ssg.yaml. The build also wrote
// the colours as /core/theme.css; core only passes the rest on — the brand to
// the shared navigation (/api/core/modules) and everything to the read-only
// Appearance view (/api/core/info). No colour maths happens here.
// A leading underscore keeps this file out of the Pages route table.
//
// The registry is the build's own output, but it is still read as data: each
// field is kept only in the shape the build writes (a #rrggbb colour, a path
// on this host, plain text of bounded length), and anything else becomes
// null or is left out — a hand-edited registry can blank a field, never put
// markup, a script URL or another host into a page.

export interface FontFile {
  family: string;
  src: string;
  format: string;
  weight: string;
  style: string;
}
export interface Adjustment {
  scheme: string;
  token: string;
  requested: string;
  used: string;
  reason: string;
  against: string;
}
export interface Check {
  scheme: string;
  fg: string;
  bg: string;
  ratio: number;
  minimum: number;
}
export interface Theme {
  name: string | null;
  logo: string | null;
  logoDark: string | null;
  fontDisplay: string | null;
  fontUi: string | null;
  fontFiles: FontFile[];
  main: string | null;
  accent: string | null;
  derived: string[];
  light: Record<string, string>;
  dark: Record<string, string>;
  adjustments: Adjustment[];
  checks: Check[];
}
export interface Brand {
  name: string | null;
  logo: string | null;
  logoDark: string | null;
}

const HEX = /^#[0-9a-f]{6}$/;
const TOKEN = /^[a-z][a-z0-9-]{0,31}$/;
const WORD = /^[a-z]{1,16}$/;
// A file on this host: the build's grammar (internal/modules/paths.go).
const FILE = /^(?:\/[A-Za-z0-9_~-][A-Za-z0-9._~-]*)+$/;
// A font stack as the build writes it: quoted names and generic keywords.
const STACK = /^[\p{L}\p{N} ."_,-]{1,600}$/u;
const SAFE = /^[\p{L}\p{N} ._-]{1,64}$/u;
const WEIGHT = /^(?:normal|bold|\d{1,4}(?: \d{1,4})?)$/;

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);
const match = (v: unknown, re: RegExp, max = 256): string | null => (typeof v === "string" && v.length <= max && re.test(v) ? v : null);
const list = (v: unknown): unknown[] => (Array.isArray(v) ? v.slice(0, 200) : []);

// name: plain text, at most 80 characters, no control characters.
function plainName(v: unknown): string | null {
  if (typeof v !== "string") return null;
  const s = v.trim();
  return s && [...s].length <= 80 && !/[\p{Cc}]/u.test(s) ? s : null;
}

function colours(v: unknown): Record<string, string> {
  const out: Record<string, string> = {};
  if (isObject(v)) for (const [k, c] of Object.entries(v)) if (TOKEN.test(k) && match(c, HEX)) out[k] = c as string;
  return out;
}

function fontFile(v: unknown): FontFile | null {
  if (!isObject(v)) return null;
  const f = { family: match(v.family, SAFE), src: match(v.src, FILE), format: match(v.format, /^(?:woff2|woff|truetype|opentype)$/), weight: match(v.weight, WEIGHT), style: match(v.style, /^(?:normal|italic|oblique)$/) };
  return Object.values(f).every((x) => x !== null) ? (f as FontFile) : null;
}

function adjustment(v: unknown): Adjustment | null {
  if (!isObject(v)) return null;
  const a = { scheme: match(v.scheme, WORD), token: match(v.token, TOKEN), requested: match(v.requested, HEX), used: match(v.used, HEX), reason: match(v.reason, WORD), against: match(v.against, TOKEN) };
  return Object.values(a).every((x) => x !== null) ? (a as Adjustment) : null;
}

function check(v: unknown): Check | null {
  if (!isObject(v)) return null;
  const ok = match(v.scheme, WORD) && match(v.fg, TOKEN) && match(v.bg, TOKEN) && Number.isFinite(v.ratio) && Number.isFinite(v.minimum);
  return ok ? { scheme: v.scheme as string, fg: v.fg as string, bg: v.bg as string, ratio: v.ratio as number, minimum: v.minimum as number } : null;
}

const kept = <T>(items: (T | null)[]): T[] => items.filter((x): x is T => x !== null);

// readTheme returns the theme in the shape above, or null when the registry
// has none (or it is not an object).
export function readTheme(raw: unknown): Theme | null {
  if (!isObject(raw)) return null;
  return {
    name: plainName(raw.name),
    logo: match(raw.logo, FILE),
    logoDark: match(raw.logoDark, FILE),
    fontDisplay: match(raw.fontDisplay, STACK, 600),
    fontUi: match(raw.fontUi, STACK, 600),
    fontFiles: kept(list(raw.fontFiles).map(fontFile)),
    main: match(raw.main, HEX),
    accent: match(raw.accent, HEX),
    derived: list(raw.derived).filter((d): d is string => d === "main" || d === "accent"),
    light: colours(raw.light),
    dark: colours(raw.dark),
    adjustments: kept(list(raw.adjustments).map(adjustment)),
    checks: kept(list(raw.checks).map(check)),
  };
}

// brandOf: what the shared navigation draws, or null for the shipped bar.
export function brandOf(theme: Theme | null): Brand | null {
  if (!theme || (!theme.name && !theme.logo && !theme.logoDark)) return null;
  return { name: theme.name, logo: theme.logo, logoDark: theme.logoDark };
}
