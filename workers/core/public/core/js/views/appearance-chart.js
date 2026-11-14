// The two-series sample chart of the Appearance view: series 1 in --c1
// (main), series 2 in --c2 (accent). Told apart by more than colour — a
// solid line with circles and a dashed line with squares, each labelled at
// its end and in the legend — so a brand whose two colours look alike to
// someone still reads correctly.

import { h } from "../vnode.js";

export const SAMPLE = { one: [3, 5, 4, 7, 6, 9, 8], two: [6, 4, 6, 5, 8, 6, 9] };
const W = 360;
const H = 160;
const PAD = { left: 16, right: 74, top: 12, bottom: 16 };

// points: a series as [x, y] in the viewBox (values 0–10).
export function points(values) {
  const step = (W - PAD.left - PAD.right) / (values.length - 1);
  return values.map((v, i) => [PAD.left + i * step, H - PAD.bottom - (v / 10) * (H - PAD.top - PAD.bottom)]);
}

// labelY: where each series' end label goes — at its line's end, pushed
// apart when the two ends are closer than a line of text.
export function labelY(a, b, gap = 16) {
  if (Math.abs(a - b) >= gap) return [a, b];
  const mid = (a + b) / 2;
  return a <= b ? [mid - gap / 2, mid + gap / 2] : [mid + gap / 2, mid - gap / 2];
}

const path = (pts) => pts.map(([x, y], i) => `${i ? "L" : "M"}${x.toFixed(1)},${y.toFixed(1)}`).join("");

function series(cls, pts, label, marker, ey) {
  const ex = pts[pts.length - 1][0];
  return h(
    "g",
    { class: `series ${cls}` },
    h("path", { d: path(pts), class: "line" }),
    pts.map(([x, y]) => (marker === "circle" ? h("circle", { cx: x, cy: y, r: 4, class: "mark" }) : h("rect", { x: x - 4, y: y - 4, width: 8, height: 8, class: "mark" }))),
    h("text", { x: ex + 8, y: ey + 4, class: "end-label" }, label),
  );
}

export function sampleChart(app) {
  const { t } = app;
  const one = t("appearance.sample.series1");
  const two = t("appearance.sample.series2");
  const p1 = points(SAMPLE.one);
  const p2 = points(SAMPLE.two);
  const [y1, y2] = labelY(p1[p1.length - 1][1], p2[p2.length - 1][1]);
  return h(
    "figure",
    { class: "chart sample-chart", "aria-labelledby": "ap-chart-title" },
    h("figcaption", null, h("h3", { id: "ap-chart-title", class: "chart-title" }, t("appearance.sample.chart")), h("p", { id: "ap-chart-text", class: "chart-summary" }, t("appearance.sample.chartText"))),
    h(
      "svg",
      { viewBox: `0 0 ${W} ${H}`, class: "chart-svg", role: "img", "aria-labelledby": "ap-chart-title", "aria-describedby": "ap-chart-text", focusable: "false" },
      h("line", { x1: PAD.left, x2: W - PAD.right, y1: H - PAD.bottom, y2: H - PAD.bottom, class: "axis" }),
      series("s1", p1, one, "circle", y1),
      series("s2", p2, two, "square", y2),
    ),
    h(
      "ul",
      { class: "legend" },
      h("li", null, h("svg", { viewBox: "0 0 28 10", class: "key s1", "aria-hidden": "true", focusable: "false" }, h("line", { x1: 0, x2: 28, y1: 5, y2: 5, class: "line" }), h("circle", { cx: 14, cy: 5, r: 4, class: "mark" })), one),
      h("li", null, h("svg", { viewBox: "0 0 28 10", class: "key s2", "aria-hidden": "true", focusable: "false" }, h("line", { x1: 0, x2: 28, y1: 5, y2: 5, class: "line" }), h("rect", { x: 10, y: 1, width: 8, height: 8, class: "mark" })), two),
    ),
  );
}
