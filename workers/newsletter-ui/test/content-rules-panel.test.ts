// Content rules (#449), the panel's side: findings in words, what is
// configured, the refused schedule recognised, and the views' vnodes — data
// (phrases, URLs) always as text, never markup. Every key exists in en
// (panel-i18n.test.ts holds the other four languages to en's keys).
import { test } from "node:test";
import assert from "node:assert/strict";
import { createI18n } from "../public/js/core/i18n.js";
import { classify } from "../public/js/core/api.js";
import en from "../public/js/strings/en.js";
import pl from "../public/js/strings/pl.js";
import { warnings } from "../public/js/models/dashboard.js";
import { RULES, checkSummary, findingNote, refusedFindings, rulesNotes, rulesShown } from "../public/js/models/content-check.js";
import { contentCheckCard, contentRulesFact, dialogRefusal, reportView, runCheck } from "../public/js/views/content-check.js";

const { t } = createI18n("en", { en }, {});
const has = (key: string) => assert.ok(Object.hasOwn(en, key), key);
const RULE_SET = { active: true, bannedPhrases: 2, blockedImagePatterns: 1, utmStrict: true, problem: null };

// texts: every text child of a vnode tree, in order.
const texts = (n: unknown): string[] => (n == null || n === false ? [] : typeof n === "string" ? [n] : Array.isArray(n) ? n.flatMap(texts) : texts((n as { children: unknown[] }).children));
const props = (n: unknown): Record<string, unknown>[] =>
  n == null || typeof n !== "object" ? [] : Array.isArray(n) ? n.flatMap(props) : [(n as { props: Record<string, unknown> }).props, ...props((n as { children: unknown[] }).children)];

test("findingNote: each rule and place has its sentence, an unknown one a fallback", () => {
  const phrase = findingNote({ rule: "banned_phrase", where: "subject", value: "last chance", match: "Last Chance", count: 2 });
  assert.deepEqual(phrase, { key: "content.finding.phrase.subject", vars: { phrase: "last chance", match: "Last Chance", count: 2 } });
  assert.equal(t(phrase.key, phrase.vars), "The subject contains the banned phrase “last chance” 2 times (first written “Last Chance”).");
  assert.deepEqual(findingNote({ rule: "banned_phrase", where: "text", value: "x" }).vars, { phrase: "x", match: "x", count: 1 });
  const image = findingNote({ rule: "blocked_image", where: "body", value: "https://example.com/wp-content/a.png", pattern: "/wp-content/" });
  assert.match(t(image.key, image.vars), /The image https:\/\/example\.com\/wp-content\/a\.png .* contains “\/wp-content\/”/);
  assert.deepEqual(findingNote({ rule: "blocked_image", value: "u" }).vars, { url: "u", pattern: "" });
  const utm = findingNote({ rule: "utm_conflict", where: "text", value: "https://example.com/?utm_source=x", host: "example.com", parameters: ["utm_source", "utm_medium"] });
  assert.equal(utm.key, "content.finding.utm.text");
  assert.equal(utm.vars.parameters, "utm_source, utm_medium");
  assert.deepEqual(findingNote({ rule: "utm_conflict", where: "body", value: "u" }).vars, { url: "u", host: "", parameters: "" });
  for (const f of [{ rule: "new_rule", where: "body", value: "v" }, { rule: "banned_phrase", where: "footer", value: "v" }, { rule: "utm_conflict", where: "subject", value: "v" }]) {
    assert.equal(findingNote(f).key, "content.finding.other");
  }
  for (const where of ["subject", "body", "text"]) has(`content.finding.phrase.${where}`);
  for (const where of ["body", "text"]) has(`content.finding.utm.${where}`);
  assert.deepEqual(RULES, ["banned_phrase", "blocked_image", "utm_conflict"]);
});

test("rulesShown, rulesNotes, checkSummary", () => {
  assert.equal(rulesShown(null), false);
  assert.equal(rulesShown(undefined), false);
  assert.equal(rulesShown({ ...RULE_SET, active: false }), false);
  assert.equal(rulesShown(RULE_SET), true);
  assert.deepEqual(rulesNotes(RULE_SET).map((n: { key: string }) => n.key), ["content.rules.phrases", "content.rules.patterns", "content.rules.utm"]);
  assert.deepEqual(rulesNotes({ ...RULE_SET, bannedPhrases: 0, blockedImagePatterns: 0, utmStrict: false }), []);
  assert.equal(t("content.rules.phrases", { count: 1 }), "1 banned phrase is looked for in the subject, the text and the text version.");
  assert.deepEqual(checkSummary([]), { note: { key: "content.summary.clean", vars: {} }, level: "ok" });
  assert.deepEqual(checkSummary([{}, {}]), { note: { key: "content.summary.problems", vars: { count: 2 } }, level: "critical" });
  const plT = createI18n("pl", { pl, en }, {}).t;
  assert.equal(plT("content.summary.problems", { count: 3 }), "3 naruszenia: popraw je przed zaplanowaniem.");
  assert.equal(plT("content.summary.problems", { count: 5 }), "5 naruszeń: popraw je przed zaplanowaniem.");
});

test("refusedFindings: only a 409 content_check_failed with findings", () => {
  const findings = [{ rule: "banned_phrase", where: "body", value: "x", match: "x", count: 1 }];
  assert.deepEqual(refusedFindings(classify(409, { error: "content_check_failed", detail: "1 finding", findings }, null)), findings);
  assert.equal(refusedFindings(classify(409, { error: "invalid_transition" }, null)), null);
  assert.equal(refusedFindings(classify(409, { error: "content_check_failed" }, null)), null);
  assert.equal(refusedFindings(null), null);
  assert.equal(refusedFindings(new Error("x")), null);
  has("code.content_check_failed");
  has("code.content_rules_invalid");
});

test("views: phrases and URLs are text; the card shows only with rules", () => {
  const app = { t, api: null } as never;
  const evil = '<img src=x onerror="alert(1)">';
  const view = reportView(app, [{ rule: "banned_phrase", where: "body", value: evil, match: evil, count: 1 }]);
  const all = texts(view).join(" ");
  assert.ok(all.includes(`“${evil}”`), "the phrase is a text child, verbatim");
  assert.ok(!props(view).some((p) => "innerHTML" in p), "no innerHTML anywhere");
  assert.equal(texts(reportView(app, [])).join(""), "The campaign follows every content rule.");
  assert.equal(contentCheckCard(app, () => null, { rules: null }), null);
  assert.equal(contentCheckCard(app, () => null, { rules: { ...RULE_SET, active: false } }), null);
  const card = contentCheckCard(app, () => null, { rules: RULE_SET });
  assert.ok(texts(card).includes("Content rules"));
  assert.ok(texts(card).includes("Check content"));
  const broken = contentCheckCard(app, () => null, { rules: { ...RULE_SET, problem: "NEWSLETTER_UTM_STRICT must be on or off" } });
  assert.ok(texts(broken).some((s) => s.endsWith("NEWSLETTER_UTM_STRICT must be on or off")));
  assert.deepEqual(texts(contentRulesFact(app, null)), [t("set.off")]);
  assert.equal(texts(contentRulesFact(app, { ...RULE_SET, problem: "bad" })).join(""), "Cannot be applied: bad");
  assert.match(texts(contentRulesFact(app, RULE_SET)).join(""), /^2 banned phrases .* 1 blocked image source is checked\. Hand-written/);
  for (const k of ["content.title", "content.lead", "content.check", "content.checking", "content.failed", "content.list", "content.refused", "content.setting", "content.warn", "content.warn.go"]) has(k);
});

test("runCheck asks for the campaign's language and never rejects; dialogRefusal knows its own refusal", async () => {
  const drawn: unknown[] = [];
  const target = { set: (v: unknown) => drawn.push(v) };
  const asked: unknown[] = [];
  const ok = { t, api: { get: async (path: string, q: unknown) => (asked.push([path, q]), { findings: [], rules: RULE_SET }) } } as never;
  await runCheck(ok, { id: "a b", segment: { languages: ["pl"] } }, target);
  assert.deepEqual(asked, [["campaigns/a%20b/content-check", { language: "pl" }]]);
  assert.equal(texts(drawn[0]).join(""), t("content.checking"));
  assert.equal(texts(drawn[1]).join(""), "The campaign follows every content rule.");
  const failing = { t, has: () => false, api: { get: async () => Promise.reject(new Error("offline")) } } as never;
  await runCheck(failing, { id: "x" }, target);
  assert.ok(texts(drawn[3]).includes(t("content.failed")));
  const d = dialogRefusal({ t } as never);
  assert.equal((d.node as { props: { role: string } }).props.role, "alert", "the live region exists before anything is drawn");
  assert.equal(d.refuse(classify(409, { error: "image_check_failed" }, null)), false);
  assert.equal(d.refuse(classify(409, { error: "content_check_failed", findings: [] }, null)), true);
});

test("dashboard: content rules that cannot be applied are a critical warning", () => {
  const base = { transport: "resend", siteUrlValid: true, sender: { email: "a@example.com" }, engine: { queue: true }, webhooks: { resend: true } };
  assert.deepEqual(warnings({ settings: { ...base, contentRules: { ...RULE_SET, problem: null } } }), []);
  const w = warnings({ settings: { ...base, contentRules: { ...RULE_SET, problem: "bad" } } });
  assert.deepEqual(w.map((x: { level: string; key: string; vars: unknown }) => [x.level, x.key, x.vars]), [["critical", "content.warn", { detail: "bad" }]]);
  for (const k of ["content.rules.phrases", "content.rules.patterns", "content.summary.problems", "content.finding.phrase.body"]) {
    assert.deepEqual(Object.keys((pl as Record<string, object>)[k]), ["one", "few", "many", "other"], k);
  }
});
