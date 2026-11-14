// Screenshots of the admin home page in each state and of the shared
// navigation in a bare panel, light and dark, wide and phone-sized, with an
// automated accessibility pass and a keyboard walk — against the dev server.
// A developer tool, not part of the test run; it needs Playwright installed on
// the machine (never a dependency of this repository):
//
//   node --import ./workers/core/test/resolve.mjs workers/core/test/dev-server.mjs &
//   node workers/core/test/core-shots.mjs <out-dir> [axe.min.js]
//
// PLAYWRIGHT=/path/to/playwright/index.mjs when it is not the global install;
// CHROMIUM=… for a browser build other than the newest under ~/.cache/ms-playwright.
import { mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";

const out = process.argv[2] ?? "core-shots";
const axePath = process.argv[3] ?? null;
const BASE = process.env.CORE_URL ?? "http://localhost:8790";
const { chromium } = await import(process.env.PLAYWRIGHT ?? "/usr/lib/node_modules/playwright/index.mjs");

async function chromiumPath() {
  if (process.env.CHROMIUM) return process.env.CHROMIUM;
  const dir = join(homedir(), ".cache", "ms-playwright");
  const builds = (await readdir(dir).catch(() => [])).filter((d) => /^chromium-\d+$/.test(d)).sort();
  return builds.length ? join(dir, builds[builds.length - 1], "chrome-linux64", "chrome") : undefined;
}

await mkdir(out, { recursive: true });
const browser = await chromium.launch({ executablePath: await chromiumPath() });
const axe = axePath ? await readFile(axePath, "utf8") : null;
const report = { axe: {}, errors: [], overflow: [], keyboard: [] };

// [name, dev-server switches, path, what to wait for]
const STATES = [
  ["home-good", "scenario=good&mode=auth&as=owner&core=on&theme=none", "/core/", ".cards"],
  ["home-todo", "scenario=todo&mode=auth&as=owner&core=on&theme=none", "/core/", ".cards"],
  ["home-down", "scenario=down&mode=auth&as=owner&core=on&theme=none", "/core/", ".cards"],
  ["home-limited", "scenario=todo&mode=auth&as=limited&core=on&theme=none", "/core/", ".cards"],
  ["home-hint", "scenario=good&mode=password&core=on&theme=none", "/core/", ".cards"],
  ["host-nav", "scenario=good&mode=auth&as=owner&core=on&theme=none", "/dev/host.html", "h1"],
  ["host-absent", "scenario=good&mode=password&core=off&theme=none", "/dev/host.html", "h1"],
];
const SIZES = [["1440", { width: 1440, height: 900 }], ["390", { width: 390, height: 844 }]];

async function settle(page, wait) {
  await page.waitForSelector(wait);
  await page.waitForLoadState("networkidle").catch(() => {});
  await page.waitForTimeout(400);
}

async function check(page, key) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  if (overflow > 0) report.overflow.push(`${key}: ${overflow}px`);
  if (!axe) return;
  await page.evaluate(axe); // through the DevTools protocol: the page's CSP stays on
  report.axe[key] = await page.evaluate(async () =>
    (await window.axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa", "best-practice"] } })).violations.map((v) => ({
      id: v.id,
      impact: v.impact,
      nodes: v.nodes.length,
      target: v.nodes.slice(0, 3).map((n) => n.target.join(" ")),
    })),
  );
}

for (const theme of ["light", "dark"]) {
  for (const [size, viewport] of SIZES) {
    const context = await browser.newContext({ viewport, colorScheme: theme, httpCredentials: { username: "admin", password: "demo" }, locale: "en-GB", reducedMotion: "reduce" });
    const page = await context.newPage();
    page.on("pageerror", (e) => report.errors.push(`pageerror ${e.message}`));
    page.on("console", (m) => m.type() === "error" && report.errors.push(`console ${page.url()} ${m.text()}`));
    page.on("dialog", (d) => (report.errors.push(`dialog ${d.message()}`), d.dismiss()));
    for (const [name, query, path, wait] of STATES) {
      await page.goto(`${BASE}/dev/set?${query}`);
      await page.goto(BASE + path);
      await settle(page, wait);
      const key = `${name}-${theme}-${size}`;
      await page.screenshot({ path: join(out, `${key}.png`), fullPage: true, caret: "initial" });
      await check(page, key);
      if (name === "host-nav" && size === "390") {
        // The disclosure, open, on a phone.
        await page.locator("ssg-admin-nav button.toggle").click();
        await page.screenshot({ path: join(out, `host-nav-open-${theme}-390.png`), caret: "initial" });
      }
      console.log(key);
    }
    await context.close();
  }
}

// The demo looks (GO-152): home, the bar on a host page and the Appearance
// view, light and dark, wide and phone-sized, into <out-dir>/themes/.
{
  const dir = join(out, "themes");
  await mkdir(dir, { recursive: true });
  const VIEWS = [["home", "/core/", ".cards"], ["host-nav", "/dev/host.html", "h1"], ["appearance", "/core/#appearance", ".contrast"]];
  for (const look of ["harbour", "lemon", "plum"]) {
    for (const theme of ["light", "dark"]) {
      for (const [size, viewport] of SIZES) {
        const context = await browser.newContext({ viewport, colorScheme: theme, httpCredentials: { username: "admin", password: "demo" }, locale: "en-GB", reducedMotion: "reduce" });
        const page = await context.newPage();
        page.on("pageerror", (e) => report.errors.push(`pageerror ${e.message}`));
        page.on("console", (m) => m.type() === "error" && report.errors.push(`console ${page.url()} ${m.text()}`));
        await page.goto(`${BASE}/dev/set?scenario=todo&mode=auth&as=owner&core=on&theme=${look}`);
        for (const [name, path, wait] of VIEWS) {
          await page.goto(BASE + path);
          await settle(page, wait);
          const key = `${look}-${name}-${theme}-${size}`;
          await page.screenshot({ path: join(dir, `${key}.png`), fullPage: true, caret: "initial" });
          await check(page, key);
          console.log(key);
        }
        await context.close();
      }
    }
  }
}

// 360px: no horizontal scroll on the busiest state.
{
  const context = await browser.newContext({ viewport: { width: 360, height: 780 }, locale: "en-GB" });
  const page = await context.newPage();
  await page.goto(`${BASE}/dev/set?scenario=todo&mode=auth&as=owner&core=on&theme=none`);
  for (const path of ["/core/", "/dev/host.html"]) {
    await page.goto(BASE + path);
    await settle(page, "h1");
    await check(page, `360${path}`);
  }
  await page.screenshot({ path: join(out, "host-nav-360.png"), caret: "initial" });
  await context.close();
}

// The keyboard walk: Tab through the bar on the home page, follow a module
// link with Enter, and come back through the bar's home link.
{
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: "en-GB" });
  const page = await context.newPage();
  await page.goto(`${BASE}/dev/set?scenario=todo&mode=auth&as=owner&core=on&theme=none`);
  await page.goto(`${BASE}/core/`);
  await settle(page, ".cards");
  const focused = () =>
    page.evaluate(() => {
      let el = document.activeElement;
      while (el?.shadowRoot?.activeElement) el = el.shadowRoot.activeElement;
      return el ? `${el.tagName.toLowerCase()} "${(el.textContent || "").trim().slice(0, 40)}" ${el.getAttribute("href") ?? ""}` : "none";
    });
  for (let i = 0; i < 7; i++) {
    await page.keyboard.press("Tab");
    report.keyboard.push(await focused());
  }
  // Tab until the Comments link of the bar, then Enter.
  for (let i = 0; i < 10 && !(await focused()).includes("/comments-admin.html"); i++) await page.keyboard.press("Tab");
  await page.keyboard.press("Enter");
  await page.waitForURL(/comments-admin/);
  await settle(page, "h1");
  report.keyboard.push(`→ ${page.url()}`);
  for (let i = 0; i < 6 && !(await focused()).includes('"Admin home"'); i++) await page.keyboard.press("Tab");
  report.keyboard.push(await focused());
  await page.keyboard.press("Enter");
  await page.waitForURL(/\/core\/$/);
  report.keyboard.push(`→ ${page.url()}`);
  await context.close();
}

await writeFile(join(out, "report.json"), JSON.stringify(report, null, 1));
console.log(JSON.stringify(report, null, 1));
await browser.close();
