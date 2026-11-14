// GO-151, the pages: provider buttons only for enabled providers, as plain
// forms, in five languages; the CSP widened only by the origins a page needs;
// Turnstile's script and frame only when it is configured; the account
// page's "How you sign in"; every new code has its sentence.
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as loginPage } from "../functions/auth/login.ts";
import { onRequestGet as invitePage } from "../functions/auth/invite.ts";
import { onRequestGet as forgotPage } from "../functions/auth/forgot.ts";
import { onRequestGet as account } from "../functions/auth/account.ts";
import { LANGS, STRINGS } from "../functions/api/auth/_i18n.ts";
import { PAGE_CSP } from "../functions/api/auth/_pages.ts";
import { TURNSTILE_ORIGIN, pageCsp } from "../functions/api/auth/_pages-csp.ts";
import { errorText, okText } from "../functions/api/auth/_pages-errors.ts";
import { jwksCache } from "../functions/api/auth/_provider-oidc.ts";
import { useClock } from "./fakes.ts";
import { invite, pageOf, tokenOf } from "./go134.ts";
import { loggedIn, signedIn } from "./seed.ts";
import { fakeProviders, providerEnv, providerUser, roundTrip } from "./go151.ts";
import { Browser } from "./fakes.ts";
import type { Env } from "../functions/api/auth/_lib.ts";

const ORIGINS = "https://accounts.google.com https://login.microsoftonline.com https://appleid.apple.com https://github.com https://www.facebook.com";
const strict = (html: string, name: string) => {
  assert.ok(!/<style/i.test(html) && !/\sstyle=/i.test(html), `${name}: inline style`);
  assert.ok(!/<script(?![^>]*\ssrc=)[^>]*>/i.test(html), `${name}: inline script`);
  assert.ok(!/\son[a-z]+=/i.test(html), `${name}: event handler attribute`);
  assert.ok(!/\{\w+\}/.test(html), `${name}: a placeholder left`);
};

test("login page: one plain form per enabled provider, in each language, with the providers' own wording; the CSP adds exactly their origins", async () => {
  useClock();
  const env = providerEnv();
  for (const lang of LANGS) {
    const t = STRINGS[lang];
    const { html, res } = await pageOf(loginPage, env, `/auth/login?lang=${lang}&next=/auth/account`);
    strict(html, `login ${lang}`);
    const buttons = [...html.matchAll(/<form method="post" action="\/api\/auth\/oauth\/(\w+)\/start"><input type="hidden" name="purpose" value="signin"><input type="hidden" name="next" value="\/auth\/account"><button class="btn btn-provider" type="submit" data-provider="\1">([^<]+)<\/button><\/form>/g)];
    assert.deepEqual(buttons.map((b) => b[1]), ["google", "microsoft", "apple", "github", "facebook"], lang);
    assert.equal(buttons[0][2], t.providerSignIn.replace("{provider}", "Google"));
    assert.equal(buttons[4][2], t.providerContinue.replace("{provider}", "Facebook").replaceAll("'", "&#39;"));
    assert.ok(html.includes(`<h2 id="providers-title" class="or">${t.providersHeading.replaceAll("'", "&#39;")}</h2>`));
    assert.equal(res.headers.get("content-security-policy"), PAGE_CSP.replace("form-action 'self'", `form-action 'self' ${ORIGINS}`));
  }
  const only = providerEnv({ AUTH_MICROSOFT_CLIENT_ID: "", AUTH_APPLE_CLIENT_ID: "", AUTH_GITHUB_CLIENT_ID: "", AUTH_FACEBOOK_CLIENT_ID: "" } as Partial<Env>);
  const one = await pageOf(loginPage, only, "/auth/login");
  assert.equal(one.res.headers.get("content-security-policy"), PAGE_CSP.replace("form-action 'self'", "form-action 'self' https://accounts.google.com"));
  assert.equal([...one.html.matchAll(/btn-provider/g)].length, 1);
});

test("Turnstile: with a site key and a secret the login page loads the widget, and its CSP adds only script-src and frame-src; otherwise nothing changes", async () => {
  useClock();
  const plain = await pageOf(loginPage, providerEnv({ AUTH_GOOGLE_CLIENT_ID: "", AUTH_MICROSOFT_CLIENT_ID: "", AUTH_APPLE_CLIENT_ID: "", AUTH_GITHUB_CLIENT_ID: "", AUTH_FACEBOOK_CLIENT_ID: "" } as Partial<Env>), "/auth/login");
  assert.equal(plain.res.headers.get("content-security-policy"), PAGE_CSP);
  assert.ok(!plain.html.includes("challenges.cloudflare.com") && !plain.html.includes("cf-turnstile"));
  for (const over of [{ AUTH_TURNSTILE_SITE_KEY: "0x4AAAA-site" }, { AUTH_TURNSTILE_SECRET: "0x4AAAA-secret" }]) {
    const half = await pageOf(loginPage, providerEnv({ ...over, AUTH_GOOGLE_CLIENT_ID: "" } as unknown as Partial<Env>), "/auth/login");
    assert.ok(!half.html.includes("cf-turnstile"), "a key without a secret (or the reverse) shows nothing");
    assert.ok(!half.res.headers.get("content-security-policy")?.includes(TURNSTILE_ORIGIN));
  }
  for (const secret of [{ AUTH_TURNSTILE_SECRET: "0x4AAAA-secret" }, { TURNSTILE_SECRET: "0x4AAAA-shared" }]) {
    const env = providerEnv({ AUTH_TURNSTILE_SITE_KEY: "0x4AAAA-site", ...secret } as Partial<Env>);
    const { html, res } = await pageOf(loginPage, env, "/auth/login?lang=pl&error=turnstile_required");
    const csp = res.headers.get("content-security-policy") as string;
    assert.equal(csp, `${PAGE_CSP.replace("script-src 'self'", `script-src 'self' ${TURNSTILE_ORIGIN}`).replace("form-action 'self'", `form-action 'self' ${ORIGINS}`)}; frame-src ${TURNSTILE_ORIGIN}`);
    assert.ok(html.includes(`<script src="${TURNSTILE_ORIGIN}/turnstile/v0/api.js" async defer></script>`));
    assert.ok(html.includes('<div class="cf-turnstile" data-sitekey="0x4AAAA-site" data-language="pl" data-theme="auto"></div>'));
    assert.ok(html.includes(STRINGS.pl.errTurnstileShown), "the message says where the check is");
    assert.ok(!html.includes("0x4AAAA-secret") && !html.includes("0x4AAAA-shared"), "never the secret");
    assert.equal((await pageOf(forgotPage, env, "/auth/forgot")).res.headers.get("content-security-policy")?.includes(TURNSTILE_ORIGIN), false, "login page only");
  }
  assert.equal(pageCsp(), PAGE_CSP);
  assert.equal(pageCsp({ formAction: ["https://a.example.com", "https://a.example.com"] }), PAGE_CSP.replace("form-action 'self'", "form-action 'self' https://a.example.com"));
  // Without the password there is no password form, so no widget either.
  const off = await pageOf(loginPage, providerEnv({ AUTH_PASSWORD_LOGIN: "off", AUTH_TURNSTILE_SITE_KEY: "k", AUTH_TURNSTILE_SECRET: "s" } as Partial<Env>), "/auth/login");
  assert.ok(!off.html.includes("cf-turnstile") && !off.res.headers.get("content-security-policy")?.includes(TURNSTILE_ORIGIN));
});

test("login page: ?reauth=1 is shown to a signed-in person (not skipped), with the reason", async () => {
  useClock();
  const env = providerEnv();
  const { browser } = await loggedIn(env);
  assert.equal((await pageOf(loginPage, env, "/auth/login", browser)).status, 303);
  const { status, html } = await pageOf(loginPage, env, "/auth/login?reauth=1&next=%2Fauth%2Faccount", browser);
  assert.equal(status, 200);
  assert.ok(html.includes(STRINGS.en.reauthText));
});

test("invitation page: the providers next to the password, each form carrying the token; the CSP lets the forms go", async () => {
  useClock();
  const env = providerEnv();
  const boss = await signedIn(env, "boss@example.com", "owner");
  const token = tokenOf((await invite(env, boss.browser, { email: "n@example.com", delivery: "link" })).body.link);
  for (const lang of LANGS) {
    const { html, res } = await pageOf(invitePage, env, `/auth/invite?token=${token}&lang=${lang}`);
    strict(html, `invite ${lang}`);
    assert.equal([...html.matchAll(new RegExp(`<input type="hidden" name="purpose" value="invite"><input type="hidden" name="token" value="${token}">`, "g"))].length, 5);
    assert.ok(html.includes('action="/api/auth/invite"'), "the password stays a choice");
    assert.ok(html.includes(STRINGS[lang].methodProvidersLead.replaceAll("'", "&#39;")));
    assert.match(res.headers.get("content-security-policy") as string, /form-action 'self' https:\/\/accounts\.google\.com/);
  }
});

test("account page: how you sign in — rows, remove buttons, the only method, adding a provider, setting a first password", async () => {
  useClock();
  jwksCache.clear();
  const env = providerEnv();
  const fake = fakeProviders();
  try {
    const { browser } = await loggedIn(env);
    let { html } = await pageOf(account, env, "/auth/account", browser);
    strict(html, "account");
    assert.match(html, /<tr class="method"><td data-label="Method">Password <span class="chip chip-current">Used for this session<\/span><\/td><td data-label="Details">Set<\/td><td data-label="Connected">—<\/td><td><span class="muted">Your only way in<\/span><\/td><\/tr>/);
    assert.equal([...html.matchAll(/name="purpose" value="link"/g)].length, 5);
    await roundTrip(env, fake, browser, "github", { sub: "77", email: "ada@github.example.com" }, { purpose: "link" });
    ({ html } = await pageOf(account, env, "/auth/account", browser));
    assert.match(html, /aria-label="Remove GitHub">Remove<\/button>/);
    assert.match(html, /aria-label="Remove Password">Remove<\/button>/);
    assert.ok(html.includes("ada@github.example.com"));
    assert.equal([...html.matchAll(/name="purpose" value="link"/g)].length, 4);
    assert.ok(html.includes('action="/api/auth/password"'), "a password account can change it");
    // A provider-only account: no password change, a "Set a password" form.
    await providerUser(env, "pat@example.com", "google", "g-pat");
    const pat = new Browser();
    await roundTrip(env, fake, pat, "google", { sub: "g-pat" });
    const p = (await pageOf(account, env, "/auth/account?lang=de", pat)).html;
    strict(p, "account provider-only");
    assert.ok(!p.includes('action="/api/auth/password"') && p.includes('action="/api/auth/methods/set-password"'));
    assert.ok(p.includes(`<span class="chip chip-current">${STRINGS.de.thisSessionMethod}</span>`));
    const off = (await pageOf(account, providerEnv({ AUTH_PASSWORD_LOGIN: "off" } as Partial<Env>), "/auth/account", pat)).html;
    assert.ok(!off.includes("set-password"), "no password to set when the password login is off");
  } finally {
    fake.restore();
  }
});

test("every GO-151 code has a real sentence in every language", () => {
  const codes = ["oauth_cancelled", "oauth_state_invalid", "not_invited", "email_unverified", "provider_failed", "domain_not_allowed", "identity_in_use", "provider_already_linked", "reauth_required", "method_last", "method_not_found", "method_disabled", "password_exists", "account_blocked"];
  for (const lang of LANGS) {
    const t = STRINGS[lang];
    for (const c of codes) assert.ok(errorText(t, c, { min: 12, max: 256, minutes: 60 }) !== t.errGeneric, `${lang} ${c}`);
    for (const c of ["method_linked", "method_removed", "password_set"]) assert.ok(okText(t, c), `${lang} ${c}`);
  }
});
