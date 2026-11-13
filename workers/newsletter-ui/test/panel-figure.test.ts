// GO-128: the chart frame — SVG helpers, the accessible figure with its
// data table, and drawing at the measured width.
import { test } from "node:test";
import assert from "node:assert/strict";
import { TEXTURES, dataTable, frame, legend, patterns, swatch } from "../public/js/charts/svg.js";
import { figure, fitPlot, toggleTable } from "../public/js/charts/figure.js";
import { find, findAll, render, text } from "../public/js/core/vnode.js";
import { createI18n } from "../public/js/core/i18n.js";
import { createFormat } from "../public/js/core/format.js";
import en from "../public/js/strings/en.js";
import { fakeDoc, fire } from "./panel-dom.ts";

const fmt = createFormat("en-GB", "UTC");
const { t } = createI18n("en", { en }, {}, fmt.number);

test("svg helpers: frame, patterns, swatches, legend, data table", () => {
  const f = frame({ width: 10, height: 5, labelledby: "a", describedby: "b" });
  assert.equal(f.props.class, "chart-svg");
  assert.equal(TEXTURES[0], "solid");
  const p = patterns("p", ["a", "b", "c", "d", "e", "f"]);
  assert.equal(p.fill("a"), null);
  assert.equal(p.fill("b"), "url(#p-b)");
  assert.equal(p.fill("f"), null, "the sixth wraps round to solid");
  assert.equal(patterns("q", ["only"]).defs, null);
  assert.ok(find(swatch("x", 2), (n) => n.tag === "rect" && n.props.fill));
  assert.ok(find(swatch("x", 0, "line"), (n) => n.tag === "line" && !n.props.class.includes("dashed")));
  assert.ok(find(swatch("x", 1, "line"), (n) => n.tag === "line" && n.props.class.includes("dashed")));
  assert.equal(legend([{ key: "a", label: "A" }]), null, "one series needs no legend");
  assert.equal(text(legend([{ key: "a", label: "A" }, { key: "b", label: "B" }])), "AB");
  const tbl = dataTable({ caption: "Cap", columns: [{ key: "day", label: "Day" }, { key: "n", label: "N", numeric: true, format: (v: number) => `#${v}` }, { key: "s", label: "S" }], rows: [{ day: "x", n: 3, s: "y" }] });
  assert.equal(find(tbl, (n) => n.tag === "caption").children[0], "Cap");
  assert.equal(findAll(tbl, (n) => n.props?.scope === "col").length, 3);
  assert.equal(find(tbl, (n) => n.props?.scope === "row").children[0], "x");
  assert.equal(find(tbl, (n) => n.tag === "td" && n.props.class === "num").children[0], "#3");
  assert.equal(find(tbl, (n) => n.tag === "td" && n.props.class === null).props["data-label"], "S");
});

test("figure: title + summary wiring, Show data toggles a real table, empty state in words", () => {
  const fig = figure({ t, title: "Growth", summary: "Up.", legend: null, plot: (i: { title: string }) => frame({ width: 1, height: 1, labelledby: i.title, describedby: "x" }), table: { columns: [{ key: "a", label: "A" }], rows: [{ a: 1 }] } });
  const h3 = find(fig, (n) => n.tag === "h3");
  const svg = find(fig, (n) => n.tag === "svg");
  assert.equal(svg.props["aria-labelledby"], h3.props.id);
  const button = find(fig, (n) => n.tag === "button");
  assert.equal(button.props["aria-expanded"], "false");
  const doc = fakeDoc();
  const el = render(fig, doc as never) as never as { children: Parameters<typeof fire>[0][] };
  const btn = el.children.find((c) => c.tag === "button") as Parameters<typeof fire>[0];
  const region = doc.getElementById(btn.getAttribute("aria-controls") as string) as unknown as { hidden: boolean };
  assert.equal(region.hidden, true);
  fire(btn, "click");
  assert.equal(btn.getAttribute("aria-expanded"), "true");
  assert.equal(btn.textContent, "Hide data");
  assert.equal(region.hidden, false);
  toggleTable({ currentTarget: btn }, { show: "S", hide: "H" });
  assert.equal(region.hidden, true);
  assert.equal(btn.textContent, "S");
  const empty = figure({ t, title: "Nothing", summary: "None.", empty: "Nobody yet.", plot: () => assert.fail("not drawn"), table: { columns: [], rows: [] }, headingLevel: "h2" });
  assert.ok(find(empty, (n) => n.tag === "h2"));
  assert.equal(text(find(empty, (n) => n.props?.class === "chart-empty")), "Nobody yet.");
  assert.equal(find(empty, (n) => n.tag === "table"), null);
});

test("fitPlot: redraws at the measured width, only when it changes", () => {
  assert.equal(fitPlot({}, () => null, undefined), null);
  let observed: (e: { contentRect: { width: number } }[]) => void = () => {};
  class Observer {
    constructor(fn: typeof observed) {
      observed = fn;
    }
    observe() {}
  }
  const doc = fakeDoc();
  const el = doc.createElement("div");
  const widths: number[] = [];
  assert.ok(fitPlot(el, (w: number) => (widths.push(w), { tag: "svg", props: {}, children: [] }), Observer));
  observed([{ contentRect: { width: 0 } }]);
  observed([{ contentRect: { width: 400.4 } }]);
  observed([{ contentRect: { width: 401 } }]);
  observed([{ contentRect: { width: 500 } }]);
  assert.deepEqual(widths, [400, 500]);
  assert.equal(el.children.length, 1);
});
