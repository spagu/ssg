// GO-173: the panel's files as the worker serves them — index.html's meta
// tags and nothing the CSP blocks or that loads from elsewhere; no DOM built
// from strings, no comparator-less sort, no literal non-ASCII in code;
// every file referenced exists; the worker serves them behind the gate with
// its headers; every colour in the CSS is a token, and every token pair
// meets WCAG 2.2 in both schemes.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, normalize } from "node:path";
import { makeEnv, request, run } from "./fixtures.ts";
import { CSP } from "../src/http.ts";
import { PERM } from "../src/permissions.ts";

const PUBLIC = new URL("../public/", import.meta.url).pathname;
const html = readFileSync(join(PUBLIC, "index.html"), "utf8");
const files = (dir: string): string[] => readdirSync(dir).flatMap((f) => (statSync(join(dir, f)).isDirectory() ? files(join(dir, f)) : [join(dir, f)]));
const code = files(PUBLIC).filter((f) => f.endsWith(".js"));

test("index.html: meta tags, noindex, both theme colours, no tag manager, core after own tokens, the shared bar", () => {
  for (const re of [/<meta charset="utf-8">/, /<meta name="viewport" content="width=device-width, initial-scale=1">/, /<title>[^<]+<\/title>/, /<meta name="description" content="[^"]{40,}">/, /<meta name="robots" content="noindex, nofollow, noarchive">/, /<meta name="referrer" content="no-referrer">/, /<meta name="color-scheme" content="light dark">/, /<html lang="en">/, /<a class="skip" href="#main"/, /aria-live="polite"/]) assert.match(html, re);
  assert.doesNotMatch(html, /googletagmanager|gtag\(|GTM-/i, "an admin tool is not measured");
  const sheets = [...html.matchAll(/<link rel="stylesheet" href="([^"]+)">/g)].map((m) => m[1]);
  assert.deepEqual(sheets.slice(0, 2), ["css/tokens.css", "/core/theme.css"]);
  assert.match(html, /<script type="module" src="\/core\/nav\.js"><\/script><ssg-admin-nav current="editor-ui"><\/ssg-admin-nav>/);
});

test("nothing the CSP blocks, nothing from elsewhere, no HTML from strings, no contenteditable", () => {
  assert.doesNotMatch(html, /style=|<style|\son[a-z]+=/i);
  for (const [, , body] of html.matchAll(/<script([^>]*)>([\s\S]*?)<\/script>/g)) assert.equal(body.trim(), "");
  for (const file of files(PUBLIC)) {
    const text = readFileSync(file, "utf8");
    assert.deepEqual([...text.matchAll(/(?:src|href|url\()\s*=?\s*["(]?(https?:)?\/\/[^"')\s]+/g)].map((m) => m[0]), [], file);
    assert.doesNotMatch(text, /\.innerHTML\s*=|outerHTML\s*=|insertAdjacentHTML|document\.write|eval\(|new Function|contenteditable["=:]|isContentEditable/, file);
  }
});

test("code hygiene SonarCloud checks: sort with a comparator; no literal non-ASCII outside strings and comments", () => {
  for (const file of code) {
    const text = readFileSync(file, "utf8");
    assert.doesNotMatch(text, /\.sort\(\)/, `${file}: sort() without a comparator`);
    if (file.includes("/strings/")) continue;
    for (const [i, line] of text.split("\n").entries()) {
      const bare = line.replace(/\/\/.*$/, "").replace(/^\s*\*.*$/, "");
      assert.doesNotMatch(bare, /[^\x00-\x7f]/, `${file}:${i + 1} has a literal non-ASCII character in code`);
    }
  }
});

test("every file a page or module refers to exists", () => {
  const refs: [string, string][] = [];
  for (const [, ref] of html.matchAll(/(?:src|href)="([^"#/][^"]*)"/g)) refs.push([PUBLIC, ref]);
  for (const file of code) {
    const text = readFileSync(file, "utf8");
    for (const [, ref] of text.matchAll(/(?:from|import\()\s*["']([^"'$`]+)["']/g)) refs.push([dirname(file), ref]);
    for (const [, name] of text.matchAll(/img: "([\w-]+)"/g)) refs.push([PUBLIC, `img/${name}.svg`]);
  }
  assert.ok(refs.length > 40);
  for (const [base, ref] of refs) assert.ok(existsSync(normalize(join(base, ref))), `${ref} (from ${base})`);
});

test("the worker serves the panel behind the gate, with the CSP", async () => {
  const assets = { fetch: async (r: Request) => { const p = new URL(r.url).pathname; const f = join(PUBLIC, p === "/" ? "index.html" : p); return existsSync(f) ? new Response(readFileSync(f)) : new Response("not found", { status: 404 }); } };
  const env = makeEnv({ ASSETS: assets });
  assert.ok([302, 401].includes((await run(await request("/editor/", null), env)).status), "no session: refused");
  const page = await run(await request("/editor/", [PERM.read]), env);
  assert.equal(page.status, 200);
  assert.equal(page.headers.get("content-security-policy"), CSP);
  assert.doesNotMatch(CSP, /unsafe-inline|unsafe-eval/);
  assert.match(CSP, /img-src 'self' data: blob:/, "pasted images preview from blob: addresses");
  assert.match(await page.text(), /js\/main\.js/);
  for (const p of ["/js/main.js", "/css/tokens.css", "/img/mark.svg", "/js/strings/hi.js"]) assert.equal((await run(await request("/editor" + p, [PERM.read]), env)).status, 200, p);
  assert.equal((await run(await request("/editor/js/main.js", []), env)).status, 403, "no read permission: not even the files");
});

// tokens: --name → {light, dark} from tokens.css (media-query block = dark).
const css = readFileSync(join(PUBLIC, "css/tokens.css"), "utf8");
function block(src: string): Map<string, string> {
  return new Map([...src.matchAll(/(--[\w-]+):\s*(#[0-9a-f]{6})/gi)].map((m) => [m[1], m[2].toLowerCase()]));
}
const light = block(css.slice(0, css.indexOf("@media")));
const dark = block(css.slice(css.indexOf("@media"), css.indexOf(':root[data-theme="dark"]')));
const darkAttr = block(css.slice(css.indexOf(':root[data-theme="dark"]')));
const lum = (hex: string) => {
  const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((v) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
};
const ratio = (a: string, b: string) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05); };
const TEXT = [["text", "bg"], ["text", "surface"], ["text-muted", "bg"], ["text-muted", "surface"], ["text-muted", "surface-sunk"], ["text", "surface-sunk"], ["primary", "surface"], ["primary", "bg"], ["on-primary", "primary"], ["link", "surface"], ["link", "bg"], ["on-primary-soft", "primary-soft"], ["on-danger", "danger"], ["danger", "surface"], ["critical", "surface"], ["ok", "surface"], ["text", "diff-add"], ["text", "diff-del"], ["text", "primary-soft"]];
const EDGE = [["control", "surface"], ["control", "bg"], ["control", "surface-sunk"], ["focus", "surface"], ["focus", "bg"], ["warning", "surface"], ["primary", "primary-soft"]];

for (const [scheme, map] of [["light", light], ["dark", new Map([...light, ...dark])]] as const) {
  test(`tokens.css ${scheme}: text 4.5:1, marks and edges 3:1`, () => {
    for (const [pairs, min] of [[TEXT, 4.5], [EDGE, 3]] as const) {
      for (const [fg, bg] of pairs) {
        const r = ratio(map.get(`--${fg}`)!, map.get(`--${bg}`)!);
        assert.ok(r >= min, `${scheme}: --${fg} on --${bg} is ${r.toFixed(2)}:1, below ${min}:1`);
      }
    }
  });
}

test("every colour in the CSS is a token; both dark copies agree", () => {
  for (const f of files(join(PUBLIC, "css")).filter((x) => !x.endsWith("tokens.css"))) {
    const text = readFileSync(f, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
    assert.doesNotMatch(text, /#[0-9a-f]{3,8}\b|rgba?\(|hsla?\(/i, `${f} writes a colour outside tokens.css`);
  }
  assert.deepEqual([...dark], [...darkAttr]);
});
