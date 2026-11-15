// Screenshots of the editor panel's views, light and dark, wide and phone
// sized, plus an optional axe pass — against the dev server (dev-server.mjs).
// A developer tool, not part of the test run; it needs Playwright on the
// machine (never a dependency of this repository):
//
//   node --import ./workers/editor-ui/test/resolve.mjs workers/editor-ui/test/dev-server.mjs &
//   node workers/editor-ui/test/panel-shots.mjs <out-dir> [axe.min.js]
//   (ONLY=list,editor limits the views; LOCALE=pl-PL the browser language;
//   AS=reader shoots as another demo account; PLAYWRIGHT=… the module path)
import { mkdir, readFile, readdir } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";

const out = process.argv[2] ?? "panel-shots";
const axePath = process.argv[3] ?? null;
const BASE = process.env.PANEL_URL ?? "http://localhost:8789/editor/";
const AS = process.env.AS ?? "owner";
const { chromium } = await import(process.env.PLAYWRIGHT ?? "/usr/lib/node_modules/playwright/index.mjs");
await mkdir(out, { recursive: true });

async function chromiumPath() {
  if (process.env.CHROMIUM) return process.env.CHROMIUM;
  const dir = join(homedir(), ".cache", "ms-playwright");
  const builds = (await readdir(dir).catch(() => [])).filter((d) => /^chromium-\d+$/.test(d)).sort((a, b) => a.localeCompare(b, "en", { numeric: true }));
  return builds.length ? join(dir, builds[builds.length - 1], "chrome-linux64", "chrome") : undefined;
}
const browser = await chromium.launch({ executablePath: await chromiumPath() });

const ARTICLE = encodeURIComponent("content/blog/posts/first-week.md");
const BREAD = encodeURIComponent("content/blog/posts/bread-notes.md");
const VIEWS = [
  ["list", "#/"],
  ["list-none", "#/?q=nothing-matches-this"],
  ["new", "#/"],
  ["editor", `#/edit/${ARTICLE}`],
  ["publish", `#/edit/${ARTICLE}`],
  ["history", `#/edit/${ARTICLE}`],
  ["conflict", `#/edit/${BREAD}`],
  ["help", `#/edit/${ARTICLE}`],
  ["insert", `#/edit/${ARTICLE}`],
  ["component", `#/edit/${ARTICLE}`],
  ["library", `#/edit/${ARTICLE}`],
];
const DIALOGS = new Set(["new", "publish", "help", "insert", "component", "library"]);

// prepare brings a view into the state worth showing.
async function prepare(page, name) {
  if (name === "new") {
    await page.click(".page-actions .btn-primary");
    await page.waitForSelector("dialog[open]");
    await page.fill("#new-title", "Spring menu");
  }
  if (name === "publish" || name === "conflict") {
    await page.fill("#fm-title", name === "conflict" ? "Notes on bread and time" : "The first week of the kitchen, again");
    await page.click("#md");
    await page.keyboard.press("Control+End");
    await page.keyboard.type("\n- a new line from the editor");
    await page.waitForTimeout(900);
  }
  if (name === "conflict") await page.evaluate(async () => fetch("/demo/touch?path=content%2Fblog%2Fposts%2Fbread-notes.md", { method: "POST" }));
  if (name === "publish" || name === "conflict") {
    await page.click(".page-actions .btn-primary");
    await page.waitForSelector("dialog[open]");
  }
  if (name === "publish") await page.click("dialog details summary");
  if (name === "conflict") {
    await page.click("dialog .btn-primary");
    await page.waitForSelector(".conflict-screen");
  }
  if (name === "history") {
    await page.click("#side-history");
    await page.waitForSelector(".rev-items");
  }
  // GO-174: the Insert list, a component's form with a refused value, the media library.
  if (name === "insert" || name === "component") {
    await page.click(".md-toolbar button:has(.visually-hidden:text-is('Insert a component, shortcode or block'))");
    await page.waitForSelector(".insert-item");
  }
  if (name === "component") {
    await page.fill("#insert-q", "recipe");
    await page.click(".insert-item");
    await page.waitForSelector("#wf-title");
    await page.fill("#wf-title", "Lentil soup");
    await page.fill("#wf-servings", "four");
    await page.fill("#wf-tags", "soup, winter");
    await page.check("#wf-vegetarian");
    await page.click("dialog .btn-primary");
  }
  if (name === "library") {
    await page.click(".md-toolbar button:has(.visually-hidden:text-is('Image'))");
    await page.waitForSelector(".library-item");
  }
  if (name === "help") {
    await page.click(".page-actions .btn-quiet");
    await page.waitForSelector("dialog[open]");
  }
}

async function phoneScreens(page, file) {
  const max = Number(process.env.PHONE_SCREENS ?? 3);
  const { height } = await page.evaluate(() => ({ height: document.documentElement.scrollHeight }));
  for (let i = 0, y = 0; i < max && (i === 0 || y < height - 640); i++, y += 640) {
    await page.evaluate((top) => window.scrollTo(0, top), y);
    await page.waitForTimeout(120);
    await page.screenshot({ path: i ? file.replace(/\.png$/, `-${i + 1}.png`) : file });
  }
  await page.evaluate(() => window.scrollTo(0, 0));
}

const SIZES = [["1280", { width: 1280, height: 860 }], ["360", { width: 360, height: 760 }]];
const only = process.env.ONLY ? process.env.ONLY.split(",") : null;
const axe = axePath ? await readFile(axePath, "utf8") : null;
const report = {};
for (const theme of ["light", "dark"]) {
  for (const [size, viewport] of SIZES) {
    const context = await browser.newContext({ viewport, colorScheme: theme, locale: process.env.LOCALE ?? "en-GB", reducedMotion: "reduce" });
    await context.addCookies([{ name: "demo-as", value: AS, url: BASE }]);
    const page = await context.newPage();
    page.on("pageerror", (e) => console.error("pageerror", e.message));
    page.on("console", (m) => m.type() === "error" && !/404/.test(m.text()) && console.error("console", m.text()));
    for (const [name, hash] of VIEWS) {
      if (only && !only.includes(name)) continue;
      await page.goto("about:blank");
      await page.goto(BASE + hash);
      await page.waitForSelector("h1");
      await page.waitForLoadState("networkidle").catch(() => {});
      await prepare(page, name).catch((e) => console.error(`prepare ${name}: ${e.message}`));
      await page.waitForTimeout(400);
      if (!DIALOGS.has(name)) await page.evaluate(() => document.activeElement?.blur());
      const file = join(out, `${name}-${theme}-${size}.png`);
      if (size === "360" && !DIALOGS.has(name)) await phoneScreens(page, file);
      else await page.screenshot({ path: file, fullPage: !DIALOGS.has(name) && size !== "360" });
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      if (overflow > 0) console.error(`horizontal overflow ${overflow}px on ${name} ${theme} ${size}`);
      if (axe) {
        await page.evaluate(axe);
        report[`${name}-${theme}-${size}`] = await page.evaluate(async () => (await window.axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"] } })).violations.map((v) => ({ id: v.id, impact: v.impact, nodes: v.nodes.length, target: v.nodes.slice(0, 3).map((n) => n.target.join(" ")) })));
      }
      // Leave nothing behind for the next view: a dialog closed, drafts dropped.
      await page.keyboard.press("Escape").catch(() => {});
      await page.evaluate(() => new Promise((r) => { const q = indexedDB.deleteDatabase("ssg-editor"); q.onsuccess = q.onerror = q.onblocked = () => r(null); }));
      console.log(file);
    }
    await context.close();
  }
}
if (axe) console.log(JSON.stringify(report, null, 1));
await browser.close();
