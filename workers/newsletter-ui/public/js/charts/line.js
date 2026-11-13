// Line chart over days: up to two count series (sign-ups vs unsubscribes).
// The first series is a solid line over a light area, the second a dashed
// line with square end markers; both are labelled directly at their last
// point, and a legend names them above the plot. The value axis starts at 0.
//
// Reading values: hover the plot, or focus it and use ←/→ (Home/End): a
// crosshair moves day by day and a line of text under the plot says the day
// and both numbers. The full table is behind "Show data" as well.

import { h } from "../core/vnode.js";
import { countTicks, dayTicks, linear } from "./scale.js";
import { frame } from "./svg.js";

export const SIZE = { width: 640, height: 260, left: 48, right: 112, top: 16, bottom: 30 };

// geometry computes everything drawn, as numbers, for the tests and render.
export function geometry(days, series, size = SIZE) {
  const max = Math.max(0, ...series.flatMap((s) => s.values));
  const y = countTicks(max);
  const x0 = size.left;
  const x1 = size.width - size.right;
  // One day has no extent: it sits in the middle.
  const sx = days.length > 1 ? linear(0, days.length - 1, x0, x1) : () => (x0 + x1) / 2;
  const sy = linear(0, y.max, size.height - size.bottom, size.top);
  const lines = series.map((s) => {
    const pts = s.values.map((v, i) => [sx(i), sy(v)]);
    const d = pts.map(([px, py], i) => `${i ? "L" : "M"}${px.toFixed(1)},${py.toFixed(1)}`).join("");
    const area = pts.length ? `${d}L${pts[pts.length - 1][0].toFixed(1)},${sy(0).toFixed(1)}L${pts[0][0].toFixed(1)},${sy(0).toFixed(1)}Z` : "";
    return { key: s.key, label: s.label, d, area, end: pts[pts.length - 1] ?? [x0, sy(0)] };
  });
  // End labels must not overlap: keep them at least 16 units apart.
  const labels = lines.map((l) => ({ key: l.key, label: l.label, x: l.end[0] + 10, y: l.end[1] + 4 }));
  if (labels.length === 2 && Math.abs(labels[0].y - labels[1].y) < 16) {
    const [hi, lo] = labels[0].y <= labels[1].y ? [labels[0], labels[1]] : [labels[1], labels[0]];
    const mid = (hi.y + lo.y) / 2;
    hi.y = mid - 8;
    lo.y = mid + 8;
  }
  return {
    yTicks: y.ticks.map((v) => ({ v, y: sy(v) })),
    xTicks: dayTicks(days, Math.max(3, Math.floor((x1 - x0) / 80))).map((i) => ({ i, x: sx(i), day: days[i] })),
    lines,
    labels,
    sx,
    plot: { x0, x1, y0: sy(0), y1: size.top },
  };
}

// cursorKey moves the reading cursor: ←/→ by a day, Home/End to the ends.
// Returns the new index, or null for a key the chart does not handle.
export function cursorKey(key, i, n) {
  if (n === 0) return null;
  const at = i === null ? n - 1 : i;
  const moves = { ArrowLeft: at - 1, ArrowRight: at + 1, Home: 0, End: n - 1 };
  if (!(key in moves)) return null;
  return Math.min(n - 1, Math.max(0, i === null ? n - 1 : moves[key]));
}

// indexAt converts a pointer's x (in viewBox units) to the nearest day.
export const indexAt = (vx, g, n) => Math.min(n - 1, Math.max(0, Math.round(((vx - g.plot.x0) / Math.max(1, g.plot.x1 - g.plot.x0)) * (n - 1))));

// sizeFor(width): the chart drawn at its real pixel width (figure.js
// measures it), so 12px text stays 12px on a phone and on a wide screen.
// The right margin holds the direct labels: about 7.5px per character of
// the longest one (12.5px semibold text), never under 40px.
export const sizeFor = (width = SIZE.width, labelChars = 12) => ({ ...SIZE, width, right: Math.max(40, Math.round(labelChars * 7.5 + 18)) });

// lineChart → {plot(ids, width)} for figure(); `read(i)` is the text for day i.
export function lineChart({ days, series, fmt, read }) {
  const refs = {};
  let g = null;
  let size = SIZE;
  const show = (i) => {
    const x = g.sx(i);
    refs.cursor.setAttribute("x1", x);
    refs.cursor.setAttribute("x2", x);
    refs.cursor.classList.add("on");
    refs.readout.textContent = read(i);
    refs.at = i;
  };
  const onKey = (ev) => {
    const i = cursorKey(ev.key, refs.at ?? null, days.length);
    if (i === null) return;
    ev.preventDefault();
    show(i);
  };
  const onMove = (ev) => {
    const box = ev.currentTarget.firstElementChild.getBoundingClientRect();
    show(indexAt(((ev.clientX - box.left) / Math.max(1, box.width)) * size.width, g, days.length));
  };
  const plot = (ids, width = SIZE.width) => {
    size = sizeFor(width, Math.max(0, ...series.map((x) => x.label.length)));
    g = geometry(days, series, size);
    return h(
      "div",
      { class: "line-plot", tabindex: "0", role: "group", "aria-label": read(days.length - 1), on: { keydown: onKey, pointermove: onMove, focus: () => show(refs.at ?? days.length - 1) } },
      frame(
        { width: size.width, height: size.height, labelledby: ids.title, describedby: ids.summary },
        h("g", { class: "grid" }, g.yTicks.map((t) => h("line", { x1: g.plot.x0, x2: g.plot.x1, y1: t.y, y2: t.y }))),
        h("g", { class: "ticks y" }, g.yTicks.map((t) => h("text", { x: g.plot.x0 - 8, y: t.y + 4, "text-anchor": "end" }, fmt.number(t.v)))),
        h("g", { class: "ticks x" }, g.xTicks.map((t) => h("text", { x: t.x, y: size.height - 8, "text-anchor": "middle" }, days.length > 92 ? fmt.month(t.day) : fmt.day(t.day)))),
        g.lines[0] ? h("path", { d: g.lines[0].area, class: `k-${g.lines[0].key} area` }) : null,
        g.lines.map((l, i) => h("path", { d: l.d, class: `k-${l.key} stroke line ${i ? "dashed" : ""}`.trim() })),
        h("line", { x1: g.plot.x0, x2: g.plot.x1, y1: g.plot.y0, y2: g.plot.y0, class: "axis" }),
        g.lines.map((l, i) =>
          i
            ? h("rect", { x: l.end[0] - 4, y: l.end[1] - 4, width: 8, height: 8, class: `k-${l.key} fill marker` })
            : h("circle", { cx: l.end[0], cy: l.end[1], r: 4.5, class: `k-${l.key} fill marker` }),
        ),
        g.labels.map((l) => h("text", { x: l.x, y: l.y, class: "direct-label" }, l.label)),
        h("line", { ref: (el) => (refs.cursor = el), x1: 0, x2: 0, y1: g.plot.y1, y2: g.plot.y0, class: "cursor" }),
      ),
      h("p", { class: "readout", "aria-live": "polite", ref: (el) => (refs.readout = el) }, ""),
    );
  };
  return { plot, refs, show, onKey, onMove, geometry: () => g };
}
