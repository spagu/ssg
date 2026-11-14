// GO-146: the moderation page as files — nothing inline (it carries a strict
// CSP of its own), every file it names exists, the site's shared admin bar
// and theme adopted as optional, and every colour a token whose name a site
// theme knows.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";

const PUBLIC = new URL("../public/", import.meta.url);
const read = (name: string) => readFileSync(new URL(name, PUBLIC), "utf8");
const html = read("comments-admin.html");
const css = read("comments-admin.css");
const tokens = read("comments-admin-tokens.css");
// Served by the `core` worker from the site root, not by this template.
const FROM_CORE = ["/core/theme.css", "/core/nav.js"];

test("page: no inline style or script, no handler attribute, a strict CSP of its own", () => {
  assert.doesNotMatch(html, /<style/i);
  assert.doesNotMatch(html, /\sstyle=/i);
  assert.doesNotMatch(html, /\son[a-z]+=/i);
  for (const [, body] of html.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)) assert.equal(body.trim(), "", "a script must be a file");
  assert.match(html, /<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'self'; style-src 'self';[^"]*">/);
  assert.doesNotMatch(html, /unsafe-inline|unsafe-eval/);
  assert.match(html, /<meta name="robots" content="noindex, nofollow">/);
  assert.match(html, /<meta name="description" content="[^"]{40,}">/);
});

test("page: every file it refers to exists; only core's bar and theme come from elsewhere", () => {
  const refs = [...html.matchAll(/(?:src|href)="([^"#][^"]*)"/g)].map((m) => m[1]);
  assert.deepEqual(refs.filter((r) => r.startsWith("/core/")), FROM_CORE);
  const own = refs.filter((r) => !r.startsWith("/core/"));
  assert.deepEqual(own, ["/comments-admin-tokens.css", "/comments-admin.css", "/comments-admin.js"]);
  for (const ref of own) assert.ok(existsSync(new URL(ref.slice(1), PUBLIC)), ref);
});

test("page: core's files exist where the page expects them (in this repository)", { skip: existsSync(new URL("../../core/public/core/", import.meta.url)) ? false : "workers/core is not next to this copy" }, () => {
  for (const ref of FROM_CORE) assert.ok(existsSync(new URL(`../../core/public${ref}`, import.meta.url)), ref);
});

test("page: the bar is adopted in one line above <main>; the theme sits between tokens and styles", () => {
  assert.match(html, /<body>\s*(?:<!--[\s\S]*?-->\s*)?<script type="module" src="\/core\/nav\.js"><\/script><ssg-admin-nav current="comments"><\/ssg-admin-nav>\s*<main id="main">/);
  const sheets = [...html.matchAll(/<link rel="stylesheet" href="([^"]+)">/g)].map((m) => m[1]);
  assert.deepEqual(sheets, ["/comments-admin-tokens.css", "/core/theme.css", "/comments-admin.css"]);
  assert.match(html, /<\/main>\s*<script src="\/comments-admin\.js"><\/script>/);
});

test("styles: no colour literal outside the tokens file; every variable is a token", () => {
  const body = css.replace(/\/\*[\s\S]*?\*\//g, "");
  assert.doesNotMatch(body, /#[0-9a-f]{3,8}\b|\b(?:rgba?|hsla?)\(|:[^;{}]*\b(?:white|black|red|green|blue|gray|grey)\b/i);
  const defined = new Set([...tokens.matchAll(/(--[\w-]+):/g)].map((m) => m[1]));
  const used = [...new Set([...body.matchAll(/var\((--[\w-]+)/g)].map((m) => m[1]))];
  assert.ok(used.length >= 10);
  assert.deepEqual(used.filter((v) => !defined.has(v)), []);
});

test("tokens: both themes name the same overrides; shared names are core's", () => {
  const names = [...tokens.matchAll(/(--[\w-]+):/g)].map((m) => m[1]);
  const shared = [...new Set(names)].filter((n) => !n.includes("comments"));
  // --link (GO-152): links take a theme's accent colour, the primary button its main colour.
  assert.deepEqual(shared.sort(), ["--bg", "--border", "--control", "--font-mono", "--font-ui", "--link", "--on-primary", "--primary", "--text", "--text-muted", "--warning"]);
  const core = new URL("../../core/public/core/tokens.css", import.meta.url);
  if (!existsSync(core)) return;
  const theirs = readFileSync(core, "utf8");
  for (const n of shared) assert.match(theirs, new RegExp(`${n}:`), `${n} is a core token`);
});

// Links are drawn in --link, never in --primary, so a site theme's accent
// reaches them (GO-152); without a theme the two are the same colour, so the
// panel looks as it did. Both schemes keep links readable on the page.
test("tokens: links are --link, equal to --primary until a theme sets them, readable in both schemes", () => {
  assert.match(css, /(?:^|\n)a \{ color: var\(--link\); \}/);
  assert.doesNotMatch(css.replace(/\/\*[\s\S]*?\*\//g, ""), /^a(?::[\w-]+)?\s*\{[^}]*color:\s*var\(--primary\)/m);
  const light = tokens.slice(0, tokens.indexOf("@media"));
  const dark = tokens.slice(tokens.indexOf("@media"));
  const value = (block: string, name: string) => block.match(new RegExp(`${name}:\\s*(#[0-9a-f]{6})`, "i"))?.[1].toLowerCase();
  for (const block of [light, dark]) assert.equal(value(block, "--link"), value(block, "--primary"));
  const lum = (hex: string) => {
    const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  };
  const ratio = (a: string, b: string) => (Math.max(lum(a), lum(b)) + 0.05) / (Math.min(lum(a), lum(b)) + 0.05);
  for (const block of [light, dark]) assert.ok(ratio(value(block, "--link")!, value(block, "--bg")!) >= 4.5);
});

test("script: data is inserted as text only", () => {
  assert.doesNotMatch(read("comments-admin.js"), /\.innerHTML\s*=|insertAdjacentHTML|document\.write/);
});

// The script shows and hides parts of the page with the `hidden` attribute
// (#login, #panel, #who, #notice, #import). A rule giving such an element its
// own `display` (`.login { display: grid }`) outranks the browser's default
// `[hidden] { display: none }`, which is how the sign-in form used to stay on
// screen after signing in. One `[hidden]` rule with !important beats any
// author rule without it, so the check is: the rule exists, and no other
// declaration in the panel's CSS sets `display` with !important (the only
// thing that could beat it).
test("styles: [hidden] wins over any display rule, so hidden parts stay hidden", () => {
  const css = read("comments-admin.css").replace(/\/\*[\s\S]*?\*\//g, "");
  assert.match(css, /\[hidden\]\s*\{\s*display:\s*none\s*!important;?\s*\}/);
  const important = [...css.matchAll(/([^{}]+)\{[^}]*display:[^;}]*!important/g)].map((m) => m[1].trim());
  for (const selector of important) {
    assert.ok(selector === "[hidden]" || selector.includes("#who"), `${selector} sets display with !important and could beat [hidden]`);
  }
  for (const id of ["login", "panel", "who", "notice"]) assert.match(read("comments-admin.html"), new RegExp(`id="${id}"`));
});
