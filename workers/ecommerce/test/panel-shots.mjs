// Screenshots of every panel screen — the sign-in screen and each view after
// signing in — light and dark, wide and phone-sized, against the dev server
// (dev-server.mjs). A developer tool, not part of the test run; it needs
// Playwright on the machine (never a dependency of this repository):
//
//   node --import ./workers/ecommerce/test/resolve.mjs workers/ecommerce/test/dev-server.mjs &
//   node workers/ecommerce/test/panel-shots.mjs <out-dir>
//
// PANEL_URL=… points elsewhere (default http://localhost:8788/ecommerce-admin/);
// PLAYWRIGHT=/path/to/playwright/index.mjs when it is not the global npm
// install; CHROMIUM=… a browser binary (default: the newest chromium-*/ under
// ~/.cache/ms-playwright). Motion is reduced, so two runs over the same data
// give the same pixels — which is how a change to the panel's styles is
// checked against the screens before it. Every screen is also measured: a
// page wider than the window (document scrollWidth > innerWidth — the panel
// must not scroll sideways at 360 px) is reported with the screen's name.
import { mkdir, readdir } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";

const out = process.argv[2] ?? "panel-shots";
const BASE = process.env.PANEL_URL ?? "http://localhost:8788/ecommerce-admin/";
const { chromium } = await import(process.env.PLAYWRIGHT ?? "/usr/lib/node_modules/playwright/index.mjs");
const VIEWS = ["dashboard", "products", "orders", "modules", "payments", "settings", "vat", "log"];
const SIZES = { wide: { width: 1280, height: 900 }, phone: { width: 390, height: 844 }, narrow: { width: 360, height: 780 } };

/** overflow: how far the page is wider than the window, in pixels (0 = fits). */
const overflow = (page) => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);

async function chromiumPath() {
  if (process.env.CHROMIUM) return process.env.CHROMIUM;
  const dir = join(homedir(), ".cache", "ms-playwright");
  const builds = (await readdir(dir).catch(() => [])).filter((d) => /^chromium-\d+$/.test(d)).sort();
  return builds.length ? join(dir, builds[builds.length - 1], "chrome-linux64", "chrome") : undefined;
}

await mkdir(out, { recursive: true });
const browser = await chromium.launch({ executablePath: await chromiumPath() });
const problems = [];
for (const scheme of ["light", "dark"]) {
  for (const [size, viewport] of Object.entries(SIZES)) {
    const context = await browser.newContext({ viewport, colorScheme: scheme, reducedMotion: "reduce", locale: "en-GB", timezoneId: "UTC" });
    const page = await context.newPage();
    page.on("console", (m) => m.type() === "error" && !/404/.test(m.text()) && problems.push(`${scheme}/${size}: ${m.text()}`));
    page.on("pageerror", (e) => problems.push(`${scheme}/${size}: ${e.message}`));
    await page.goto(BASE, { waitUntil: "networkidle" });
    await page.locator("#login-password").waitFor();
    await page.screenshot({ path: join(out, `login-${scheme}-${size}.png`), fullPage: true });
    await page.fill("#login-password", "demo");
    await page.click("#login-submit");
    await page.locator("#view-dashboard:not([hidden])").waitFor();
    for (const view of VIEWS) {
      await page.evaluate((v) => (window.location.hash = v), view);
      await page.locator(`#view-${view}:not([hidden])`).waitFor();
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(150);
      const wider = await overflow(page);
      if (wider > 0) problems.push(`${scheme}/${size}: ${view} is ${wider}px wider than the window`);
      await page.screenshot({ path: join(out, `${view}-${scheme}-${size}.png`), fullPage: true });
    }
    await context.close();
  }
}
await browser.close();
if (problems.length) console.log(problems.join("\n"));
console.log(`screenshots in ${out}`);
