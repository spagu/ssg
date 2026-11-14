// The pages (GO-134): every page in every language with its words filled in;
// HTML a strict CSP allows (nothing inline, nothing from elsewhere); complete
// meta tags; autocomplete on every credential field; `next` never leaves the
// host; sign-in-only pages send the browser to pick the session up.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as panels } from "../functions/auth/index.ts";
import { onRequestGet as loginPage } from "../functions/auth/login.ts";
import { onRequestGet as account } from "../functions/auth/account.ts";
import { onRequestGet as invitePage } from "../functions/auth/invite.ts";
import { onRequestGet as resetPage } from "../functions/auth/reset.ts";
import { onRequestGet as forgotPage } from "../functions/auth/forgot.ts";
import { onRequestPost as login } from "../functions/api/auth/login.ts";
import { onRequestPost as resetLink } from "../functions/api/auth/admin/users/[id]/reset-link.ts";
import { LANGS, LANG_COOKIE, STRINGS } from "../functions/api/auth/_i18n.ts";
import { PAGE_CSP } from "../functions/api/auth/_pages.ts";
import { safeNext } from "../functions/api/auth/_next.ts";
import { Browser, PASSWORD, call, makeEnv, useClock } from "./fakes.ts";
import { invite, pageOf, tokenOf, withMail } from "./go134.ts";
import { seedUser, signedIn } from "./seed.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

// fixture: an owner, an invitation and a reset link, and every page's URL.
async function fixture(env: Env = withMail()) {
  const boss = await signedIn(env, "boss@example.com", "owner");
  const inv = await invite(env, boss.browser, { email: "new@example.com", name: "Nina", delivery: "link" });
  const ada = await seedUser(env, "ada@example.com");
  const link = await (await call(resetLink, env, `/api/auth/admin/users/${ada}/reset-link`, { method: "POST", browser: boss.browser, params: { id: ada }, json: { delivery: "link" } })).json();
  const pages: [string, unknown, string, Browser | undefined, string][] = [
    ["login", loginPage, "/auth/login", undefined, "loginTitle"],
    ["panels", panels, "/auth/", boss.browser, "panelsTitle"],
    ["account", account, "/auth/account", boss.browser, "accountTitle"],
    ["invite", invitePage, `/auth/invite?token=${tokenOf(inv.body.link)}`, undefined, "inviteTitle"],
    ["reset", resetPage, `/auth/reset?token=${tokenOf(link.link)}`, undefined, "resetTitle"],
    ["forgot", forgotPage, "/auth/forgot", undefined, "forgotTitle"],
  ];
  return { env, boss, pages };
}

const sep = (path: string) => (path.includes("?") ? "&" : "?");

test("pages: every page in every language — <html lang>, its own h1, no placeholder left, the language kept in a cookie", async () => {
  useClock();
  const { env, pages } = await fixture();
  for (const [name, handler, path, browser, titleKey] of pages) {
    for (const lang of LANGS) {
      const { status, html, res } = await pageOf(handler, env, `${path}${sep(path)}lang=${lang}`, browser);
      assert.equal(status, 200, `${name} ${lang}`);
      assert.match(html, new RegExp(`<html lang="${lang}">`));
      const h1 = [...html.matchAll(/<h1[^>]*>([^<]*)<\/h1>/g)];
      assert.equal(h1.length, 1, `${name}: one h1`);
      assert.equal(h1[0][1], STRINGS[lang][titleKey as keyof typeof STRINGS.en].replaceAll("'", "&#39;"));
      assert.ok(!/\{\w+\}/.test(html), `${name} ${lang}: a placeholder left`);
      assert.ok(res.headers.getSetCookie().includes(`${LANG_COOKIE}=${lang}; Max-Age=31536000; Path=/auth; Secure; SameSite=Lax`));
    }
  }
});

test("pages: strict-CSP HTML — nothing inline, nothing from elsewhere; complete meta tags; the security headers", async () => {
  useClock();
  const { env, pages } = await fixture();
  for (const [name, handler, path, browser] of pages) {
    const { html, res } = await pageOf(handler, env, path, browser);
    assert.ok(!/<style/i.test(html) && !/\sstyle=/i.test(html), `${name}: inline style`);
    assert.ok(!/<script(?![^>]*\ssrc=)[^>]*>/i.test(html), `${name}: inline script`);
    assert.ok(!/\son[a-z]+=/i.test(html), `${name}: event handler attribute`);
    assert.ok(!/(?:src|href|action)="(?:https?:)?\/\//i.test(html), `${name}: a resource or link to another host`);
    assert.ok(!/googletagmanager|gtag|analytics/i.test(html), `${name}: nothing measured`);
    for (const meta of ['<meta charset="utf-8">', '<meta name="viewport" content="width=device-width, initial-scale=1">', '<meta name="robots" content="noindex, nofollow">', '<meta name="referrer" content="strict-origin">', '<meta name="color-scheme" content="light dark">', '<meta name="description" content="', '<link rel="icon" href="/auth/assets/icon.svg"', '<link rel="stylesheet" href="/auth/assets/tokens.css">', '<script src="/auth/assets/auth.js" defer></script>']) {
      assert.ok(html.includes(meta), `${name}: ${meta}`);
    }
    assert.equal(res.headers.get("content-security-policy"), PAGE_CSP);
    assert.equal(res.headers.get("cache-control"), "no-store");
    assert.equal(res.headers.get("referrer-policy"), "strict-origin", "no-referrer would make form posts carry Origin: null");
    assert.equal(res.headers.get("x-robots-tag"), "noindex, nofollow");
    assert.equal(res.headers.get("x-frame-options"), "DENY");
    assert.match(res.headers.get("content-type") as string, /^text\/html; charset=utf-8/);
    assert.match(html, /<nav class="langs"/, `${name}: the language switcher`);
  }
});

test("pages: credential fields carry the right autocomplete; pasting is never blocked", async () => {
  useClock();
  const { env, pages } = await fixture();
  const expected: Record<string, string[]> = {
    login: ["username", "current-password"],
    account: ["username", "current-password", "new-password"],
    invite: ["username", "name", "new-password"],
    reset: ["username", "new-password"],
    forgot: ["username"],
  };
  for (const [name, handler, path, browser] of pages) {
    const { html } = await pageOf(handler, env, path, browser);
    if (expected[name]) assert.deepEqual([...html.matchAll(/autocomplete="([^"]+)"/g)].map((m) => m[1]).sort(), expected[name].sort(), name);
    assert.ok(!/onpaste|autocomplete="off"/i.test(html), `${name}: paste and password managers allowed`);
    for (const id of [...html.matchAll(/<input [^>]*id="([^"]+)"/g)].map((m) => m[1])) assert.ok(html.includes(`<label for="${id}">`), `${name}: #${id} has a label`);
  }
});

const HOSTILE = [
  "//evil.example.com",
  "\\evil.example.com",
  "/\\evil.example.com",
  "\\/evil.example.com",
  "https://evil.example.com",
  "http:evil.example.com",
  "javascript:alert(1)",
  "%2F%2Fevil.example.com",
  "/.//evil.example.com",
  "/..//evil.example.com",
  "/a/..//evil.example.com",
  "/%2e//evil.example.com",
  "/\t/evil.example.com",
  "/\n/evil.example.com",
  "/\u0000/evil.example.com",
  " //evil.example.com",
  "evil.example.com",
  "",
];

test("next: a table of hostile values — none leaves the host, through safeNext, the login page and the login form", async () => {
  useClock();
  const env = makeEnv();
  await seedUser(env);
  for (const raw of HOSTILE) {
    assert.equal(safeNext(raw, "FALLBACK"), "FALLBACK", JSON.stringify(raw));
    const { html } = await pageOf(loginPage, env, `/auth/login?next=${encodeURIComponent(raw)}`);
    assert.match(html, /<input type="hidden" name="next" value="\/auth\/">/, `login page with next=${JSON.stringify(raw)}`);
    const res = await call(login, env, "/api/auth/login", { form: { email: "ada@example.com", password: PASSWORD, next: raw }, browser: new Browser() });
    assert.equal(res.headers.get("location"), "/", `login form with next=${JSON.stringify(raw)}`);
  }
  // Encoded slashes inside a path stay a path on this host.
  assert.equal(safeNext("/%2F%2Fevil.example.com", "F"), "/%2F%2Fevil.example.com");
  const { html } = await pageOf(loginPage, env, `/auth/login?next=${encodeURIComponent("/newsletter-admin/#/campaigns?x=\"<b>")}`);
  assert.match(html, /name="next" value="\/newsletter-admin\/#\/campaigns\?x=%22%3Cb%3E"/, "kept, escaped");
});
