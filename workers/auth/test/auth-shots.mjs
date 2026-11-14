// Screenshots of the sign-in pages, light and dark, wide and phone-sized, an
// automated accessibility pass, and two journeys: the whole sign-in → account
// → sign-out path with JavaScript OFF, and signing in with the keyboard only.
// Against the dev server (dev-server.mjs). A developer tool, not part of the
// test run; it needs Playwright on the machine (never a dependency here):
//
//   node --import ./workers/auth/test/resolve.mjs workers/auth/test/dev-server.mjs &
//   node workers/auth/test/auth-shots.mjs <out-dir> [axe.min.js]
//
// Shots are what the screen shows at the real viewport (1280×800, 390×844),
// screen by screen, never one stitched page. PLAYWRIGHT=… points at the
// Playwright module (default: the global npm install), CHROMIUM=… at a browser.
import { mkdir, readFile, readdir } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";

const out = process.argv[2] ?? "auth-shots";
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
const PASSWORD = demo.password;

async function signIn(page, email = "editor@example.com") {
  await page.goto(`${BASE}/auth/login`);
  await page.fill("#login-email", email);
  await page.fill("#login-password", PASSWORD);
  await Promise.all([page.waitForNavigation(), page.click("button[type=submit]:not(.btn-quiet)")]);
}

const VIEWS = [
  ["login", "/auth/login", false],
  ["login-error", "/auth/login?error=invalid_credentials", false],
  ["login-not-invited", "/auth/login?error=not_invited", false], // GO-151
  ["panels", "/auth/", true],
  ["account", "/auth/account", true],
  ["account-error", "/auth/account?error=password_too_short", true],
  ["invite", new URL(demo.invite).pathname + new URL(demo.invite).search, false],
  ["reset", new URL(demo.reset).pathname + new URL(demo.reset).search, false],
  ["forgot", "/auth/forgot", false],
];
const SIZES = [["1280", { width: 1280, height: 800 }], ["390", { width: 390, height: 844 }]];
const only = process.env.ONLY ? process.env.ONLY.split(",") : null;
const axe = axePath ? await readFile(axePath, "utf8") : null;
const report = {};

async function screens(page, file, max) {
  const { height, inner } = await page.evaluate(() => ({ height: document.documentElement.scrollHeight, inner: innerHeight }));
  for (let i = 0, y = 0; i < max; i++, y += inner - 120) {
    await page.evaluate((top) => window.scrollTo(0, top), y);
    await page.waitForTimeout(100);
    await page.screenshot({ path: i ? file.replace(/\.png$/, `-${i + 1}.png`) : file });
    if (y + inner >= height) break;
  }
}

for (const theme of ["light", "dark"]) {
  for (const [size, viewport] of SIZES) {
    const context = await browser.newContext({ viewport, colorScheme: theme, locale: process.env.LOCALE ?? "en-GB", reducedMotion: "reduce" });
    const page = await context.newPage();
    page.on("pageerror", (e) => console.error("pageerror", e.message));
    page.on("console", (m) => m.type() === "error" && console.error("console", m.text()));
    let signedIn = false;
    for (const [name, path, needsSession] of VIEWS) {
      if (only && !only.includes(name)) continue;
      if (needsSession && !signedIn) {
        await signIn(page);
        signedIn = true;
      }
      await page.goto(BASE + path);
      await page.waitForSelector("h1");
      await page.evaluate(() => document.activeElement?.blur());
      const file = join(out, `${name}-${theme}-${size}.png`);
      await screens(page, file, size === "390" ? 3 : 2);
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      if (overflow > 0) console.error(`horizontal overflow ${overflow}px on ${name} ${theme} ${size}`);
      if (axe) {
        await page.evaluate(axe);
        report[`${name}-${theme}-${size}`] = await page.evaluate(async () =>
          (await window.axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa", "best-practice"] } })).violations.map((v) => ({ id: v.id, impact: v.impact, nodes: v.nodes.length, target: v.nodes.slice(0, 3).map((n) => n.target.join(" ")) })),
        );
      }
      console.log(file);
    }
    await context.close();
  }
}

// Journey 1: JavaScript off — sign in, change the password, sign out one
// other session, sign out; every step a plain form post.
const checks = [];
const check = (what, ok) => (checks.push({ what, ok }), console.log(`${ok ? "ok  " : "FAIL"} ${what}`));
{
  const context = await browser.newContext({ javaScriptEnabled: false, viewport: { width: 390, height: 844 } });
  const page = await context.newPage();
  await page.goto(`${BASE}/auth/account`);
  check("no JS: /auth/account without a session lands on the login page with next", page.url().endsWith("/auth/login?next=%2Fauth%2Faccount"));
  await page.fill("#login-email", "owner@example.com");
  await page.fill("#login-password", "wrong password here");
  await page.click("button[type=submit]");
  check("no JS: a wrong password comes back with the generic error", (await page.textContent("#notice")).includes("is not right"));
  await page.fill("#login-email", "owner@example.com");
  await page.fill("#login-password", PASSWORD);
  await page.click("button[type=submit]");
  check("no JS: signed in, back to the account page (next kept through the error)", page.url().endsWith("/auth/login") === false && (await page.textContent("h1")) === "Your account");
  await page.fill("#pw-current", PASSWORD);
  await page.fill("#new-password, #pw-new", "a new demo password");
  await page.click("form[action='/api/auth/password'] button[type=submit]");
  check("no JS: password changed", (await page.textContent("#notice")).includes("Your password is changed"));
  await page.goto(`${BASE}/auth/`);
  check("no JS: Your panels lists the newsletter link", (await page.locator("a[href='/newsletter-admin/']").count()) === 1);
  await page.goto(`${BASE}/auth/account`);
  await page.click("form[action='/api/auth/logout'] button");
  check("no JS: signed out", page.url().endsWith("/auth/login?ok=signed_out") && (await page.textContent("#notice")).includes("signed out"));
  await page.goto(`${BASE}/auth/account`);
  check("no JS: after signing out the account page needs a sign-in again", page.url().includes("/auth/login?next="));
  await page.fill("#login-email", "owner@example.com");
  await page.fill("#login-password", "a new demo password");
  await page.click("button[type=submit]");
  await page.fill("#pw-current", "a new demo password");
  await page.fill("#pw-new", PASSWORD);
  await page.click("form[action='/api/auth/password'] button[type=submit]");
  await page.click("form[action='/api/auth/logout-all'] button");
  check("no JS: sign out everywhere", page.url().endsWith("/auth/login?ok=signed_out_all"));
  await context.close();
}

// Journey 2: the keyboard only — Tab to the fields, type, Enter.
{
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await context.newPage();
  await page.goto(`${BASE}/auth/login`);
  await page.keyboard.press("Tab");
  const first = await page.evaluate(() => document.activeElement?.id);
  await page.keyboard.type("editor@example.com");
  await page.keyboard.press("Tab");
  await page.keyboard.type(PASSWORD);
  await Promise.all([page.waitForNavigation(), page.keyboard.press("Enter")]);
  check(`keyboard: first Tab lands on the e-mail field (${first}), Enter signs in`, first === "login-email" && (await page.textContent("h1")) === "Your panels");
  await page.goto(`${BASE}/auth/login?error=invalid_credentials`);
  await page.goto(`${BASE}/auth/account?error=password_too_short`);
  check("JS: the error message takes focus", (await page.evaluate(() => document.activeElement?.id)) === "notice");
  await page.click("#pw-toggle-2");
  check("JS: Show password reveals the new password field", (await page.getAttribute("#pw-new", "type")) === "text" && (await page.getAttribute("#pw-toggle-2", "aria-pressed")) === "true");
  await context.close();
}

// Journey 3 (GO-151): login providers with JavaScript OFF, through the dev
// server's stand-in consent screen — sign in with a linked GitHub account,
// see it on the account page; a stranger at Apple (its form_post answer) is
// refused; the invitation accepted by choosing Google.
{
  const context = await browser.newContext({ javaScriptEnabled: false, viewport: { width: 390, height: 844 } });
  const page = await context.newPage();
  await page.goto(`${BASE}/auth/login`);
  await page.click("button[data-provider=github]");
  await page.click("button:has-text('Eda')");
  check("no JS: GitHub sign-in through the stand-in lands on Your panels", (await page.textContent("h1")) === "Your panels");
  await page.goto(`${BASE}/auth/account`);
  check("no JS: the account page lists GitHub, marked as this session's method", (await page.locator("tr.method:has-text('GitHub') .chip-current").count()) === 1);
  await page.click("form[action='/api/auth/logout'] button");
  await page.click("button[data-provider=apple]");
  await page.click("button:has-text('stranger')");
  await page.click("button:has-text('Continue')");
  check("no JS: a stranger at Apple is refused with the not-invited message", page.url().includes("error=not_invited") && (await page.textContent("#notice")).includes("not invited"));
  await page.goto(demo.invite.replace(/^https?:\/\/[^/]+/, BASE));
  await page.click("button[data-provider=google]");
  await page.click("button:has-text('Noor')");
  check("no JS: the invitation accepted with Google lands on the welcome", page.url().endsWith("/auth/?welcome=1") && (await page.textContent("#notice")).includes("Welcome"));
  await context.close();
}

if (axe) console.log(JSON.stringify(report, null, 1));
console.log(`${checks.filter((c) => c.ok).length}/${checks.length} journey checks passed`);
await browser.close();
