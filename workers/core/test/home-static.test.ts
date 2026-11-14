// The home page's and the navigation's files as shipped: strings complete in
// five languages, WCAG contrast of every token pair in both themes (computed
// from tokens.css), the bar's fallbacks equal to the tokens, the page's meta
// tags and CSP rules, nothing from another origin, every referenced file
// present.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, normalize } from "node:path";
import { LANGS } from "../public/core/js/i18n.js";

const CORE = new URL("../public/core/", import.meta.url).pathname;
const read = (p: string) => readFileSync(join(CORE, p), "utf8");
const files = (dir: string): string[] => readdirSync(dir).flatMap((f) => (statSync(join(dir, f)).isDirectory() ? files(join(dir, f)) : [join(dir, f)]));

// ---- strings ---------------------------------------------------------------
const tables: Record<string, Record<string, unknown>> = {};
for (const l of LANGS) tables[l] = (await import(`../public/core/js/strings/${l}.js`)).default;
const holes = (v: unknown): string[] => [...new Set([...JSON.stringify(v).matchAll(/\{(\w+)\}/g)].map((m) => m[1]))].sort();

for (const lang of LANGS) {
  test(`strings ${lang}: en's keys in en's order, plural forms the language needs, the same placeholders`, () => {
    const t = tables[lang];
    assert.deepEqual(Object.keys(t), Object.keys(tables.en));
    const rules = new Intl.PluralRules(lang);
    for (const [k, v] of Object.entries(t)) {
      assert.equal(typeof v, typeof tables.en[k], `${lang} ${k}`);
      if (typeof v === "object") for (const cat of rules.resolvedOptions().pluralCategories) assert.ok(cat in (v as object) || "other" in (v as object), `${lang} ${k}: ${cat}`);
      else assert.ok((v as string).trim(), `${lang} ${k} is empty`);
      assert.deepEqual(holes(v).filter((x) => x !== "count"), holes(tables.en[k]).filter((x) => x !== "count"), `${lang} ${k}`);
    }
  });
}

test("strings: every key the code asks for exists (literal and built at run time)", () => {
  const used = new Set<string>();
  for (const f of files(join(CORE, "js")).filter((f) => f.endsWith(".js") && !f.includes("/strings/"))) {
    for (const [, key] of readFileSync(f, "utf8").matchAll(/\bt\(\s*"([\w.]+)"/g)) used.add(key);
    for (const [, key] of readFileSync(f, "utf8").matchAll(/\?\s*"([a-z]+\.[\w.]+)"\s*:\s*"([a-z]+\.[\w.]+)"/g)) used.add(key);
  }
  const dynamic = [
    ...LANGS.map((l) => `pref.lang.${l}`), ...["auto", "light", "dark"].map((v) => `pref.theme.${v}`),
    ...["working", "needsConfig", "unavailable", "denied", "unchecked"].map((s) => `state.${s}`),
    ...["action", "warning", "info"].flatMap((s) => [`severity.${s}`, `attention.count.${s}`]),
    ...["timeout", "deadline", "network", "http_status", "redirect", "refused_target", "invalid_manifest", "other"].map((r) => `reason.${r}`),
    ...["unavailable", "refused_target", "invalid_manifest"].map((c) => `check.${c}`),
    ...["password", "auth", "access", "unknown"].map((m) => `mode.${m}`),
    ...["registered", "live", "unreachable", "invalid_manifest", "other"].map((s) => `modstatus.${s}`),
    ...["network", "missing", "signin", "auth", "forbidden", "rate", "config", "conflict", "invalid", "server"].map((e) => `error.${e}`),
    ...["module", "version", "contract", "status"].map((c) => `col.${c}`),
    ...["ok", "size", "sizeTime", "absent", "invalid", "unreadable"].map((b) => `site.build.${b}`),
    "attention.none.partial", "attention.none.partialText", "attention.none.title", "attention.none.text",
    // The Appearance view (GO-152): keys built from roles, places, schemes,
    // reasons and token names.
    "appearance.pass", "appearance.fail", ...["main", "accent"].map((r) => `appearance.${r}`), ...["light", "dark", "bar"].map((w) => `appearance.logo.${w}`),
    ...["text", "ui"].map((k) => `appearance.needs.${k}`), ...["pair", "colour", "on", "ratio", "needs", "result"].map((c) => `col.${c}`),
    ...["light", "dark", "both"].map((s) => `theme.scheme.${s}`), ...["contrast", "under", "distinct"].map((r) => `theme.adjust.${r}`),
    ...["primary", "on-primary", "primary-soft", "on-primary-soft", "link", "focus", "c1", "c2", "c3", "c4", "c5", "nav-bg", "on-nav", "nav-muted", "nav-hover", "nav-control", "nav-current", "surface", "bg", "surface-sunk"].map((k) => `theme.token.${k}`),
    // The Modules view (GO-149): keys built from a direction, an actor, an
    // action and a kind.
    ...["up", "down"].flatMap((d) => [`manage.${d}`, `manage.${d}.label`]),
    ...["password", "access", "auth"].map((w) => `manage.who.${w}`),
    ...["hidden", "shown", "labels", "note", "order", "import", "other"].map((a) => `manage.change.${a}`),
    ...["pages-function", "worker", "other"].map((k) => `manage.kind.${k}`),
    // The second branch of a two-way choice (the scan above reads the first).
    "manage.readonly.no_database", "manage.cosmetic.nopanel", "manage.state.shown", "manage.move.last", "manage.shown.done", "manage.status.up",
    "manage.import.show", "manage.import.place", "manage.import.nolabels", "manage.import.nonote",
  ];
  for (const k of [...used, ...dynamic]) assert.ok(Object.hasOwn(tables.en, k), `missing string ${k}`);
  for (const k of Object.keys(tables.en)) assert.ok(used.has(k) || dynamic.includes(k), `unused string ${k}`);
});

// ---- contrast --------------------------------------------------------------
const tokens = read("tokens.css");
const pairs = (css: string) => Object.fromEntries([...css.matchAll(/--([\w-]+):\s*(#[0-9a-f]{6})/gi)].map((m) => [m[1], m[2].toLowerCase()]));
const block = (selector: string) => pairs(tokens.slice(tokens.indexOf(selector + " {"), tokens.indexOf("}", tokens.indexOf(selector + " {"))));
const LIGHT = block(":root");
const DARK = block(':root[data-theme="dark"]');
const DARK_MEDIA = pairs(tokens.slice(tokens.indexOf("@media (prefers-color-scheme: dark)"), tokens.indexOf(':root[data-theme="dark"]')));
const lum = (hex: string) => {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};
const ratio = (a: string, b: string) => {
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
  return (x + 0.05) / (y + 0.05);
};
// [foreground, background]: text 4.5:1; marks and control edges 3:1.
const TEXT: [string, string][] = [
  ["text", "bg"], ["text", "surface"], ["text", "surface-sunk"], ["text-muted", "bg"], ["text-muted", "surface"], ["text-muted", "surface-sunk"],
  ["primary", "surface"], ["primary", "bg"], ["on-primary", "primary"], ["on-primary-soft", "primary-soft"], ["surface", "on-primary-soft"],
  ["link", "surface"], ["link", "bg"], ["link", "surface-sunk"],
  ["critical", "surface"], ["critical", "bg"], ["ok", "surface"],
  ["on-nav", "nav-bg"], ["nav-muted", "nav-bg"], ["on-nav", "nav-hover"],
];
const UI: [string, string][] = [
  ["control", "surface"], ["control", "bg"], ["focus", "surface"], ["focus", "bg"], ["focus", "surface-sunk"], ["axis", "surface"],
  ["warning", "surface"], ["critical", "surface"], ["ok", "surface"], ["primary", "surface"], ["c1", "surface"],
  ["nav-current", "nav-bg"], ["nav-current", "nav-hover"], ["nav-control", "nav-bg"],
];
for (const [name, theme] of [["light", LIGHT], ["dark", DARK]] as const) {
  test(`contrast ${name}: text ≥ 4.5:1, marks and boundaries ≥ 3:1`, () => {
    const bad: string[] = [];
    for (const [fg, bg] of TEXT) if (ratio(theme[fg], theme[bg]) < 4.5) bad.push(`${fg} on ${bg}: ${ratio(theme[fg], theme[bg]).toFixed(2)}`);
    for (const [fg, bg] of UI) if (ratio(theme[fg], theme[bg]) < 3) bad.push(`${fg} on ${bg}: ${ratio(theme[fg], theme[bg]).toFixed(2)}`);
    assert.deepEqual(bad, []);
  });
}
test("tokens: both dark definitions agree; every token in both themes; the bar's fallbacks are the light tokens", () => {
  assert.deepEqual(DARK_MEDIA, DARK);
  assert.deepEqual(Object.keys(DARK).sort(), Object.keys(LIGHT).sort());
  assert.equal(ratio("#000000", "#ffffff").toFixed(1), "21.0");
  for (const [, name, hex] of read("nav.css").matchAll(/var\(--([\w-]+),\s*(#[0-9a-f]{6})\)/gi)) assert.equal(hex.toLowerCase(), LIGHT[name], `nav.css fallback for --${name}`);
});
// every custom property of a block, whatever its value (#hex, rgb(), a font
// stack, a length).
const props = (css: string) => Object.fromEntries([...css.matchAll(/--([\w-]+):\s*([^;]+);/g)].map((m) => [m[1], m[2].trim().replace(/\s+/g, " ")]));
const blockOf = (css: string, selector: string) => props(css.slice(css.indexOf(selector + " {"), css.indexOf("}", css.indexOf(selector + " {"))));
// PANEL_ONLY: newsletter-ui's own layout, not part of the shared palette.
const PANEL_ONLY = ["sidebar"];
test("tokens: every custom property newsletter-ui defines is here with the same value, in both themes (panels match)", { skip: existsSync(new URL("../../newsletter-ui/public/css/tokens.css", import.meta.url)) ? false : "newsletter-ui is not next to this copy" }, () => {
  const theirs = readFileSync(new URL("../../newsletter-ui/public/css/tokens.css", import.meta.url), "utf8");
  for (const selector of [":root", ':root[data-theme="dark"]']) {
    const mine = blockOf(tokens, selector);
    const other = blockOf(theirs, selector);
    assert.ok(Object.keys(other).length > 10, selector);
    for (const [k, v] of Object.entries(other)) if (!PANEL_ONLY.includes(k)) assert.equal(mine[k], v, `${selector} --${k}`);
  }
  assert.deepEqual(blockOf(tokens, ":root").shadow, "rgb(0 0 0 / .12)");
});

// ---- files -----------------------------------------------------------------
const html = read("index.html");
test("index.html: meta tags, noindex, a CSP of its own, no tag manager", () => {
  for (const re of [
    /<meta charset="utf-8">/, /<meta name="viewport" content="width=device-width, initial-scale=1">/, /<title>[^<]+<\/title>/,
    /<meta name="description" content="[^"]{60,}">/, /<meta name="robots" content="noindex, nofollow, noarchive">/, /<meta name="referrer" content="no-referrer">/,
    /<meta name="color-scheme" content="light dark">/, /<meta name="theme-color" content="#[0-9a-f]{6}" media="\(prefers-color-scheme: light\)">/,
    /<meta name="theme-color" content="#[0-9a-f]{6}" media="\(prefers-color-scheme: dark\)">/, /<html lang="en">/,
    /<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'self'; style-src 'self';[^"]*">/,
    /<ssg-admin-nav current="core"><\/ssg-admin-nav>/,
  ]) assert.match(html, re);
  assert.doesNotMatch(html, /unsafe-inline|googletagmanager|gtag\(|GTM-/i);
});

test("nothing the CSP blocks, nothing from another origin, no HTML built from strings", () => {
  assert.doesNotMatch(html, /style=|<style|\son[a-z]+=/i);
  for (const [, attrs, body] of html.matchAll(/<script([^>]*)>([\s\S]*?)<\/script>/g)) {
    if (/application\/json/.test(attrs)) assert.doesNotThrow(() => JSON.parse(body));
    else assert.equal(body.trim(), "");
  }
  for (const file of files(CORE)) {
    const text = readFileSync(file, "utf8");
    assert.deepEqual([...text.matchAll(/(?:src|href|url\()\s*=?\s*["(]?(https?:)?\/\/[^"')\s]+/g)].map((m) => m[0]), [], file);
    assert.doesNotMatch(text, /\.innerHTML\s*=|insertAdjacentHTML|document\.write|eval\(/, file);
  }
  assert.doesNotMatch(read("nav.js"), /^\s*import\s/m, "nav.js is one file with no imports");
  assert.doesNotMatch(read("theme.css").replace(/\/\*[\s\S]*?\*\//g, ""), /\S/, "theme.css holds no rule until GO-152");
});

test("every file the page, a stylesheet or a module refers to exists", () => {
  const refs: [string, string][] = [];
  const at = (base: string, ref: string) => (ref.startsWith("/core/") ? [CORE, ref.slice(6)] : [base, ref]) as [string, string];
  for (const [, ref] of html.matchAll(/(?:src|href)="([^"#][^"]*)"/g)) refs.push(at(CORE, ref));
  for (const file of files(CORE).filter((f) => f.endsWith(".js"))) {
    const text = readFileSync(file, "utf8");
    for (const [, ref] of text.matchAll(/(?:^import\s[^"']*?from\s*|import\(\s*)["']([^"'`$]+)["']/gm)) refs.push(at(dirname(file), ref));
    for (const [, ref] of text.matchAll(/src: "(img\/[^"]+)"/g)) refs.push([CORE, ref]);
    for (const [, ref] of text.matchAll(/new URL\("([\w.]+)", import\.meta\.url\)/g)) refs.push([dirname(file), ref]);
  }
  for (const id of ["default", "comments", "newsletter", "auth", "web-push", "contact-form", "cookie-consent", "rate-limit", "stripe-checkout", "republish-trigger", "dynamic-price", "conversions-proxy"]) refs.push([CORE, `img/modules/${id}.svg`]);
  for (const l of LANGS) refs.push([join(CORE, "js"), `strings/${l}.js`]);
  assert.ok(refs.length > 40);
  for (const [base, ref] of refs) assert.ok(existsSync(normalize(join(base, ref))), `${ref} (from ${base})`);
  for (const svg of files(join(CORE, "img"))) assert.match(readFileSync(svg, "utf8"), /^<svg xmlns="http:\/\/www\.w3\.org\/2000\/svg" viewBox="[\d ]+"><!-- Decorative: /);
});
