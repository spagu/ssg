// Building blocks every view shares: the page heading, errors in words with
// the worker's code and what to do, loading and empty states, a busy wrapper
// for buttons, a re-renderable slot. Generic — `app` brings t(), has() and
// notify(); no panel-specific import, so another panel can copy it as is.
//
// Strings it asks for: error.title, error.<kind>, error.code, error.detail,
// error.permission, error.rate, error.unexpected, code.<worker code>,
// action.retry, state.loading.

import { h, mount } from "../core/vnode.js";

// pageHead: the view's one <h1>; tabindex=-1 so navigation can move focus to it.
export function pageHead(title, { lead = null, actions = null } = {}) {
  return h(
    "header",
    { class: "page-head" },
    h("div", { class: "page-head-row" }, h("h1", { id: "view-title", tabindex: "-1" }, title), actions ? h("div", { class: "page-actions" }, actions) : null),
    lead ? h("p", { class: "lead" }, lead) : null,
  );
}

// errorText: an API failure in words, with what to do about it. A code the
// strings know (code.<code>) says it precisely; else the kind says it broadly.
export function errorText(app, err) {
  const { t } = app;
  if (!err || !err.kind) return t("error.unexpected", { detail: String(err?.message ?? err) });
  if (err.code && app.has(`code.${err.code}`)) return t(`code.${err.code}`, { detail: err.detail ?? "", permission: err.permission ?? "" });
  if (err.kind === "rate") return t("error.rate", { seconds: err.retryAfter ?? 60 });
  if (err.kind === "permission") return t("error.permission", { permission: err.permission });
  const base = t(`error.${err.kind}`);
  return err.detail && !["auth", "network"].includes(err.kind) ? `${base} ${t("error.detail", { detail: err.detail })}` : base;
}

// errorBlock: a failed load in words, the worker's code, and a Retry button.
export function errorBlock(app, err, retry) {
  return h(
    "div",
    { class: "callout critical", role: "alert" },
    h("p", { class: "callout-title" }, app.t("error.title")),
    h("p", null, errorText(app, err)),
    err?.code ? h("p", { class: "hint" }, app.t("error.code", { code: err.code })) : null,
    retry ? h("button", { type: "button", class: "btn", on: { click: retry } }, app.t("action.retry")) : null,
  );
}

export const loading = (app) => h("p", { class: "loading", role: "status" }, app.t("state.loading"));

// emptyState: what this is and what to do next; the illustration is decorative.
export function emptyState({ img, title, body, action = null }) {
  return h(
    "div",
    { class: "empty" },
    h("img", { src: `img/${img}.svg`, alt: "", width: "160", height: "120", class: "empty-art" }),
    h("div", null, h("h2", { class: "empty-title" }, title), h("p", null, body), action),
  );
}

// btn: a <button type=button>; variant = primary | danger | quiet.
export const btn = (label, onClick, { variant = "", ...rest } = {}) =>
  h("button", { type: "button", class: `btn ${variant ? "btn-" + variant : ""}`.trim(), on: { click: onClick }, ...rest }, label);

// busy runs an async action with its button disabled and aria-busy set,
// reporting failure through app.notify. The action's result, or undefined.
// A promise chain rather than try/finally: the same order of effects, and
// every branch is one a test can take.
export async function busy(app, button, fn) {
  markBusy(button, true);
  const out = await Promise.resolve()
    .then(fn)
    .catch((e) => void app.notify(errorText(app, e), "critical"));
  markBusy(button, false);
  return out;
}

// markBusy: the button (if any) disabled and aria-busy, or back to normal.
function markBusy(button, on) {
  if (!button) return;
  button.disabled = on;
  if (on) button.setAttribute("aria-busy", "true");
  else button.removeAttribute("aria-busy");
}

// slot: a placeholder element a view re-renders on its own (a list, a panel).
export function slot(className = "") {
  const ref = { el: null };
  const node = h("div", { class: className || null, ref: (el) => (ref.el = el) });
  return { node, set: (vnode) => ref.el && mount(ref.el, vnode), get el() { return ref.el; } };
}
