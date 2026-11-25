// The pre-send image check (#446) in the panel: a "Check images" card for a
// campaign that is yet to send, and the same report, shorter, inside the
// Schedule dialog. It runs on demand, never while typing: each run renders
// the mail and makes up to 32 requests to the image addresses, which is
// right for a deliberate check and wrong for every keystroke. The Schedule
// dialog runs it once as it opens — the moment the author commits — and
// fills it in when it arrives, so it never holds up the dialog. It warns
// only: no button waits for it and nothing is disabled by what it finds.

import { h } from "../core/vnode.js";
import { shortUrl } from "../models/campaign.js";
import { checkLanguage, checkSummary, probeNote, problemKey } from "../models/image-check.js";
import { btn, errorText, slot } from "./ui.js";

const words = (app, note) => app.t(note.key, note.vars);

function itemView(app, item) {
  const { t } = app;
  const bad = item.problems.length > 0;
  const alt = item.alt?.trim();
  return h(
    "li",
    { class: "image-item" },
    h("p", { class: "image-item-head" }, h("span", { class: `chip ${bad ? "chip-attention" : "chip-fine"}` }, t(bad ? "images.attention" : "images.fine")), " ", h("span", { class: "mono" }, shortUrl(item.url))),
    alt ? h("p", { class: "muted" }, t("images.alt", { alt })) : null,
    bad ? h("ul", { class: "plain" }, item.problems.map((p) => h("li", null, t(problemKey(p))))) : null,
    h("p", { class: "note" }, words(app, probeNote(item.probe))),
  );
}

// reportView: the summary, then each image (only those with a problem when
// `compact`, for the Schedule dialog).
export function reportView(app, report, { compact = false } = {}) {
  const { notes, level } = checkSummary(report);
  const shown = compact ? report.items.filter((i) => i.problems.length) : report.items;
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
export function imageCheckCard(app, getCampaign, { ready = async () => true } = {}) {
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
    h("p", { class: "note" }, t("images.lead")),
    btn(t("images.check"), (e) => {
      check(e.currentTarget).catch((err) => result.set(h("p", { class: "bad" }, errorText(app, err))));
    }),
    h("div", { role: "status", "aria-live": "polite", class: "image-check-live" }, result.node),
  );
}

// dialogCheck: the block the Schedule dialog shows; it starts the check.
export function dialogCheck(app, c) {
  const result = slot("image-check-result");
  const node = h("section", { class: "dialog-images", "aria-labelledby": "dlg-img-title" }, h("h3", { id: "dlg-img-title" }, app.t("images.title")), h("div", { role: "status", "aria-live": "polite" }, result.node));
  // The slot's element exists once the dialog renders; start right after.
  const start = () => {
    runCheck(app, c, result, { compact: true }).catch((e) => result.set(h("p", { class: "bad" }, errorText(app, e))));
  };
  return { node, start };
}
