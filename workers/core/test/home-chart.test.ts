// The small chart (js/chart.js): geometry from zero with a nice top, the
// summary sentence, and "Show data" revealing the same numbers as a table.
import { test } from "node:test";
import assert from "node:assert/strict";
import { chartFigure, facts, geometry, niceMax, summaryText, toggleTable } from "../public/core/js/chart.js";
import { createFormat } from "../public/core/js/format.js";
import { createI18n } from "../public/core/js/i18n.js";
import { render } from "../public/core/js/vnode.js";
import en from "../public/core/js/strings/en.js";
import { all, fakeDoc, fire, walk } from "./dom.ts";
import type { FakeEl } from "./dom.ts";

const fmt = createFormat("en", "UTC");
const { t } = createI18n("en", { en }, {}, fmt.number);

test("chart: geometry from zero, a nice top, labels at the ends; summary sentences; Show data toggles the table", () => {
  assert.deepEqual([niceMax(0), niceMax(-1), niceMax(7), niceMax(19), niceMax(100), niceMax(101)], [4, 4, 10, 20, 100, 200]);
  const pts = [{ day: "2026-11-11", value: 2 }, { day: "2026-11-12", value: 9 }, { day: "2026-11-13", value: 4 }];
  const g = geometry(pts);
  assert.equal(g.top, 10);
  assert.deepEqual(g.ticks.map((k: { v: number }) => k.v), [0, 5, 10]);
  assert.deepEqual(g.labels.map((l: { i: number }) => l.i), [0, 1, 2]);
  assert.match(g.line, /^M40\.0,101\.6L193\.0,23\.2L346\.0,79\.2$/);
  assert.match(g.area, /Z$/);
  const one = geometry([{ day: "2026-11-13", value: 0 }]);
  assert.deepEqual([one.top, one.labels.length, one.end![0]], [4, 1, 193]);
  const none = geometry([]);
  assert.deepEqual([none.line, none.area, none.end, none.labels], ["", "", null, []]);
  assert.deepEqual(facts(pts), { days: 3, total: 15, peak: pts[1] });
  assert.equal(summaryText(pts, t, fmt), "Last 3 days: 15 in total, the most on November 12, 2026 (9).");
  assert.equal(summaryText([{ day: "2026-11-13", value: 0 }, { day: "2026-11-14", value: 0 }], t, fmt), "Last 2 days: none yet.");
  const doc = fakeDoc();
  const fig = render(chartFigure({ t, fmt, title: "Sign-ups", points: pts }), doc as never) as never as FakeEl;
  const button = walk(fig, (e) => e.tag === "button")!;
  const region = doc.getElementById(button.attrs["aria-controls"])!;
  assert.equal(region.hidden, true);
  fire(button, "click", { currentTarget: { ...button, ownerDocument: doc, getAttribute: (k: string) => button.getAttribute(k), setAttribute: (k: string, v: string) => button.setAttribute(k, v) } });
  assert.deepEqual([button.attrs["aria-expanded"], region.hidden], ["true", false]);
  toggleTable({ currentTarget: Object.assign(button, { ownerDocument: doc }) }, { show: "Show data", hide: "Hide data" });
  assert.deepEqual([button.attrs["aria-expanded"], region.hidden, button.textContent], ["false", true, "Show data"]);
  assert.equal(all(fig, (e) => e.tag === "tr").length, 4);
  const svg = walk(fig, (e) => e.tag === "svg")!;
  assert.deepEqual([svg.attrs.role, svg.attrs["aria-labelledby"].endsWith("-t")], ["img", true]);
  const flat = render(chartFigure({ t, fmt, title: "x", points: [] }), doc as never) as never as FakeEl;
  assert.equal(all(flat, (e) => e.tag === "circle").length, 0);
});

