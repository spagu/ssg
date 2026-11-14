// Building blocks every view shares: the page heading, errors in words with
// a way out, the scrollable table region, status chips, the confirmation
// dialog, and the wrapper that runs an action (busy state, errors, step-up).
// DOM glue — kept thin; the words come from strings/*.

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
  if (err.kind === "permission") return t("error.permission", { permission: err.permission ?? "" });
  if (err.kind === "escalation") return t("error.escalation", { permissions: err.permissions.join(", ") });
  if (err.kind === "rate") return t("error.rate", { seconds: err.retryAfter ?? 60 });
  const base = t(`error.${err.kind}`);
  return err.detail && !["auth", "network"].includes(err.kind) ? `${base} ${t("error.detail", { detail: err.detail })}` : base;
}

// errorBlock: a failed load, in words, with Retry (or Sign in, when the session ended).
export function errorBlock(app, err, retry) {
  const signIn = err && err.kind === "auth";
  return h(
    "div",
    { class: "callout critical", role: "alert" },
    h("p", { class: "callout-title" }, app.t("error.title")),
    h("p", null, errorText(app, err)),
    signIn ? h("a", { class: "btn btn-primary", href: app.loginHref() }, app.t("action.signIn")) : retry ? h("button", { type: "button", class: "btn", on: { click: retry } }, app.t("action.retry")) : null,
  );
}

export const chip = (label, tone = "") => h("span", { class: `chip${tone ? " chip-" + tone : ""}` }, label);
export const loading = (app) => h("p", { class: "loading", role: "status" }, app.t("state.loading"));
export const empty = (text) => h("p", { class: "empty" }, text);

// btn: a <button type=button>; variant = primary | danger | quiet.
export const btn = (label, onClick, { variant = "", ...rest } = {}) =>
  h("button", { type: "button", class: `btn${variant ? " btn-" + variant : ""}`, on: { click: onClick }, ...rest }, label);

// field: a labelled control; `hint` is wired with aria-describedby.
export function field(id, label, control, hint = null) {
  control.props.id = id;
  if (hint) control.props["aria-describedby"] = `${id}-hint`;
  return h("div", { class: "field" }, h("label", { for: id }, label), control, hint ? h("p", { id: `${id}-hint`, class: "hint" }, hint) : null);
}

// tableRegion: a data table that scrolls inside its own focusable region on
// a narrow screen (the page never does), with a visible "scroll sideways"
// hint and the caption as the region's name.
export function tableRegion(app, caption, head, rows, { id = null, className = "" } = {}) {
  return h(
    "div",
    { class: "table-wrap" },
    h("p", { class: "scroll-hint", "aria-hidden": "true" }, app.t("table.scroll")),
    h(
      "div",
      { class: "table-scroll", role: "region", tabindex: "0", "aria-label": caption },
      h("table", { class: `data ${className}`.trim(), id }, h("caption", { class: "sr-only" }, caption), h("thead", null, h("tr", null, head.map((c) => (typeof c === "string" ? h("th", { scope: "col" }, c) : c)))), h("tbody", null, rows)),
    ),
  );
}

// slot: a placeholder a view re-renders on its own.
export function slot(className = "") {
  const ref = { el: null };
  const node = h("div", { class: className || null, ref: (el) => (ref.el = el) });
  return { node, set: (vnode) => ref.el && mount(ref.el, vnode), get el() { return ref.el; } };
}

// confirmDialog: a modal <dialog> naming the object and the consequence.
// Resolves true on confirm, false on Cancel or Escape. showModal makes the
// rest of the page inert, so Tab stays inside; focus starts on Cancel (the
// safe choice) unless `focus` names a field, and returns to the opener.
export function confirmDialog(app, { title, body, confirm, danger = true, extra = null, focus = null }) {
  const opener = document.activeElement;
  return new Promise((resolve) => {
    let dialog = null;
    const close = (ok) => {
      dialog.close();
      dialog.remove();
      if (opener && opener.focus) opener.focus();
      resolve(ok);
    };
    dialog = render(
      h(
        "dialog",
        { class: "dialog", "aria-labelledby": "dlg-title", "aria-describedby": "dlg-body", on: { cancel: (e) => (e.preventDefault(), close(false)) } },
        h(
          "form",
          { on: { submit: (e) => (e.preventDefault(), close(true)) } },
          h("h2", { id: "dlg-title" }, title),
          h("p", { id: "dlg-body" }, body),
          extra,
          h(
            "div",
            { class: "dialog-actions" },
            h("button", { type: "button", class: "btn", autofocus: true, on: { click: () => close(false) } }, app.t("action.cancel")),
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

// act runs an action from a button: disabled and aria-busy while it runs,
// a failure reported in words, and a refusal for want of a recent second
// factor handed to the step-up flow (never retried by itself). Resolves to
// the action's result, or undefined when it did not happen.
export async function act(app, button, fn) {
  if (button) {
    button.disabled = true;
    button.setAttribute("aria-busy", "true");
  }
  try {
    return await fn();
  } catch (e) {
    if (e && e.kind === "stepup") await app.stepUp(e);
    else app.notify(errorText(app, e), "critical");
    return undefined;
  } finally {
    if (button) {
      button.disabled = false;
      button.removeAttribute("aria-busy");
    }
  }
}

// copyButton: copies `value` and says so in the status region.
export function copyButton(app, value, label) {
  return btn(label, () => {
    navigator.clipboard.writeText(value).then(
      () => app.notify(app.t("copy.done")),
      () => app.notify(app.t("copy.failed"), "warning"),
    );
  });
}
