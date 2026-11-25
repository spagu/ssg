// #448, the panel's side: the image gate in words — the card's lead in every
// mode, the summary that replaces "only a warning", blocking problems marked
// in words, a refused schedule read from the 409, and the settings line.
// Every key these build exists in en (panel-i18n.test.ts holds the other four
// languages to en's keys).
import { test } from "node:test";
import assert from "node:assert/strict";
import { createI18n } from "../public/js/core/i18n.js";
import { text } from "../public/js/core/vnode.js";
import en from "../public/js/strings/en.js";
import pl from "../public/js/strings/pl.js";
import { checkSummary, needsAttention } from "../public/js/models/image-check.js";
import { RULES, gateMode, gateNote, isBlocking, itemProblems, leadNote, refusal, ruleKey, ruleNames } from "../public/js/models/image-gate.js";
import { warnings } from "../public/js/models/dashboard.js";
import { imageRulesFact, reportView } from "../public/js/views/image-check.js";
import { IMAGE_RULES } from "../src/image-gate-rules.ts";

const { t } = createI18n("en", { en }, {});
const app = { t };
const has = (key: string) => assert.ok(Object.hasOwn(en, key), key);
const item = (problems: string[], blocking?: string[], probe: unknown = null) => ({ url: "https://img.example.net/a.png", alt: "A", stored: false, probe, problems, ...(blocking ? { blocking } : {}) });

test("rules: the panel knows the worker's rules, and words for an unknown one", () => {
  assert.deepEqual(RULES, [...IMAGE_RULES]);
  for (const r of [...RULES, "newer"]) has(ruleKey(r));
  assert.equal(ruleNames(t, ["alt", "load"]), "alt text, loading through a mail image proxy");
  assert.equal(ruleNames(t), "");
});

test("gateMode and leadNote: off keeps the 1.9.15 lead; on, broken and unknown tell the truth", () => {
  assert.equal(gateMode(null), "unknown");
  assert.equal(gateMode({ rules: [], problem: null }), "off");
  assert.equal(gateMode({ rules: ["alt"], problem: null }), "on");
  assert.equal(gateMode({ rules: ["alt"], problem: "bad" }), "broken");
  assert.equal(gateMode({ rules: [], rulesProblem: "bad" }), "broken");
  assert.equal(gateMode({}), "off");
  assert.deepEqual(leadNote({ rules: [] }), { key: "images.lead", vars: {} });
  assert.deepEqual(leadNote({ rules: ["alt"], problem: null }), { key: "imageGate.lead.on", vars: {}, rules: ["alt"] });
  assert.deepEqual(leadNote({ rules: [], problem: "x" }), { key: "imageGate.lead.broken", vars: { detail: "x" } });
  assert.deepEqual(leadNote({ rules: [], rulesProblem: "y" }), { key: "imageGate.lead.broken", vars: { detail: "y" } });
  assert.deepEqual(leadNote(null), { key: "imageGate.lead.unknown", vars: {} });
  for (const k of ["imageGate.lead.on", "imageGate.lead.broken", "imageGate.lead.unknown"]) has(k);
  assert.doesNotMatch(t("imageGate.lead.on", { rules: "alt text" }), /only warns: sending/);
});

test("checkSummary: off says warning only; on says what blocks; a broken rule set is critical", () => {
  const keys = (r: Record<string, unknown>) => checkSummary(r).notes.map((n: { key: string }) => n.key);
  assert.deepEqual(keys({ items: [item(["no_alt"])] }), ["images.summary.problems", "images.summary.warnOnly"]);
  const on = checkSummary({ items: [item(["no_alt"], ["no_alt"]), item(["not_stored"], [])], rules: ["alt"], rulesProblem: null });
  assert.deepEqual(on, {
    notes: [{ key: "images.summary.problems", vars: { count: 2, total: 2 } }, { key: "imageGate.summary.blocking", vars: { count: 1 }, rules: ["alt"] }],
    level: "critical",
  });
  const passes = checkSummary({ items: [item(["not_stored"], [])], rules: ["alt"], rulesProblem: null });
  assert.deepEqual([passes.notes[1], passes.level], [{ key: "imageGate.summary.passes", vars: {}, rules: ["alt"] }, "warning"]);
  assert.equal(checkSummary({ items: [], rules: ["alt"] }).level, "ok");
  const broken = checkSummary({ items: [item(["no_alt"], [])], rules: ["alt"], rulesProblem: "bad" });
  assert.deepEqual([broken.notes[1], broken.level], [{ key: "imageGate.summary.broken", vars: { detail: "bad" } }, "critical"]);
  // Beyond the probe bound under `load`: no problem of its own, still blocking.
  const unprobed = item([], ["not_probed"]);
  assert.equal(needsAttention(unprobed), true);
  assert.deepEqual(checkSummary({ items: [unprobed], rules: ["load"], skipped: 1 }).notes.map((n: { key: string }) => n.key), [
    "images.summary.problems", "images.summary.skipped", "imageGate.summary.blocking",
  ]);
  assert.equal(gateNote({ items: [] }), null);
  assert.deepEqual(gateNote({ rules: ["alt"] }), { note: { key: "imageGate.summary.passes", vars: {}, rules: ["alt"] }, level: null });
  for (const k of ["imageGate.summary.blocking", "imageGate.summary.passes", "imageGate.summary.broken"]) has(k);
});

test("itemProblems: blocking problems are marked; not_probed joins them", () => {
  assert.deepEqual(itemProblems(item(["not_stored", "no_alt"], ["no_alt", "not_probed"])), [
    { code: "not_stored", blocking: false }, { code: "no_alt", blocking: true }, { code: "not_probed", blocking: true },
  ]);
  assert.deepEqual(itemProblems(item(["no_alt"])), [{ code: "no_alt", blocking: false }]);
  assert.equal(isBlocking(item([])), false);
  assert.equal(isBlocking(item([], ["no_alt"])), true);
});

test("refusal: reads only a 409 image_check_failed with items", () => {
  const body = { error: "image_check_failed", items: [{ url: "u", alt: "", stored: false, probe: null, problems: ["no_alt"] }], rules: ["alt"] };
  const r = refusal({ code: "image_check_failed", body });
  assert.deepEqual(r, {
    items: [{ url: "u", alt: "", stored: false, probe: null, problems: ["no_alt"], blocking: ["no_alt"] }],
    rules: ["alt"],
    note: { key: "imageGate.refused", vars: { count: 1 }, rules: ["alt"] },
  });
  assert.deepEqual(refusal({ code: "image_check_failed", body: { items: [{ url: "u", blocking: ["not_probed"] }] } })!.items[0], { url: "u", problems: [], blocking: ["not_probed"] });
  assert.equal(refusal({ code: "image_check_failed", body: { items: [{ url: "u" }] } })!.items[0].blocking.length, 0);
  assert.equal(refusal({ code: "invalid_transition", body }), null);
  assert.equal(refusal({ code: "image_check_failed", body: {} }), null);
  assert.equal(refusal(null), null);
  has("imageGate.refused");
  assert.equal(t("imageGate.refused", { count: 2, rules: "alt text" }), "Not scheduled: 2 images break a rule enforced before scheduling (alt text). Fix them in the editor and schedule again; a test send still works.");
});

test("reportView: blocking images say so in words; the refusal's own summary replaces the check's", () => {
  const report = { items: [item(["no_alt", "not_stored"], ["no_alt"]), item([], ["not_probed"]), item([])], rules: ["alt", "load"], rulesProblem: null, skipped: 0 };
  const out = text({ tag: "div", props: {}, children: reportView(app, report).filter(Boolean) });
  assert.match(out, /Blocks scheduling.*Blocks scheduling:.*no alt text.*not in the image store/s);
  assert.equal(out.match(/Blocks scheduling:/g)!.length, 2, "one label per blocking problem, none on a warning");
  assert.match(out, /not tested for loading, because one check/);
  assert.match(out, /Looks fine/);
  const compact = reportView(app, report, { compact: true, summary: { notes: [refusal({ code: "image_check_failed", body: { ...report, items: report.items.slice(0, 2) } })!.note], level: "critical" } });
  const c = text({ tag: "div", props: {}, children: compact.filter(Boolean) });
  assert.match(c, /^Not scheduled: 2 images break a rule enforced before scheduling \(alt text, loading through a mail image proxy\)/);
  assert.doesNotMatch(c, /Looks fine/, "the dialog lists only the images that need attention");
});

test("imageRulesFact and the dashboard warning: the settings say what is enforced or why nothing can be", () => {
  assert.equal(text(imageRulesFact(app, { rules: ["alt", "store"], problem: null })), "alt text, image store");
  assert.equal(text(imageRulesFact(app, { rules: [], problem: null })), t("imageGate.setting.off"));
  assert.equal(text(imageRulesFact(app, undefined)), t("imageGate.setting.off"), "an older worker without the field");
  assert.equal(text(imageRulesFact(app, { rules: ["alt"], problem: "x" })), "Cannot be applied: x");
  const base = { transport: "resend", siteUrlValid: true, sender: { email: "a@example.com" }, engine: { queue: true }, webhooks: { resend: true } };
  assert.deepEqual(warnings({ settings: base }), []);
  const w = warnings({ settings: { ...base, imageRules: { rules: [], problem: "bad" } } });
  assert.deepEqual(w.map((x: { level: string; key: string; vars: unknown }) => [x.level, x.key, x.vars]), [["critical", "imageGate.warn", { detail: "bad" }]]);
  for (const k of ["imageGate.warn", "imageGate.warn.go", "imageGate.setting", "code.image_rules_unusable", "code.image_check_failed", "code.campaign_changed", "imageGate.blocks", "imageGate.blocking", "imageGate.problem.not_probed"]) has(k);
  // Polish plural forms are all there for the counted sentences.
  for (const k of ["imageGate.summary.blocking", "imageGate.refused"]) assert.deepEqual(Object.keys((pl as Record<string, object>)[k]), ["one", "few", "many", "other"]);
});
