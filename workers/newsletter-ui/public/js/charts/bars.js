// Horizontal bars, single or stacked, and stacked columns over time.
//
// Horizontal bars put each label above its bar (no guessing how wide a label
// in Hindi or German will be) and the value right after the bar's end, so no
// axis is needed to read them. Bars start at zero; stacked segments are split
// by a 2-unit surface gap and textured per series (see svg.js).

import { h, uid } from "../core/vnode.js";
import { countTicks, linear } from "./scale.js";
import { frame, patterns } from "./svg.js";

const DEFAULT_W = 640;
const ROW = 46;
const BAR = 16;
const VALUE_GUTTER = 128;

// clip shortens a label that would run past the plot (the table has it whole).
export const clip = (s, n) => (String(s).length > n ? String(s).slice(0, Math.max(1, n - 1)) + "…" : String(s));

// barGeometry: rows = [{label, parts: [{key, value}], note?}], max = the
// scale's top (default: the largest row total).
export function barGeometry(rows, max = null, W = DEFAULT_W) {
  const totals = rows.map((r) => r.parts.reduce((n, p) => n + Math.max(0, p.value), 0));
  const top = max ?? Math.max(1, ...totals);
  const sx = linear(0, top, 0, W - VALUE_GUTTER);
  return {
    height: rows.length * ROW + 4,
    rows: rows.map((r, i) => {
      let x = 0;
      const y = i * ROW + 22;
      const segments = r.parts
        .filter((p) => p.value > 0)
        .map((p) => {
          const w = sx(p.value);
          const seg = { key: p.key, value: p.value, x, w, y };
          x += w;
          return seg;
        });
      return { ...r, total: totals[i], end: x, y, labelY: y - 7, segments };
    }),
  };
}

// hbars → plot(ids) for figure(); `keys` lists the series in legend order
// (their textures); `valueText(row)` is the text after the bar.
export function hbars({ rows, keys, valueText, max = null }) {
  const p = patterns(uid("pat"), keys);
  return (ids, W = DEFAULT_W) => {
    const g = barGeometry(rows, max, W);
    return frame(
      { width: W, height: g.height, labelledby: ids.title, describedby: ids.summary, className: "bars" },
      p.defs,
      g.rows.map((r) =>
        h(
          "g",
          null,
          h("text", { x: 0, y: r.labelY, class: "bar-label" }, clip(r.label, Math.floor(W / 7.4))),
          h("rect", { x: 0, y: r.y, width: W - VALUE_GUTTER, height: BAR, class: "track", rx: 3 }),
          r.segments.map((s) =>
            h("rect", { x: s.x, y: r.y, width: Math.max(0, s.w - (s.x + s.w < r.end ? 2 : 0)), height: BAR, rx: 3, class: `k-${s.key} fill seg`, ...(p.fill(s.key) ? { fill: p.fill(s.key) } : {}) }),
          ),
          h("text", { x: r.end + 8, y: r.y + BAR - 3, class: "bar-value" }, valueText(r)),
        ),
      ),
    );
  };
}

const CH = 220;
const C = { left: 48, right: 8, top: 12, bottom: 30 };

// columnGeometry: stacked columns, one per bucket; keys in stacking order.
export function columnGeometry(series, keys, W = DEFAULT_W) {
  const totals = series.map((r) => keys.reduce((n, k) => n + (Number(r[k]) || 0), 0));
  const y = countTicks(Math.max(0, ...totals));
  const n = Math.max(1, series.length);
  const band = (W - C.left - C.right) / n;
  const width = Math.max(1, band - Math.min(6, band * 0.3));
  const sy = linear(0, y.max, CH - C.bottom, C.top);
  return {
    yTicks: y.ticks.map((v) => ({ v, y: sy(v) })),
    base: sy(0),
    columns: series.map((r, i) => {
      let top = sy(0);
      const x = C.left + i * band + (band - width) / 2;
      const segments = keys
        .filter((k) => Number(r[k]) > 0)
        .map((k) => {
          const hgt = sy(0) - sy(Number(r[k]));
          top -= hgt;
          return { key: k, value: Number(r[k]), x, y: top, w: width, h: hgt };
        });
      return { day: r.day, x, w: width, total: totals[i], segments };
    }),
  };
}

// columns → plot(ids); labels every `every`-th bucket with label(day).
export function columns({ series, keys, fmt, label, every = 1 }) {
  const p = patterns(uid("pat"), keys);
  return (ids, W = DEFAULT_W) => {
    const g = columnGeometry(series, keys, W);
    // Fewer labels when narrow: at least ~64px between two of them.
    const step = Math.max(every, Math.ceil(64 / Math.max(1, (W - C.left - C.right) / Math.max(1, series.length))));
    return frame(
      { width: W, height: CH, labelledby: ids.title, describedby: ids.summary, className: "columns" },
      p.defs,
      h("g", { class: "grid" }, g.yTicks.map((t) => h("line", { x1: C.left, x2: W - C.right, y1: t.y, y2: t.y }))),
      h("g", { class: "ticks y" }, g.yTicks.map((t) => h("text", { x: C.left - 8, y: t.y + 4, "text-anchor": "end" }, fmt.number(t.v)))),
      g.columns.map((c) =>
        c.segments.map((s, i) =>
          h("rect", { x: s.x, y: s.y, width: s.w, height: Math.max(0, s.h - (i > 0 ? 1.5 : 0)), class: `k-${s.key} fill seg`, ...(p.fill(s.key) ? { fill: p.fill(s.key) } : {}) }),
        ),
      ),
      h("line", { x1: C.left, x2: W - C.right, y1: g.base, y2: g.base, class: "axis" }),
      h(
        "g",
        { class: "ticks x" },
        g.columns.map((c, i) => ((series.length - 1 - i) % step === 0 ? h("text", { x: c.x + c.w / 2, y: CH - 8, "text-anchor": "middle" }, label(c.day)) : null)),
      ),
    );
  };
}

// The parts of a Progress the bar shows, in stacking order; the rest is the track.
export const PROGRESS_KEYS = ["sent", "failed", "skipped", "uncertain"];

// progress → a role=progressbar block: one stacked bar of the campaign's
// total with sent / failed / skipped / uncertain; the rest is the track.
export function progress({ p, t, fmt, label = null }) {
  const total = Math.max(0, p.total);
  const handled = PROGRESS_KEYS.reduce((n, k) => n + p[k], 0);
  const ratio = total ? Math.min(1, handled / total) : 0;
  const text = t("progress.text", { handled: fmt.number(handled), total: fmt.number(total), percent: fmt.percent(ratio) });
  const rows = [{ label: "", parts: PROGRESS_KEYS.map((k) => ({ key: k, value: p[k] })) }];
  const g = barGeometry(rows, Math.max(1, total), DEFAULT_W);
  const pat = patterns(uid("pat"), PROGRESS_KEYS);
  const r = g.rows[0];
  return h(
    "div",
    { class: "progress", role: "progressbar", "aria-label": label ?? t("progress.of", { subject: p.subject || t("campaign.untitled") }), "aria-valuemin": "0", "aria-valuemax": String(total), "aria-valuenow": String(handled), "aria-valuetext": text },
    h(
      "svg",
      { viewBox: `0 0 ${DEFAULT_W - VALUE_GUTTER} ${BAR}`, class: "progress-svg", "aria-hidden": "true", focusable: "false", preserveAspectRatio: "none" },
      pat.defs,
      h("rect", { x: 0, y: 0, width: DEFAULT_W - VALUE_GUTTER, height: BAR, rx: 3, class: "track" }),
      r.segments.map((s) => h("rect", { x: s.x, y: 0, width: s.w, height: BAR, class: `k-${s.key} fill seg`, ...(pat.fill(s.key) ? { fill: pat.fill(s.key) } : {}) })),
    ),
    h("p", { class: "progress-text" }, text),
  );
}
