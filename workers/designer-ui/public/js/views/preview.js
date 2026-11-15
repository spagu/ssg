// The look preview in the Appearance tab (GO-188): a page of the site, as it
// is published now, with the colours and fonts of this tab applied — before
// anything is published. The worker fetches the page and strips everything
// that could run (src/preview*.ts); this frame adds its own sandbox without
// allow-scripts or allow-same-origin. Only tokens are previewed: templates
// and settings stay as the site was built, and the panel says so.

import { h } from "../core/vnode.js";
import { isSitePath, previewHref, schemeValues } from "../models/preview-url.js";
import { btn, slot } from "../panel/ui.js";

const DELAY = 800; // ms after the last edit before the frame reloads

// previewSection: the controls and the frame. fields() returns the tokens as
// the tab holds them now ({name, light, dark}); refresh() reloads the frame
// after an edit, once it is shown.
export function previewSection(app, fields) {
  const { t } = app;
  const state = { page: "home", path: "/", scheme: "light", phone: false, shown: false, timer: null };
  const frame = slot("preview-frame-slot");
  const problem = { el: null };
  const radios = {};

  const draw = () => {
    if (!state.shown) return;
    const path = state.path.trim();
    const pathOK = state.page !== "path" || isSitePath(path);
    problem.el.textContent = pathOK ? "" : t("preview.badPath");
    if (!pathOK) return;
    const src = previewHref({ page: state.page, path, scheme: state.scheme, lang: app.lang, values: schemeValues(fields(), state.scheme) });
    // sandbox="" — every restriction: no scripts, an opaque origin, no forms, no popups.
    frame.set(h("iframe", { class: state.phone ? "preview-frame phone" : "preview-frame", sandbox: "", src, title: t("preview.frameTitle"), referrerpolicy: "no-referrer" }));
  };
  const show = () => ((state.shown = true), draw());
  const radio = (value, label) =>
    h("label", { class: "radio" }, h("input", { type: "radio", name: "preview-page", value, checked: state.page === value, ref: (el) => (radios[value] = el), on: { change: () => ((state.page = value), draw()) } }), label);

  const node = h(
    "section",
    { "aria-labelledby": "ap-preview", class: "preview" },
    h("h2", { id: "ap-preview" }, t("preview.title")),
    h("p", { class: "note" }, t("preview.what")),
    h("p", { class: "note" }, t("preview.fonts")),
    h(
      "div",
      { class: "preview-controls" },
      h("fieldset", null, h("legend", null, t("preview.page")), radio("home", t("preview.home")), radio("latest", t("preview.latest")), radio("path", t("preview.path"))),
      h(
        "div",
        { class: "field" },
        h("label", { for: "preview-path" }, t("preview.pathLabel")),
        h("input", { type: "text", id: "preview-path", value: state.path, spellcheck: "false", autocomplete: "off", "aria-describedby": "preview-path-problem", on: { input: (e) => (state.path = e.target.value), change: () => ((state.page = "path"), (radios.path.checked = true), draw()) } }),
        h("span", { id: "preview-path-problem", class: "token-problem", role: "alert", ref: (el) => (problem.el = el) }),
      ),
      h(
        "div",
        { class: "field" },
        h("label", { for: "preview-scheme" }, t("preview.scheme")),
        h("select", { id: "preview-scheme", on: { change: (e) => ((state.scheme = e.target.value), draw()) } }, ["light", "dark"].map((s) => h("option", { value: s, selected: s === state.scheme }, t(`scheme.${s}`)))),
      ),
      h("label", { class: "check" }, h("input", { type: "checkbox", id: "preview-phone", on: { change: (e) => ((state.phone = e.target.checked), draw()) } }), h("span", null, t("preview.phone"))),
      btn(t("preview.show"), show, { variant: "primary", id: "preview-show" }),
    ),
    frame.node,
  );
  return {
    node,
    refresh: () => {
      if (!state.shown) return;
      clearTimeout(state.timer);
      state.timer = setTimeout(draw, DELAY);
    },
  };
}
