// GO-179: the panel's strings in five languages. Every language has exactly
// en's keys in en's order, the plural forms its rules need and the same
// {placeholders}; every group and every key in TRANSLATED has a name in all
// five and a description in the four besides English; English group names
// equal the schema's; and every key the code can ask for exists.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { LANGS } from "../public/js/core/i18n.js";
import { TRANSLATED } from "../public/js/models/labels.js";
import { ADVANCED } from "../public/js/models/form-model.js";
import { SCHEMA, namedNodes } from "../src/schema.ts";

const JS = new URL("../public/js/", import.meta.url).pathname;
const ui: Record<string, Record<string, unknown>> = {};
const keys: Record<string, Record<string, unknown>> = {};
for (const l of LANGS) {
  ui[l] = (await import(`../public/js/strings/${l}.js`)).default;
  keys[l] = (await import(`../public/js/strings/keys/${l}.js`)).default;
}
const holes = (v: unknown): string[] => [...new Set([...JSON.stringify(v).matchAll(/\{(\w+)\}/g)].map((m) => m[1]))].sort();

for (const lang of LANGS) {
  test(`strings ${lang}: en's keys in order, plural forms, placeholders, nothing blank`, () => {
    assert.deepEqual(Object.keys(ui[lang]), Object.keys(ui.en));
    const rules = new Intl.PluralRules(lang);
    for (const [k, v] of Object.entries(ui[lang])) {
      assert.equal(typeof v, typeof ui.en[k], `${lang} ${k}`);
      if (typeof v === "object") {
        assert.ok("other" in (v as object), `${lang} ${k}: other`);
        for (const cat of rules.resolvedOptions().pluralCategories) assert.ok(cat in (v as object) || cat === "many" || cat === "few" || cat === "zero" || cat === "two", `${lang} ${k}: ${cat}`);
        for (const f of Object.values(v as object)) assert.ok(String(f).trim());
      } else assert.ok(String(v).trim(), `${lang} ${k} is empty`);
      for (const h of holes(ui.en[k]).filter((x) => x !== "count")) assert.ok(holes(v).includes(h), `${lang} ${k}: {${h}} missing`);
      for (const h of holes(v)) assert.ok(holes(ui.en[k]).includes(h), `${lang} ${k}: unknown {${h}}`);
    }
  });

  test(`setting names ${lang}: every group, every TRANSLATED key, descriptions besides English`, () => {
    const want = [...[...SCHEMA.groups.map((g) => g.id), ADVANCED].flatMap((g) => [`group.${g}.title`, `group.${g}.desc`]), ...TRANSLATED.map((k: string) => `key.${k}`)];
    const descs = lang === "en" ? [] : TRANSLATED.map((k: string) => `desc.${k}`);
    assert.deepEqual(Object.keys(keys[lang]).sort(), [...want, ...descs].sort());
    for (const [k, v] of Object.entries(keys[lang])) assert.ok(String(v).trim(), `${lang} ${k}`);
    for (const k of Object.keys(keys[lang])) assert.ok(!(k in ui[lang]), `${k} is in both tables`);
  });
}

test("English group names are the schema's; TRANSLATED keys are plain keys of the schema", () => {
  for (const g of SCHEMA.groups) {
    assert.equal(keys.en[`group.${g.id}.title`], g.title);
    assert.equal(keys.en[`group.${g.id}.desc`], g.description);
  }
  const named = new Map(namedNodes().map((n) => [n.segments.join("."), n.node]));
  for (const k of TRANSLATED) {
    assert.ok(named.has(k), k);
    assert.equal(named.get(k)!.class ?? "plain", "plain", k);
  }
});

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f);
    if (statSync(p).isDirectory()) return f === "strings" ? [] : sources(p);
    return f.endsWith(".js") ? [p] : [];
  });
}

// Keys built at run time: every value they can take.
const DYNAMIC = [
  ...["options", "appearance", "files"].map((v) => `nav.${v}`),
  ...LANGS.map((l) => `pref.lang.${l}`),
  ...["auto", "light", "dark"].map((v) => `pref.theme.${v}`),
  ...["network", "auth", "forbidden", "config", "notfound", "conflict", "invalid", "server"].map((v) => `error.${v}`),
  ...["required", "number", "integer", "secretName", "secretLogin", "secretScheme", "secretReEnter", "secretLine", "enum", "bool", "emptyKey", "duplicateKey", "badKey"].map((v) => `problem.${v}`),
  ...["config_unreadable", "value_unread", "include_not_followed", "overlay_unreadable", "unknown_key", "overlay_needs_yaml"].map((v) => `settingsProblem.${v}`),
  ...["default", "file", "overlay", "unknown", "pending", "reverting"].map((v) => `source.${v}`),
  ...["light", "dark"].map((v) => `scheme.${v}`),
  ...["theme_not_in_repository", "online_theme", "no_template", "theme_dir_unknown", "other"].flatMap((r) => [`theme.${r}.title`, `theme.${r}.body`]),
  "value.on", "value.off", "secret.set", "secret.unset", "advanced.youCan", "advanced.youCannot", "contrast.passes", "contrast.below",
  "field.undo", "field.revert", "options.noneChanged", "options.noneFound", "rows.up", "rows.down", "rows.remove", "rows.upOf", "rows.downOf", "rows.removeOf",
  "value.secretSet", "value.secretUnset", "value.unsetDefault", "app.skip", "app.name", "app.menu",
  "contrast.pair", "contrast.scheme", "contrast.sampleHead", "contrast.result", "contrast.suggestion", "files.pick", "files.gone", "conflict.discarded", "conflict.kept",
];

test("every string the code asks for exists in en; nothing in en goes unused", () => {
  const text = sources(JS).map((f) => readFileSync(f, "utf8")).join("\n") + readFileSync(join(JS, "..", "index.html"), "utf8");
  const asked = new Set([...text.matchAll(/\bt\(\s*"([a-zA-Z0-9_.-]+)"/g)].map((m) => m[1]).filter((k) => k.includes(".")));
  for (const k of DYNAMIC) asked.add(k);
  for (const k of asked) assert.ok(k in ui.en, `${k} is asked for and missing`);
  // code.* are looked up by the worker's error code; everything else must be asked for.
  const unused = Object.keys(ui.en).filter((k) => !asked.has(k) && !k.startsWith("code.") && !/^(diff|value)\.(added|removed|items|keys|more|reverted|unset|empty|none)$/.test(k) && !/^(error\.(title|code|detail|rate|permission|unexpected)|action\.(retry|dismiss)|state\.loading)$/.test(k));
  assert.deepEqual(unused, []);
});
