// GO-179: a theme's tokens file as colour and font fields, the edit that
// changes one value and nothing else, and the text/background pairs checked
// for contrast (public/js/models/tokens-css.js, pairs.js) — on a sample and
// on the tokens of the themes ssg ships.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { declarations, findTokensFile, kindOf, resolve, setToken, tokens, valueMap } from "../public/js/models/tokens-css.js";
import { evaluate, failingWith, pairsOf } from "../public/js/models/pairs.js";

const CSS = `/* tokens */
:root {
  --bg: #ffffff; /* page */
  --surface: #f2f3f6;
  --text: #0f172a;
  --text-muted: #64748b;
  --link: var(--brand, #0050a6);
  --primary: #00695c;
  --on-primary: #ffffff;
  --font-ui: "Inter", system-ui, sans-serif;
  --font-size-base: 1rem;
  --space-2: 0.5rem;
  --palette-ink-500: #64748b;
}
.card { --bg: #000000; }
@media (min-width: 40rem) { :root { --space-2: 1rem; } }
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #121212;
    --text: #eceff1;
    --primary: #80cbc4;
    --on-primary: #004d40;
  }
}
:root[data-theme="dark"] {
  --bg: #121212;
  --text: #eceff1;
  --primary: #80cbc4;
  --on-primary: #004d40;
}
@supports (color: red) { :root { --nested: #fff; } }
`;

test("declarations: light and dark :root rules only, with exact offsets", () => {
  const d = declarations(CSS);
  for (const x of d) assert.equal(CSS.slice(x.start, x.end), x.value);
  assert.equal(d.filter((x) => x.name === "--bg").map((x) => x.scheme).join(","), "light,dark,dark");
  assert.ok(!d.some((x) => x.value === "#000000"), "a .card rule is not the theme's tokens");
  assert.ok(!d.some((x) => x.name === "--nested"), "nothing nested in @supports");
  assert.equal(d.filter((x) => x.name === "--space-2").length, 1, "a width query overrides; the token is the base value");
  assert.deepEqual(declarations("/* :root { --x: #fff; } */"), [], "a comment is not a rule");
  assert.deepEqual(declarations(":root[data-theme=light] { --x: #fff; } @media (prefers-color-scheme: light) { :root { --y: #000; } } @media print { :root { --z: #000; } }").map((x) => `${x.name}:${x.scheme}`), ["--x:light", "--y:light"]);
  assert.deepEqual(declarations("html { --x: #fff; }"), []);
  assert.deepEqual(declarations("} :root { --x: #fff; }").map((x) => x.name), ["--x"], "a stray brace is skipped");
  assert.deepEqual(tokens(':root[data-theme="dark"] { --only-dark: #000; }'), [{ name: "--only-dark", kind: "colour", light: null, dark: "#000" }]);
});

test("tokens: colours and fonts, light and dark", () => {
  const t = tokens(CSS);
  assert.deepEqual(t.map((x) => `${x.name}:${x.kind}`), ["--bg:colour", "--surface:colour", "--text:colour", "--text-muted:colour", "--primary:colour", "--on-primary:colour", "--font-ui:font", "--palette-ink-500:colour"]);
  assert.deepEqual(t.find((x) => x.name === "--bg"), { name: "--bg", kind: "colour", light: "#ffffff", dark: "#121212" });
  assert.equal(t.find((x) => x.name === "--surface")!.dark, null, "not restated for dark");
  assert.equal(kindOf("--font-size-base", "1rem"), null);
  assert.equal(kindOf("--font-weight-bold", "700"), null);
  assert.equal(kindOf("--font-sans", "var(--x)"), null);
  assert.equal(kindOf("--font-sans", "Georgia, serif"), "font");
  assert.equal(kindOf("--x", "#abc"), "colour");
});

test("resolve follows var() with fallbacks, and stops on a loop", () => {
  const all = valueMap(CSS + ":root { --brand: #112233; --a: var(--b); --b: var(--a); --c: var(--nope, var(--text)); --d: var(--nope); --e: 1px; }");
  assert.equal(resolve(all, "--link", "light"), "#112233");
  assert.equal(resolve(all, "--text", "dark"), "#eceff1");
  assert.equal(resolve(all, "--surface", "dark"), "#f2f3f6", "dark falls back to light");
  assert.equal(resolve(all, "--a", "light"), null);
  assert.equal(resolve(all, "--c", "dark"), "#eceff1");
  assert.equal(resolve(all, "--d", "light"), null);
  assert.equal(resolve(all, "--e", "light"), null);
  assert.equal(resolve(all, "--missing", "light"), null);
  assert.equal(resolve(valueMap(CSS), "--link", "light"), "#0050a6");
  const darkOnly = valueMap(':root[data-theme="dark"] { --x: #000; }');
  assert.equal(resolve(darkOnly, "--x", "light"), "#000", "light falls back to dark");
});

test("setToken changes one value in every copy of its scheme, nothing else", () => {
  const out = setToken(CSS, "--text", "dark", "#ffffff")!;
  assert.equal(out.split("#ffffff").length - CSS.split("#ffffff").length, 2);
  assert.equal(out.replaceAll("--text: #ffffff;", "--text: #eceff1;"), CSS);
  const light = setToken(CSS, "--bg", "light", "#fafafa")!;
  assert.ok(light.includes("--bg: #fafafa; /* page */"), "the comment stays");
  assert.ok(light.includes(".card { --bg: #000000; }"));
  assert.equal(setToken(CSS, "--surface", "dark", "#000"), null, "dark does not restate it");
});

test("findTokensFile prefers css/tokens.css", () => {
  assert.equal(findTokensFile(["t/a/tokens.css", "t/css/tokens.css"], "t"), "t/css/tokens.css");
  assert.equal(findTokensFile(["t/b/x/tokens.css", "t/a/tokens.css", "t/c/tokens.css"], "t"), "t/a/tokens.css");
  assert.equal(findTokensFile(["t/style.css", "t/mytokens.css"], "t"), null);
  assert.equal(findTokensFile(["t/d/tokens.css", "t/c/tokens.css", "t/b/tokens.css"], "t"), "t/b/tokens.css", "same length: by name");
  assert.equal(findTokensFile(["t/b/tokens.css", "t/c/tokens.css"], "t"), "t/b/tokens.css");
});

test("pairs: text on backgrounds, on-X on X, X-fg on X-bg*, inverse on inverse", () => {
  const names = ["--color-bg-canvas", "--color-bg-inverse", "--color-fg-primary", "--color-fg-onInverse", "--color-accent-bg", "--color-accent-bgHover", "--color-accent-fg", "--primary", "--on-primary", "--palette-ink-50", "--on-nothing", "--border"];
  assert.deepEqual(pairsOf(names).map((p) => `${p.fg}>${p.bg}`), [
    "--color-fg-primary>--color-bg-canvas", "--color-fg-onInverse>--color-bg-inverse",
    "--color-accent-fg>--color-accent-bg", "--color-accent-fg>--color-accent-bgHover",
    "--on-primary>--primary",
  ]);
  assert.deepEqual(pairsOf(["--ad-text", "--ad-bg", "--ps-fg-muted", "--ps-bg-subtle"]).map((p) => `${p.fg}>${p.bg}`), ["--ad-text>--ad-bg", "--ad-text>--ps-bg-subtle", "--ps-fg-muted>--ad-bg", "--ps-fg-muted>--ps-bg-subtle"], "a short theme prefix is not part of the role");
  assert.deepEqual(pairsOf(["--text", "--text"]), [], "a token is never its own pair");
});

test("evaluate both schemes; failingWith blames only what the panel changed", () => {
  const all = valueMap(CSS);
  const pairs = pairsOf([...all.keys()]);
  const r = evaluate(pairs, all);
  const muted = r.find((x) => x.fg === "--text-muted" && x.bg === "--surface" && x.scheme === "light")!;
  assert.equal(muted.passes, false);
  assert.ok(muted.suggestion!.ratio >= 4.5);
  assert.ok(r.every((x) => x.scheme === "light" || x.scheme === "dark"));
  assert.equal(r.filter((x) => x.fg === "--on-primary").length, 2);
  assert.deepEqual(failingWith(r, new Set(["--primary"])), []);
  // muted text on surface in both schemes (surface is not restated for dark),
  // and dark text on that light surface — a real finding, not a false alarm
  assert.deepEqual(failingWith(r, new Set(["--surface"])).map((x) => `${x.scheme} ${x.fg}`).sort(), ["dark --text", "dark --text-muted", "light --text-muted"]);
  assert.deepEqual(evaluate([{ fg: "--font-ui", bg: "--bg" }], all), [], "not colours: no result");
});

const SHIPPED = ["ssgtheme", "picostore", "apidoc"].map((t) => new URL(`../../../templates/${t}/css/tokens.css`, import.meta.url));
test("the themes ssg ships: every colour token readable, every pair measured", { skip: SHIPPED.every((u) => existsSync(u)) ? false : "templates/ is not next to this copy" }, () => {
  for (const url of SHIPPED) {
    const css = readFileSync(url, "utf8");
    const t = tokens(css);
    assert.ok(t.filter((x) => x.kind === "colour").length >= 10, url.pathname);
    for (const d of declarations(css)) assert.equal(css.slice(d.start, d.end), d.value);
    const all = valueMap(css);
    const r = evaluate(pairsOf([...all.keys()]), all);
    assert.ok(r.length >= 4, url.pathname);
    for (const x of r) assert.ok(x.ratio! > 1, `${x.fg} on ${x.bg}`);
  }
});
