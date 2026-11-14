// Screenshots of every view of the accounts admin panel (GO-138), light and
// dark, at 1440×900 and 390×844 real viewports, with an axe-core pass, a
// check that nothing scrolls the page sideways and a log of CSP refusals —
// against the dev server (dev-server.mjs). A developer tool, not part of the
// test run; it needs Playwright on the machine (never a dependency here):
//
//   node --import ./workers/auth/test/resolve.mjs workers/auth/test/dev-server.mjs &
//   node workers/auth/test/admin-shots.mjs <out-dir> [axe.min.js]
//
// Wide shots are the whole page; phone shots the first screen and the whole
// page. AUTH_URL=… points at another server, PLAYWRIGHT=… at the module,
// CHROMIUM=… at a browser, ONLY=people,status limits the views.
import { mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";

const out = process.argv[2] ?? "admin-shots";
const axePath = process.argv[3] ?? null;
const BASE = process.env.AUTH_URL ?? "http://localhost:8788";
const { chromium } = await import(process.env.PLAYWRIGHT ?? "/usr/lib/node_modules/playwright/index.mjs");
await mkdir(out, { recursive: true });

async function chromiumPath() {
  if (process.env.CHROMIUM) return process.env.CHROMIUM;
  const dir = join(homedir(), ".cache", "ms-playwright");
  const builds = (await readdir(dir).catch(() => [])).filter((d) => /^chromium-\d+$/.test(d)).sort();
  return builds.length ? join(dir, builds[builds.length - 1], "chrome-linux64", "chrome") : undefined;
}
const browser = await chromium.launch({ executablePath: await chromiumPath() });
const demo = await (await fetch(`${BASE}/__demo`)).json();

// signIn: the login API from inside the context, so its cookies are the page's.
async function context(opts) {
  const ctx = await browser.newContext({ baseURL: BASE, ...opts });
  const r = await ctx.request.post("/api/auth/login", { data: { email: "owner@example.com", password: demo.password }, headers: { "x-ssg-auth": "1", origin: BASE } });
  if (!r.ok()) throw new Error(`login failed: ${r.status()}`);
  return ctx;
}

const probe = await context({});
const api = async (path) => (await probe.request.get(`/api/auth/admin/${path}`)).json();
const person = (await api("users?q=amara")).items[0].id;
const group = (await api("groups")).items.find((g) => g.name === "Editors").id;
const role = (await api("roles")).items.find((r) => r.name === "support-desk").id;
const service = (await api("service-accounts")).items.find((a) => a.name === "ci-deploy").id;
await probe.close();

const VIEWS = [
  ["people", "#/people"],
  ["person", `#/people/${person}`],
  ["invitations", "#/invitations"],
  ["groups", "#/groups"],
  ["group", `#/groups/${group}`],
  ["roles", "#/roles"],
  ["role", `#/roles/${role}`],
  ["matrix", "#/matrix"],
  ["sessions", "#/sessions"],
  ["services", "#/services"],
  ["service", `#/services/${service}`],
  ["mappings", "#/mappings"],
  ["audit", "#/audit"],
  ["status", "#/status"],
].filter(([name]) => !process.env.ONLY || process.env.ONLY.split(",").includes(name));

const axe = axePath ? await readFile(axePath, "utf8") : null;
const report = { overflow: [], csp: [], errors: [], axe: {} };
for (const [width, height, size] of [[1440, 900, "wide"], [390, 844, "phone"]]) {
  for (const scheme of ["light", "dark"]) {
    // bypassCSP only so axe can be injected; CSP refusals are logged by the
    // browser all the same in a second, strict context below.
    const ctx = await context({ viewport: { width, height }, colorScheme: scheme, bypassCSP: Boolean(axe), deviceScaleFactor: 1 });
    const page = await ctx.newPage();
    page.on("pageerror", (e) => report.errors.push(`${size}/${scheme}: ${e.message}`));
    for (const [name, hash] of VIEWS) {
      await page.goto(`/auth/admin/${hash}`);
      await page.waitForSelector("#view-title");
      await page.waitForFunction(() => !document.querySelector(".loading"));
      await page.waitForTimeout(150);
      const wide = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
      if (wide > 0) report.overflow.push(`${name} ${size} ${scheme}: ${wide}px`);
      await page.screenshot({ path: join(out, `${name}-${size}-${scheme}.png`), fullPage: true });
      if (size === "phone") await page.screenshot({ path: join(out, `${name}-${size}-${scheme}-screen.png`) });
      if (axe && scheme === "light" || axe && size === "wide") {
        await page.addScriptTag({ content: axe });
        const v = await page.evaluate(async () => (await window.axe.run(document, { runOnly: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"] })).violations.map((x) => ({ id: x.id, nodes: x.nodes.map((n) => n.target.join(" ")).slice(0, 4) })));
        if (v.length) report.axe[`${name}-${size}-${scheme}`] = v;
      }
    }
    await ctx.close();
  }
}
// The strict context: every view once, the console's CSP refusals collected.
const strict = await context({ viewport: { width: 1440, height: 900 } });
const sp = await strict.newPage();
sp.on("console", (m) => /Content Security Policy|Refused/.test(m.text()) && report.csp.push(m.text()));
for (const [, hash] of VIEWS) {
  await sp.goto(`/auth/admin/${hash}`);
  await sp.waitForSelector("#view-title");
}
await strict.close();
await browser.close();
await writeFile(join(out, "report.json"), JSON.stringify(report, null, 1));
console.log(JSON.stringify(report, null, 1));
