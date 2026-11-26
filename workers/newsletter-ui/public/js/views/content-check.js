// The content rules (#449) in the panel: a "Content rules" card next to
// "Images in this mail" in the editor (only when a rule is configured), and
// the findings of a refused schedule inside the Schedule dialog. The words
// come from models/content-check.js; phrases and URLs are put in as text
// nodes, never as markup. The card shows only on a draft: once scheduled, the
// content can no longer change and has passed the same check.

import { h } from "../core/vnode.js";
import { checkSummary, findingNote, refusedFindings, rulesNotes, rulesShown } from "../models/content-check.js";
import { checkLanguage } from "../models/image-check.js";
import { btn, errorText, slot } from "./ui.js";

const say = (app, note) => app.t(note.key, note.vars);

// findingsList: the findings, one sentence each.
const findingsList = (app, findings) =>
  findings.length ? h("ul", { class: "plain", "aria-label": app.t("content.list") }, findings.map((f) => h("li", null, say(app, findingNote(f))))) : null;

// reportView: the summary sentence, then the findings.
export function reportView(app, findings) {
  const { note, level } = checkSummary(findings);
  return [h("div", { class: `callout ${level}` }, h("p", null, say(app, note))), findingsList(app, findings)];
}

// runCheck asks the worker and draws the answer, or the failure, into
// `target`; it never rejects.
export async function runCheck(app, c, target) {
  target.set(h("p", { class: "loading" }, app.t("content.checking")));
  try {
    const r = await app.api.get(`campaigns/${encodeURIComponent(c.id)}/content-check`, { language: checkLanguage(c) });
    target.set(reportView(app, r.findings));
  } catch (e) {
    target.set(h("div", { class: "callout warning" }, h("p", null, app.t("content.failed")), h("p", null, errorText(app, e))));
  }
}

// contentCheckCard: the card under the editor, or null without rules.
// `rules` is settings.contentRules; `ready` saves the draft first, so the
// check reads what is on screen.
export function contentCheckCard(app, getCampaign, { rules, ready = async () => true }) {
  if (!rulesShown(rules)) return null;
  const { t } = app;
  const result = slot("content-check-result");
  const check = async (button) => {
    button.disabled = true;
    button.setAttribute("aria-busy", "true");
    try {
      if (await ready()) await runCheck(app, getCampaign(), result);
    } finally {
      button.disabled = false;
      button.removeAttribute("aria-busy");
    }
  };
  return h(
    "section",
    { class: "card", "aria-labelledby": "content-check-title" },
    h("h2", { id: "content-check-title" }, t("content.title")),
    h("p", { class: "note" }, t("content.lead")),
    rules.problem
      ? h("div", { class: "callout critical" }, h("p", null, t("content.warn", { detail: rules.problem })))
      : h("ul", { class: "plain" }, rulesNotes(rules).map((n) => h("li", null, say(app, n)))),
    btn(t("content.check"), (e) => {
      check(e.currentTarget).catch((err) => result.set(h("p", { class: "bad" }, errorText(app, err))));
    }),
    h("div", { role: "status", "aria-live": "polite" }, result.node),
  );
}

// contentRulesFact: the Settings view's value for the content rules — off,
// what is on (counts only), or why they cannot be applied.
export function contentRulesFact(app, rules) {
  if (!rulesShown(rules)) return h("span", { class: "flag off" }, app.t("set.off"));
  if (rules.problem) return h("span", { class: "flag off bad" }, app.t("content.setting.broken", { detail: rules.problem }));
  return h("span", null, rulesNotes(rules).map((n) => say(app, n)).join(" "));
}

// dialogRefusal: the block the Schedule dialog keeps for a refusal. The
// alert region exists from the start (empty), so what refuse(err) draws into
// it is announced. refuse answers true when it drew a content refusal.
export function dialogRefusal(app) {
  const result = slot("content-refusal");
  const node = h("div", { role: "alert" }, result.node);
  const refuse = (err) => {
    const findings = refusedFindings(err);
    if (!findings) return false;
    result.set(
      h(
        "section",
        { class: "callout critical", "aria-labelledby": "dlg-content-title" },
        h("h3", { id: "dlg-content-title", class: "callout-title" }, app.t("content.refused")),
        findingsList(app, findings),
      ),
    );
    return true;
  };
  return { node, refuse };
}
