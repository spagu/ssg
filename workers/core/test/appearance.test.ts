// The panels' look (GO-152) on the worker's side: the registry's theme as the
// Go build writes it (test/fixtures/themes/*.json, made by
// internal/admintheme — UPDATE_THEME_FIXTURES=1 go test ./internal/admintheme/
// -run DemoTheme), read defensively, passed on by /api/core/modules (brand)
// and /api/core/info (theme), and laid out by the Appearance view.
import { after, test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import * as modules from "../functions/api/core/modules.ts";
import * as info from "../functions/api/core/info.ts";
import { useRegistry } from "../functions/api/core/_registry.ts";
import { brandOf, readTheme } from "../functions/api/core/_theme.ts";
import { NEUTRAL, ROUTE, adjustmentText, brandColours, colour, failing, hexOf, logos, routeOf, rows, tokenLabel } from "../public/core/js/appearance.js";
import { appearanceView } from "../public/core/js/views/appearance.js";
import { SAMPLE, labelY, points, sampleChart } from "../public/core/js/views/appearance-chart.js";
import { pageTree } from "../public/core/js/page.js";
import { createFormat } from "../public/core/js/format.js";
import { createI18n } from "../public/core/js/i18n.js";
import { find, findAll, render, text } from "../public/core/js/vnode.js";
import en from "../public/core/js/strings/en.js";
import pl from "../public/core/js/strings/pl.js";
import { fakeDoc, fire } from "./dom.ts";
import type { FakeEl } from "./dom.ts";
import { PASSWORD, PW_ENV, basic, call, freshIP, registry } from "./harness.ts";

const fixture = (name: string) => JSON.parse(readFileSync(new URL(`./fixtures/themes/${name}.json`, import.meta.url), "utf8"));
const HARBOUR = fixture("harbour");
const LEMON = fixture("lemon");
const PLUM = fixture("plum");
const fmt = createFormat("en", "UTC");
const app = { t: createI18n("en", { en }, {}, fmt.number).t, fmt, lang: "en", theme: "auto", route: "appearance", setLang() {}, setTheme() {}, refresh() {} };
after(() => useRegistry());

test("readTheme: the build's theme comes through as written", () => {
  for (const raw of [HARBOUR, LEMON, PLUM]) assert.deepEqual(readTheme(raw), raw);
  assert.equal(readTheme(null), null);
  assert.equal(readTheme([1]), null);
});

test("readTheme: a hand-edited theme can blank a field, never smuggle markup, script or another host", () => {
  const bad = readTheme({
    ...HARBOUR,
    name: "a\u0007b",
    logo: "https://evil.example.com/x.svg",
    logoDark: "javascript:alert(1)",
    fontDisplay: '"x"; } body { display: none',
    fontUi: 7,
    main: "red",
    accent: "#E07A5F",
    derived: ["main", "both", 3],
    light: { primary: "#123456", "--x": "#000000", focus: "url(x)" },
    dark: "no",
    fontFiles: [HARBOUR.fontFiles, { family: "A", src: "//x/y.woff2", format: "woff2", weight: "400", style: "normal" }, { family: "A", src: "/a.woff2", format: "woff2", weight: "400", style: "normal" }, null],
    adjustments: [{ ...HARBOUR.adjustments[0], used: "#zzzzzz" }, HARBOUR.adjustments[0], "x"],
    checks: [{ ...HARBOUR.checks[0], ratio: "4.5" }, HARBOUR.checks[0], null],
  })!;
  assert.deepEqual([bad.name, bad.logo, bad.logoDark, bad.fontDisplay, bad.fontUi, bad.main, bad.accent], [null, null, null, null, null, null, null]);
  assert.deepEqual([bad.derived, bad.light, bad.dark], [["main"], { primary: "#123456" }, {}]);
  assert.deepEqual([bad.fontFiles.length, bad.adjustments.length, bad.checks.length], [1, 1, 1]);
  assert.equal(readTheme({ name: "x".repeat(81) })!.name, null);
  assert.equal(readTheme({ adjustments: "x" })!.adjustments.length, 0);
});

test("brandOf: name and logos for the bar, or null for the shipped bar", () => {
  assert.deepEqual(brandOf(readTheme(HARBOUR)), { name: "Harbour Kitchen", logo: "/dev/logo.svg", logoDark: "/dev/logo-dark.svg" });
  assert.equal(brandOf(null), null);
  assert.equal(brandOf(readTheme({ main: "#123456" })), null);
});

const headers = () => ({ authorization: basic(PASSWORD), "cf-connecting-ip": freshIP() });

test("/api/core/modules carries the brand; /api/core/info the whole theme; both null without one", async () => {
  useRegistry({ ...registry(), theme: HARBOUR });
  const m = (await call(modules.onRequest, { path: "/api/core/modules", headers: headers(), env: PW_ENV })).json();
  assert.deepEqual(m.brand, { name: "Harbour Kitchen", logo: "/dev/logo.svg", logoDark: "/dev/logo-dark.svg" });
  const i = (await call(info.onRequest, { path: "/api/core/info", headers: headers(), env: PW_ENV })).json();
  assert.deepEqual(i.theme, HARBOUR);
  useRegistry(registry());
  assert.equal((await call(modules.onRequest, { path: "/api/core/modules", headers: headers(), env: PW_ENV })).json().brand, null);
  assert.equal((await call(info.onRequest, { path: "/api/core/info", headers: headers(), env: PW_ENV })).json().theme, null);
});

test("model: route, colours, logos, rows and adjustments in words", () => {
  assert.deepEqual([routeOf(ROUTE), routeOf(""), routeOf("#x")], ["appearance", "home", "home"]);
  assert.deepEqual([colour("#abcdef"), colour("#ABCDEF"), colour(3)], ["#abcdef", null, null]);
  assert.deepEqual(brandColours(PLUM), [{ role: "main", hex: "#6a1b9a", derived: false }, { role: "accent", hex: "#4d7000", derived: true }]);
  assert.deepEqual(logos(HARBOUR).map((l) => `${l.where}:${l.src}`), ["light:/dev/logo.svg", "dark:/dev/logo-dark.svg", "bar:/dev/logo-dark.svg"]);
  assert.deepEqual(logos({ logo: null, logoDark: "/d.svg" }).map((l) => l.src), ["/d.svg", "/d.svg", "/d.svg"]);
  assert.deepEqual(logos(LEMON), []);
  assert.equal(hexOf(LEMON, "dark", "surface"), NEUTRAL.dark.surface);
  assert.equal(hexOf(LEMON, "light", "primary"), LEMON.light.primary);
  assert.equal(hexOf(LEMON, "sepia", "x"), null);
  const light = rows(LEMON, "light");
  assert.ok(light.length > 15 && light.every((r) => r.pass && r.fgHex && r.bgHex));
  assert.deepEqual([light[0].kind, light.find((r) => r.minimum === 3)!.kind], ["text", "ui"]);
  assert.equal(rows({ ...LEMON, checks: [{ scheme: "light", fg: "nowhere", bg: "surface", ratio: 1, minimum: 3 }] }, "light").length, 0);
  assert.equal(failing(LEMON), 0);
  assert.equal(failing({ checks: [{ ratio: 1.07, minimum: 4.5 }] }), 1);
  const t = app.t;
  assert.equal(tokenLabel(t, "focus"), "the focus ring");
  assert.equal(tokenLabel(t, "mystery"), "mystery");
  const said = LEMON.adjustments.map((a: unknown) => adjustmentText(t, a));
  assert.ok(said.includes("Light scheme: for primary buttons and the current menu item, #776c04 is used instead of #fff59d, so it stands out enough against cards and fields."), said.join("\n"));
  // GO-152: links follow the accent, so a grey too light for text on white moves for links.
  assert.ok(said.includes("Light scheme: for links, #6a6a6a is used instead of #9e9e9e, so it stands out enough against cards and fields."), said.join("\n"));
  assert.ok(said.includes("Both schemes: for the navigation bar, #635a00 is used instead of #fff59d, so that text in the navigation bar stays readable on it."), said.join("\n"));
  assert.match(adjustmentText(t, { scheme: "dark", token: "focus", used: "#000001", requested: "#000002", reason: "distinct", against: "primary" }), /^Dark scheme: for the focus ring, #000001 is used instead of #000002, so it can be told apart from primary buttons and the current menu item beside it\.$/);
  const polish = createI18n("pl", { en, pl }, {}, fmt.number).t;
  assert.match(adjustmentText(polish, LEMON.adjustments[0]), /^Oba schematy: dla elementu „pasek nawigacji” użyto #635a00 zamiast #fff59d/);
});

test("view: palette with ratios for both schemes, adjustments, samples, fonts, the logo — values always as text", () => {
  const doc = fakeDoc();
  const tree = appearanceView(app, HARBOUR);
  const all = { tag: "div", props: {}, children: tree };
  const words = text(all);
  for (const s of ["Appearance", "edit admin_theme in .ssg.yaml and rebuild", "Harbour Kitchen", "#1b3a4b", "#e07a5f", "Light scheme", "Dark scheme", "Passes", "4.5:1 (text)", "3:1 (marks and edges)", "Georgia", "ssg's own font stack", "A chart with two series"]) assert.ok(words.includes(s), s);
  const tables = findAll(all, (n) => n.tag === "table");
  assert.equal(tables.length, 2);
  // A fill is a validated #rrggbb, never anything else.
  for (const n of findAll(all, (n) => n.props.fill !== undefined)) assert.match(String(n.props.fill), /^#[0-9a-f]{6}$|^none$/);
  // The logo: alt is the name; a failed load leaves the name.
  const img = find(all, (n) => n.tag === "img")!;
  assert.deepEqual([img.props.src, img.props.alt], ["/dev/logo.svg", "Harbour Kitchen"]);
  const el = render(img, doc) as unknown as FakeEl;
  let replaced: unknown = null;
  fire(el, "error", { currentTarget: { ownerDocument: doc, replaceWith: (n: unknown) => (replaced = n) } });
  assert.deepEqual(replaced, { text: "Harbour Kitchen" });
  // A failing pair says so in words, not by colour.
  const broken = appearanceView(app, { ...HARBOUR, checks: [{ scheme: "light", fg: "primary", bg: "surface", ratio: 1.07, minimum: 4.5 }] });
  const row = find({ tag: "div", props: {}, children: broken }, (n) => n.tag === "tr" && n.props.class === "fails")!;
  assert.match(text(row), /1\.07:1.*Too low/);
  // Plum: a derived accent, a name and no fonts, a logo that may fail.
  const plum = text({ tag: "div", props: {}, children: appearanceView(app, PLUM) });
  assert.match(plum, /not set, so the build derived it/);
  // Lemon: no logo, no adjustment-free sentence; font files listed when there are some.
  const lemon = text({ tag: "div", props: {}, children: appearanceView(app, { ...LEMON, adjustments: [], fontFiles: [{ family: "Brand", src: "/f.woff2", format: "woff2", weight: "400", style: "normal" }], name: null }) });
  for (const s of ["No logo is set", "Every brand colour could be used exactly as given.", "Brand (400, normal) — /f.woff2", "not set — the navigation shows its own"]) assert.ok(lemon.includes(s), s);
  // No theme at all: how to set one.
  const none = text({ tag: "div", props: {}, children: appearanceView(app, null) });
  assert.match(none, /sets no admin_theme[\s\S]*main: "#1b3a4b"/);
});

test("sample chart: two series told apart by line, marker and label, not only colour", () => {
  const chart = sampleChart(app);
  const s1 = find(chart, (n) => n.props.class === "series s1")!;
  const s2 = find(chart, (n) => n.props.class === "series s2")!;
  assert.equal(findAll(s1, (n) => n.tag === "circle").length, SAMPLE.one.length);
  assert.equal(findAll(s2, (n) => n.tag === "rect").length, SAMPLE.two.length);
  assert.deepEqual([text(find(s1, (n) => n.tag === "text")!), text(find(s2, (n) => n.tag === "text")!)], ["Series 1", "Series 2"]);
  assert.equal(points([0, 10]).length, 2);
  assert.deepEqual([labelY(10, 50), labelY(40, 44), labelY(44, 40)], [[10, 50], [34, 50], [50, 34]]);
  assert.match(text(chart), /solid and dashed/);
});

test("page: the appearance route draws loading, an error, or the view; the home route is unchanged", () => {
  const head = (tree: unknown[]) => text({ tag: "div", props: {}, children: tree });
  assert.match(head(pageTree(app, null, true)), /Asking the modules/);
  assert.match(head(pageTree(app, { error: { kind: "network" } }, false)), /That did not work/);
  let retried = 0;
  const failed = pageTree({ ...app, refresh: () => retried++ }, { error: { kind: "network" } }, false);
  find({ tag: "div", props: {}, children: failed }, (n) => n.tag === "button")!.props.on.click();
  assert.equal(retried, 1);
  assert.match(head(pageTree(app, { error: null, info: { failed: { kind: "server" } } }, false)), /That did not work/);
  assert.match(head(pageTree(app, { error: null, info: { theme: HARBOUR } }, false)), /Palette and contrast/);
  assert.match(head(pageTree(app, { error: null, info: {} }, false)), /sets no admin_theme/);
  assert.equal(fmt.ratio(4.499), "4.49");
  assert.equal(fmt.ratio(21), "21.0");
  assert.equal(fmt.ratio("x"), "0.0");
});
