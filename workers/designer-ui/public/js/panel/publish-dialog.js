// The publish dialog: one list of everything a publication changes, in the
// person's language ("Site name: Example → My kitchen"), the commit message
// (prefilled, editable), who it will be signed as, and any warning the person
// must confirm before it goes. Generic — the caller turns its own changes
// into lines; the words come from `t`; no panel-specific import.
//
//   publishDialog(t, {lines: [{label, from, to, note}], files: [{path, text}],
//     message, author, confirm: [{id, text}]}) → {message} | null
//
// Strings: publish.title, publish.settings, publish.files, publish.message,
// publish.messageHint, publish.messageEmpty, publish.author, publish.confirm,
// publish.confirmMissing, publish.go, action.cancel.

import { h, uid } from "../core/vnode.js";
import { openDialog } from "./dialog.js";

export async function publishDialog(t, { lines = [], files = [], message = "", author = "", confirm = [] }) {
  const id = uid("pub");
  const state = { message, checked: new Set() };
  const problem = { el: null };
  const say = (text) => problem.el && (problem.el.textContent = text);
  const check = () => {
    if (!state.message.trim()) return say(t("publish.messageEmpty")), document.getElementById(`${id}-msg`)?.focus(), false;
    const missing = confirm.find((c) => !state.checked.has(c.id));
    if (missing) return say(t("publish.confirmMissing")), document.getElementById(`${id}-c-${missing.id}`)?.focus(), false;
    return true;
  };
  const content = [
    lines.length ? h("h3", null, t("publish.settings")) : null,
    lines.length
      ? h("ul", { class: "change-list" }, lines.map((l) => h("li", null, h("strong", null, l.label), ": ", l.from, " \u2192 ", l.to, l.note ? h("span", { class: "hint" }, " \u2014 ", l.note) : null)))
      : null,
    files.length ? h("h3", null, t("publish.files")) : null,
    files.length ? h("ul", { class: "change-list" }, files.map((f) => h("li", null, h("code", null, f.path), " \u2014 ", f.text))) : null,
    confirm.length
      ? h(
          "fieldset",
          { class: "confirm-list" },
          h("legend", null, t("publish.confirm")),
          confirm.map((c) =>
            h(
              "div",
              { class: "check" },
              h("input", { type: "checkbox", id: `${id}-c-${c.id}`, on: { change: (e) => (e.target.checked ? state.checked.add(c.id) : state.checked.delete(c.id)) } }),
              h("label", { for: `${id}-c-${c.id}` }, c.text),
            ),
          ),
        )
      : null,
    h(
      "div",
      { class: "field" },
      h("label", { for: `${id}-msg` }, t("publish.message")),
      h("textarea", { id: `${id}-msg`, rows: "3", "aria-describedby": `${id}-msg-hint`, value: message, on: { input: (e) => (state.message = e.target.value) } }),
      h("p", { id: `${id}-msg-hint`, class: "hint" }, t("publish.messageHint")),
    ),
    author ? h("p", { class: "muted" }, t("publish.author", { author })) : null,
    h("p", { class: "form-problem", role: "alert", ref: (el) => (problem.el = el) }),
  ];
  const go = await openDialog({
    title: t("publish.title"),
    content,
    wide: true,
    actions: [
      { label: t("action.cancel"), value: null },
      { label: t("publish.go"), value: "go", variant: "primary", submit: true, check },
    ],
  });
  return go ? { message: state.message.trim() } : null;
}
