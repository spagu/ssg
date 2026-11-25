// The pre-send image check (#446) in the panel: a "Check images" card for a
// campaign that is yet to send, and the same report, shorter, inside the
// Schedule dialog. It runs on demand, never while typing: each run renders
// the mail and makes up to 32 requests to the image addresses, which is
// right for a deliberate check and wrong for every keystroke. The Schedule
// dialog runs it once as it opens — the moment the author commits — and
// fills it in when it arrives, so it never holds up the dialog. No button
// waits for it. With NEWSLETTER_IMAGES_REQUIRE set (#448) the worker refuses
// the schedule itself; the dialog then shows the refused images here.

import { h } from "../core/vnode.js";
import { shortUrl } from "../models/campaign.js";
import { checkLanguage, checkSummary, needsAttention, probeNote, problemKey } from "../models/image-check.js";
import { gateMode, isBlocking, itemProblems, leadNote, refusal, ruleNames } from "../models/image-gate.js";
import { btn, errorText, slot } from "./ui.js";

// words: a note in words; a note naming rules gets them as {rules}.
const words = (app, note) => app.t(note.key, note.rules ? { ...note.vars, rules: ruleNames(app.t, note.rules) } : note.vars);
const problemWords = (code) => (code === "not_probed" ? "imageGate.problem.not_probed" : problemKey(code));

function itemView(app, item) {
  const { t } = app;
  const bad = needsAttention(item);
  const head = isBlocking(item) ? "imageGate.blocks" : bad ? "images.attention" : "images.fine";
  const alt = item.alt?.trim();
  return h(
    "li",
    { class: "image-item" },
    h("p", { class: "image-item-head" }, h("span", { class: `chip ${bad ? "chip-attention" : "chip-fine"}` }, t(head)), " ", h("span", { class: "mono" }, shortUrl(item.url))),
    alt ? h("p", { class: "muted" }, t("images.alt", { alt })) : null,
    // A blocking problem says so in words, not by colour alone.
    bad ? h("ul", { class: "plain" }, itemProblems(item).map((p) => h("li", null, p.blocking ? h("strong", null, t("imageGate.blocking")) : null, p.blocking ? " " : null, t(problemWords(p.code))))) : null,
    h("p", { class: "note" }, words(app, probeNote(item.probe))),
  );
}

// reportView: the summary, then each image (only those with a problem when
// `compact`, for the Schedule dialog). `summary` replaces the sentences
// above the list (a refused schedule says why in its own words).
export function reportView(app, report, { compact = false, summary = null } = {}) {
  const { notes, level } = summary ?? checkSummary(report);
  const shown = compact ? report.items.filter(needsAttention) : report.items;
  return [
    h("div", { class: `callout ${level}` }, notes.map((n) => h("p", null, words(app, n)))),
    shown.length ? h("ul", { class: "image-report", "aria-label": app.t("images.list") }, shown.map((i) => itemView(app, i))) : null,
  ];
}

// runCheck asks the worker and draws the answer (or the failure) into
// `target`; it never rejects, so a caller cannot leave a failure unshown.
export async function runCheck(app, c, target, options = {}) {
  target.set(h("p", { class: "loading" }, app.t("images.checking")));
  try {
    const report = await app.api.get(`campaigns/${encodeURIComponent(c.id)}/images`, { language: checkLanguage(c) });
    target.set(reportView(app, report, options));
  } catch (e) {
    target.set(h("div", { class: "callout warning" }, h("p", null, app.t("images.failed")), h("p", null, errorText(app, e))));
  }
}

// imageCheckCard: the card under the editor or on a scheduled campaign.
// `ready` saves the draft first, so the check reads what is on screen.
// `gate` is settings.imageRules (#448) for the lead; null when the settings
// could not be read. Without it — a campaign already scheduled, where no rule
// can stop anything any more — the lead is the warning-only one.
export function imageCheckCard(app, getCampaign, { ready = async () => true, gate = { rules: [] } } = {}) {
  const { t } = app;
  const result = slot("image-check-result");
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
    { class: "card", "aria-labelledby": "imgchk-title" },
    h("h2", { id: "imgchk-title" }, t("images.title")),
    h("p", { class: "note" }, words(app, leadNote(gate))),
    btn(t("images.check"), (e) => {
      check(e.currentTarget).catch((err) => result.set(h("p", { class: "bad" }, errorText(app, err))));
    }),
    h("div", { role: "status", "aria-live": "polite", class: "image-check-live" }, result.node),
  );
}

// dialogCheck: the block the Schedule dialog shows; it starts the check.
// refuse(err) draws a refused schedule (409 image_check_failed) into the
// same live region, announced there; a check still running never overwrites it.
export function dialogCheck(app, c) {
  const slotted = slot("image-check-result");
  let refused = false;
  const result = { node: slotted.node, set: (v) => (refused ? undefined : slotted.set(v)) };
  const node = h("section", { class: "dialog-images", "aria-labelledby": "dlg-img-title" }, h("h3", { id: "dlg-img-title" }, app.t("images.title")), h("div", { role: "status", "aria-live": "polite" }, result.node));
  // The slot's element exists once the dialog renders; start right after.
  const start = () => {
    runCheck(app, c, result, { compact: true }).catch((e) => result.set(h("p", { class: "bad" }, errorText(app, e))));
  };
  // refuse: true when `err` is a 409 image_check_failed, now drawn here.
  const refuse = (err) => {
    const report = refusal(err);
    if (!report) return false;
    refused = true;
    slotted.set(reportView(app, report, { compact: true, summary: { notes: [report.note], level: "critical" } }));
    return true;
  };
  return { node, start, refuse };
}

// imageRulesFact: the Settings view's line for NEWSLETTER_IMAGES_REQUIRE.
export function imageRulesFact(app, info) {
  const mode = gateMode(info ?? { rules: [] });
  if (mode === "broken") return h("span", { class: "flag off bad" }, app.t("imageGate.setting.broken", { detail: info.problem }));
  return mode === "on" ? h("strong", null, ruleNames(app.t, info.rules)) : h("span", { class: "flag off" }, app.t("imageGate.setting.off"));
}
