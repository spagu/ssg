// One setting in the Options tab: its label and description, where its value
// comes from (default / .ssg.yaml / the panel), the control — or the value
// alone for an account that may not change it — the problems of what was
// typed, and "Revert". Edits go to app.changes (models/settings-changes.js).

import { h, mount } from "../core/vnode.js";
import { editable, needsAdvanced } from "../models/access.js";
import { fromDraft, toDraft } from "../models/controls.js";
import { descriptionOf, labelOf } from "../models/labels.js";
import { keyOf } from "../models/settings-changes.js";
import { control } from "./control.js";
import { valueView } from "./value-view.js";

export const fieldId = (entry) => "f-" + entry.segments.join("--").replace(/[^A-Za-z0-9_-]/g, "_");

// draftOf: the entry's draft, kept across redraws so typing is never lost.
function draftOf(app, entry) {
  const k = keyOf(entry.segments);
  if (!app.drafts.has(k)) {
    const d = toDraft(entry.node, entry.value);
    app.drafts.set(k, d);
    app.baselines.set(k, fromDraft(d).value);
  }
  return app.drafts.get(k);
}

// problemText: a problem of the draft in words, naming the row it is in.
function problemText(app, e) {
  const where = e.at.length ? app.t("problem.at", { at: e.at.map((x) => (typeof x === "number" ? x + 1 : x)).join(" \u203a ") }) : "";
  return `${where}${app.t(`problem.${e.code}`)}`;
}

// sourceChip: where the value comes from now, and whether it is changed here.
function sourceChip(app, entry) {
  const pending = app.changes.get(entry);
  const words = [app.t(`source.${entry.source}`)];
  if (pending) words.push(app.t(pending.value === null ? "source.reverting" : "source.pending"));
  return h("span", { class: `chip chip-source chip-${pending ? "pending" : entry.source}` }, words.join(" \u00b7 "));
}

export function fieldView(app, entry) {
  const id = fieldId(entry);
  const box = { el: null };
  // draw again; then focus the first of `focus` (ids) that exists.
  const draw = (focus) => {
    mount(box.el, body(app, entry, id, draw));
    const el = [focus].flat().map((f) => f && document.getElementById(f)).find(Boolean);
    el?.focus();
  };
  return h("div", { class: "setting", id: `${id}-box`, ref: (el) => ((box.el = el), mount(el, body(app, entry, id, draw))) });
}

function body(app, entry, id, draw) {
  const { t } = app;
  const can = editable(entry, app.caps);
  const desc = descriptionOf(app, entry);
  const pending = app.changes.get(entry);
  const k = keyOf(entry.segments);
  const problems = { el: null };
  const chip = { el: null };
  const tools = { el: null };
  const revertButton = () =>
    can && (app.changes.get(entry) || entry.source === "overlay")
      ? h("button", { type: "button", id: `${id}-revert`, class: "btn btn-quiet", on: { click: revert } }, t(app.changes.get(entry) ? "field.undo" : "field.revert"))
      : null;
  const showProblems = (errors) => problems.el && mount(problems.el, errors.map((e) => h("li", null, problemText(app, e))));
  const edit = () => {
    const d = app.drafts.get(k);
    const r = fromDraft(d);
    showProblems(r.errors);
    if (r.errors.length) {
      app.problems.set(k, r.errors);
      app.changes.discard(entry);
    } else {
      app.problems.delete(k);
      app.changes.set(entry, r.value, app.baselines.get(k));
    }
    mount(chip.el, sourceChip(app, entry));
    mount(tools.el, revertButton());
    app.changed();
  };
  const revert = () => {
    app.drafts.delete(k);
    app.problems.delete(k);
    if (pending || entry.source !== "overlay") app.changes.discard(entry);
    else app.changes.revert(entry);
    app.changed();
    // Focus stays on the button's place: the restored control's Revert, or
    // the control itself when nothing is left to revert.
    draw([`${id}-revert`, id, `${id}-0-row`, `${id}-add`]);
  };
  const reverting = pending && pending.value === null;
  const d = can && !reverting ? draftOf(app, entry) : null;
  const ref = can && pending && pending.value !== null && entry.class === "plain" && !app.caps.advanced && needsAdvanced(pending.value);
  return [
    h(
      "div",
      { class: "setting-head" },
      h("label", { id: `${id}-label`, for: id, class: "setting-label" }, labelOf(app, entry)),
      h("code", { class: "setting-path" }, entry.segments.join(".")),
      h("span", { ref: (el) => (chip.el = el) }, sourceChip(app, entry)),
    ),
    desc ? h("p", { id: `${id}-hint`, class: "hint", lang: desc.lang }, desc.text) : null,
    entry.unread ? h("p", { class: "callout warning" }, t("source.unreadNote", { detail: entry.unread })) : null,
    d ? control(app, d, { id, describedBy: desc ? `${id}-hint` : null, edit, redraw: draw }) : h("div", { class: "setting-value" }, valueView(app, entry.value)),
    reverting ? h("p", { class: "note" }, t("field.revertNote")) : null,
    ref ? h("p", { class: "callout warning" }, t("field.referenceNeedsAdvanced")) : null,
    h("ul", { class: "field-problems", "aria-live": "polite", ref: (el) => ((problems.el = el), mount(el, (app.problems.get(k) ?? []).map((e) => h("li", null, problemText(app, e))))) }),
    h("div", { class: "setting-tools", ref: (el) => (tools.el = el) }, revertButton()),
  ];
}
