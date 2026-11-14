// The words (GO-134): every language has every key, the same placeholders,
// real text; the language is picked query → cookie → Accept-Language → en.
import { test } from "node:test";
import assert from "node:assert/strict";
import { LANGS, LANG_COOKIE, STRINGS, asLang, fill, fromAcceptLanguage, langCookie, langOf, when } from "../functions/api/auth/_i18n.ts";
import { errorText, okText } from "../functions/api/auth/_pages-errors.ts";
import { inviteMessage, resetMessage } from "../functions/api/auth/_mail.ts";

const placeholders = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

test("i18n: five languages, each with exactly the English keys, non-empty, with the same placeholders", () => {
  assert.deepEqual([...LANGS], ["en", "pl", "hi", "de", "fr"]);
  const keys = Object.keys(STRINGS.en).sort();
  for (const lang of LANGS) {
    assert.deepEqual(Object.keys(STRINGS[lang]).sort(), keys, lang);
    for (const k of keys) {
      const v = STRINGS[lang][k as keyof typeof STRINGS.en];
      assert.ok(typeof v === "string" && v.trim() !== "", `${lang}.${k}`);
      assert.deepEqual(placeholders(v), placeholders(STRINGS.en[k as keyof typeof STRINGS.en]), `${lang}.${k} placeholders`);
      if (lang !== "en" && v.length > 12 && !/\{|example\.com/.test(v)) assert.notEqual(v, STRINGS.en[k as keyof typeof STRINGS.en], `${lang}.${k} is translated`);
    }
  }
});

test("i18n: query, then the switcher's cookie, then Accept-Language, then English", () => {
  const r = (url: string, headers: Record<string, string> = {}) => new Request("https://example.com" + url, { headers });
  assert.equal(langOf(r("/auth/login?lang=pl", { cookie: `${LANG_COOKIE}=de`, "accept-language": "fr" })), "pl");
  assert.equal(langOf(r("/auth/login?lang=xx", { cookie: `${LANG_COOKIE}=de`, "accept-language": "fr" })), "de");
  assert.equal(langOf(r("/auth/login", { cookie: `${LANG_COOKIE}=zz`, "accept-language": "es, hi-IN;q=0.8, fr;q=0.9" })), "fr");
  assert.equal(langOf(r("/auth/login", { "accept-language": "es, de;q=0" })), "en");
  assert.equal(langOf(r("/auth/login")), "en");
  assert.equal(fromAcceptLanguage("pl;q=0.5, de;q=0.5"), "pl", "the earlier wins a tie");
  assert.equal(fromAcceptLanguage("de;q=abc"), "de", "an unreadable weight counts as 1");
  assert.equal(asLang("PL_pl"), "pl");
  assert.equal(asLang(undefined), null);
  assert.equal(langCookie("hi"), "ssg-auth-lang=hi; Max-Age=31536000; Path=/auth; Secure; SameSite=Lax");
});

test("i18n: fill keeps an unknown placeholder visible; dates are the language's own, in UTC", () => {
  assert.equal(fill("{a} and {b}", { a: 1 }), "1 and {b}");
  assert.equal(when("en", "2026-11-13T10:05:00.000Z"), "Nov 13, 2026, 10:05 AM UTC");
  assert.match(when("pl", "2026-11-13T10:05:00.000Z"), /13 lis 2026.*10:05 UTC/);
  assert.match(when("de", "2026-11-13T10:05:00.000Z"), /13\.11\.2026.*10:05 UTC/);
});

test("i18n: every error code the API sends has its sentence; an unknown code shows the generic one, never itself", () => {
  const t = STRINGS.en;
  const rules = { min: 12, max: 256, minutes: 60 };
  for (const code of ["invalid_credentials", "too_many_attempts", "turnstile_required", "invalid_request", "permissions_too_large", "session_ended", "not_authenticated", "password_too_short", "password_too_long", "password_equals_email", "password_required", "invitation_invalid", "reset_invalid", "account_exists", "unsupported_method", "too_many_requests", "invalid_email", "session_not_found"]) {
    const s = errorText(t, code, rules);
    assert.ok(s && s !== t.errGeneric && !/\{\w+\}/.test(s), code);
  }
  assert.equal(errorText(t, "password_too_short", rules), "The new password is too short: it needs at least 12 characters.");
  assert.equal(errorText(t, "<script>", rules), t.errGeneric);
  assert.equal(errorText(t, "constructor", rules), t.errGeneric);
  assert.equal(errorText(t, null, rules), "");
  assert.equal(okText(t, "password_changed"), t.okPasswordChanged);
  assert.equal(okText(t, "toString"), "");
  assert.equal(okText(t, null), "");
});

test("mail texts: every language fills every placeholder", () => {
  for (const lang of LANGS) {
    const inv = inviteMessage({ to: "a@example.com", lang, site: "example.com", inviter: "Ada", url: "https://example.com/auth/invite?token=T", expires: "2026-11-16T10:00:00.000Z", hours: 72 });
    const res = resetMessage({ to: "a@example.com", lang, site: "example.com", url: "https://example.com/auth/reset?token=T", minutes: 60 });
    for (const m of [inv, res]) {
      assert.ok(!/\{\w+\}/.test(m.subject + m.text), `${lang}: ${m.subject}`);
      assert.match(m.text, /https:\/\/example\.com\/auth\/(invite|reset)\?token=T/);
      assert.equal(m.to, "a@example.com");
    }
    assert.match(inv.text, /2026-11-16 10:00/);
    assert.match(inv.text, /72/);
  }
});
