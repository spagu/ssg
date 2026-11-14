// Screenshots of the second factor's pages (GO-140), light and dark, 1280 and
// 390 wide, with an automated accessibility pass and — where the browser has
// the Shape Detection API — the QR code on the screen decoded back to the
// otpauth URI. Against the dev server with the second factor on. A developer
// tool, not part of the test run; it needs Playwright on the machine:
//
//   MFA=1 node --import ./workers/auth/test/resolve.mjs workers/auth/test/dev-server.mjs &
//   node workers/auth/test/mfa-shots.mjs <out-dir> [axe.min.js]
//
// The journey: owner@example.com signs in and is led to enrolment; the QR
// page; the first code (read from the dev server's /__totp stand-in for the
// phone); the backup codes; the account page; then a new sign-in meets the
// challenge page (with the backup-code fold open in a second shot).
import { mkdir, readFile, readdir } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";

const out = process.argv[2] ?? "mfa-shots";
const axe = process.argv[3] ? await readFile(process.argv[3], "utf8") : null;
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
const PASSWORD = (await (await fetch(`${BASE}/__demo`)).json()).password;
const report = {};

async function signIn(page, email = "owner@example.com") {
  await page.goto(`${BASE}/auth/login?lang=en`);
  await page.fill("#login-email", email);
  await page.fill("#login-password", PASSWORD);
  await Promise.all([page.waitForNavigation(), page.click("form[action='/api/auth/login'] button[type=submit]")]);
}

async function shoot(page, name) {
  await page.waitForTimeout(150);
  await page.screenshot({ path: join(out, `${name}.png`) });
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth);
  if (overflow) console.log(`${name}: horizontal scroll!`);
  if (axe) {
    await page.evaluate(axe);
    report[name] = await page.evaluate(async () =>
      (await window.axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa", "best-practice"] } })).violations.map((v) => ({ id: v.id, impact: v.impact, nodes: v.nodes.length })),
    );
  }
}

const SIZES = [["1280", { width: 1280, height: 800 }], ["390", { width: 390, height: 844 }]];
const THEMES = ["light", "dark"];
const combos = SIZES.flatMap(([w, viewport]) => THEMES.map((theme) => ({ tag: `${w}-${theme}`, viewport, theme })));

// One owner session, shared by every combination through its storage state.
const first = await browser.newContext({ viewport: { width: 1280, height: 800 } });
const p0 = await first.newPage();
await signIn(p0);
const enrolUrl = p0.url();
await Promise.all([p0.waitForNavigation(), p0.click("form[action='/api/auth/mfa/totp/start'] button")]);
const qrUrl = p0.url();
const state = await first.storageState();

for (const c of combos) {
  const ctx = await browser.newContext({ viewport: c.viewport, colorScheme: c.theme, storageState: state });
  const page = await ctx.newPage();
  await page.goto(enrolUrl);
  await shoot(page, `enrol-${c.tag}`);
  await page.goto(qrUrl);
  await shoot(page, `enrol-qr-${c.tag}`);
  if (c.tag === "1280-light") {
    const decoded = await page.evaluate(async () => {
      if (!("BarcodeDetector" in window)) return "no BarcodeDetector in this browser";
      const svg = document.querySelector("svg.qr");
      const img = new Image();
      img.src = "data:image/svg+xml;base64," + btoa(new XMLSerializer().serializeToString(svg));
      await img.decode();
      const canvas = Object.assign(document.createElement("canvas"), { width: 400, height: 400 });
      canvas.getContext("2d").drawImage(img, 0, 0, 400, 400);
      try {
        const found = await new window.BarcodeDetector({ formats: ["qr_code"] }).detect(canvas);
        return found.map((f) => f.rawValue).join(" | ") || "nothing found";
      } catch (e) {
        return "BarcodeDetector failed: " + e;
      }
    });
    console.log("QR on screen:", decoded);
  }
  await ctx.close();
}

// Confirm with the code the "phone" shows; the backup-codes page.
const secret = (await p0.textContent("code.secret")).replace(/\s/g, "");
await p0.fill("#mfa-code", await (await fetch(`${BASE}/__totp?secret=${secret}`)).text());
await p0.fill("#mfa-name", "Phone");
await Promise.all([p0.waitForNavigation(), p0.click("form[action='/api/auth/mfa/totp/confirm'] button[type=submit]")]);
await shoot(p0, "codes-1280-light");
const after = await first.storageState();

for (const c of combos) {
  const ctx = await browser.newContext({ viewport: c.viewport, colorScheme: c.theme, storageState: after });
  const page = await ctx.newPage();
  await page.goto(`${BASE}/auth/account?lang=en`);
  await page.locator("#mfa-title").scrollIntoViewIfNeeded();
  await shoot(page, `account-factors-${c.tag}`);
  // A fresh sign-in meets the challenge.
  const fresh = await browser.newContext({ viewport: c.viewport, colorScheme: c.theme });
  const p = await fresh.newPage();
  await signIn(p);
  await shoot(p, `challenge-${c.tag}`);
  await p.click("details.fold summary");
  await shoot(p, `challenge-backup-${c.tag}`);
  await fresh.close();
  await ctx.close();
}
if (axe) console.log(JSON.stringify(report, null, 1));
await browser.close();
