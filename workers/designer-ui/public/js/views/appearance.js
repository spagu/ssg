// The Appearance tab: the colours and fonts the theme declares in its tokens
// file (README, "Theme tokens"), as fields, and the WCAG 2.2 contrast of
// every text/background pair, live while editing. A failing pair is shown
// with the nearest colour that passes; it never blocks — publishing asks to
// confirm it. Edits change only the values in the file's text (comments and
// order stay) and are published with the theme's files (designer-ui:theme).

import { h, mount } from "../core/vnode.js";
import { parseHex } from "../models/contrast.js";
import { evaluate, pairsOf } from "../models/pairs.js";
import { findTokensFile, resolve, setToken, tokens, valueMap } from "../models/tokens-css.js";
import { emptyState, pageHead, slot } from "../panel/ui.js";
import { contrastTable } from "./contrast-table.js";
import { previewSection } from "./preview.js";
import { themeUnavailable } from "./theme-unavailable.js";

const SAMPLE = "Aa Za\u017c\u00f3\u0142\u0107 g\u0119\u015bl\u0105 ja\u017a\u0144 \u2014 \u0928\u092e\u0938\u094d\u0924\u0947 \u2014 0123";

export default async function appearance(app, view) {
  const { t } = app;
  const theme = await app.loadTheme();
  if (!theme.available) return mount(view, [pageHead(t("appearance.title")), themeUnavailable(app, theme)]);
  const path = findTokensFile(theme.files.map((f) => f.path), theme.dir);
  if (!path) {
    return mount(view, [pageHead(t("appearance.title")), emptyState({ img: "no-tokens", title: t("appearance.noTokensTitle"), body: t("appearance.noTokens", { dir: theme.dir }) })]);
  }
  const file = theme.files.find((f) => f.path === path);
  const original = (await app.readFile(path)).text;
  const canEdit = app.caps.theme;
  const text = () => app.themeDrafts.get(path)?.text ?? original;
  const fields = tokens(original);
  const checks = slot("contrast");
  const preview = previewSection(app, () => tokens(text()));

  const update = (next) => {
    app.themeDrafts.setText(file, original, next);
    app.changed();
    drawChecks();
    preview.refresh();
  };
  const setValue = (name, scheme, value) => {
    const next = setToken(text(), name, scheme, value);
    if (next !== null) update(next);
  };
  function drawChecks() {
    const all = valueMap(text());
    const names = [...all.keys()].filter((n) => resolve(all, n, "light") || resolve(all, n, "dark"));
    const results = evaluate(pairsOf(names), all);
    checks.set(contrastTable(app, results, { canEdit, onUse: (r) => useSuggestion(r) }));
  }
  function useSuggestion(r) {
    setValue(r.fg, r.scheme, r.suggestion.colour);
    for (const id of [`tok-${r.fg}-${r.scheme}`, `tok-${r.fg}-${r.scheme}-hex`]) {
      const el = document.getElementById(id);
      if (el) el.value = r.suggestion.colour;
    }
    app.notify(t("contrast.used", { colour: r.suggestion.colour, token: r.fg }));
  }

  const current = valueMap(text());
  const colours = fields.filter((f) => f.kind === "colour" && !/palette/i.test(f.name));
  const ramps = fields.filter((f) => f.kind === "colour" && /palette/i.test(f.name));
  const fonts = fields.filter((f) => f.kind === "font");
  const colourRow = (f) =>
    h("tr", null, h("th", { scope: "row" }, h("code", null, f.name)), ["light", "dark"].map((scheme) => h("td", { "data-label": t(`scheme.${scheme}`) }, colourCell(app, f, scheme, current.get(f.name)?.[scheme] ?? null, canEdit, setValue))));
  const colourTable = (list, caption) =>
    h("div", { class: "table-scroll" }, h("table", { class: "data list tokens" }, h("caption", null, caption), h("thead", null, h("tr", null, h("th", { scope: "col" }, t("appearance.token")), h("th", { scope: "col" }, t("scheme.light")), h("th", { scope: "col" }, t("scheme.dark")))), h("tbody", null, list.map(colourRow))));

  mount(view, [
    pageHead(t("appearance.title"), { lead: t("appearance.lead", { path }) }),
    canEdit ? null : h("p", { class: "callout" }, t("appearance.readOnly")),
    h("section", { "aria-labelledby": "ap-contrast" }, h("h2", { id: "ap-contrast" }, t("appearance.contrast")), h("p", { class: "note" }, t("appearance.contrastHow")), checks.node),
    h("section", { "aria-labelledby": "ap-colours" }, h("h2", { id: "ap-colours" }, t("appearance.colours")), colours.length ? colourTable(colours, t("appearance.coloursCaption")) : h("p", { class: "muted" }, t("appearance.noColours"))),
    fonts.length ? h("section", { "aria-labelledby": "ap-fonts" }, h("h2", { id: "ap-fonts" }, t("appearance.fonts")), fonts.map((f) => fontField(app, f, current.get(f.name).light, canEdit, setValue))) : null,
    ramps.length ? h("details", { class: "card" }, h("summary", null, t("appearance.ramps", { count: ramps.length })), h("p", { class: "note" }, t("appearance.rampsWhy")), colourTable(ramps, t("appearance.rampsCaption"))) : null,
    preview.node,
  ]);
  drawChecks();
}

// colourCell: a colour picker and its hex text, kept in step; a value that is
// not a plain colour (var(), a name) or not restated for dark is shown, not edited.
function colourCell(app, f, scheme, value, canEdit, setValue) {
  const { t } = app;
  if (value === null) return h("span", { class: "muted" }, t("appearance.sameAsLight"));
  const hex = parseHex(value) ? value : null;
  const swatch = hex ? h("span", { class: "swatch", style: { "background-color": value }, "aria-hidden": "true" }) : null;
  if (!canEdit || !hex) return h("span", { class: "token-value" }, swatch, h("code", null, value));
  const id = `tok-${f.name}-${scheme}`;
  const name = t("appearance.colourOf", { token: f.name, scheme: t(`scheme.${scheme}`) });
  const problem = { el: null };
  const full = (v) => (v.length === 4 ? "#" + [...v.slice(1)].map((c) => c + c).join("") : v).toLowerCase();
  return h(
    "span",
    { class: "token-edit" },
    h("input", { type: "color", id, value: full(hex), "aria-label": name, on: { input: (e) => ((document.getElementById(`${id}-hex`).value = e.target.value), setValue(f.name, scheme, e.target.value)) } }),
    h("input", {
      type: "text", id: `${id}-hex`, value: hex, class: "hex-input", "aria-label": t("appearance.hexOf", { token: f.name, scheme: t(`scheme.${scheme}`) }), spellcheck: "false", maxlength: "7",
      on: {
        input: (e) => {
          const v = e.target.value.trim();
          const ok = Boolean(parseHex(v));
          problem.el.textContent = ok || !v ? "" : t("appearance.notHex");
          if (ok) ((document.getElementById(id).value = full(v)), setValue(f.name, scheme, v.toLowerCase()));
        },
      },
    }),
    h("span", { class: "token-problem", role: "alert", ref: (el) => (problem.el = el) }),
  );
}

// fontField: a font stack and a sample drawn in it (through the CSSOM).
function fontField(app, f, value, canEdit, setValue) {
  const { t } = app;
  const id = `tok-${f.name}`;
  const sample = { el: null };
  return h(
    "div",
    { class: "field font-field" },
    h("label", { for: id }, h("code", null, f.name)),
    canEdit
      ? h("input", { type: "text", id, value: value.replace(/\s+/g, " "), spellcheck: "false", "aria-describedby": `${id}-sample`, on: { input: (e) => (sample.el.style.setProperty("font-family", e.target.value), setValue(f.name, "light", e.target.value)) } })
      : h("code", { id }, value),
    h("p", { id: `${id}-sample`, class: "font-sample", style: { "font-family": value }, ref: (el) => (sample.el = el) }, h("span", { class: "visually-hidden" }, t("appearance.sample"), " "), SAMPLE),
  );
}
