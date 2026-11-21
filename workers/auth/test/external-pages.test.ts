// #435: the external issuer's words on the pages — the link button and its
// confirmation in five languages; the method's own error codes with their
// own sentence on the login and account pages in English and Polish, the
// generic one in the other languages.
import { test } from "node:test";
import assert from "node:assert/strict";
import { EXTERNAL_STRINGS, externalErrorText } from "../functions/api/auth/_external-i18n.ts";
import { LANGS, STRINGS } from "../functions/api/auth/_i18n.ts";
import { onRequestGet as accountGet } from "../functions/auth/account.ts";
import { onRequestGet as loginGet } from "../functions/auth/login.ts";
import { call, useClock } from "./fakes.ts";
import { loggedIn } from "./seed.ts";
import { jwtEnv } from "./external-fake.ts";

const CODES = ["jwt_failed", "jwt_unavailable", "jwt_not_linked", "jwt_email_unusable", "jwt_link_expired"];
const shown = (s: string) => s.replaceAll("'", "&#39;");

const placeholders = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

test("EXTERNAL_STRINGS: every language has the English keys, real text and the same placeholders", () => {
  const keys = Object.keys(EXTERNAL_STRINGS.en).sort() as (keyof (typeof EXTERNAL_STRINGS)["en"])[];
  for (const lang of LANGS) {
    assert.deepEqual(Object.keys(EXTERNAL_STRINGS[lang]).sort(), keys, lang);
    for (const k of keys) {
      const v = EXTERNAL_STRINGS[lang][k];
      const en = EXTERNAL_STRINGS.en[k];
      assert.ok(v.trim(), `${lang}.${k}`);
      assert.deepEqual(placeholders(v), placeholders(en), `${lang}.${k}`);
      if (lang !== "en" && en !== "Connect" && en !== "Cancel") assert.notEqual(v, en, `${lang}.${k} is translated`);
    }
  }
});

test("externalErrorText: a sentence of its own for every code in English and Polish, none elsewhere or for other codes", () => {
  for (const code of CODES) {
    const en = externalErrorText("en", code);
    const pl = externalErrorText("pl", code);
    assert.ok(en.trim() && pl.trim() && en !== pl, code);
    for (const lang of ["hi", "de", "fr"] as const) assert.equal(externalErrorText(lang, code), "", `${lang} ${code}`);
  }
  assert.equal(externalErrorText("en", "constructor"), "");
  assert.equal(externalErrorText("en", "access_failed"), "");
  assert.equal(externalErrorText("en", null), "");
});

test("login and account pages: the method's sentence in English and Polish, the generic sentence in the other languages", async () => {
  useClock();
  const env = jwtEnv();
  const { browser } = await loggedIn(env);
  for (const lang of LANGS) {
    for (const code of CODES) {
      const login = await (await call(loginGet, env, `/auth/login?lang=${lang}&error=${code}`, { csrf: false })).text();
      const account = await (await call(accountGet, env, `/auth/account?lang=${lang}&error=${code}`, { browser, csrf: false })).text();
      const want = shown(externalErrorText(lang, code) || STRINGS[lang].errGeneric);
      assert.ok(login.includes(want), `login ${lang} ${code}`);
      assert.ok(account.includes(want), `account ${lang} ${code}`);
    }
  }
});
