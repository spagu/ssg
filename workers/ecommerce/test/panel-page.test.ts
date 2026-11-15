// GO-156: the panel page as files — nothing inline under a strict CSP of its
// own, every file it names present, the site's shared admin bar and theme
// adopted as optional, every colour a token whose name a site theme knows.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";

const PANEL = new URL("../public/ecommerce-admin/", import.meta.url);
const read = (name: string) => readFileSync(new URL(name, PANEL), "utf8");
const html = read("index.html");
const css = read("admin.css");
const tokens = read("tokens.css");
const FROM_CORE = ["/core/theme.css", "/core/nav.js"];
const uncommented = (s: string) => s.replace(/\/\*[\s\S]*?\*\//g, "");

test("page: no inline style or script, no handler attribute, a strict CSP of its own", () => {
  const markup = html.replace(/<!--[\s\S]*?-->/g, "");
  assert.doesNotMatch(markup, /<style/i);
  assert.doesNotMatch(markup, /\sstyle=/i);
  assert.doesNotMatch(markup, /\son[a-z]+=/i);
  for (const [, body] of markup.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)) assert.equal(body.trim(), "", "a script must be a file");
  assert.match(html, /<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'self'; style-src 'self';[^"]*form-action 'self'">/);
  assert.doesNotMatch(html, /unsafe-inline|unsafe-eval/);
});

test("page: every file it refers to exists; only core's bar and theme come from elsewhere", () => {
  const refs = [...html.matchAll(/(?:src|href)="(\/[^"#]*)"/g)].map((m) => m[1]!).filter((r) => !r.startsWith("/api/"));
  assert.deepEqual(refs.filter((r) => r.startsWith("/core/")), FROM_CORE);
  for (const ref of refs.filter((r) => !r.startsWith("/core/"))) {
    assert.ok(ref.startsWith("/ecommerce-admin/"), ref);
    assert.ok(existsSync(new URL(ref.slice("/ecommerce-admin/".length), PANEL)), ref);
  }
  const core = new URL("../../core/public", import.meta.url);
  if (existsSync(core)) for (const ref of FROM_CORE) assert.ok(existsSync(new URL(`../../core/public${ref}`, import.meta.url)), ref);
});

test("page: the bar is adopted in one line after the skip link; the theme sits between tokens and styles", () => {
  assert.match(html, /<a class="skip" href="#main">[^<]+<\/a>\s*(?:<!--[\s\S]*?-->\s*)?<script type="module" src="\/core\/nav\.js"><\/script><ssg-admin-nav current="ecommerce"><\/ssg-admin-nav>\s*(?:<!--[\s\S]*?-->\s*)?<div class="brandbar"/);
  const sheets = [...html.matchAll(/<link rel="stylesheet" href="([^"]+)">/g)].map((m) => m[1]);
  assert.deepEqual(sheets, ["/ecommerce-admin/tokens.css", "/core/theme.css", "/ecommerce-admin/admin.css"]);
});

test("styles: the bar gets a row of its own, and the panel's own sign-out steps aside for the bar's", () => {
  assert.match(css, /ssg-admin-nav \{ grid-area: bar; \}/);
  assert.match(css, /grid-template-areas: "bar bar" "brand top" "nav main";/);
  assert.match(css, /grid-template-areas: "bar" "brand" "top" "nav" "main";/);
  assert.match(css, /ssg-admin-nav\[data-signout\] ~ \.topbar #whoami,\s*ssg-admin-nav\[data-signout\] ~ \.topbar #logout \{ display: none !important; \}/);
});

test("styles: no colour literal outside the tokens file; every variable is a token", () => {
  const body = uncommented(css);
  assert.doesNotMatch(body, /#[0-9a-f]{3,8}\b|\b(?:rgba?|hsla?)\(|:[^;{}]*\b(?:white|black|red|green|blue|gray|grey)\b/i);
  const defined = new Set([...tokens.matchAll(/(--[\w-]+):/g)].map((m) => m[1]));
  defined.add("--ecommerce-sidebar");
  const used = [...new Set([...body.matchAll(/var\((--[\w-]+)/g)].map((m) => m[1]))];
  assert.ok(used.length >= 20);
  assert.deepEqual(used.filter((v) => !defined.has(v)), []);
});

test("tokens: both schemes name the same overrides; every shared name is core's", () => {
  const light = tokens.slice(0, tokens.indexOf("@media"));
  const dark = tokens.slice(tokens.indexOf("@media"));
  const names = (block: string) => new Set([...uncommented(block).matchAll(/(--[\w-]+):/g)].map((m) => m[1]!));
  for (const n of names(dark)) assert.ok(names(light).has(n), `${n} has no light value`);
  const shared = [...names(light)].filter((n) => !n.startsWith("--ecommerce-"));
  const core = new URL("../../core/public/core/tokens.css", import.meta.url);
  if (!existsSync(core)) return;
  const theirs = readFileSync(core, "utf8");
  for (const n of shared) assert.match(theirs, new RegExp(`${n}:`), `${n} is not a core token: prefix it --ecommerce-`);
});

test("tokens: links are --link and the focus ring --focus, equal to --primary until a theme sets them", () => {
  assert.match(css, /(?:^|\n)a \{ color: var\(--link\); \}/);
  assert.match(css, /:focus-visible \{\s*outline: 2px solid var\(--focus\);/);
  const value = (block: string, name: string) => block.match(new RegExp(`${name}:\\s*(#[0-9a-f]{6})`, "i"))?.[1];
  for (const block of [tokens.slice(0, tokens.indexOf("@media")), tokens.slice(tokens.indexOf("@media"))]) {
    assert.equal(value(block, "--link"), value(block, "--primary"));
    assert.equal(value(block, "--focus"), value(block, "--primary"));
  }
});
