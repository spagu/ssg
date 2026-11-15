// The editable control for a draft (models/controls.js), recursively: a
// switch, a number or text box, a list, a secret's variable name, and row
// editors for lists of objects and maps. Edits write into the draft and call
// `ctx.edit()`; adding, removing or moving a row calls `ctx.redraw()`. DOM
// glue — what a value means is decided in the model.

import { h } from "../core/vnode.js";
import { addRow, moveRow, removeRow } from "../models/controls.js";
import { humanise } from "../models/form-model.js";

const rows = (text) => String(Math.min(10, Math.max(2, text.split("\n").length + 1)));

// control(app, d, ctx) → vnode. ctx: {id, describedBy, edit(), redraw(), label}.
export function control(app, d, ctx) {
  const { t } = app;
  const aria = ctx.describedBy ? { "aria-describedby": ctx.describedBy } : {};
  const input = (key) => (e) => ((d[key] = e.target.value), ctx.edit());
  switch (d.kind) {
    case "switch":
      return h(
        "span",
        { class: "switch" },
        h("input", { type: "checkbox", role: "switch", id: ctx.id, checked: d.on, indeterminate: d.unset, ...aria, on: { change: (e) => ((d.on = e.target.checked), (d.unset = false), (e.target.nextElementSibling.textContent = t(d.on ? "value.on" : "value.off")), ctx.edit()) } }),
        h("span", { class: "switch-state" }, t(d.unset ? "value.unsetDefault" : d.on ? "value.on" : "value.off")),
      );
    case "number":
      return h("input", { type: "text", inputmode: d.node.type === "int" ? "numeric" : "decimal", id: ctx.id, value: d.text, class: "narrow-input", ...aria, on: { input: input("text") } });
    case "text":
      return h("input", { type: "text", id: ctx.id, value: d.text, ...aria, on: { input: input("text") } });
    case "select":
      return h(
        "select",
        { id: ctx.id, ...aria, on: { change: input("text") } },
        (d.node.enum ?? []).map((v) => h("option", { value: v, selected: v === d.text }, v === "" ? t("value.none") : v)),
        (d.node.enum ?? []).includes(d.text) ? null : h("option", { value: d.text, selected: true }, d.text),
      );
    case "secret":
      return secret(app, d, ctx, aria, input);
    case "lines":
      return h("textarea", { id: ctx.id, rows: rows(d.text), value: d.text, spellcheck: "false", ...aria, on: { input: input("text") } });
    case "rows":
      return rowsEditor(app, d, ctx);
    case "map":
      return mapEditor(app, d, ctx);
    case "fields":
      return fields(app, d, ctx);
    default:
      return h("textarea", { id: ctx.id, class: "code", rows: rows(d.text), value: d.text, spellcheck: "false", ...aria, on: { input: input("text") } });
  }
}

// secret: whether it is set, and the parts of a reference — never a value.
function secret(app, d, ctx, aria, input) {
  const { t } = app;
  const part = (key, label, extra = {}) =>
    h("span", { class: "secret-part" }, h("label", { for: `${ctx.id}-${key}` }, label), h("input", { type: "text", id: `${ctx.id}-${key}`, value: d[key], autocomplete: "off", spellcheck: "false", ...extra, on: { input: input(key) } }));
  return h(
    "div",
    { class: "secret", role: "group", "aria-labelledby": `${ctx.id}-label`, ...aria },
    h("p", { class: "secret-state" }, h("span", { class: `chip ${d.set ? "chip-set" : "chip-unset"}` }, t(d.set ? "secret.set" : "secret.unset")), d.keep ? h("span", { class: "hint" }, " ", t("secret.notReference")) : null),
    d.format === "login" ? part("login", t("secret.login")) : null,
    d.format === "inline" ? part("scheme", t("secret.scheme"), { placeholder: "Bearer" }) : null,
    h("span", { class: "secret-part secret-name" }, h("label", { for: ctx.id }, t("secret.name")), h("span", { class: "prefix", "aria-hidden": "true" }, "$"), h("input", { type: "text", id: ctx.id, value: d.name, autocomplete: "off", spellcheck: "false", pattern: "[A-Za-z_][A-Za-z0-9_]*", on: { input: input("name") } })),
  );
}

// rowTools: move up, move down, remove — each naming its row. After the
// row editor is drawn again, focus goes where the person would look next:
// the same button on the moved row, or the Add button after a removal.
function rowTools(app, d, i, label, ctx) {
  const base = `${ctx.id}-${i}`;
  const act = (fn, focus) => () => (fn(), ctx.edit(), ctx.redraw(focus));
  const tool = (key, text, of, disabled, fn, focus) =>
    h("button", { type: "button", id: `${base}-${key}`, class: "btn btn-quiet", disabled, "aria-label": app.t(of, { row: label }), on: { click: act(fn, focus) } }, app.t(text));
  return h(
    "div",
    { class: "row-tools" },
    tool("up", "rows.up", "rows.upOf", i === 0, () => moveRow(d, i, -1), i - 1 === 0 ? `${ctx.id}-${i - 1}-down` : `${ctx.id}-${i - 1}-up`),
    tool("down", "rows.down", "rows.downOf", i === d.rows.length - 1, () => moveRow(d, i, 1), i + 1 === d.rows.length - 1 ? `${ctx.id}-${i + 1}-up` : `${ctx.id}-${i + 1}-down`),
    tool("remove", "rows.remove", "rows.removeOf", false, () => removeRow(d, i), `${ctx.id}-add`),
  );
}

const addButton = (app, d, ctx, label) =>
  h("button", { type: "button", id: `${ctx.id}-add`, class: "btn", on: { click: () => (addRow(d), ctx.edit(), ctx.redraw(`${ctx.id}-${d.rows.length - 1}-row`)) } }, label);

function rowsEditor(app, d, ctx) {
  return h(
    "div",
    { class: "rows", id: ctx.id, role: "group", "aria-labelledby": `${ctx.id}-label` },
    d.rows.length ? null : h("p", { class: "muted" }, app.t("rows.empty")),
    d.rows.map((r, i) => {
      const label = app.t("rows.item", { n: i + 1 });
      return h("fieldset", { class: "row", id: `${ctx.id}-${i}-row`, tabindex: "-1" }, h("legend", null, label), control(app, r, { ...ctx, id: `${ctx.id}-${i}`, describedBy: null }), rowTools(app, d, i, label, ctx));
    }),
    addButton(app, d, ctx, app.t("rows.add")),
  );
}

function mapEditor(app, d, ctx) {
  return h(
    "div",
    { class: "rows", id: ctx.id, role: "group", "aria-labelledby": `${ctx.id}-label` },
    d.rows.length ? null : h("p", { class: "muted" }, app.t("rows.empty")),
    d.rows.map((r, i) => {
      const label = app.t("rows.item", { n: i + 1 });
      const id = `${ctx.id}-${i}`;
      return h(
        "fieldset",
        { class: "row", id: `${id}-row`, tabindex: "-1" },
        h("legend", null, label),
        h("div", { class: "field" }, h("label", { for: `${id}-key` }, app.t("rows.key")), h("input", { type: "text", id: `${id}-key`, value: r.key, spellcheck: "false", on: { input: (e) => ((r.key = e.target.value), ctx.edit()) } })),
        h("div", { class: "field" }, h("label", { id: `${id}-v-label`, for: `${id}-v` }, app.t("rows.value")), control(app, r.d, { ...ctx, id: `${id}-v`, describedBy: null })),
        rowTools(app, d, i, label, ctx),
      );
    }),
    addButton(app, d, ctx, app.t("rows.addKey")),
  );
}

// fields: an object's fields, each labelled with its key and described in
// English from the schema (lang="en").
function fields(app, d, ctx) {
  return h(
    "div",
    { class: "subfields" },
    Object.entries(d.fields).map(([key, f]) => {
      const node = (d.node.fields ?? []).find((n) => n.key === key);
      const id = `${ctx.id}-${key}`;
      const desc = node?.description ? h("p", { id: `${id}-hint`, class: "hint", lang: "en" }, node.description) : null;
      return h(
        "div",
        { class: "field" },
        h("label", { id: `${id}-label`, for: id }, humanise(key)),
        control(app, f, { ...ctx, id, describedBy: desc ? `${id}-hint` : null }),
        desc,
      );
    }),
  );
}
