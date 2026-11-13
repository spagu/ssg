// figure() puts a chart in its accessible frame:
//
//   <figure>
//     <figcaption> title (h3) + one summary sentence computed from the data
//     legend (when there are two or more series)
//     the plot: <svg role=img aria-labelledby=title aria-describedby=summary>
//     "Show data" — a real button (aria-expanded) revealing a <table> with the
//     same numbers, so nothing is reachable only by looking at the picture
//   </figure>
//
// An empty chart is not drawn at all: the figure says in words that there is
// nothing yet and what will fill it.

import { h, mount, uid } from "../core/vnode.js";
import { dataTable } from "./svg.js";

// toggleTable is the "Show data" handler: flips `hidden` on the table region
// and the button's state and label. Exported for the tests.
export function toggleTable(ev, labels) {
  const button = ev.currentTarget;
  const region = button.ownerDocument.getElementById(button.getAttribute("aria-controls"));
  const open = button.getAttribute("aria-expanded") !== "true";
  button.setAttribute("aria-expanded", String(open));
  button.textContent = open ? labels.hide : labels.show;
  region.hidden = !open;
}

// fitPlot draws the plot at the width its box really has, and again when
// that changes (a resized window, a rotated phone): SVG text then keeps its
// CSS pixel size instead of shrinking with the viewBox. Without
// ResizeObserver (tests) the first drawing stays.
export function fitPlot(el, draw, Observer = globalThis.ResizeObserver) {
  if (!Observer) return null;
  let last = 0;
  const ro = new Observer((entries) => {
    const w = Math.round(entries[0].contentRect.width);
    if (w > 0 && Math.abs(w - last) > 1) {
      last = w;
      mount(el, draw(w), el.ownerDocument);
    }
  });
  ro.observe(el);
  return ro;
}

// figure({t, title, summary, empty, plot(ids), legend, table}) — `plot` gets
// {title, summary} ids to wire its <svg>; `table` is {columns, rows}.
export function figure({ t, title, summary, empty = null, plot, legend = null, table, className = "", headingLevel = "h3" }) {
  const id = uid("fig");
  const ids = { title: `${id}-t`, summary: `${id}-d`, table: `${id}-tbl` };
  const head = h("figcaption", null, h(headingLevel, { id: ids.title, class: "chart-title" }, title), h("p", { id: ids.summary, class: "chart-summary" }, summary));
  if (empty) {
    return h("figure", { class: `chart is-empty ${className}`.trim(), "aria-labelledby": ids.title }, head, h("p", { class: "chart-empty" }, empty));
  }
  const labels = { show: t("chart.showData"), hide: t("chart.hideData") };
  return h(
    "figure",
    { class: `chart ${className}`.trim(), "aria-labelledby": ids.title },
    head,
    legend,
    h("div", { class: "plot", ref: (el) => fitPlot(el, (w) => plot(ids, w)) }, plot(ids)),
    h(
      "button",
      { type: "button", class: "btn btn-quiet data-toggle", "aria-expanded": "false", "aria-controls": ids.table, on: { click: (ev) => toggleTable(ev, labels) } },
      labels.show,
    ),
    h(
      "div",
      { id: ids.table, class: "table-scroll", hidden: true, role: "region", tabindex: "0", "aria-label": t("chart.dataOf", { title }) },
      dataTable({ caption: title, ...table }),
    ),
  );
}
