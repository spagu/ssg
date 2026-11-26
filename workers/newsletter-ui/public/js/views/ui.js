// Building blocks every view shares: the page heading, errors in words with a
// way out, empty states, status chips, the confirmation dialog, and a busy
// wrapper for buttons. DOM glue — kept thin; the words come from strings/*.

import { h, mount, render } from "../core/vnode.js";

// pageHead: the view's one <h1>; tabindex=-1 so navigation can move focus to it.
export function pageHead(app, title, { lead = null, actions = null, back = null } = {}) {
  return h(
    "header",
    { class: "page-head" },
    back ? h("a", { class: "back", href: back.href }, "← ", back.label) : null,
    h("div", { class: "page-head-row" }, h("h1", { id: "view-title", tabindex: "-1" }, title), actions ? h("div", { class: "page-actions" }, actions) : null),
    lead ? h("p", { class: "lead" }, lead) : null,
  );
}

// errorText: an API failure in words, with what to do about it.
export function errorText(app, err) {
  const { t } = app;
  if (!err || !err.kind) return t("error.unexpected", { detail: String(err && err.message ? err.message : err) });
  if (err.code && app.has(`code.${err.code}`)) return t(`code.${err.code}`, { detail: err.detail ?? "" });
  if (err.kind === "rate") return t("error.rate", { seconds: err.retryAfter ?? 60 });
  if (err.kind === "install") return t("error.install");
  if (err.kind === "permission") return t("error.permission", { permission: err.permission });
  const base = t(`error.${err.kind}`);
  return err.detail && !["auth", "csrf", "network"].includes(err.kind) ? `${base} ${t("error.detail", { detail: err.detail })}` : base;
}

// errorBlock: a failed load, in words, with a Retry button.
export function errorBlock(app, err, retry) {
  return h(
    "div",
    { class: "callout critical", role: "alert" },
    h("p", { class: "callout-title" }, app.t("error.title")),
    h("p", null, errorText(app, err)),
    retry ? h("button", { type: "button", class: "btn", on: { click: retry } }, app.t("action.retry")) : null,
  );
}

// emptyState: what this list is, and what to do next. The illustration is
// decorative (alt="").
export function emptyState({ img, title, body, action = null }) {
  return h(
    "div",
    { class: "empty" },
    h("img", { src: `img/${img}.svg`, alt: "", width: "160", height: "120", class: "empty-art" }),
    h("div", null, h("h2", { class: "empty-title" }, title), h("p", null, body), action),
  );
}

export const chip = (app, status, prefix = "status") => h("span", { class: `chip chip-${status}` }, app.t(`${prefix}.${status}`));

export const loading = (app) => h("p", { class: "loading", role: "status" }, app.t("state.loading"));

// btn: a <button type=button>; variant = primary | danger | quiet.
export const btn = (label, onClick, { variant = "", ...rest } = {}) =>
  h("button", { type: "button", class: `btn ${variant ? "btn-" + variant : ""}`.trim(), on: { click: onClick }, ...rest }, label);

// field: a labelled control; `hint` is wired with aria-describedby.
export function field(id, label, control, hint = null) {
  control.props.id = id;
  if (hint) control.props["aria-describedby"] = `${id}-hint`;
  return h("div", { class: "field" }, h("label", { for: id }, label), control, hint ? h("p", { id: `${id}-hint`, class: "hint" }, hint) : null);
}

// busy runs an async action with its button disabled and aria-busy set,
// reporting failure through app.notify. Returns the action's result, or
// undefined when it failed.
export async function busy(app, button, fn) {
  if (button) {
    button.disabled = true;
    button.setAttribute("aria-busy", "true");
  }
  try {
    return await fn();
  } catch (e) {
    app.notify(errorText(app, e), "critical");
    return undefined;
  } finally {
    if (button) {
      button.disabled = false;
      button.removeAttribute("aria-busy");
    }
  }
}

// confirmDialog: a modal <dialog> naming the object and the consequence.
// Resolves true on confirm, false on cancel or Escape. Focus starts on
// Cancel (the safe choice) unless `focus` names a field of `extra`, and
// returns to the opener afterwards. showModal makes the rest of the page
// inert, so Tab stays inside the dialog.
// `onConfirm` (optional, async) runs the action with the dialog still open
// and its confirm button busy (through `busy`): it answers false to keep the
// dialog open (a refusal it has drawn into `extra`); any other value except
// undefined closes the dialog and becomes the result. A rejection is shown
// with app.notify and keeps the dialog open, so the author can try again.
// Cancel and Escape are ignored while it runs.
export function confirmDialog(app, { title, body, confirm, danger = true, extra = null, focus = null, onConfirm = null }) {
  const opener = document.activeElement;
  return new Promise((resolve) => {
    let dialog;
    const close = (ok) => {
      dialog.close();
      dialog.remove();
      if (opener && opener.focus) opener.focus();
      resolve(ok);
    };
    // While onConfirm runs, Cancel and Escape do nothing: closing then would
    // answer false for an action that may still succeed, and the page would
    // never show its result.
    let pending = false;
    const cancel = (e) => {
      e.preventDefault();
      if (!pending) close(false);
    };
    const settle = (r) => {
      pending = false;
      if (r !== false && r !== undefined) close(r);
    };
    const submit = (e) => {
      e.preventDefault();
      if (pending) return;
      if (!onConfirm) return close(true);
      const button = e.currentTarget.querySelector("button[type=submit]");
      pending = true;
      busy(app, button, onConfirm).then(settle, () => settle(false));
    };
    dialog = render(
      h(
        "dialog",
        { class: "dialog", "aria-labelledby": "dlg-title", "aria-describedby": "dlg-body", on: { cancel } },
        h(
          "form",
          { on: { submit } },
          h("h2", { id: "dlg-title" }, title),
          h("p", { id: "dlg-body" }, body),
          extra,
          h(
            "div",
            { class: "dialog-actions" },
            h("button", { type: "button", class: "btn", autofocus: true, on: { click: cancel } }, app.t("action.cancel")),
            h("button", { type: "submit", class: `btn ${danger ? "btn-danger" : "btn-primary"}` }, confirm),
          ),
        ),
      ),
    );
    document.body.appendChild(dialog);
    dialog.showModal();
    if (focus) document.getElementById(focus)?.focus();
  });
}

// panelDialog: a modal <dialog> holding a small view of its own (the image
// library): a heading, the content and a Close button; Escape closes it too.
// Focus starts on Close and returns to the opener. `id` prefixes the
// heading's id, so it never collides with confirmDialog's, which can open
// on top of it. Returns {close}.
export function panelDialog(app, { id, title, content }) {
  const opener = document.activeElement;
  let dialog;
  const close = () => {
    if (!dialog.open) return;
    dialog.close();
    dialog.remove();
    if (opener && opener.focus) opener.focus();
  };
  dialog = render(
    h(
      "dialog",
      { class: "dialog dialog-wide", "aria-labelledby": `${id}-title`, on: { cancel: (e) => (e.preventDefault(), close()) } },
      h("div", { class: "dialog-head" }, h("h2", { id: `${id}-title`, tabindex: "-1" }, title), h("button", { type: "button", class: "btn btn-quiet", autofocus: true, on: { click: close } }, app.t("action.close"))),
      content,
    ),
  );
  document.body.appendChild(dialog);
  dialog.showModal();
  return { close };
}

// slot: a placeholder element a view re-renders on its own (a list, a panel).
export function slot(className = "") {
  const ref = { el: null };
  const node = h("div", { class: className || null, ref: (el) => (ref.el = el) });
  return { node, set: (vnode) => ref.el && mount(ref.el, vnode), get el() { return ref.el; } };
}

// download saves a URL's response as a file (CSV export, GDPR export).
export function download(url) {
  const a = document.createElement("a");
  a.href = url;
  a.download = "";
  document.body.appendChild(a);
  a.click();
  a.remove();
}
