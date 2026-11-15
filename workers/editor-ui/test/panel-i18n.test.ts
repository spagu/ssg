// GO-173: the panel's strings in five languages. Every language has exactly
// en's keys in en's order, the plural forms its rules need and the same
// {placeholders}; every key the code can ask for exists in English, and
// nothing in English goes unused.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { LANGS } from "../public/js/core/i18n.js";
import { FORM } from "../public/js/models/fm-form.js";
import { HELP_ONLY, SHORTCUTS } from "../public/js/models/shortcuts.js";
import { STATES } from "../public/js/models/article-list.js";

const JS = new URL("../public/js/", import.meta.url).pathname;
const ui: Record<string, Record<string, unknown>> = {};
for (const l of LANGS) ui[l] = (await import(`../public/js/strings/${l}.js`)).default;
const holes = (v: unknown): string[] => [...new Set([...JSON.stringify(v).matchAll(/\{(\w+)\}/g)].map((m) => m[1]))].sort((a, b) => a.localeCompare(b));

for (const lang of LANGS) {
  test(`strings ${lang}: en's keys in order, plural forms, placeholders, nothing blank`, () => {
    assert.deepEqual(Object.keys(ui[lang]), Object.keys(ui.en));
    const rules = new Intl.PluralRules(lang);
    for (const [k, v] of Object.entries(ui[lang])) {
      assert.equal(typeof v, typeof ui.en[k], `${lang} ${k}`);
      if (typeof v === "object") {
        for (const cat of rules.resolvedOptions().pluralCategories) assert.ok(cat in (v as object) || cat === "other" || cat === "many" || cat === "zero" || cat === "two", `${lang} ${k}: ${cat}`);
        assert.ok("other" in (v as object), `${lang} ${k}: other`);
        for (const f of Object.values(v as object)) assert.ok(String(f).trim());
      } else assert.ok(String(v).trim(), `${lang} ${k} is empty`);
      for (const h of holes(ui.en[k]).filter((x) => x !== "count")) assert.ok(holes(v).includes(h), `${lang} ${k}: {${h}} missing`);
      for (const h of holes(v)) assert.ok(holes(ui.en[k]).includes(h), `${lang} ${k}: unknown {${h}}`);
    }
  });
}

test("the other languages are translations, not copies of English", () => {
  for (const lang of LANGS.filter((l) => l !== "en")) {
    const same = Object.keys(ui.en).filter((k) => typeof ui.en[k] === "string" && ui[lang][k] === ui.en[k] && !/^(pref\.lang\.|editor\.lead$|ph\.cell$|diff\.gap$|insert\.sample\.)/.test(k) && String(ui.en[k]).length > 12);
    assert.ok(same.length <= 3, `${lang}: ${same.join(", ")}`);
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
const WORKER_CODES = Object.keys(ui.en).filter((k) => k.startsWith("code."));
const DYNAMIC = [
  ...LANGS.map((l) => `pref.lang.${l}`),
  ...["auto", "light", "dark"].map((v) => `pref.theme.${v}`),
  ...["network", "auth", "forbidden", "config", "notfound", "conflict", "invalid", "server"].map((v) => `error.${v}`),
  ...["title", "code", "detail", "rate", "permission", "unexpected"].map((v) => `error.${v}`),
  ...FORM.flatMap((f) => [`form.${f.key}`, `form.${f.key}Hint`]),
  ...["toml", "none", "unclosed"].map((v) => `form.unusable.${v}`),
  ...[...new Set(SHORTCUTS.map(([c]) => c)), "table", "image", "rule", "insert", "help"].map((c) => `tb.${c}`),
  // The Insert list (GO-174): kinds, components.json states, blocks, refusals.
  ...["component", "shortcode", "block"].map((v) => `insert.kind.${v}`),
  ...["not_published", "redirected", "unavailable", "too_large", "invalid", "bad_url"].map((v) => `insert.components.${v}`),
  ...["table", "code", "mermaid", "math", "footnote", "embed"].flatMap((v) => [`insert.block.${v}`, `insert.blockHint.${v}`]),
  ...["required", "number", "enum", "oneLine", "terminator", "quotes"].map((v) => `insert.error.${v}`),
  "images.altHint", "library.altHint", "library.empty", "library.noMatch",
  ...["bold", "italic", "code", "link", "col1", "col2", "cell", "alt"].map((c) => `ph.${c}`),
  ...HELP_ONLY.map(([k]) => `help.${k}`),
  ...STATES.map((s) => `list.show.${s}`),
  ...["file", "total", "count"].map((v) => `images.fits.${v}`),
  ...["elsewhere", "relative"].map((v) => `preview.image.${v}`),
  ...["component", "shortcode", "html", "taskOpen", "taskDone"].map((v) => `preview.${v}`),
  ...["budget", "not_text", "unavailable"].map((v) => `conflict.unread.${v}`),
  ...["preview", "history"].map((v) => `side.${v}`),
  ...["title", "body", "diff", "commit"].map((v) => `conflict.${v}`),
  ...["title", "settings", "files", "message", "messageHint", "messageEmpty", "author", "confirm", "confirmMissing", "go"].map((v) => `publish.${v}`),
  ...["by", "viewOf", "view", "more", "empty"].map((v) => `rev.${v}`),
  ...["added", "removed", "none", "gap", "label", "noNewline"].map((v) => `diff.${v}`),
  "app.skip", "app.name", "app.menu", "action.retry", "action.dismiss", "state.loading",
  "publish.messageUpdate", "publish.messageNew", "list.colTitle", "list.colSection", "list.colLanguage", "list.colDate", "list.colState",
  "editor.lead", "editor.leadNew", "list.empty", "list.emptyReader",
  // Named in models as ids the views pass to t().
  "form.titleMissing", "form.dateInvalid", "form.slugInvalid", "form.rawHint", "form.rawLocked", "form.rawNone",
  "images.noMediaFolder", "images.needsRepository", "images.noPermission", "publish.mddbRebuild", "publish.mddbSaved",
];

test("every string the code asks for exists in en; nothing in en goes unused", () => {
  const code = sources(JS).map((f) => readFileSync(f, "utf8")).join("\n") + readFileSync(join(JS, "..", "index.html"), "utf8");
  const asked = new Set([...code.matchAll(/\bt\(\s*"([a-zA-Z0-9_.-]+)"/g)].map((m) => m[1]).filter((k) => k.includes(".")));
  for (const k of DYNAMIC) asked.add(k);
  for (const k of asked) assert.ok(k in ui.en, `${k} is asked for and missing`);
  const unused = Object.keys(ui.en).filter((k) => !asked.has(k) && !WORKER_CODES.includes(k));
  assert.deepEqual(unused, []);
});
