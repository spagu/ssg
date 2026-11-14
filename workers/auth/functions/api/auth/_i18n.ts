// The words of the sign-in pages and their mail, in the five languages of the
// panels (en, pl, hi, de, fr), and how a request picks one. A leading
// underscore keeps this file out of the Pages route table.
//
// en (_i18n-en.ts) is the reference: every other table has exactly its keys —
// the compiler checks it (Strings), and a test checks it again at runtime.
// Terms follow the newsletter panel's (workers/newsletter-ui/public/js/strings).

import { DE } from "./_i18n-de";
import { EN } from "./_i18n-en";
import { FR } from "./_i18n-fr";
import { HI } from "./_i18n-hi";
import { PL } from "./_i18n-pl";
import { readCookie } from "./_cookies";
import { MFA_EN, MFA_STRINGS } from "./_mfa-i18n";

// BaseStrings: the keys of _i18n-en.ts, which each language table has.
// GO-140: the second factor's words (_mfa-i18n.ts) join every table, so the
// pages' Strings has both.
export type BaseStrings = { readonly [K in keyof typeof EN]: string };
export type Strings = BaseStrings & { readonly [K in keyof typeof MFA_EN]: string };
export const LANGS = ["en", "pl", "hi", "de", "fr"] as const;
export type Lang = (typeof LANGS)[number];

export const STRINGS: Record<Lang, Strings> = {
  en: { ...EN, ...MFA_STRINGS.en },
  pl: { ...PL, ...MFA_STRINGS.pl },
  hi: { ...HI, ...MFA_STRINGS.hi },
  de: { ...DE, ...MFA_STRINGS.de },
  fr: { ...FR, ...MFA_STRINGS.fr },
};

// Each language named in itself, for the switcher.
export const LANG_NAMES: Record<Lang, string> = { en: "English", pl: "Polski", hi: "हिन्दी", de: "Deutsch", fr: "Français" };

// The choice made with the switcher, kept for the pages under /auth only.
export const LANG_COOKIE = "ssg-auth-lang";
export const langCookie = (lang: Lang): string => `${LANG_COOKIE}=${lang}; Max-Age=31536000; Path=/auth; Secure; SameSite=Lax`;

// asLang reads one language tag by its primary subtag ("pl-PL" → pl); null
// when it is not one of ours.
export function asLang(raw: unknown): Lang | null {
  const primary = String(raw ?? "").trim().toLowerCase().split(/[-_]/)[0];
  return (LANGS as readonly string[]).includes(primary) ? (primary as Lang) : null;
}

// fromAcceptLanguage picks the supported language with the highest q-value;
// the earlier one wins a tie, q=0 means "not this one".
export function fromAcceptLanguage(header: string | null): Lang | null {
  let best: { lang: Lang; q: number } | null = null;
  for (const part of (header ?? "").split(",")) {
    const [tag, ...params] = part.split(";");
    const q = params.map((p) => /^\s*q=([0-9.]+)\s*$/.exec(p)).find(Boolean);
    const weight = q ? Number(q[1]) : 1;
    const lang = asLang(tag);
    if (lang && weight > 0 && (!best || weight > best.q)) best = { lang, q: weight };
  }
  return best?.lang ?? null;
}

// langOf: `?lang=` → the switcher's cookie → Accept-Language → en.
export function langOf(request: Request): Lang {
  return (
    asLang(new URL(request.url).searchParams.get("lang")) ??
    asLang(readCookie(request, LANG_COOKIE)) ??
    fromAcceptLanguage(request.headers.get("accept-language")) ??
    "en"
  );
}

// fill replaces {name} with vars.name; an unknown name stays visible, so a
// missing value shows up in a test instead of vanishing.
export const fill = (s: string, vars: Record<string, string | number>): string =>
  s.replace(/\{(\w+)\}/g, (m, k: string) => (k in vars ? String(vars[k]) : m));

// when formats an ISO timestamp for people: the language's own date order,
// always in UTC (the worker does not know the reader's time zone), labelled.
export function when(lang: Lang, isoTime: string): string {
  const d = new Date(isoTime);
  return `${new Intl.DateTimeFormat(lang, { dateStyle: "medium", timeStyle: "short", timeZone: "UTC" }).format(d)} UTC`;
}
