// "Appearance" (GO-152, /core/#appearance): the panels' look as the last
// build computed it from admin_theme in .ssg.yaml — read-only. The palette
// of both schemes with every contrast ratio, each adjustment in words,
// sample components in the scheme shown now, the fonts and the logo.
//
// Colours are drawn as SVG fill attributes (allowed by the page's strict
// CSP, unlike style attributes), always next to their value as text; the
// swatches themselves are decorative.

import { h } from "../vnode.js";
import { prefs } from "./shell.js";
import { SCHEMES, adjustmentText, brandColours, logos, rows, tokenLabel } from "../appearance.js";
import { sampleChart } from "./appearance-chart.js";

const swatch = (fill) => h("svg", { class: "swatch", viewBox: "0 0 40 24", "aria-hidden": "true", focusable: "false" }, h("rect", { x: 0.5, y: 0.5, width: 39, height: 23, rx: 4, fill }));

// pairSwatch: the pair itself — "Aa" in the foreground on the background for
// text, a ring on the background for a mark or an edge.
function pairSwatch(r) {
  const mark = r.kind === "text"
    ? h("text", { x: 28, y: 21, "text-anchor": "middle", fill: r.fgHex, "font-weight": "700", "font-size": "15" }, "Aa")
    : h("rect", { x: 14, y: 8, width: 28, height: 16, rx: 3, fill: "none", stroke: r.fgHex, "stroke-width": 3 });
  return h("svg", { class: "swatch pair", viewBox: "0 0 56 32", "aria-hidden": "true", focusable: "false" }, h("rect", { x: 0.5, y: 0.5, width: 55, height: 31, rx: 4, fill: r.bgHex }), mark);
}

function head(app) {
  const { t } = app;
  return h(
    "div",
    { class: "page-head" },
    h("div", null, h("p", { class: "back" }, h("a", { href: "#", id: "appearance-back" }, t("appearance.back"))), h("h1", { tabindex: "-1", id: "appearance-title" }, t("appearance.title")), h("p", { class: "lead" }, t("appearance.lead"))),
    h("div", { class: "toolbar" }, prefs(app)),
  );
}

function brand(app, theme) {
  const { t } = app;
  const name = theme.name ?? t("appearance.name.default");
  const shown = logos(theme);
  return h(
    "section",
    { class: "card", "aria-labelledby": "ap-brand" },
    h("h2", { id: "ap-brand" }, t("appearance.brand")),
    h("p", null, t("appearance.name", { name })),
    h("ul", { class: "plain colours" }, brandColours(theme).map((c) => h("li", null, swatch(c.hex), " ", h("strong", null, t(`appearance.${c.role}`)), ": ", h("code", null, c.hex), c.derived ? [" — ", h("span", { class: "muted" }, t("appearance.derived"))] : null))),
    h("h3", null, t("appearance.logo")),
    shown.length
      ? h("ul", { class: "logos" }, shown.map((l) => h("li", { class: `logo-box on-${l.where}` }, h("img", { src: l.src, alt: name, class: "brand-logo", on: { error: (ev) => ev.currentTarget.replaceWith(ev.currentTarget.ownerDocument.createTextNode(name)) } }), h("span", { class: "logo-where" }, t(`appearance.logo.${l.where}`)))))
      : h("p", { class: "muted" }, t("appearance.logo.none")),
  );
}

function adjustments(app, theme) {
  const { t } = app;
  return h(
    "section",
    { class: "card", "aria-labelledby": "ap-adjusted" },
    h("h2", { id: "ap-adjusted" }, t("appearance.adjusted")),
    theme.adjustments.length
      ? [h("p", null, t("appearance.adjusted.intro")), h("ul", { class: "plain" }, theme.adjustments.map((a) => h("li", null, adjustmentText(t, a))))]
      : h("p", null, t("appearance.adjusted.none")),
  );
}

function samples(app) {
  const { t } = app;
  return h(
    "section",
    { class: "card", "aria-labelledby": "ap-samples" },
    h("h2", { id: "ap-samples" }, t("appearance.samples")),
    h("p", { class: "note" }, t("appearance.samples.lead")),
    h(
      "div",
      { class: "samples" },
      h("button", { type: "button", class: "btn btn-primary" }, t("appearance.sample.primary")),
      h("button", { type: "button", class: "btn" }, t("appearance.sample.button")),
      h("a", { href: "#appearance" }, t("appearance.sample.link")),
      h("label", { class: "field" }, h("span", null, t("appearance.sample.field")), h("input", { type: "text", value: "", autocomplete: "off" })),
    ),
    sampleChart(app),
  );
}

function palette(app, theme) {
  const { t, fmt } = app;
  return h(
    "section",
    { class: "card", "aria-labelledby": "ap-palette" },
    h("h2", { id: "ap-palette" }, t("appearance.palette")),
    h("p", { class: "note" }, t("appearance.palette.lead")),
    SCHEMES.map((scheme) =>
      h(
        "div",
        { class: "table-scroll", role: "region", tabindex: "0", "aria-label": t(`theme.scheme.${scheme}`) },
        h(
          "table",
          { class: "data list contrast" },
          h("caption", null, t(`theme.scheme.${scheme}`)),
          h("thead", null, h("tr", null, ["pair", "colour", "on", "ratio", "needs", "result"].map((c) => h("th", { scope: "col" }, t(`col.${c}`))))),
          h(
            "tbody",
            null,
            rows(theme, scheme).map((r) =>
              h(
                "tr",
                { class: r.pass ? null : "fails" },
                h("td", { "data-label": t("col.pair") }, pairSwatch(r)),
                h("th", { scope: "row", "data-label": t("col.colour") }, tokenLabel(t, r.fg), " ", h("code", null, r.fgHex)),
                h("td", { "data-label": t("col.on") }, tokenLabel(t, r.bg), " ", h("code", null, r.bgHex)),
                h("td", { class: "num", "data-label": t("col.ratio") }, `${fmt.ratio(r.ratio)}:1`),
                h("td", { "data-label": t("col.needs") }, t(`appearance.needs.${r.kind}`, { ratio: fmt.number(r.minimum) })),
                h("td", { "data-label": t("col.result") }, h("strong", null, t(r.pass ? "appearance.pass" : "appearance.fail"))),
              ),
            ),
          ),
        ),
      ),
    ),
  );
}

function fonts(app, theme) {
  const { t } = app;
  const stack = (label, cls, value) => h("div", { class: "font-row" }, h("h3", null, label), h("p", { class: `font-sample ${cls}` }, t("appearance.font.sample")), h("p", null, h("code", null, value ?? t("appearance.font.default"))));
  return h(
    "section",
    { class: "card", "aria-labelledby": "ap-fonts" },
    h("h2", { id: "ap-fonts" }, t("appearance.fonts")),
    stack(t("appearance.font.display"), "font-display", theme.fontDisplay),
    stack(t("appearance.font.ui"), "font-ui", theme.fontUi),
    theme.fontFiles.length ? [h("h3", null, t("appearance.font.files")), h("ul", { class: "plain" }, theme.fontFiles.map((f) => h("li", null, `${f.family} (${f.weight}, ${f.style}) — `, h("code", null, f.src))))] : null,
  );
}

// appearanceView: the whole view for info.theme (null: the site sets none).
export function appearanceView(app, theme) {
  const { t } = app;
  if (!theme) {
    return [head(app), h("section", { class: "card" }, h("p", null, t("appearance.none")), h("pre", { class: "example" }, h("code", null, t("appearance.example"))))];
  }
  return [head(app), brand(app, theme), adjustments(app, theme), samples(app), palette(app, theme), fonts(app, theme)];
}
