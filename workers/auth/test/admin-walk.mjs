// The admin panel with the keyboard alone (GO-138): sign in (password and
// the authenticator code), invite someone and get the one-time link, grant
// an existing person a role, end another person's session after the
// step-up window has passed — the step-up dialog, the step-up page, the way
// back with nothing done, and the action repeated. Against a dev server
// started with MFA=1 STEP_UP_MINUTES=1 (the walk waits a minute for it).
//
//   MFA=1 STEP_UP_MINUTES=1 node --import ./workers/auth/test/resolve.mjs workers/auth/test/dev-server.mjs &
//   node workers/auth/test/admin-walk.mjs <out-dir>
//
// Only Tab, Shift+Tab, Enter, Space, arrow keys and typing move it; every
// step is printed, with a screenshot.
import { mkdir, readdir } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";

const out = process.argv[2] ?? "admin-walk";
const BASE = process.env.AUTH_URL ?? "http://localhost:8788";
const { chromium } = await import(process.env.PLAYWRIGHT ?? "/usr/lib/node_modules/playwright/index.mjs");
await mkdir(out, { recursive: true });
const dir = join(homedir(), ".cache", "ms-playwright");
const builds = (await readdir(dir).catch(() => [])).filter((d) => /^chromium-\d+$/.test(d)).sort();
const browser = await chromium.launch({ executablePath: process.env.CHROMIUM ?? (builds.length ? join(dir, builds.at(-1), "chrome-linux64", "chrome") : undefined) });
const page = await (await browser.newContext({ baseURL: BASE, viewport: { width: 1280, height: 900 }, permissions: ["clipboard-read", "clipboard-write"] })).newPage();
const demo = await (await fetch(`${BASE}/__demo`)).json();
const totp = async () => (await fetch(`${BASE}/__totp?secret=${demo.ownerTotp}`)).text();
let n = 0;
const step = async (what) => {
  console.log(`${++n}. ${what}`);
  await page.screenshot({ path: join(out, `${String(n).padStart(2, "0")}.png`) });
};

// tabTo presses Tab until the focused element matches (a CSS selector, or a
// test on its text); fails after 120 presses.
async function tabTo(selector, text = null) {
  for (let i = 0; i < 120; i++) {
    await page.keyboard.press("Tab");
    const hit = await page.evaluate(([s, t]) => {
      const el = document.activeElement;
      return Boolean(el && el.matches(s) && (!t || el.textContent.trim() === t || el.getAttribute("aria-label") === t));
    }, [selector, text]);
    if (hit) return;
  }
  throw new Error(`could not reach ${selector} ${text ?? ""} with Tab`);
}
const enter = () => page.keyboard.press("Enter");

await page.goto("/auth/login?next=/auth/admin/");
await tabTo("#login-email, input[type=email]");
await page.keyboard.type("owner@example.com");
await tabTo("input[type=password]");
await page.keyboard.type(demo.password);
await enter();
await page.waitForURL(/\/auth\/mfa/);
await tabTo("input[name=code]");
await page.keyboard.type(await totp());
await enter();
await page.waitForURL(/\/auth\/admin\//);
await page.waitForSelector("#view-title");
await step("signed in with the password and the app code; the panel opens on People");

// Invite someone, with a role, by the one-time link.
await tabTo("#nav-list a", "Invitations");
await enter();
await page.waitForSelector("#inv-email");
await tabTo("#inv-email");
await page.keyboard.type("keyboard.person@example.com");
await tabTo("input[name=roleIds]");
for (;;) {
  const label = await page.evaluate(() => document.activeElement.parentElement.textContent.trim());
  if (label === "support-desk") break;
  await page.keyboard.press("Tab");
}
await page.keyboard.press("Space");
await tabTo("input[name=delivery][value=mail]");
await page.keyboard.press("ArrowDown"); // the radio group: "Give me the link"
await tabTo("button[type=submit]", "Send the invitation");
await enter();
await page.waitForSelector("#once-link");
console.log("   one-time link focused:", await page.evaluate(() => document.activeElement.id === "once-link"));
await step("invited keyboard.person@example.com with support-desk; the link is shown once with Copy");

// Grant a role to an existing person.
await tabTo("#nav-list a", "People");
await enter();
await page.waitForSelector("#f-q");
await tabTo("#f-q");
await page.keyboard.type("bruno");
await enter();
await page.waitForFunction(() => document.querySelectorAll("tbody tr").length === 1);
await tabTo("tbody a", "Bruno Silva");
await enter();
await page.waitForSelector("#grant-role");
await tabTo("#grant-role");
await page.keyboard.press("ArrowDown"); // the first role on offer
const role = await page.evaluate(() => document.querySelector("#grant-role").selectedOptions[0].textContent);
await tabTo("button", "Grant");
await enter();
await page.waitForFunction((r) => [...document.querySelectorAll(".rows a")].some((a) => a.textContent === r), role);
await step(`granted ${role} to Bruno Silva`);

// End his session after the step-up window: the panel asks for the factor.
console.log("   waiting 65 s for the step-up window (STEP_UP_MINUTES=1) to pass…");
await page.waitForTimeout(65_000);
await page.reload();
await page.waitForSelector("#view-title");
await tabTo("button", "End");
await enter();
await page.waitForSelector("dialog[open]");
await step("confirmation names the person and the consequence; focus on Cancel");
await tabTo("dialog button[type=submit]");
await enter();
await page.waitForSelector("dialog[open] #dlg-title");
await step(`refused: "${await page.textContent("#dlg-title")}" — the step-up dialog`);
await tabTo("dialog button[type=submit]");
await enter();
await page.waitForURL(/\/auth\/mfa\/step-up/);
await tabTo("input[name=code]");
await page.keyboard.type(await totp());
await enter();
await page.waitForURL(/\/auth\/admin\/#\/people\//);
await page.waitForSelector("#view-title");
await step(`back on the same person; notice: "${await page.textContent("#notice")}"; sessions still ${await page.locator("#sec-sessions ~ * tbody tr, section[aria-labelledby=sec-sessions] tbody tr").count()}`);
await tabTo("button", "End");
await enter();
await tabTo("dialog button[type=submit]");
await enter();
await page.waitForFunction(() => /Session ended/.test(document.querySelector("#notice").textContent));
await step(`repeated by hand: "${await page.textContent("#notice p")}"`);
await browser.close();
