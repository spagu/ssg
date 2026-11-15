// The forms of the Insert list (GO-174). A component's form is generated
// from its schema (components.json): one field per prop — a checkbox for a
// bool, a list for an enum, text for the rest — with "required", the
// default and the description shown, and the call it will insert written
// out live under the fields. The syntax rules are in models/widget-call.js;
// a value the syntax cannot carry is marked on its field, never altered.
// The built-in blocks that need an answer (table size, code language,
// footnote text, video address) get a one- or two-field form here too.

import { h } from "../core/vnode.js";
import { componentCall, defaultText, initialValues } from "../models/widget-call.js";
import { LANG, MAX_COLS, MAX_ROWS, embed, embedUrl, fenced, footnote, table } from "../models/widget-blocks.js";
import { openDialog } from "../panel/dialog.js";

const fieldId = (name) => `wf-${name}`;

// mark shows (or clears) a field's problem and sets aria-invalid.
function mark(name, message) {
  const input = document.getElementById(fieldId(name));
  const problem = document.getElementById(`${fieldId(name)}-problem`);
  if (problem) problem.textContent = message ?? "";
  if (input) input.setAttribute("aria-invalid", message ? "true" : "false");
}

function propField(app, prop, values, update) {
  const { t } = app;
  const id = fieldId(prop.name);
  const notes = [prop.description, prop.type === "array" ? t("insert.arrayHint") : null, prop.required ? t("insert.requiredHint") : null, defaultText(prop) ? t("insert.default", { value: defaultText(prop) }) : null].filter(Boolean).join(" ");
  const described = `${id}-hint ${id}-problem`;
  const after = [h("p", { id: `${id}-hint`, class: "hint" }, notes), h("p", { id: `${id}-problem`, class: "form-problem", role: "alert" })];
  const label = h("label", { for: id }, prop.name, prop.required ? h("span", { "aria-hidden": "true" }, " *") : null);
  if (prop.type === "bool") {
    return h("div", { class: "check" }, h("input", { id, type: "checkbox", checked: values[prop.name], "aria-describedby": described, on: { change: (e) => update(prop.name, e.target.checked) } }), label, after);
  }
  if (prop.enum) {
    const blank = !prop.required && !defaultText(prop) ? [h("option", { value: "" }, t("insert.notSet"))] : [];
    const options = prop.enum.map((v) => h("option", { value: v, selected: v === values[prop.name] }, v));
    return h("div", { class: "field" }, label, h("select", { id, required: prop.required, "aria-describedby": described, on: { change: (e) => update(prop.name, e.target.value) } }, blank, options), after);
  }
  const extra = prop.type === "number" ? { inputmode: "decimal" } : {};
  return h("div", { class: "field" }, label, h("input", { id, type: "text", value: values[prop.name], required: prop.required, "aria-describedby": described, ...extra, on: { input: (e) => update(prop.name, e.target.value) } }), after);
}

// componentForm: the call to insert, or null (cancelled).
export async function componentForm(app, item) {
  const { t } = app;
  const values = initialValues(item);
  const out = { el: null };
  const show = () => {
    const r = componentCall(item, values);
    if (out.el) out.el.textContent = r.text ?? t("insert.fixFields");
    return r;
  };
  const update = (name, value) => {
    values[name] = value;
    mark(name, null);
    show();
  };
  const check = () => {
    const r = show();
    if (r.text) return true;
    for (const prop of item.props) mark(prop.name, r.errors[prop.name] ? t(`insert.error.${r.errors[prop.name]}`) : null);
    document.getElementById(fieldId(Object.keys(r.errors)[0]))?.focus();
    return false;
  };
  const pending = openDialog({
    title: t("insert.componentTitle", { name: item.name }),
    wide: true,
    content: [
      item.description ? h("p", null, item.description) : null,
      h("div", { class: "dialog-fields" }, item.props.map((p) => propField(app, p, values, update))),
      h("p", { class: "md-label" }, t("insert.callPreview")),
      h("pre", { class: "insert-call", "aria-live": "polite" }, h("code", { ref: (el) => (out.el = el) })),
    ],
    actions: [{ label: t("action.cancel"), value: null }, { label: t("insert.insert"), value: "go", variant: "primary", submit: true, check }],
  });
  show();
  return (await pending) ? componentCall(item, values).text : null;
}

// FORMS: the blocks that ask something first — their fields, what each
// field must be, and the text command built from the answers.
const FORMS = {
  table: (t) => ({
    fields: [
      { name: "rows", label: t("insert.rows"), value: "2", inputmode: "numeric", check: (v) => between(v, MAX_ROWS) || t("insert.error.range", { max: MAX_ROWS }) },
      { name: "cols", label: t("insert.cols"), value: "2", inputmode: "numeric", check: (v) => between(v, MAX_COLS) || t("insert.error.range", { max: MAX_COLS }) },
    ],
    build: (v) => (text, sel) => table(text, sel, Number(v.rows), Number(v.cols), { column: (n) => t("insert.column", { n }), cell: t("ph.cell") }),
  }),
  code: (t) => ({
    fields: [{ name: "lang", label: t("insert.lang"), hint: t("insert.langHint"), value: "", check: (v) => LANG.test(v.trim()) || t("insert.error.lang") }],
    build: (v) => (text, sel) => fenced(text, sel, v.lang.trim(), t("insert.sample.code")),
  }),
  footnote: (t) => ({
    fields: [{ name: "note", label: t("insert.note"), hint: t("insert.noteHint"), value: "", check: (v) => !!v.trim() || t("insert.error.required") }],
    build: (v) => (text, sel) => footnote(text, sel, v.note),
  }),
  embed: (t) => ({
    fields: [{ name: "url", label: t("insert.url"), hint: t("insert.urlHint"), value: "", type: "url", check: (v) => !!embedUrl(v) || t("insert.error.youtube") }],
    build: (v) => (text, sel) => embed(text, sel, embedUrl(v.url)),
  }),
};
const between = (v, max) => /^\d{1,2}$/.test(v.trim()) && Number(v) >= 1 && Number(v) <= max;

// blockForm: the text command for a block, or null (cancelled).
export async function blockForm(app, name) {
  const { t } = app;
  const form = FORMS[name](t);
  const values = Object.fromEntries(form.fields.map((f) => [f.name, f.value]));
  const check = () => {
    const bad = form.fields.map((f) => [f.name, f.check(values[f.name])]).filter(([, r]) => r !== true);
    for (const f of form.fields) mark(f.name, null);
    for (const [n, message] of bad) mark(n, message);
    if (bad.length) document.getElementById(fieldId(bad[0][0]))?.focus();
    return !bad.length;
  };
  const ok = await openDialog({
    title: t(`insert.block.${name}`),
    content: h("div", { class: "dialog-fields" }, form.fields.map((f) =>
      h("div", { class: "field" }, h("label", { for: fieldId(f.name) }, f.label),
        h("input", { id: fieldId(f.name), type: f.type ?? "text", value: f.value, inputmode: f.inputmode ?? null, autofocus: f === form.fields[0], "aria-describedby": `${fieldId(f.name)}-hint ${fieldId(f.name)}-problem`, on: { input: (e) => (values[f.name] = e.target.value) } }),
        h("p", { id: `${fieldId(f.name)}-hint`, class: "hint" }, f.hint ?? ""),
        h("p", { id: `${fieldId(f.name)}-problem`, class: "form-problem", role: "alert" })))),
    actions: [{ label: t("action.cancel"), value: null }, { label: t("insert.insert"), value: "go", variant: "primary", submit: true, check }],
  });
  return ok ? form.build(values) : null;
}
