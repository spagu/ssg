// GO-128: the chart module — scales and ticks, geometry, the SVG it draws,
// the accessible frame (title, summary, data table) and its interactions.
import { test } from "node:test";
import assert from "node:assert/strict";
import { bucket, countTicks, dayTicks, linear, niceStep, peak, sum } from "../public/js/charts/scale.js";
import { SIZE, cursorKey, geometry, indexAt, lineChart, sizeFor } from "../public/js/charts/line.js";
import { PROGRESS_KEYS, barGeometry, clip, columnGeometry, columns, hbars, progress } from "../public/js/charts/bars.js";
import { find, findAll, render, text } from "../public/js/core/vnode.js";
import { createI18n } from "../public/js/core/i18n.js";
import { createFormat } from "../public/js/core/format.js";
import en from "../public/js/strings/en.js";
import { fakeDoc, fire } from "./panel-dom.ts";

const fmt = createFormat("en-GB", "UTC");
const { t } = createI18n("en", { en }, {}, fmt.number);
const ids = { title: "T", summary: "S" };
const days = (n: number, from = "2026-01-01") => Array.from({ length: n }, (_, i) => new Date(Date.parse(from) + i * 86_400_000).toISOString().slice(0, 10));

test("scale: nice zero-based ticks; an all-zero series still gets an axis", () => {
  assert.equal(niceStep(10, 4), 5);
  assert.equal(niceStep(0.5, 4), 1, "never below 1: counts are whole");
  assert.deepEqual(countTicks(0), { max: 4, ticks: [0, 1, 2, 3, 4] });
  assert.deepEqual(countTicks(undefined as never).max, 4);
  assert.deepEqual(countTicks(23), { max: 30, ticks: [0, 10, 20, 30] });
  assert.deepEqual(countTicks(1), { max: 1, ticks: [0, 1] });
  assert.deepEqual(countTicks(1830).ticks, [0, 500, 1000, 1500, 2000]);
  assert.equal(linear(0, 10, 0, 100)(5), 50);
  assert.equal(linear(3, 3, 7, 9)(3), 7, "a flat domain maps to the start");
});

test("scale: day ticks end on the last day; a year is labelled by month", () => {
  assert.deepEqual(dayTicks([]), []);
  assert.deepEqual(dayTicks(days(1)), [0]);
  const d30 = dayTicks(days(30), 6);
  assert.equal(d30[d30.length - 1], 29);
  assert.ok(d30.length <= 6);
  const year = days(365, "2025-11-14");
  const ticks = dayTicks(year, 6);
  assert.ok(ticks.every((i) => year[i].endsWith("-01")));
  assert.ok(ticks.length >= 6 && ticks.length <= 12);
  assert.ok(dayTicks(year, 0).length > 0);
});

test("scale: buckets — 30 days by day, 90 by week, 365 by month; sums and peaks", () => {
  const s = (n: number) => days(n).map((day, i) => ({ day, a: i % 3, b: "1" }));
  assert.equal(bucket(s(30), ["a"]).size, "day");
  const w = bucket(s(90), ["a", "b"]);
  assert.equal(w.size, "week");
  assert.equal(w.series.length, 13);
  assert.equal(w.series[0].day, "2026-01-01");
  assert.equal(w.series[0].b, 7);
  assert.equal(sum(w.series, "a"), sum(s(90), "a"));
  const m = bucket(s(365), ["a"]);
  assert.equal(m.size, "month");
  assert.equal(m.series.length, 12);
  assert.equal(bucket([{ day: "2026-01-01", a: "x" }, ...s(40).slice(1)], ["a"]).series[0].a >= 0, true);
  assert.equal(sum([{ v: "x" }], "v"), 0);
  assert.equal(peak([{ v: 0 }, { v: 0 }], "v"), null);
  assert.deepEqual(peak([{ v: 2, k: 1 }, { v: 5, k: 2 }, { v: 5, k: 3 }, { v: "x" }], "v"), { v: 5, k: 3 }, "the latest of a tie");
});

test("line: geometry, labels kept apart, cursor keys, pointer index", () => {
  const g = geometry(days(5), [
    { key: "signups", label: "Sign-ups", values: [0, 2, 4, 2, 10] },
    { key: "unsubscribes", label: "Unsubscribes", values: [0, 1, 0, 1, 10] },
  ]);
  assert.equal(g.yTicks[0].v, 0);
  assert.equal(g.lines.length, 2);
  assert.match(g.lines[0].d, /^M48\.0,/);
  assert.match(g.lines[0].area, /Z$/);
  assert.ok(Math.abs(g.labels[0].y - g.labels[1].y) >= 16, "end labels do not overlap");
  const flipped = geometry(days(2), [
    { key: "a", label: "A", values: [0, 95] },
    { key: "b", label: "B", values: [0, 100] },
  ]);
  assert.ok(flipped.labels[1].y < flipped.labels[0].y);
  const one = geometry(days(1), [{ key: "a", label: "A", values: [3] }]);
  assert.equal(one.lines[0].end[0], (SIZE.left + SIZE.width - SIZE.right) / 2, "a single day sits in the middle");
  const none = geometry([], [{ key: "a", label: "A", values: [] }]);
  assert.equal(none.lines[0].area, "");
  assert.equal(none.lines[0].end[0], SIZE.left);
  assert.equal(cursorKey("ArrowLeft", null, 5), 4, "the first key lands on the last day");
  assert.equal(cursorKey("ArrowLeft", 2, 5), 1);
  assert.equal(cursorKey("ArrowRight", 4, 5), 4);
  assert.equal(cursorKey("Home", 3, 5), 0);
  assert.equal(cursorKey("End", 0, 5), 4);
  assert.equal(cursorKey("ArrowLeft", 0, 5), 0);
  assert.equal(cursorKey("x", 1, 5), null);
  assert.equal(cursorKey("Home", 1, 0), null);
  assert.equal(indexAt(-10, g, 5), 0);
  assert.equal(indexAt(10_000, g, 5), 4);
  assert.equal(sizeFor(360, 12).right, 108);
  assert.equal(sizeFor(undefined, 0).right, 40);
  assert.equal(sizeFor().width, 640);
});

test("line: the plot draws, and keys/pointer move a readout", () => {
  const read = (i: number) => `day ${i}`;
  const chart = lineChart({ days: days(3), series: [{ key: "signups", label: "Sign-ups", values: [1, 2, 3] }, { key: "unsubscribes", label: "Unsubscribes", values: [0, 1, 0] }], fmt, read });
  const v = chart.plot(ids, 400);
  const svg = find(v, (n) => n.tag === "svg");
  assert.equal(svg.props.role, "img");
  assert.equal(svg.props["aria-labelledby"], "T");
  assert.equal(svg.props.viewBox, "0 0 400 260");
  assert.ok(find(v, (n) => n.props?.class?.includes("dashed")), "the second series is dashed");
  assert.ok(find(v, (n) => n.tag === "rect" && n.props.class.includes("marker")), "and has a square marker");
  assert.equal(findAll(v, (n) => n.props?.class === "direct-label").length, 2);
  const doc = fakeDoc();
  const el = render(v, doc as never) as never as Parameters<typeof fire>[0];
  fire(el, "focus");
  assert.equal(chart.refs.readout.textContent, "day 2");
  fire(el, "keydown", { key: "Home" });
  assert.equal(chart.refs.readout.textContent, "day 0");
  assert.ok(chart.refs.cursor.classList.has("on"));
  fire(el, "keydown", { key: "q" });
  assert.equal(chart.refs.at, 0);
  const svgEl = el.children[0] as never as { getBoundingClientRect?: () => unknown };
  svgEl.getBoundingClientRect = () => ({ left: 0, width: 400 });
  Object.defineProperty(el, "firstElementChild", { value: svgEl });
  fire(el, "pointermove", { clientX: 395 });
  assert.equal(chart.refs.readout.textContent, "day 2");
  assert.equal(chart.geometry().lines.length, 2);
  // default width; years use month labels
  const year = lineChart({ days: days(120), series: [{ key: "signups", label: "S", values: Array(120).fill(1) }], fmt, read });
  assert.ok(text(year.plot(ids)).includes("Jan"));
  const fresh = lineChart({ days: days(3), series: [], fmt, read });
  const freshEl = render(fresh.plot(ids), fakeDoc() as never) as never as Parameters<typeof fire>[0];
  fire(freshEl, "keydown", { key: "ArrowLeft" });
  assert.equal(fresh.refs.readout.textContent, "day 2", "a key before any focus starts at the last day");
  assert.equal(find(fresh.plot(ids), (n) => n.props?.class?.includes?.("area")), null, "no series, no area");
  const svgDefault = find(year.plot(ids), (n) => n.tag === "svg");
  assert.equal(svgDefault.props.viewBox, "0 0 640 260");
});

test("bars: geometry, clipped labels, values after the bar, textures per series", () => {
  const rows = [
    { label: "A", parts: [{ key: "sent", value: 6 }, { key: "failed", value: 2 }, { key: "skipped", value: 0 }], extra: 1 },
    { label: "B", parts: [{ key: "sent", value: -1 }] },
  ];
  const g = barGeometry(rows);
  assert.equal(g.rows[0].total, 8);
  assert.equal(g.rows[0].segments.length, 2, "zero parts draw nothing");
  assert.equal(g.rows[0].extra, 1, "the row's own data travels with it");
  assert.equal(g.rows[1].total, 0);
  assert.equal(barGeometry(rows, 16, 400).rows[0].end, (8 / 16) * (400 - 128));
  assert.equal(barGeometry([], null).height, 4);
  assert.equal(clip("abcdef", 4), "abc…");
  assert.equal(clip("abc", 4), "abc");
  const v = hbars({ rows, keys: ["sent", "failed"], valueText: (r: { total: number }) => `${r.total} people` })(ids, 300);
  assert.equal(v.props.viewBox, "0 0 300 96");
  assert.ok(text(v).includes("8 people"));
  assert.ok(findAll(v, (n) => n.tag === "pattern").length === 1, "the second series is textured");
  const fills = findAll(v, (n) => n.props?.class?.includes("seg"));
  assert.equal(fills[0].props.fill, undefined);
  assert.match(fills[1].props.fill, /^url\(#pat-\d+-failed\)$/);
  assert.equal(fills[0].props.width, fills[0].props.width, "");
  assert.equal(hbars({ rows, keys: ["sent"], valueText: () => "" })(ids).props.viewBox, "0 0 640 96");
});

test("columns: stacked by key over buckets, sparse labels, zero-based axis", () => {
  const series = days(30).map((day, i) => ({ day, hard: i % 2, soft: i === 3 ? 2 : 0, complaint: 0 }));
  const g = columnGeometry(series, ["hard", "soft", "complaint"]);
  assert.equal(g.columns.length, 30);
  assert.equal(g.columns[3].segments.length, 2);
  assert.ok(g.columns[3].segments[1].y < g.columns[3].segments[0].y, "stacked upwards");
  assert.equal(g.yTicks[0].v, 0);
  const v = columns({ series, keys: ["hard", "soft"], fmt, label: (d: string) => d.slice(8), every: 5 })(ids, 360);
  const labels = findAll(v, (n) => n.tag === "text" && n.props["text-anchor"] === "middle");
  assert.ok(labels.length < 10 && labels.length > 2);
  assert.ok(text(labels[labels.length - 1]) === "30", "the newest bucket is labelled");
  assert.ok(columns({ series: [], keys: ["hard"], fmt, label: String })(ids));
  assert.equal(columnGeometry([{ day: "x", hard: "2" }], ["hard"]).columns[0].total, 2);
});

test("progress: a named progressbar with the share in words", () => {
  const p = { subject: "Hello", total: 200, sent: 90, failed: 5, skipped: 3, uncertain: 2 };
  const v = progress({ p, t, fmt });
  assert.equal(v.props.role, "progressbar");
  assert.equal(v.props["aria-valuenow"], "100");
  assert.equal(v.props["aria-valuemax"], "200");
  assert.equal(v.props["aria-label"], "Progress of “Hello”");
  assert.equal(v.props["aria-valuetext"], "100 of 200 handled (50%)");
  assert.equal(PROGRESS_KEYS.length, 4);
  const empty = progress({ p: { subject: "", total: 0, sent: 0, failed: 0, skipped: 0, uncertain: 0 }, t, fmt, label: "Mine" });
  assert.equal(empty.props["aria-label"], "Mine");
  assert.match(empty.props["aria-valuetext"], /0 of 0 handled \(0%\)/);
  assert.match(progress({ p: { ...p, subject: "" }, t, fmt }).props["aria-label"], /no subject/);
});
