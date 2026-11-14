// A small chart of one daily series from a module's summary — own SVG, no
// library, and never only a picture: a visible title, one summary sentence
// computed from the data, and "Show data" revealing the same numbers as a
// table. The value axis starts at zero (a count that does not lies about
// proportions). Drawn in a fixed viewBox: the cards it sits in stay between
// about 300 and 460 px wide, so its text keeps a readable size.

import { h, uid } from "./vnode.js";

export const SIZE = { width: 360, height: 150, left: 40, right: 14, top: 12, bottom: 26 };

// niceMax: the top of the axis — 1, 2 or 5 × 10^k at or above max (4 for an
// all-zero series, so a flat line sits on a readable axis).
export function niceMax(max) {
  if (!(max > 0)) return 4;
  const mag = 10 ** Math.floor(Math.log10(max));
  return [1, 2, 5, 10].map((m) => m * mag).find((v) => v >= max);
}

// geometry: everything drawn, as numbers (for the tests and the SVG).
export function geometry(points, size = SIZE) {
  const top = niceMax(Math.max(0, ...points.map((p) => p.value)));
  const x0 = size.left;
  const x1 = size.width - size.right;
  const y0 = size.height - size.bottom;
  const sx = (i) => (points.length > 1 ? x0 + (i / (points.length - 1)) * (x1 - x0) : (x0 + x1) / 2);
  const sy = (v) => y0 - (v / top) * (y0 - size.top);
  const pts = points.map((p, i) => [sx(i), sy(p.value)]);
  const line = pts.map(([x, y], i) => `${i ? "L" : "M"}${x.toFixed(1)},${y.toFixed(1)}`).join("");
  const area = pts.length ? `${line}L${pts[pts.length - 1][0].toFixed(1)},${y0}L${pts[0][0].toFixed(1)},${y0}Z` : "";
  const ticks = [0, top / 2, top].map((v) => ({ v, y: sy(v) }));
  const labels = points.length ? [...new Set([0, Math.floor((points.length - 1) / 2), points.length - 1])].map((i) => ({ i, x: sx(i), day: points[i].day })) : [];
  return { top, line, area, ticks, labels, end: pts[pts.length - 1] ?? null, plot: { x0, x1, y0, y1: size.top } };
}

// facts: the numbers the summary sentence states.
export function facts(points) {
  let peak = null;
  for (const p of points) if (p.value > 0 && (!peak || p.value >= peak.value)) peak = p;
  return { days: points.length, total: points.reduce((n, p) => n + p.value, 0), peak };
}

// summaryText: the sentence under the title.
export function summaryText(points, t, fmt) {
  const f = facts(points);
  if (!f.peak) return t("chart.flat", { days: fmt.number(f.days) });
  return t("chart.summary", { days: fmt.number(f.days), total: fmt.number(f.total), day: fmt.dayLong(f.peak.day), max: fmt.number(f.peak.value) });
}

// toggleTable is "Show data": flips the table region and the button.
export function toggleTable(ev, labels) {
  const button = ev.currentTarget;
  const region = button.ownerDocument.getElementById(button.getAttribute("aria-controls"));
  const open = button.getAttribute("aria-expanded") !== "true";
  button.setAttribute("aria-expanded", String(open));
  button.textContent = open ? labels.hide : labels.show;
  region.hidden = !open;
}

// chartFigure: the accessible frame around the plot.
export function chartFigure({ t, fmt, title, points }) {
  const id = uid("chart");
  const ids = { title: `${id}-t`, summary: `${id}-d`, table: `${id}-tbl` };
  const g = geometry(points);
  const labels = { show: t("chart.showData"), hide: t("chart.hideData") };
  return h(
    "figure",
    { class: "chart", "aria-labelledby": ids.title },
    h("figcaption", null, h("h4", { id: ids.title, class: "chart-title" }, title), h("p", { id: ids.summary, class: "chart-summary" }, summaryText(points, t, fmt))),
    h(
      "svg",
      { viewBox: `0 0 ${SIZE.width} ${SIZE.height}`, class: "chart-svg", role: "img", "aria-labelledby": ids.title, "aria-describedby": ids.summary, focusable: "false" },
      h("g", { class: "grid" }, g.ticks.map((k) => h("line", { x1: g.plot.x0, x2: g.plot.x1, y1: k.y, y2: k.y }))),
      h("g", { class: "ticks" }, g.ticks.map((k) => h("text", { x: g.plot.x0 - 6, y: k.y + 4, "text-anchor": "end" }, fmt.number(k.v)))),
      h("g", { class: "ticks" }, g.labels.map((l, n) => h("text", { x: l.x, y: SIZE.height - 7, "text-anchor": n === 0 ? "start" : n === g.labels.length - 1 ? "end" : "middle" }, fmt.day(l.day)))),
      h("path", { d: g.area, class: "area" }),
      h("path", { d: g.line, class: "line" }),
      h("line", { x1: g.plot.x0, x2: g.plot.x1, y1: g.plot.y0, y2: g.plot.y0, class: "axis" }),
      g.end ? h("circle", { cx: g.end[0], cy: g.end[1], r: 4, class: "marker" }) : null,
    ),
    h("button", { type: "button", class: "btn btn-quiet data-toggle", "aria-expanded": "false", "aria-controls": ids.table, on: { click: (ev) => toggleTable(ev, labels) } }, labels.show),
    h(
      "div",
      { id: ids.table, class: "table-scroll", hidden: true, role: "region", tabindex: "0", "aria-label": t("chart.dataOf", { title }) },
      h(
        "table",
        { class: "data" },
        h("caption", null, title),
        h("thead", null, h("tr", null, h("th", { scope: "col" }, t("chart.day")), h("th", { scope: "col", class: "num" }, title))),
        h("tbody", null, points.map((p) => h("tr", null, h("th", { scope: "row" }, fmt.dayLong(p.day)), h("td", { class: "num" }, fmt.number(p.value))))),
      ),
    ),
  );
}
