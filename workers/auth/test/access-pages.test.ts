// GO-139: the Access parts of the pages in five languages, the sign-out that
// also ends the Access session (to the configured team only), the pages' CSP,
// and no token in any log line or audit row.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ACCESS_STRINGS, accessErrorText } from "../functions/api/auth/_access-i18n.ts";
import { LANGS } from "../functions/api/auth/_i18n.ts";
import { PAGE_CSP } from "../functions/api/auth/_pages.ts";
import { onRequestPost as logout } from "../functions/api/auth/logout.ts";
import { onRequestGet as loginGet } from "../functions/auth/login.ts";
import { onRequestGet as accountGet } from "../functions/auth/account.ts";
import { onRequestGet as panelsGet } from "../functions/auth/index.ts";
import { Browser, call, makeEnv, useClock } from "./fakes.ts";
import { loggedIn } from "./seed.ts";
import { providerUser } from "./go151.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { FakeAccess, OTHER_AUD, PERSON, accessEnv, enter, mint } from "./access-fake.ts";

const placeholders = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
const html = async (env: Env, path: string, browser?: Browser) => (await call(path.startsWith("/auth/account") ? accountGet : path.startsWith("/auth/login") ? loginGet : panelsGet, env, path, { browser, csrf: false })).text();

test("ACCESS_STRINGS: every language has the English keys, real text and the same placeholders", () => {
  const keys = Object.keys(ACCESS_STRINGS.en).sort();
  for (const lang of LANGS) {
    assert.deepEqual(Object.keys(ACCESS_STRINGS[lang]).sort(), keys, lang);
    for (const k of keys) {
      const v = ACCESS_STRINGS[lang][k as keyof (typeof ACCESS_STRINGS)["en"]];
      const en = ACCESS_STRINGS.en[k as keyof (typeof ACCESS_STRINGS)["en"]];
      assert.ok(v.trim(), `${lang}.${k}`);
      assert.deepEqual(placeholders(v), placeholders(en), `${lang}.${k}`);
      if (lang !== "en" && en !== "Connect" && en !== "Cancel") assert.notEqual(v, en, `${lang}.${k} is translated`);
    }
  }
  assert.equal(accessErrorText("en", "constructor"), "");
  assert.equal(accessErrorText("en", null), "");
});

test("login page: the Access entry only when configured, in five languages, carrying next; alone when it is the only method", async () => {
  useClock();
  assert.doesNotMatch(await html(makeEnv(), "/auth/login"), /cloudflare/i);
  const env = accessEnv();
  for (const lang of LANGS) {
    const page = await html(env, `/auth/login?lang=${lang}&next=%2Fnewsletter-admin%2F`);
    assert.ok(page.includes(`<a class="btn btn-provider" href="/auth/access?next=%2Fnewsletter-admin%2F" data-provider="access">${ACCESS_STRINGS[lang].signIn}</a>`), lang);
    for (const code of ["access_failed", "access_unavailable", "access_not_linked", "access_email_unusable"]) {
      const shown = await html(env, `/auth/login?lang=${lang}&error=${code}`);
      assert.ok(shown.includes(accessErrorText(lang, code).replaceAll("'", "&#39;")), `${lang} ${code}`);
    }
  }
  const only = await html(accessEnv({ AUTH_PASSWORD_LOGIN: "off" }), "/auth/login");
  assert.doesNotMatch(only, /name="password"/);
  assert.match(only, /href="\/auth\/access\?next=%2Fauth%2F" data-provider="access"/);
  const account = await loggedIn(env);
  assert.ok((await html(env, "/auth/account?error=access_failed", account.browser)).includes(ACCESS_STRINGS.en.errFailed.replaceAll("'", "&#39;")));
});

test("sign-out of an Access session: cookies cleared here, then the team's Access logout — form redirect, JSON field; others unchanged", async () => {
  useClock();
  const env = accessEnv();
  const fake = new FakeAccess().install();
  try {
    await providerUser(env, "ada@example.com", "access", PERSON.sub);
    const LOGOUT = "https://myteam.cloudflareaccess.com/cdn-cgi/access/logout";
    const viaForm = new Browser();
    await enter(env, await mint(), { browser: viaForm });
    const form = await call(logout, env, "/api/auth/logout", { form: { next: "https://evil.example.com/" }, browser: viaForm });
    assert.deepEqual([form.status, form.headers.get("location"), viaForm.jar.size], [303, LOGOUT, 0]);
    const viaJson = new Browser();
    await enter(env, await mint(), { browser: viaJson });
    assert.deepEqual(await (await call(logout, env, "/api/auth/logout", { method: "POST", browser: viaJson })).json(), { ok: true, accessLogoutUrl: LOGOUT });
    // A password session: as before.
    const pw = await loggedIn(env, "pw@example.com");
    assert.deepEqual(await (await call(logout, env, "/api/auth/logout", { method: "POST", browser: pw.browser })).json(), { ok: true });
    // An Access session after Access was switched off: no URL (nothing to build it from).
    const late = new Browser();
    await enter(env, await mint(), { browser: late });
    const off = { ...env, AUTH_ACCESS_TEAM: undefined, AUTH_ACCESS_AUD: undefined } as Env;
    const res = await call(logout, off, "/api/auth/logout", { form: { next: "/bye" }, browser: late });
    assert.equal(res.headers.get("location"), "/bye");
  } finally {
    fake.restore();
  }
});

test("CSP: pages with a sign-out form allow the team host as form-action when Access is on; exactly as before when off", async () => {
  useClock();
  const off = makeEnv();
  const a = await loggedIn(off);
  const panelsOff = await call(panelsGet, off, "/auth/", { browser: a.browser, csrf: false });
  assert.equal(panelsOff.headers.get("content-security-policy"), PAGE_CSP);
  const on = accessEnv();
  const b = await loggedIn(on);
  for (const handler of [panelsGet, accountGet, loginGet]) {
    const res = await call(handler, on, handler === panelsGet ? "/auth/" : handler === accountGet ? "/auth/account" : "/auth/login?ok=signed_out", { browser: b.browser, csrf: false });
    assert.match(res.headers.get("content-security-policy") as string, /form-action 'self' https:\/\/myteam\.cloudflareaccess\.com;/);
  }
});

test("no token, signature or claim value reaches a log line or an audit row", async () => {
  useClock();
  const env = accessEnv();
  const fake = new FakeAccess().install();
  const lines: string[] = [];
  const saved = { log: console.log, warn: console.warn, error: console.error };
  for (const k of ["log", "warn", "error"] as const) console[k] = (...a: unknown[]) => lines.push(a.map(String).join(" "));
  try {
    await providerUser(env, "ada@example.com", "access", PERSON.sub);
    const tokens = [await mint(), await mint(PERSON, { aud: [OTHER_AUD] }), await mint(PERSON, {}, { forge: "bad-signature" }), await mint({ sub: "x", email: "x@example.com" })];
    for (const t of tokens) await enter(env, t);
    fake.mode = "500";
    fake.restore();
    fake.install();
    await enter(env, tokens[0]);
    const audit = JSON.stringify((env.AUTH_DB as unknown as { sqlite: { prepare(s: string): { all(): unknown[] } } }).sqlite.prepare("SELECT * FROM auth_events").all());
    for (const t of tokens) {
      for (const part of t.split(".")) {
        assert.ok(!lines.some((l) => l.includes(part)), "log");
        assert.ok(!audit.includes(part), "audit");
      }
    }
    assert.ok(lines.length > 0 && lines.every((l) => l.startsWith("auth: ")));
  } finally {
    Object.assign(console, saved);
    fake.restore();
  }
});
