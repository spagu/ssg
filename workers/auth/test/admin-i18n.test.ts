// GO-138: the admin panel's words. Every language has exactly en's keys, the
// plural forms its rules need and the same {placeholders}; every key the code
// can ask for exists — read from the source, so a new t("…") cannot ship
// blank; and the shell's server-side words (_admin-i18n.ts) likewise.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { LANGS } from "../public/auth/assets/admin/js/core/i18n.js";
import { EVENT_KINDS, EVENT_GROUPS } from "../public/auth/assets/admin/js/models/events.js";
import { NAV } from "../public/auth/assets/admin/js/models/access.js";
import { ADMIN_STRINGS } from "../functions/api/auth/_admin-i18n.ts";

const JS = new URL("../public/auth/assets/admin/js/", import.meta.url).pathname;
const tables: Record<string, Record<string, unknown>> = {};
for (const l of LANGS) tables[l] = (await import(`../public/auth/assets/admin/js/strings/${l}.js`)).default;
const en = tables.en;
const holes = (v: unknown): string[] => [...new Set([...JSON.stringify(v).matchAll(/\{(\w+)\}/g)].map((m) => m[1]))].sort();

for (const lang of LANGS) {
  test(`admin strings ${lang}: en's keys in order, real text, the same placeholders, every plural form`, () => {
    const t = tables[lang];
    assert.deepEqual(Object.keys(t), Object.keys(en));
    const categories = new Intl.PluralRules(lang).resolvedOptions().pluralCategories;
    for (const [k, v] of Object.entries(t)) {
      assert.equal(typeof v, typeof en[k], `${lang} ${k}`);
      if (typeof v === "object") {
        for (const cat of categories) assert.ok(cat in (v as object) || "other" in (v as object), `${lang} ${k}: ${cat}`);
        assert.ok("other" in (v as object), `${lang} ${k}: other`);
      } else assert.ok((v as string).trim(), `${lang} ${k} is empty`);
      const want = holes(en[k]).filter((h) => h !== "count");
      for (const h of want) assert.ok(holes(v).includes(h), `${lang} ${k}: {${h}} missing`);
      for (const h of holes(v)) assert.ok(holes(en[k]).includes(h), `${lang} ${k}: unknown {${h}}`);
      if (lang !== "en" && typeof v === "string" && v.length > 24 && !/^[A-Z][A-Za-z ]+$/.test(v)) assert.notEqual(v, en[k], `${lang} ${k} is untranslated`);
    }
  });
}

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f);
    if (statSync(p).isDirectory()) return f === "strings" ? [] : sources(p);
    return f.endsWith(".js") ? [p] : [];
  });
}

// Keys built at run time: every value they can take.
const DYNAMIC = [
  ...NAV.map((v) => `nav.${v}`),
  ...LANGS.map((l) => `pref.lang.${l}`),
  ...["auto", "light", "dark"].map((v) => `pref.theme.${v}`),
  ...["network", "auth", "stepup", "forbidden", "invalid", "notfound", "conflict", "config", "server"].map((k) => `error.${k}`),
  ...["active", "blocked"].map((s) => `status.${s}`),
  ...["totp", "passkey", "backup"].map((f) => `factor.${f}`),
  ...["password", "google", "microsoft", "apple", "github", "facebook", "access"].map((p) => `provider.${p}`),
  ...["pending", "accepted", "expired", "revoked"].map((s) => `invite.status.${s}`),
  ...["all", "some", "none"].map((l) => `matrix.${l}`),
  ...["active", "disabled"].map((s) => `service.status.${s}`),
  ...["active", "expired", "revoked"].map((s) => `token.status.${s}`),
  ...["absent", "enabled", "misconfigured"].map((s) => `status.access.${s}`),
  ...["off", "ready", "misconfigured"].map((s) => `status.mfa.${s}`),
  ...["default", "custom", "invalid"].map((s) => `status.policy.${s}`),
  ...["none", "ready", "broken"].map((s) => `status.mail.${s}`),
  ...["ok", "rotate", "unknown", "current", "previous"].map((s) => `status.key.${s}`),
  ...["keyOld", "mfaMisconfigured", "mfaPolicyInvalid", "missingFactor", "mailBroken", "accessMisconfigured", "registry"].map((w) => `status.warn.${w}`),
  ...Object.keys(EVENT_GROUPS).map((g) => `audit.group.${g}`),
  ...EVENT_KINDS.map((k) => `event.${k}`),
  "role.wildcard.all", "role.wildcard.module", "person.resetLink.bodyLink", "person.resetLink.bodyMail",
  "invite.caption", "invite.caption.all", "service.disabled", "service.enabled", "service.disable", "service.enable",
];

test("admin strings: every key the code can ask for exists in en, and no key is unused", () => {
  const prefixes = new Set(Object.keys(en).map((k) => k.split(".")[0]));
  const used = new Set<string>(DYNAMIC);
  for (const file of sources(JS)) {
    for (const [, key] of readFileSync(file, "utf8").matchAll(/["`]([a-z][A-Za-z]*\.[A-Za-z][\w.]*)["`]/g)) {
      if (prefixes.has(key.split(".")[0]) && !/\.(js|css|svg|html)$/.test(key)) used.add(key);
    }
  }
  assert.deepEqual([...used].filter((k) => !Object.hasOwn(en, k)), []);
  // code.* are chosen by the API's error code; everything else must be asked for.
  assert.deepEqual(Object.keys(en).filter((k) => !used.has(k) && !k.startsWith("code.")), []);
});

test("admin shell words: five languages, the same keys and placeholders", () => {
  const base = ADMIN_STRINGS.en;
  for (const lang of LANGS) {
    const t = ADMIN_STRINGS[lang as keyof typeof ADMIN_STRINGS];
    assert.deepEqual(Object.keys(t), Object.keys(base), lang);
    for (const [k, v] of Object.entries(t)) {
      assert.ok(v.trim(), `${lang} ${k}`);
      assert.deepEqual(holes(v), holes(base[k as keyof typeof base]), `${lang} ${k}`);
    }
  }
});
