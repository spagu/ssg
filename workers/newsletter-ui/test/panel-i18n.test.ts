// GO-128: the panel's strings. Every language has exactly en's keys, plural
// forms its rules need, the same {placeholders}; and every key the code can
// ask for exists — read from the source, so a new t("…") cannot ship blank.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { LANGS } from "../public/js/core/i18n.js";

const PUBLIC = new URL("../public/js/", import.meta.url).pathname;
const tables: Record<string, Record<string, unknown>> = {};
for (const l of LANGS) tables[l] = (await import(`../public/js/strings/${l}.js`)).default;
const en = tables.en;

const holes = (v: unknown): string[] => [...new Set([...JSON.stringify(v).matchAll(/(?<!\{)\{(\w+)\}(?!\})/g)].map((m) => m[1]))].sort();

for (const lang of LANGS) {
  test(`strings ${lang}: same keys as en, in order, with the same placeholders`, () => {
    const t = tables[lang];
    assert.deepEqual(Object.keys(t), Object.keys(en));
    const rules = new Intl.PluralRules(lang);
    for (const [k, v] of Object.entries(t)) {
      assert.equal(typeof v, typeof en[k], `${lang} ${k}: string vs plural object`);
      if (typeof v === "object") {
        for (const cat of rules.resolvedOptions().pluralCategories) assert.ok(cat in (v as object) || "other" in (v as object), `${lang} ${k}: ${cat}`);
        assert.ok("other" in (v as object), `${lang} ${k}: other`);
        for (const form of Object.values(v as object)) assert.equal(typeof form, "string");
      } else {
        assert.ok((v as string).trim(), `${lang} ${k} is empty`);
      }
      // A translation may drop {count} in a "one" form; every other placeholder stays.
      const want = holes(en[k]).filter((h) => h !== "count");
      for (const h of want) assert.ok(holes(v).includes(h), `${lang} ${k}: {${h}} missing`);
      for (const h of holes(v)) assert.ok(holes(en[k]).includes(h), `${lang} ${k}: unknown {${h}}`);
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

// Keys built at run time, by template: every value they can take.
const STATUSES = ["draft", "scheduled", "sending", "paused", "sent", "cancelled", "confirmed", "pending", "unsubscribed"];
const ACTIONS = ["save", "test", "schedule", "unschedule", "pause", "resume", "cancel", "duplicate", "delete"];
const DYNAMIC = [
  ...STATUSES.map((s) => `status.${s}`),
  ...["claimed", "retry", "sent", "failed", "skipped", "uncertain"].map((s) => `send.${s}`),
  ...["hard", "soft", "complaint", "manual"].map((s) => `reason.${s}`),
  ...["sent", "failed", "skipped", "pending", "uncertain", "unsubscribes"].map((s) => `result.${s}`),
  ...["email", "status", "signedUpAt", "unsubscribedAt", "bouncedAt", "language", "tags", "sourcePage"].map((s) => `role.${s}`),
  ...["toConfirm", "toPending", "toUnsubscribed", "toSuppress", "duplicate", "existing", "suppressed", "invalid"].map((s) => `import.${s}`),
  ...["invalid_email", "invalid_language", "invalid_tags", "invalid_status"].map((s) => `import.invalid.${s}`),
  "import.date.unparseable", "import.date.future", "import.dateOrder.dmy", "import.dateOrder.mdy",
  ...["names", "lost", "other"].map((s) => `import.reason.${s}`),
  ...["comma", "semicolon", "tab", "other"].map((s) => `delimiter.${s}`),
  ...ACTIONS.map((a) => `act.${a}`),
  ...["unschedule", "pause", "resume", "cancel", "duplicate", "delete"].map((a) => `act.${a}.done`),
  ...["cancel", "delete"].flatMap((a) => ["title", "body", "go"].map((s) => `act.${a}.${s}`)),
  ...["resend", "dismiss"].flatMap((k) => ["title", "go", "done"].map((s) => `unc.${k}.${s}`)),
  ...LANGS.map((l) => `pref.lang.${l}`),
  ...["auto", "light", "dark"].map((v) => `pref.theme.${v}`),
  ...["dashboard", "subscribers", "import", "campaigns", "suppressions", "settings", "help"].map((v) => `nav.${v}`),
  ...["critical", "warning", "info"].map((v) => `level.${v}`),
  ...["noTransport", "noSiteUrl", "noSender", "noQueue", "enginePaused", "uncertain", "noWebhook"].map((w) => `warn.${w}.go`),
  ...["confirmed", "pending", "unsubscribed", "suppressed"].map((v) => `tile.${v}`),
  ...["network", "auth", "csrf", "forbidden", "config", "notfound", "conflict", "invalid", "server"].map((v) => `error.${v}`),
  ...["day", "week", "month"].map((v) => `col.${v}`),
  "editor.format.markdown", "editor.format.html", "editor.tagMode.any", "editor.tagMode.all",
  "preview.tab.html", "preview.tab.text", "seg.tags.any", "seg.tags.all",
  "smtp.tls.implicit", "smtp.tls.starttls", "set.auth.access", "set.auth.basic",
  ...["resend", "mailchannels", "smtp"].map((v) => `transport.${v}`),
  ...["subscribers", "import", "campaigns", "sending", "suppressions", "settings", "charts", "privacy"].flatMap((k) => [`help.${k}.title`, `help.${k}.body`]),
];

test("strings: every key the code can ask for exists in en", () => {
  const prefixes = new Set(Object.keys(en).map((k) => k.split(".")[0]));
  const used = new Set<string>(DYNAMIC);
  for (const file of sources(PUBLIC)) {
    for (const [, key] of readFileSync(file, "utf8").matchAll(/["`]([a-z][A-Za-z]*\.[A-Za-z][\w.]*)["`]/g)) {
      if (prefixes.has(key.split(".")[0]) && !/\.(js|css|svg|html)$/.test(key)) used.add(key);
    }
  }
  const missing = [...used].filter((k) => !Object.hasOwn(en, k));
  assert.deepEqual(missing, []);
  assert.ok(used.size > 400, `found ${used.size} keys`);
});

test("strings: the error codes the API can send have words", () => {
  for (const code of ["no_site_url", "no_transport", "no_sender", "test_limit", "invalid_transition", "empty_segment", "unknown_variable", "send_failed", "rate_limited", "confirm_required"]) {
    assert.ok(Object.hasOwn(en, `code.${code}`), code);
  }
});
