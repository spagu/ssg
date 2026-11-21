// #435: the external issuer's words on the pages — the link button and its
// confirmation in five languages.
import { test } from "node:test";
import assert from "node:assert/strict";
import { EXTERNAL_STRINGS } from "../functions/api/auth/_external-i18n.ts";
import { LANGS } from "../functions/api/auth/_i18n.ts";

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
