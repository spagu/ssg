// The page's frame: heading, Refresh, the preferences, a loading line and the
// error block. Pure: each returns a tree; handlers come in through `app`.

import { LANGS } from "../i18n.js";
import { THEMES } from "../prefs.js";
import { h } from "../vnode.js";
import { errorKey, loginHref } from "../model.js";

// pageHead: h1, one sentence, and the toolbar. `busy` while asking.
export function pageHead(app, { busy = false } = {}) {
  const { t } = app;
  return h(
    "div",
    { class: "page-head" },
    h("div", null, h("h1", { tabindex: "-1" }, t("app.title")), h("p", { class: "lead" }, t("app.lead"))),
    h(
      "div",
      { class: "toolbar" },
      h(
        "button",
        { type: "button", class: "btn btn-primary", id: "refresh", "aria-busy": busy ? "true" : "false", disabled: busy, on: { click: () => app.refresh() } },
        busy ? t("action.refreshing") : t("action.refresh"),
      ),
      prefs(app),
    ),
  );
}

// prefs: language and theme, each a labelled native select.
export function prefs(app) {
  const { t } = app;
  return h(
    "div",
    { class: "prefs", role: "group", "aria-label": t("pref.title") },
    h("label", { for: "pref-lang" }, t("pref.language")),
    h("select", { id: "pref-lang", on: { change: (e) => app.setLang(e.target.value) } }, LANGS.map((l) => h("option", { value: l, selected: l === app.lang }, t(`pref.lang.${l}`)))),
    h("label", { for: "pref-theme" }, t("pref.theme")),
    h("select", { id: "pref-theme", on: { change: (e) => app.setTheme(e.target.value) } }, THEMES.map((v) => h("option", { value: v, selected: v === app.theme }, t(`pref.theme.${v}`)))),
  );
}

export const loading = (app) => h("p", { class: "loading", role: "status" }, app.t("loading"));

// errorBlock: what went wrong in words, what to do, and Try again. An
// `auth`-mode refusal links to the sign-in page, back to this page after.
export function errorBlock(app, err, retry) {
  const { t } = app;
  const vars = { seconds: err?.retryAfter ?? 60, detail: err?.detail ?? err?.code ?? "" };
  return h(
    "div",
    { class: "callout critical", role: "alert" },
    h("p", { class: "callout-title" }, t("error.title")),
    h("p", null, t(errorKey(err), vars)),
    err?.kind === "signin" ? h("p", null, h("a", { class: "btn btn-primary", href: loginHref(app.config, app.here) }, t("error.signin.go"))) : null,
    h("button", { type: "button", class: "btn", on: { click: retry } }, t("action.retry")),
  );
}
