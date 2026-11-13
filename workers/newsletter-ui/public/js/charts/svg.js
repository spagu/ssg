// Shared pieces of every chart: the <svg> frame with its accessible name,
// texture patterns, and the data table that carries the same numbers.
//
// Identity never rests on colour alone. Each entity has a fixed colour class
// (k-<key>, see css/charts.css — colour follows the entity, never its rank),
// and every series after the first also gets a texture: diagonal lines one
// way, the other way, dots, horizontal lines. With legends and direct labels
// on top, a chart reads in greyscale, in forced colours, and for every kind of
// colour vision.

import { h } from "../core/vnode.js";

export const TEXTURES = ["solid", "rise", "fall", "dots", "rows"];

// frame: the <svg>, scalable through its viewBox, named by the figure's
// title and described by its summary sentence.
export function frame({ width, height, labelledby, describedby, className = "" }, ...children) {
  return h(
    "svg",
    {
      viewBox: `0 0 ${width} ${height}`,
      class: `chart-svg ${className}`.trim(),
      role: "img",
      "aria-labelledby": labelledby,
      "aria-describedby": describedby,
      preserveAspectRatio: "xMidYMid meet",
      focusable: "false",
    },
    children,
  );
}

const marks = {
  rise: () => h("path", { d: "M-1,1 l2,-2 M0,8 l8,-8 M7,9 l2,-2", class: "tex" }),
  fall: () => h("path", { d: "M-1,7 l2,2 M0,0 l8,8 M7,-1 l2,2", class: "tex" }),
  dots: () => h("circle", { cx: 4, cy: 4, r: 1.6, class: "tex-dot" }),
  rows: () => h("path", { d: "M0,2 h8 M0,6 h8", class: "tex" }),
};

// patterns returns <defs> with one pattern per key (index i gets TEXTURES[i])
// and a fill() lookup: "url(#…)" for textured keys, null for the solid one.
export function patterns(prefix, keys) {
  const defs = [];
  const ids = {};
  keys.forEach((key, i) => {
    const texture = TEXTURES[i % TEXTURES.length];
    if (texture === "solid") return;
    const id = `${prefix}-${key}`;
    ids[key] = id;
    defs.push(
      h("pattern", { id, width: 8, height: 8, patternUnits: "userSpaceOnUse" }, h("rect", { width: 8, height: 8, class: `k-${key} fill` }), marks[texture]()),
    );
  });
  return { defs: defs.length ? h("defs", null, defs) : null, fill: (key) => (ids[key] ? `url(#${ids[key]})` : null) };
}

// swatch: a small legend sample in the series' colour and texture, drawn as
// its own tiny SVG so the legend shows exactly what the plot shows.
export function swatch(key, index, kind = "box") {
  const p = patterns(`sw-${key}-${index}`, Array.from({ length: index + 1 }, (_, i) => (i === index ? key : `x${i}`)));
  const fill = p.fill(key);
  const shape =
    kind === "line"
      ? h("line", { x1: 1, y1: 7, x2: 23, y2: 7, class: `k-${key} stroke line ${index ? "dashed" : ""}`.trim() })
      : h("rect", { x: 1, y: 1, width: 22, height: 12, rx: 2, class: `k-${key} fill`, ...(fill ? { fill } : {}) });
  return h("svg", { class: "swatch", viewBox: "0 0 24 14", width: 24, height: 14, "aria-hidden": "true", focusable: "false" }, p.defs, shape);
}

// legend: an HTML list of swatches and labels (for >= 2 series).
export function legend(series, kind = "box") {
  if (series.length < 2) return null;
  return h(
    "ul",
    { class: "legend" },
    series.map((s, i) => h("li", null, swatch(s.key, i, kind), h("span", null, s.label))),
  );
}

// dataTable: the same numbers as the chart, as a real table. The first column
// heads each row (scope=row); numeric columns are right-aligned.
export function dataTable({ caption, columns, rows }) {
  return h(
    "table",
    { class: "data" },
    h("caption", null, caption),
    h("thead", null, h("tr", null, columns.map((c) => h("th", { scope: "col", class: c.numeric ? "num" : null }, c.label)))),
    h(
      "tbody",
      null,
      rows.map((r) =>
        h(
          "tr",
          null,
          columns.map((c, i) => {
            const v = c.format ? c.format(r[c.key], r) : r[c.key];
            return i === 0 ? h("th", { scope: "row" }, v) : h("td", { class: c.numeric ? "num" : null, "data-label": c.label }, v);
          }),
        ),
      ),
    ),
  );
}
