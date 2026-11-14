// Screenshots of every panel view, light and dark, wide and phone-sized, plus
// an automated accessibility pass — against the dev server (dev-server.mjs).
// Phone shots are viewport screens, not one stitched page (see phoneScreens).
// A developer tool, not part of the test run; it needs Playwright installed on
// the machine (it is never a dependency of this repository):
//
//   node --import ./workers/newsletter-ui/test/resolve.mjs workers/newsletter-ui/test/dev-server.mjs &
//   node workers/newsletter-ui/test/panel-shots.mjs <out-dir> [axe.min.js]
//   (ONLY=dashboard,import limits the views; LOCALE=pl-PL the browser language;
//   AUTH_AS=editor|reader|… shoots a DEMO_AUTH=1 server as that account)
//
// PLAYWRIGHT=/path/to/playwright/index.mjs points at the module when it is not
// importable by name (default: the global npm install). With an axe-core
// build passed as the second argument, each view is also checked with axe
// (WCAG 2.0–2.2 A/AA rules) and the violations are printed as JSON.
import { mkdir, readFile, readdir } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";

const out = process.argv[2] ?? "panel-shots";
const axePath = process.argv[3] ?? null;
const BASE = process.env.PANEL_URL ?? "http://localhost:8787/newsletter-admin/";
const { chromium } = await import(process.env.PLAYWRIGHT ?? "/usr/lib/node_modules/playwright/index.mjs");

await mkdir(out, { recursive: true });
// The installed Playwright may expect another browser build than the one
// downloaded: CHROMIUM=… or the newest chromium-*/ under ~/.cache/ms-playwright.
async function chromiumPath() {
  if (process.env.CHROMIUM) return process.env.CHROMIUM;
  const dir = join(homedir(), ".cache", "ms-playwright");
  const builds = (await readdir(dir).catch(() => [])).filter((d) => /^chromium-\d+$/.test(d)).sort();
  return builds.length ? join(dir, builds[builds.length - 1], "chrome-linux64", "chrome") : undefined;
}
const browser = await chromium.launch({ executablePath: await chromiumPath() });
// AUTH_AS: the dev server runs with DEMO_AUTH=1; the views are shot as that
// account, the data below is read as the owner.
const AS = process.env.AUTH_AS ?? null;
const api = async (path) => (await fetch(new URL("api/" + path, BASE), { headers: AS ? { cookie: "demo-as=owner" } : { authorization: "Basic " + btoa("admin:demo") } })).json();
const list = (await api("campaigns?limit=50")).items;
const byStatus = (s) => list.find((c) => c.status === s)?.id;
const firstSub = (await api("subscribers?limit=1")).items[0].id;

const VIEWS = [
  ["dashboard", "#/"],
  ["subscribers", "#/subscribers"],
  ["subscriber", `#/subscribers/${firstSub}`],
  ["import", "#/import"],
  ["import-preview", "#/import"],
  ["campaigns", "#/campaigns"],
  ["editor", `#/campaigns/${byStatus("draft")}`],
  ["campaign-sending", `#/campaigns/${byStatus("sending")}`],
  ["campaign-paused", `#/campaigns/${byStatus("paused")}`],
  ["campaign-sent", `#/campaigns/${list.filter((c) => c.status === "sent")[1]?.id}`],
  ["suppressions", "#/suppressions"],
  ["settings", "#/settings"],
  ["activity", "#/activity"],
  ["help", "#/help"],
];
// A list as a spreadsheet saves it: semicolons, a name column, day/month
// dates that never decide their order — so the panel has to ask.
const CSV = [
  "E-mail;First name;Status;Date subscribed;Unsubscribed at;Groups;Notes",
  'ana.new@example.com;Ana;Subscribed;03/04/2024;;"Weekly;Offers";vip',
  "ben.new@example.com;Ben;Opted out;05/01/2023;02/02/2024;;",
  "cid.new@example.com;Cid;Cleaned;;;;",
  "dan.new@example.com;Dan;;;;news;",
  "not-an-address;Eve;Subscribed;;;;",
  "ana.new@example.com;Ana;Subscribed;;;;",
].join("\r\n");

// prepare brings a view into the state worth showing.
async function prepare(page, name) {
  if (name !== "import-preview" || !(await page.$("#imp-file"))) return;
  await page.setInputFiles("#imp-file", { name: "old-list.csv", mimeType: "text/csv", buffer: Buffer.from(CSV) });
  await page.waitForSelector("input[name=dateOrder]");
  await page.check("input[value=dmy]");
  await page.click(".ask .btn-primary");
  await page.waitForSelector("#pv-title");
  await page.fill("#imp-consent", "Signed up on the old site's form, 2019–2024");
}

// phoneScreens captures what a phone actually shows: the 390×844 viewport,
// screen by screen (…-390.png, …-390-2.png, …), at most PHONE_SCREENS (4). A
// stitched full-page capture paints the sticky bar and the sticky notice in
// the middle of the page, where no phone ever shows them.
async function phoneScreens(page, file) {
  const max = Number(process.env.PHONE_SCREENS ?? 4);
  const { height, step } = await page.evaluate(() => {
    const bar = document.querySelector(".bar")?.getBoundingClientRect().height ?? 0;
    return { height: document.documentElement.scrollHeight, step: Math.max(200, innerHeight - bar - 48) };
  });
  for (let i = 0, y = 0; i < max && (i === 0 || y < height - 844 + step); i++, y += step) {
    await page.evaluate((top) => window.scrollTo(0, top), y);
    await page.waitForTimeout(150);
    await page.screenshot({ path: i ? file.replace(/\.png$/, `-${i + 1}.png`) : file });
  }
  await page.evaluate(() => window.scrollTo(0, 0));
}

const SIZES = [["1440", { width: 1440, height: 900 }], ["390", { width: 390, height: 844 }]];
const only = process.env.ONLY ? process.env.ONLY.split(",") : null;
const axe = axePath ? await readFile(axePath, "utf8") : null;
const report = {};

for (const theme of ["light", "dark"]) {
  for (const [size, viewport] of SIZES) {
    const context = await browser.newContext({ viewport, colorScheme: theme, ...(AS ? {} : { httpCredentials: { username: "admin", password: "demo" } }), locale: process.env.LOCALE ?? "en-GB", reducedMotion: "reduce" });
    if (AS) await context.addCookies([{ name: "demo-as", value: AS, url: BASE }]);
    const page = await context.newPage();
    page.on("pageerror", (e) => console.error("pageerror", e.message));
    page.on("console", (m) => m.type() === "error" && console.error("console", m.text()));
    for (const [name, hash] of VIEWS) {
      if (only && !only.includes(name)) continue;
      await page.goto(BASE + hash);
      await page.waitForSelector("h1");
      await page.waitForLoadState("networkidle").catch(() => {});
      await prepare(page, name);
      await page.waitForTimeout(600);
      // A filled field keeps focus; its ring is not part of the view.
      await page.evaluate(() => document.activeElement?.blur());
      const file = join(out, `${name}-${theme}-${size}.png`);
      if (size === "390") await phoneScreens(page, file);
      else await page.screenshot({ path: file, fullPage: true });
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      if (overflow > 0) console.error(`horizontal overflow ${overflow}px on ${name} ${theme} ${size}`);
      if (axe) {
        // Through the DevTools protocol, not a <script> tag: the panel's CSP
        // (rightly) refuses inline scripts, and it stays switched on here.
        await page.evaluate(axe);
        const r = await page.evaluate(async () => (await window.axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa", "best-practice"] } })).violations.map((v) => ({ id: v.id, impact: v.impact, nodes: v.nodes.length, target: v.nodes.slice(0, 3).map((n) => n.target.join(" ")) })));
        report[`${name}-${theme}-${size}`] = r;
      }
      console.log(file);
    }
    await context.close();
  }
}
if (axe) console.log(JSON.stringify(report, null, 1));
await browser.close();
