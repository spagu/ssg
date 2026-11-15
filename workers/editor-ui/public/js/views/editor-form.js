// The article's details: the frontmatter's simple keys as a form, and the
// whole frontmatter as text for everything else (README of the round,
// decision 9). The two exclude each other — the worker takes one or the
// other — so while one has changes the other is locked, with a sentence
// saying why and how to unlock it.

import { h } from "../core/vnode.js";
import { FORM } from "../models/fm-form.js";
import { formChanges, formProblems, formUsable, rawUsable, withRaw } from "../models/article-doc.js";
import { slot } from "../panel/ui.js";

const field = (app, doc, f, problem, set) => {
  const { t } = app;
  const id = `fm-${f.key}`;
  const fixed = doc.fixed.includes(f.key);
  const locked = fixed || doc.rawEdited;
  const hint = fixed ? t("form.fixed") : t(`form.${f.key}Hint`);
  const describedBy = `${id}-hint${problem ? ` ${id}-problem` : ""}`;
  const common = { id, disabled: locked, "aria-describedby": describedBy, "aria-invalid": problem ? "true" : null };
  let control;
  if (f.kind === "status") {
    return h("div", { class: "check" },
      h("input", { ...common, type: "checkbox", checked: doc.inputs[f.key] === true, on: { change: (e) => set(f.key, e.target.checked) } }),
      h("label", { for: id }, t(`form.${f.key}`)),
      h("p", { id: `${id}-hint`, class: "hint" }, hint));
  }
  if (f.kind === "textarea") control = h("textarea", { ...common, rows: "2", value: doc.inputs[f.key], on: { input: (e) => set(f.key, e.target.value) } });
  else control = h("input", { ...common, type: "text", value: doc.inputs[f.key], autocomplete: "off", on: { input: (e) => set(f.key, e.target.value) } });
  return h("div", { class: `field fm-${f.kind}` },
    h("label", { for: id }, t(`form.${f.key}`)), control,
    h("p", { id: `${id}-hint`, class: "hint" }, hint),
    problem ? h("p", { id: `${id}-problem`, class: "bad" }, t(problem)) : null);
};

// detailsForm(app, ed): ed.doc is read, ed.update(doc) writes.
export function detailsForm(app, ed) {
  const { t } = app;
  const box = slot("fm");
  const draw = () => {
    const doc = ed.doc;
    const problems = formProblems(doc);
    // set: one input changed. The text box locks without a redraw, so the
    // focus stays in the field being typed into.
    const set = (key, value) => (ed.update({ ...ed.doc, inputs: { ...ed.doc.inputs, [key]: value } }), lockRaw());
    const lockRaw = () => {
      const raw = document.getElementById("fm-raw");
      if (raw) raw.disabled = Object.keys(formChanges(ed.doc)).length > 0;
    };
    const formLocked = Object.keys(formChanges(doc)).length > 0;
    box.set([
      formUsable(doc)
        ? h("div", { class: "fm-grid" }, FORM.map((f) => field(app, doc, f, problems[f.key], set)))
        : h("p", { class: "callout warning" }, t(`form.unusable.${doc.format}`)),
      doc.rawEdited ? h("p", { class: "callout", role: "status" }, t("form.lockedByRaw"), " ", h("button", { type: "button", class: "btn btn-quiet", on: { click: () => (ed.update(withRaw(ed.doc, ed.doc.originalRaw)), draw()) } }, t("form.undoRaw"))) : null,
      rawUsable(doc)
        ? h("details", { class: "fm-raw", open: doc.rawEdited },
            h("summary", null, t("form.rawTitle")),
            h("label", { for: "fm-raw" }, t("form.raw")),
            h("textarea", { id: "fm-raw", class: "code", rows: "6", disabled: formLocked, "aria-describedby": "fm-raw-hint", value: doc.raw, on: { input: (e) => { const was = ed.doc.rawEdited; ed.update(withRaw(ed.doc, e.target.value)); if (was !== ed.doc.rawEdited) draw(); } } }),
            h("p", { id: "fm-raw-hint", class: "hint" }, t(formLocked ? "form.rawLocked" : doc.format === "none" ? "form.rawNone" : "form.rawHint")))
        : null,
    ]);
  };
  ed.redrawForm = draw;
  queueMicrotask(draw);
  return h("section", { class: "card fm-card", "aria-labelledby": "fm-heading" },
    h("h2", { id: "fm-heading" }, t("form.heading")),
    ed.doc.bom ? h("p", { class: "callout warning" }, t("form.bom")) : null,
    box.node);
}
