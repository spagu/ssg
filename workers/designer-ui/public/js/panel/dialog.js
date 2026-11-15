// A modal <dialog>: a title, any content, and buttons that close it with a
// value. showModal makes the rest of the page inert, so Tab stays inside;
// Escape closes it with null; focus returns to the opener afterwards.
// Generic — no panel-specific import.
//
//   const choice = await openDialog({title, content, actions: [
//     {label: "Cancel", value: null},
//     {label: "Publish", value: "go", variant: "primary", submit: true, check: () => ok}]});
//
// A `submit` action submits the form (Enter in a field does too); its
// `check()` may return false to keep the dialog open (a field it marked).

import { h, render, uid } from "../core/vnode.js";

export function openDialog({ title, content = null, actions, wide = false, labelledBy = null }) {
  const opener = document.activeElement;
  const id = uid("dlg");
  return new Promise((resolve) => {
    let dialog;
    const close = (value) => {
      dialog.close();
      dialog.remove();
      if (opener && opener.focus && opener.isConnected) opener.focus();
      resolve(value);
    };
    const submit = actions.find((a) => a.submit);
    const onSubmit = (e) => {
      e.preventDefault();
      if (submit && (!submit.check || submit.check())) close(submit.value);
    };
    dialog = render(
      h(
        "dialog",
        { class: wide ? "dialog dialog-wide" : "dialog", "aria-labelledby": labelledBy ?? `${id}-title`, on: { cancel: (e) => (e.preventDefault(), close(null)) } },
        h(
          "form",
          { on: { submit: onSubmit }, novalidate: true },
          h("h2", { id: `${id}-title` }, title),
          content,
          h(
            "div",
            { class: "dialog-actions" },
            actions.map((a, i) =>
              h(
                "button",
                { type: a.submit ? "submit" : "button", class: `btn ${a.variant ? "btn-" + a.variant : ""}`.trim(), autofocus: i === 0, on: a.submit ? {} : { click: () => close(a.value) } },
                a.label,
              ),
            ),
          ),
        ),
      ),
    );
    document.body.appendChild(dialog);
    dialog.showModal();
  });
}
