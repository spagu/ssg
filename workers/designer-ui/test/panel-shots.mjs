// Screenshots of the designer panel's views, light and dark, wide and phone
// sized, plus an optional axe pass — against the dev server (dev-server.mjs).
// A developer tool, not part of the test run; it needs Playwright on the
// machine (never a dependency of this repository):
//
//   node --import ./workers/designer-ui/test/resolve.mjs workers/designer-ui/test/dev-server.mjs &
//   node workers/designer-ui/test/panel-shots.mjs <out-dir> [axe.min.js]
//   (ONLY=options,files limits the views; LOCALE=pl-PL the browser language;
//   AS=reader shoots as another demo account; PLAYWRIGHT=… the module path)
import { mkdir, readFile, readdir } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";

const out = process.argv[2] ?? "panel-shots";
const axePath = process.argv[3] ?? null;
const BASE = process.env.PANEL_URL ?? "http://localhost:8788/designer/";
const AS = process.env.AS ?? "owner";
const { chromium } = await import(process.env.PLAYWRIGHT ?? "/usr/lib/node_modules/playwright/index.mjs");
await mkdir(out, { recursive: true });

async function chromiumPath() {
  if (process.env.CHROMIUM) return process.env.CHROMIUM;
  const dir = join(homedir(), ".cache", "ms-playwright");
  const builds = (await readdir(dir).catch(() => [])).filter((d) => /^chromium-\d+$/.test(d)).sort();
  return builds.length ? join(dir, builds[builds.length - 1], "chrome-linux64", "chrome") : undefined;
}
const browser = await chromium.launch({ executablePath: await chromiumPath() });

const TOKENS = "templates/ssgtheme/css/tokens.css";
const VIEWS = [
  ["options", "#/"],
  ["options-urls", "#/options/urls"],
  ["options-advanced", "#/options/advanced"],
  ["options-search", "#/options/site?q=feed"],
  ["options-edited", "#/"],
  ["appearance", "#/appearance"],
  ["appearance-failing", "#/appearance"],
  ["files", `#/files?path=${encodeURIComponent(TOKENS)}`],
  ["files-image", `#/files?path=${encodeURIComponent("templates/ssgtheme/img/logo.svg")}`],
  ["publish", "#/"],
];

// prepare brings a view into the state worth showing.
async function prepare(page, name) {
  if (name === "options-edited" || name === "publish") {
    await page.fill("#f-title", "Example Kitchen, Garden and Cellar");
    await page.click("#f-description");
    await page.fill("#f-description", "");
    await page.waitForTimeout(100);
  }
  if (name === "publish") {
    await page.click(".pending-bar .btn-primary");
    await page.waitForSelector("dialog[open]");
  }
  if (name === "appearance-failing") {
    await page.fill("#tok---color-fg-muted-light-hex", "#8a96a8");
    await page.waitForTimeout(100);
  }
  if (name === "files") await page.click(".files-layout .btn");
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
    page.on("console", (m) => m.type() === "error" && console.error("console", m.text()));
    for (const [name, hash] of VIEWS) {
      if (only && !only.includes(name)) continue;
      await page.goto("about:blank");
      await page.goto(BASE + hash);
      await page.waitForSelector("h1");
      await page.waitForLoadState("networkidle").catch(() => {});
      await prepare(page, name).catch((e) => console.error(`prepare ${name}: ${e.message}`));
      await page.waitForTimeout(400);
      if (name !== "publish") await page.evaluate(() => document.activeElement?.blur());
      const file = join(out, `${name}-${theme}-${size}.png`);
      if (size === "360" && name !== "publish") await phoneScreens(page, file);
      else await page.screenshot({ path: file, fullPage: name !== "publish" && size !== "360" });
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      if (overflow > 0) console.error(`horizontal overflow ${overflow}px on ${name} ${theme} ${size}`);
      if (axe) {
        await page.evaluate(axe);
        report[`${name}-${theme}-${size}`] = await page.evaluate(async () => (await window.axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"] } })).violations.map((v) => ({ id: v.id, impact: v.impact, nodes: v.nodes.length, target: v.nodes.slice(0, 3).map((n) => n.target.join(" ")) })));
      }
      console.log(file);
    }
    await context.close();
  }
}
if (axe) console.log(JSON.stringify(report, null, 1));
await browser.close();
