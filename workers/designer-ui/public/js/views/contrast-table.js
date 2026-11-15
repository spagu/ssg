// The contrast of the theme's text/background pairs (models/pairs.js), both
// schemes: failing pairs first, each with its ratio, the threshold, a sample
// drawn in the two colours, and the nearest passing colour with a button to
// use it. Passing pairs fold away under a disclosure. Words, not colour
// alone: every result says "passes" or "below".

import { h } from "../core/vnode.js";
import { rounded } from "../models/contrast.js";

const ratioText = (app, r) => app.t("contrast.ratio", { ratio: app.fmt.number(rounded(r)) });

function row(app, r, { canEdit, onUse }) {
  const { t } = app;
  // Drawn as a picture, not text: it shows a failing pair on purpose, and its
  // result is said in words in the next cell.
  const sample = h(
    "svg",
    { class: "sample", viewBox: "0 0 72 28", width: "72", height: "28", role: "img", "aria-label": t("contrast.sampleOf", { fg: r.fgColour, bg: r.bgColour }) },
    h("rect", { width: "72", height: "28", rx: "5", fill: r.bgColour }),
    h("text", { x: "36", y: "19", "text-anchor": "middle", "font-size": "14", "font-weight": "600", fill: r.fgColour }, t("contrast.sample")),
  );
  return h(
    "tr",
    { class: r.passes ? "pass" : "fail" },
    h("th", { scope: "row" }, h("code", null, r.fg), h("span", { class: "on" }, t("contrast.on")), h("code", null, r.bg)),
    h("td", { "data-label": t("contrast.scheme") }, t(`scheme.${r.scheme}`)),
    h("td", { "data-label": t("contrast.sampleHead") }, sample),
    h("td", { "data-label": t("contrast.result"), class: "num" }, ratioText(app, r.ratio), " ", h("strong", null, t(r.passes ? "contrast.passes" : "contrast.below", { target: app.fmt.number(r.target) }))),
    h(
      "td",
      { "data-label": t("contrast.suggestion") },
      r.suggestion
        ? [
            h("span", { class: "swatch", style: { "background-color": r.suggestion.colour }, "aria-hidden": "true" }),
            h("code", null, r.suggestion.colour), " ", ratioText(app, r.suggestion.ratio),
            canEdit ? h("button", { type: "button", class: "btn btn-quiet", "aria-label": t("contrast.useFor", { colour: r.suggestion.colour, token: r.fg, scheme: t(`scheme.${r.scheme}`) }), on: { click: () => onUse(r) } }, t("contrast.use")) : null,
          ]
        : r.passes ? "" : t("contrast.noSuggestion"),
    ),
  );
}

function table(app, list, opts, caption) {
  const { t } = app;
  return h(
    "div",
    { class: "table-scroll" },
    h(
      "table",
      { class: "data list contrast" },
      h("caption", null, caption),
      h("thead", null, h("tr", null, ["contrast.pair", "contrast.scheme", "contrast.sampleHead", "contrast.result", "contrast.suggestion"].map((k) => h("th", { scope: "col" }, t(k))))),
      h("tbody", null, list.map((r) => row(app, r, opts))),
    ),
  );
}

// contrastTable(app, results, {canEdit, onUse(result)}) → vnode.
export function contrastTable(app, results, opts) {
  const { t } = app;
  if (!results.length) return h("p", { class: "muted" }, t("contrast.noPairs"));
  const failing = results.filter((r) => !r.passes);
  const passing = results.filter((r) => r.passes);
  return [
    failing.length
      ? h("div", { class: "callout warning", role: "status" }, h("p", null, t("contrast.failing", { count: failing.length })))
      : h("div", { class: "callout ok", role: "status" }, h("p", null, t("contrast.allPass", { count: results.length }))),
    failing.length ? table(app, failing, opts, t("contrast.failingCaption")) : null,
    passing.length ? h("details", { class: "card" }, h("summary", null, t("contrast.passingSummary", { count: passing.length })), table(app, passing, opts, t("contrast.passingCaption"))) : null,
  ];
}
