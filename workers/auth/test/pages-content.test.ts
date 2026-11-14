// What the pages say (GO-134): the login page's messages and links, the
// account page, "Your panels", the pages one-time links open. The frame
// itself (languages, CSP, meta tags, autocomplete, next) is pages.test.ts.
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
import { LANG_COOKIE, STRINGS } from "../functions/api/auth/_i18n.ts";
import { Browser, PASSWORD, call, makeEnv, useClock } from "./fakes.ts";
import { invite, pageOf, tokenOf, withMail } from "./go134.ts";
import { loggedIn, seedUser, signedIn } from "./seed.ts";
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


test("login page: errors are generic and tied to both fields; ok messages; forgot link only with mail; AUTH_AFTER_LOGIN_URL respected", async () => {
  useClock();
  const plain = makeEnv();
  const bad = (await pageOf(loginPage, plain, "/auth/login?error=invalid_credentials")).html;
  assert.match(bad, /<title>Error: Sign in<\/title>/);
  assert.match(bad, /id="notice" role="alert"/);
  assert.equal([...bad.matchAll(/aria-describedby="notice" aria-invalid="true"/g)].length, 2);
  assert.ok(!bad.includes('href="/auth/forgot"'), "no forgot link without mail");
  const crafted = (await pageOf(loginPage, plain, "/auth/login?error=%3Cimg%20src%3Dx%3E")).html;
  assert.ok(!crafted.includes("<img") && crafted.includes(STRINGS.en.errGeneric));
  assert.match((await pageOf(loginPage, plain, "/auth/login?ok=signed_out")).html, /role="status"><p class="callout-title">Done<\/p><p>You are signed out\.<\/p>/);
  assert.ok((await pageOf(loginPage, withMail(), "/auth/login")).html.includes('<a href="/auth/forgot">Forgot your password?</a>'));
  assert.ok((await pageOf(loginPage, makeEnv({ AUTH_AFTER_LOGIN_URL: "/x" }), "/auth/login")).html.includes('name="next" value=""'));
});

test("login page: already signed in and nothing to say — straight on; sign-in-only pages pick the session up or go to login", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  const on = await pageOf(loginPage, env, "/auth/login?next=/auth/account", browser);
  assert.deepEqual([on.status, on.res.headers.get("location")], [303, "/auth/account"]);
  assert.equal((await pageOf(loginPage, env, "/auth/login", browser)).res.headers.get("location"), "/auth/");
  assert.equal((await pageOf(loginPage, env, "/auth/login?error=session_ended", browser)).status, 200);
  for (const [handler, path] of [[account, "/auth/account?ok=password_changed"], [panels, "/auth/"]] as const) {
    const away = await pageOf(handler, env, path, new Browser());
    assert.equal(away.status, 303);
    assert.equal(away.res.headers.get("location"), `/api/auth/resume?next=${encodeURIComponent(path)}`);
  }
  for (const handler of [loginPage, panels, account, invitePage, resetPage, forgotPage]) {
    const off = await pageOf(handler, makeEnv({ AUTH_SIGNING_KEY: undefined }), "/auth/x");
    assert.equal(off.status, 503, "an unconfigured worker says so");
  }
});

test("account page: data, sessions with this one marked, errors on the right field, the sign-out forms", async () => {
  useClock();
  const env = makeEnv();
  const { browser } = await loggedIn(env);
  await call(login, env, "/api/auth/login", { json: { email: "ada@example.com", password: PASSWORD }, browser: new Browser() });
  const { html } = await pageOf(account, env, "/auth/account", browser);
  assert.equal([...html.matchAll(/<tr><td data-label=/g)].length, 2);
  assert.equal([...html.matchAll(/This device/g)].length, 1);
  assert.match(html, /<dd>ada@example\.com<\/dd>/);
  assert.match(html, /<dd>Ada<\/dd>/);
  env.AUTH_DB.sqlite.exec("UPDATE users SET display_name = ''");
  assert.match((await pageOf(account, env, "/auth/account", browser)).html, /<dd>—<\/dd>/, "no name: a dash");
  assert.match(html, /action="\/api\/auth\/password"/);
  assert.match(html, /action="\/api\/auth\/logout-all"><input type="hidden" name="next" value="\/auth\/login\?ok=signed_out_all">/);
  assert.match(html, /action="\/api\/auth\/logout"><input type="hidden" name="next" value="\/auth\/login\?ok=signed_out">/);
  assert.match(html, /minlength="12" maxlength="256"/);
  const wrong = (await pageOf(account, env, "/auth/account?error=invalid_credentials", browser)).html;
  assert.match(wrong, /id="pw-current-error">The current password is not right\.<\/p><input id="pw-current"[^>]*aria-describedby="pw-current-error" aria-invalid="true"/);
  const short = (await pageOf(account, env, "/auth/account?error=password_too_short", browser)).html;
  assert.match(short, /aria-describedby="pw-new-hint pw-new-error" aria-invalid="true"/);
  const other = (await pageOf(account, env, "/auth/account?error=too_many_attempts", browser)).html;
  assert.ok(other.includes(STRINGS.en.errTooManyAttempts) && !other.includes('aria-invalid="true"'));
  assert.match((await pageOf(account, env, "/auth/account?ok=password_changed", browser)).html, /role="status"/);
});

test("panels page: modules by permission; links only from configured panel paths; owner, wildcard, nothing at all; welcome and skipped", async () => {
  useClock();
  const known = JSON.stringify([
    { id: "newsletter-ui", title: { en: "Newsletter", pl: "Newsletter PL" }, panel: "/newsletter-admin/", permissions: [{ name: "newsletter:read", description: "View." }, { name: "newsletter:send", description: { en: "Send.", pl: "Wysyłaj." } }] },
    { id: "comments", title: "Comments", panel: "https://evil.example.com/", permissions: ["comments:read"] },
    { id: "comments", panel: "/dup" },
    { id: "odd", title: 5, panel: 7, permissions: ["odd:b", "odd:a"] },
    { id: "odd2", title: ["x"], permissions: ["odd2:z"] },
    null,
  ]);
  const env = makeEnv({ AUTH_KNOWN_PERMISSIONS: known });
  const warn = console.warn;
  console.warn = () => {};
  try {
    const reader = await signedIn(env, "r@example.com", ["odd:b", "odd:a", "odd2:z", "newsletter:read", "comments:read", "shop:orders"]);
    const html = (await pageOf(panels, env, "/auth/?welcome=1&skipped=2", reader.browser)).html;
    assert.match(html, /<a class="btn btn-primary" href="\/newsletter-admin\/">Open Newsletter<\/a>/);
    assert.ok(!html.includes("evil.example.com"), "a panel path to another host is no link");
    assert.match(html, /<h2>Comments<\/h2><p class="muted">This site has not told/);
    assert.match(html, /<h2>shop<\/h2>.*<code>shop:orders<\/code>/s);
    assert.match(html, /<code>newsletter:read<\/code> — View\./);
    assert.match(html, /<h2>odd<\/h2><p class="muted">[^<]*<\/p><ul class="perms"><li><code>odd:a<\/code><\/li><li><code>odd:b<\/code>/, "unusable title and panel ignored; sorted");
    assert.match(html, /<h2>odd2<\/h2>/);
    assert.match(html, /Welcome\. Your account is ready/);
    assert.match(html, /did not get them \(2\)/);
    const pl = (await pageOf(panels, env, "/auth/?lang=pl", reader.browser)).html;
    assert.match(pl, /Otwórz: Newsletter PL/);
    const wild = await signedIn(env, "w@example.com", ["newsletter:*", "x:*"]);
    const w = (await pageOf(panels, env, "/auth/", wild.browser)).html;
    assert.match(w, /Everything in this module \(newsletter:\*\)/);
    assert.match(w, /<code>newsletter:send<\/code> — Send\./);
    assert.match(w, /<h2>x<\/h2>.*Everything in this module \(x:\*\)/s);
    const owner = await signedIn(env, "o@example.com", "owner");
    const o = (await pageOf(panels, env, "/auth/", owner.browser)).html;
    assert.match(o, /holds every permission, in every module/);
    assert.match(o, /<code>auth:invite<\/code>/);
    const broken = makeEnv({ AUTH_KNOWN_PERMISSIONS: "{not json" });
    const inviter = await signedIn(broken, "n@example.com", ["auth:invite"]);
    assert.match((await pageOf(panels, broken, "/auth/", inviter.browser)).html, /<h2>Accounts and sign-in<\/h2>.*<code>auth:invite<\/code> — Invite people/s, "unreadable config: own manifest only");
    const plain = makeEnv();
    const nobody = await loggedIn(plain, "nobody@example.com");
    assert.match((await pageOf(panels, plain, "/auth/", nobody.browser)).html, /<p>Your account holds no permissions yet\./);
  } finally {
    console.warn = warn;
  }
});

test("invite, reset and forgot pages: a dead link is one 410 page; errors land on the field; forgot only with mail", async () => {
  useClock();
  const { env, pages } = await fixture();
  const inviteUrl = pages[3][2];
  const resetUrl = pages[4][2];
  for (const [handler, path] of [[invitePage, "/auth/invite?token=x"], [invitePage, "/auth/invite"], [resetPage, "/auth/reset?token=" + "A".repeat(43)]] as const) {
    const r = await pageOf(handler, env, path);
    assert.equal(r.status, 410);
    assert.match(r.html, /<h1>This link does not work any more<\/h1>/);
  }
  assert.ok((await pageOf(resetPage, env, "/auth/reset")).html.includes('href="/auth/forgot"'), "with mail, a dead reset link offers a new one");
  assert.ok(!(await pageOf(resetPage, makeEnv(), "/auth/reset")).html.includes('href="/auth/forgot"'));
  const inv = (await pageOf(invitePage, env, inviteUrl)).html;
  assert.match(inv, /You were invited to example\.com as new@example\.com\./);
  assert.match(inv, /name="name" type="text" autocomplete="name" value="Nina"/);
  assert.match(inv, /<input type="hidden" name="method" value="password">/);
  assert.match((await pageOf(invitePage, env, `${inviteUrl}&error=password_too_short`)).html, /id="new-password-error"/);
  assert.match((await pageOf(invitePage, env, `${inviteUrl}&error=account_exists`)).html, /exists already/);
  assert.match((await pageOf(resetPage, env, `${resetUrl}&error=password_equals_email`)).html, /id="new-password-error"/);
  assert.match((await pageOf(resetPage, env, resetUrl)).html, /For the account ada@example\.com\./);
  const off = await pageOf(forgotPage, makeEnv(), "/auth/forgot");
  assert.deepEqual([off.status, /<h1>Not available<\/h1>/.test(off.html)], [404, true]);
  assert.equal((await pageOf(forgotPage, makeEnv({ AUTH_SMTP_HOST: "smtp.example.com" }), "/auth/forgot")).status, 503);
  assert.match((await pageOf(forgotPage, env, "/auth/forgot?sent=1")).html, /a link is on its way/);
  assert.match((await pageOf(forgotPage, env, "/auth/forgot?error=invalid_email")).html, /id="forgot-email-error"/);
  assert.match((await pageOf(forgotPage, env, "/auth/forgot?error=too_many_requests")).html, /Wait an hour/);
});

test("pages: a stored value is text, never markup", async () => {
  useClock();
  const env = makeEnv();
  const boss = await signedIn(env, "boss@example.com", "owner");
  const inv = await invite(env, boss.browser, { email: "x@example.com", name: '<img src=x onerror="alert(1)">' });
  const html = (await pageOf(invitePage, env, `/auth/invite?token=${tokenOf(inv.body.link)}`, undefined, { cookie: `${LANG_COOKIE}=hi` })).html;
  assert.ok(!html.includes("<img"));
  assert.match(html, /value="&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;"/);
  assert.match(html, /<html lang="hi">/, "the cookie picks the language");
});
